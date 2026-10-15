//go:build integration

package pgstore_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

// openStore открывает Store на свежей базе; migrate=true — копия шаблонной базы со всеми миграциями.
func openStore(t *testing.T, migrate bool) *pgstore.Store {
	t.Helper()
	store, err := pgstore.Open(t.Context(), testutil.NewPostgresDB(t, migrate))
	require.NoError(t, err)
	t.Cleanup(store.Close)
	return store
}

var (
	wantTables = []string{
		"alert_states", "api_keys", "app_merges", "apps", "auth_sessions", "closed_days", "closed_days_reopen", "deletion_requests", "job_runs",
		"owner", "passkey_setup_tokens", "passkeys", "settings", "webauthn_ceremonies", "write_keys",
	}
	wantEnums = []string{
		"app_merge_status", "app_merge_step", "app_platform", "app_status", "ceremony_kind", "deletion_source", "deletion_status",
		"deletion_subject", "job_name", "job_status",
	}
)

func publicTables(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND table_name <> 'goose_db_version'
		ORDER BY table_name`)
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return names
}

func publicEnums(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT t.typname FROM pg_type AS t JOIN pg_namespace AS n ON n.oid = t.typnamespace
		WHERE n.nspname = 'public' AND t.typtype = 'e'
		ORDER BY t.typname`)
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return names
}

func TestPostgresMigrations_UpDownUp(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, false)
	p, err := pgstore.NewMigrationProvider(store.Pool())
	require.NoError(t, err)
	defer func() { require.NoError(t, p.Close()) }()

	_, err = p.Up(ctx)
	require.NoError(t, err)
	require.Equal(t, wantTables, publicTables(t, store.Pool()))
	require.Equal(t, wantEnums, publicEnums(t, store.Pool()))

	_, err = p.DownTo(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, publicTables(t, store.Pool()), "Down должен удалить все таблицы")
	require.Empty(t, publicEnums(t, store.Pool()), "Down должен удалить все enum-ы")
	v, err := p.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Zero(t, v)

	_, err = p.Up(ctx)
	require.NoError(t, err)
	require.Equal(t, wantTables, publicTables(t, store.Pool()))
	require.Equal(t, wantEnums, publicEnums(t, store.Pool()))

	v, err = p.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), v)

	// Повторный Up на актуальной схеме — no-op.
	res, err := p.Up(ctx)
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestPostgresMigrations_SettingsSeeded(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)

	var count int
	require.NoError(t, store.Pool().QueryRow(ctx, "SELECT count(*) FROM settings").Scan(&count))
	require.Equal(t, 1, count)

	s, err := store.Queries().GetSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "Europe/Moscow", s.OwnerTimezone)
	require.Nil(t, s.TelegramChatID)
	require.False(t, s.DigestEnabled)
	require.True(t, s.AlertsEnabled)

	var digestTime string
	require.NoError(t, store.Pool().QueryRow(ctx, "SELECT digest_time::text FROM settings").Scan(&digestTime))
	require.Equal(t, "09:00:00", digestTime)

	// Вторая строка settings невозможна: singleton — первичный ключ с CHECK (singleton).
	_, err = store.Pool().Exec(ctx, "INSERT INTO settings (singleton) VALUES (true)")
	requirePgError(t, err, pgUniqueViolation, "settings_pkey")
	_, err = store.Pool().Exec(ctx, "INSERT INTO settings (singleton) VALUES (false)")
	requirePgError(t, err, pgCheckViolation, "settings_singleton_chk")

	// Дайджест без чата запрещён.
	_, err = store.Pool().Exec(ctx, "UPDATE settings SET digest_enabled = true")
	requirePgError(t, err, pgCheckViolation, "settings_digest_requires_chat_chk")
}

const (
	pgUniqueViolation = "23505"
	pgCheckViolation  = "23514"
)

func requirePgError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, code, pgErr.Code)
	require.Equal(t, constraint, pgErr.ConstraintName)
}

func createApp(t *testing.T, store *pgstore.Store, name string) sqlc.App {
	t.Helper()
	app, err := store.Queries().CreateApp(t.Context(), sqlc.CreateAppParams{
		Name:      name,
		Platforms: []sqlc.AppPlatform{sqlc.AppPlatformIos},
		Timezone:  "Europe/Moscow",
	})
	require.NoError(t, err)
	return app
}

func TestDeletionRequests_ActiveSubjectUnique(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	appA := createApp(t, store, "A")
	appB := createApp(t, store, "B")

	insert := func(appID uuid.UUID, subjectType, value string) error {
		_, err := store.Pool().Exec(ctx, `
			INSERT INTO deletion_requests (app_id, subject_type, subject_value, source)
			VALUES ($1, $2, $3, 'api')`, appID, subjectType, value)
		return err
	}

	require.NoError(t, insert(appA.ID, "user", "u1"))
	requirePgError(t, insert(appA.ID, "user", "u1"), pgUniqueViolation, "deletion_requests_active_subject_uq")

	// Другой тип субъекта, другое значение или другое приложение — не конфликт.
	require.NoError(t, insert(appA.ID, "install", "u1"))
	require.NoError(t, insert(appA.ID, "user", "u2"))
	require.NoError(t, insert(appB.ID, "user", "u1"))

	// running тоже считается активным.
	_, err := store.Pool().Exec(ctx,
		`UPDATE deletion_requests SET status = 'running' WHERE app_id = $1 AND subject_value = 'u1' AND subject_type = 'user'`, appA.ID)
	require.NoError(t, err)
	requirePgError(t, insert(appA.ID, "user", "u1"), pgUniqueViolation, "deletion_requests_active_subject_uq")

	_, err = store.Pool().Exec(ctx,
		`UPDATE deletion_requests SET status = 'done' WHERE app_id = $1 AND subject_value = 'u1' AND subject_type = 'user'`, appA.ID)
	require.NoError(t, err)
	require.NoError(t, insert(appA.ID, "user", "u1"))

	// failed не мешает новому запросу.
	_, err = store.Pool().Exec(ctx,
		`UPDATE deletion_requests SET status = 'failed' WHERE app_id = $1 AND subject_value = 'u1' AND subject_type = 'user' AND status = 'pending'`, appA.ID)
	require.NoError(t, err)
	require.NoError(t, insert(appA.ID, "user", "u1"))
}

func TestCreateDeletionRequest_IdempotentForActiveSubject(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	app := createApp(t, store, "A")
	q := store.Queries()

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	params := sqlc.CreateDeletionRequestParams{
		AppID:        app.ID,
		SubjectType:  sqlc.DeletionSubjectUser,
		SubjectValue: "u1",
		Source:       sqlc.DeletionSourceApi,
		Now:          now,
	}
	first, err := q.CreateDeletionRequest(ctx, params)
	require.NoError(t, err)
	require.Equal(t, sqlc.DeletionStatusPending, first.Status)
	require.True(t, first.RequestedAt.Equal(now))

	params.Now = now.Add(time.Hour)
	second, err := q.CreateDeletionRequest(ctx, params)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "повторный запрос возвращает активный")
	require.True(t, second.RequestedAt.Equal(now), "requested_at не сдвигается")

	completedAt := now.Add(2 * time.Hour)
	require.NoError(t, q.CompleteDeletionRequest(ctx, sqlc.CompleteDeletionRequestParams{ID: first.ID, Now: ptr(completedAt)}))
	third, err := q.CreateDeletionRequest(ctx, params)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, third.ID, "после done создаётся новый запрос")
}

func TestLoadWriteKeyCache_SkipsRevokedKeys(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	app := createApp(t, store, "A")
	q := store.Queries()

	active, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: "pk_" + strings.Repeat("a", 32), Label: "main"})
	require.NoError(t, err)
	revoked, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: "pk_" + strings.Repeat("b", 32)})
	require.NoError(t, err)
	_, err = q.RevokeWriteKey(ctx, sqlc.RevokeWriteKeyParams{ID: revoked.ID, AppID: app.ID, Now: ptr(time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC))})
	require.NoError(t, err)

	rows, err := q.LoadWriteKeyCache(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, active.ID, rows[0].WriteKeyID)
	require.Equal(t, app.ID, rows[0].AppID)
	require.Equal(t, sqlc.AppStatusActive, rows[0].AppStatus)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos}, rows[0].AppPlatforms)
	require.Nil(t, rows[0].KeyPlatform, "ключ без платформы — любая платформа")

	// Формат ключа проверяется схемой.
	_, err = q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: "pk_short"})
	requirePgError(t, err, pgCheckViolation, "write_keys_key_format_chk")

	// Повторно отозвать нельзя: строки нет.
	_, err = q.RevokeWriteKey(ctx, sqlc.RevokeWriteKeyParams{ID: revoked.ID, AppID: app.ID, Now: ptr(time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC))})
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestApps_NameUniqueCaseInsensitive(t *testing.T) {
	store := openStore(t, true)
	createApp(t, store, "Notes")
	_, err := store.Queries().CreateApp(t.Context(), sqlc.CreateAppParams{
		Name: "notes", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformAndroid}, Timezone: "UTC",
	})
	requirePgError(t, err, pgUniqueViolation, "apps_name_lower_uq")
}

func ptr[T any](v T) *T { return &v }

var errBoom = errors.New("boom")

func TestInTx_CommitsAndRollsBack(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)

	err := store.InTx(ctx, func(q *sqlc.Queries) error {
		_, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "committed", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC"})
		return err
	})
	require.NoError(t, err)

	err = store.InTx(ctx, func(q *sqlc.Queries) error {
		if _, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "rolled-back", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC"}); err != nil {
			return err
		}
		return errBoom
	})
	require.ErrorIs(t, err, errBoom)

	require.Panics(t, func() {
		_ = store.InTx(ctx, func(q *sqlc.Queries) error {
			if _, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "panicked", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC"}); err != nil {
				return err
			}
			panic("boom")
		})
	})

	apps, err := store.Queries().ListApps(ctx, true)
	require.NoError(t, err)
	require.Len(t, apps, 1)
	require.Equal(t, "committed", apps[0].Name)
}

func TestTouchWriteKeysLastUsed_OnlyMovesForward(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	app := createApp(t, store, "A")
	q := store.Queries()

	k1, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: "pk_" + strings.Repeat("c", 32)})
	require.NoError(t, err)
	k2, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: "pk_" + strings.Repeat("d", 32)})
	require.NoError(t, err)

	t1 := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)
	require.NoError(t, q.TouchWriteKeysLastUsed(ctx, sqlc.TouchWriteKeysLastUsedParams{
		Ids: []uuid.UUID{k1.ID, k2.ID}, LastUsed: []time.Time{t2, t1},
	}))
	// Более старое значение не перезаписывает свежее.
	require.NoError(t, q.TouchWriteKeysLastUsed(ctx, sqlc.TouchWriteKeysLastUsedParams{
		Ids: []uuid.UUID{k1.ID}, LastUsed: []time.Time{t1},
	}))
	// Пустой батч — no-op.
	require.NoError(t, q.TouchWriteKeysLastUsed(ctx, sqlc.TouchWriteKeysLastUsedParams{
		Ids: []uuid.UUID{}, LastUsed: []time.Time{},
	}))

	keys, err := q.ListWriteKeys(ctx, app.ID)
	require.NoError(t, err)
	got := map[uuid.UUID]time.Time{}
	for _, k := range keys {
		require.NotNil(t, k.LastUsedAt)
		got[k.ID] = *k.LastUsedAt
	}
	require.True(t, got[k1.ID].Equal(t2))
	require.True(t, got[k2.ID].Equal(t1))
}

func queryStrings(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), sql, args...)
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return out
}

func TestMigration00002BackfillsPlatforms(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewPostgresAt(t, 1)
	var appID uuid.UUID
	require.NoError(t, db.QueryRow(ctx,
		`INSERT INTO apps (name, platform, timezone) VALUES ('A', 'android', 'UTC') RETURNING id`).Scan(&appID))
	_, err := db.Exec(ctx, `INSERT INTO write_keys (app_id, key) VALUES ($1, $2), ($1, $3)`,
		appID, "pk_"+strings.Repeat("a", 32), "pk_"+strings.Repeat("b", 32))
	require.NoError(t, err)

	require.NoError(t, testutil.MigrateTo(ctx, db, 2))

	require.Equal(t, []string{"{android}"}, queryStrings(t, db, `SELECT platforms::text FROM apps WHERE id = $1`, appID))
	require.Equal(t, []string{"android"}, queryStrings(t, db, `SELECT platform::text FROM apps WHERE id = $1`, appID))
	require.Equal(t, []string{"android", "android"},
		queryStrings(t, db, `SELECT platform::text FROM write_keys WHERE app_id = $1`, appID))
	require.Subset(t, queryStrings(t, db, `SELECT indexname::text FROM pg_indexes WHERE tablename = 'app_merges'`),
		[]string{"app_merges_active_target_uq", "app_merges_active_source_uq"})
	require.Contains(t, queryStrings(t, db, `SELECT unnest(enum_range(NULL::job_name))::text`), "app_merge")
}

func TestMigration00002Down(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewPostgresAt(t, 2)
	var appID uuid.UUID
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO apps (name, platform, platforms, timezone) VALUES ('A', 'ios', '{ios,android}', 'UTC')
		RETURNING id`).Scan(&appID))
	_, err := db.Exec(ctx, `INSERT INTO job_runs (job) VALUES ('app_merge'), ('close_day'), ('digest')`)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO app_merges (target_app_id, source_name, source_platforms, source_timezone)
		VALUES ($1, 'S', '{web}', 'UTC')`, appID)
	require.NoError(t, err)

	require.NoError(t, testutil.MigrateTo(ctx, db, 1))

	require.Empty(t, queryStrings(t, db, `
		SELECT table_name || '.' || column_name FROM information_schema.columns
		WHERE (table_name = 'apps' AND column_name = 'platforms') OR (table_name = 'write_keys' AND column_name = 'platform')`))
	require.Equal(t, []string{""}, queryStrings(t, db, `SELECT coalesce(to_regclass('app_merges')::text, '')`))
	require.Equal(t, []string{"close_day", "digest", "system_check", "deletion", "session_cleanup"},
		queryStrings(t, db, `SELECT unnest(enum_range(NULL::job_name))::text`))
	require.Equal(t, []string{"close_day", "digest"}, queryStrings(t, db, `SELECT job::text FROM job_runs ORDER BY job`))
	require.Equal(t, []string{"ios"}, queryStrings(t, db, `SELECT platform::text FROM apps WHERE id = $1`, appID))

	// Повторный up восстанавливает набор из оставшейся platform: остальные платформы потеряны.
	require.NoError(t, testutil.MigrateTo(ctx, db, 2))
	require.Equal(t, []string{"{ios}"}, queryStrings(t, db, `SELECT platforms::text FROM apps WHERE id = $1`, appID))
}

func TestAppsPlatformChecks(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	app := createApp(t, store, "A")

	_, err := store.Pool().Exec(ctx, `UPDATE apps SET platforms = '{}' WHERE id = $1`, app.ID)
	requirePgError(t, err, pgCheckViolation, "apps_platforms_len_chk")
	_, err = store.Pool().Exec(ctx,
		`UPDATE apps SET platforms = array_fill('ios'::app_platform, ARRAY[10]) WHERE id = $1`, app.ID)
	requirePgError(t, err, pgCheckViolation, "apps_platforms_len_chk")
	_, err = store.Pool().Exec(ctx, `UPDATE apps SET platform = 'web' WHERE id = $1`, app.ID)
	requirePgError(t, err, pgCheckViolation, "apps_platform_first_chk")
	_, err = store.Pool().Exec(ctx, `UPDATE apps SET platforms = '{android,ios}' WHERE id = $1`, app.ID)
	requirePgError(t, err, pgCheckViolation, "apps_platform_first_chk")

	_, err = store.Pool().Exec(ctx, `UPDATE apps SET platforms = '{ios,web}' WHERE id = $1`, app.ID)
	require.NoError(t, err, "platforms[1] совпадает с platform")
}

func TestScanPlatformsArray(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	both := []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid}

	created, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Notes", Platforms: both, Timezone: "UTC"})
	require.NoError(t, err)
	require.Equal(t, both, created.Platforms)
	require.Equal(t, sqlc.AppPlatformIos, created.Platform, "platform = platforms[1]")

	got, err := q.GetApp(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, both, got.Platforms)

	apps, err := q.ListApps(ctx, true)
	require.NoError(t, err)
	require.Len(t, apps, 1)
	require.Equal(t, both, apps[0].Platforms)

	require.NoError(t, store.InTx(ctx, func(q *sqlc.Queries) error {
		locked, err := q.GetAppForUpdate(ctx, created.ID)
		require.Equal(t, both, locked.Platforms)
		return err
	}))
}

// Пул, открытый до миграций (migrate up, MIGRATE_ON_START), регистрирует типы при первом обращении после них.
func TestScanPlatformsArray_PoolOpenedBeforeMigrations(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, false)
	require.NoError(t, testutil.MigrateTo(ctx, store.Pool(), 2))

	app, err := store.Queries().CreateApp(ctx, sqlc.CreateAppParams{
		Name: "Site", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC",
	})
	require.NoError(t, err)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformWeb}, app.Platforms)
}

func TestUpdateApp_Platforms(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	app := createApp(t, store, "A")
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	renamed, err := q.UpdateApp(ctx, sqlc.UpdateAppParams{ID: app.ID, Name: ptr("B"), Now: now})
	require.NoError(t, err)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos}, renamed.Platforms, "без platforms набор не меняется")

	updated, err := q.UpdateApp(ctx, sqlc.UpdateAppParams{
		ID: app.ID, Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb, sqlc.AppPlatformBot}, Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformWeb, sqlc.AppPlatformBot}, updated.Platforms)
	require.Equal(t, sqlc.AppPlatformWeb, updated.Platform)
	require.Equal(t, "B", updated.Name)
}

func TestWriteKeyPlatform(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	app, err := q.CreateApp(ctx, sqlc.CreateAppParams{
		Name: "Notes", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid}, Timezone: "UTC",
	})
	require.NoError(t, err)

	android, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{
		AppID: app.ID, Key: "pk_" + strings.Repeat("e", 32), Label: "Android", Platform: ptr(sqlc.AppPlatformAndroid),
	})
	require.NoError(t, err)
	require.Equal(t, ptr(sqlc.AppPlatformAndroid), android.Platform)

	rows, err := q.LoadWriteKeyCache(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, ptr(sqlc.AppPlatformAndroid), rows[0].KeyPlatform)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid}, rows[0].AppPlatforms)

	keys, err := q.ListWriteKeys(ctx, app.ID)
	require.NoError(t, err)
	require.Equal(t, ptr(sqlc.AppPlatformAndroid), keys[0].Platform)
}

func TestAbsorbAppForMergeAndMoveWriteKeys(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	target, err := q.CreateApp(ctx, sqlc.CreateAppParams{
		Name: "T", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC",
	})
	require.NoError(t, err)
	source, err := q.CreateApp(ctx, sqlc.CreateAppParams{
		Name: "S", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformAndroid}, Timezone: "UTC",
	})
	require.NoError(t, err)
	anyKey, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: source.ID, Key: "pk_" + strings.Repeat("f", 32)})
	require.NoError(t, err)
	webKey, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{
		AppID: source.ID, Key: "pk_" + strings.Repeat("g", 32), Platform: ptr(sqlc.AppPlatformWeb),
	})
	require.NoError(t, err)

	moved, err := q.MoveWriteKeys(ctx, sqlc.MoveWriteKeysParams{
		TargetID: target.ID, SourceID: source.ID, DefaultPlatform: ptr(sqlc.AppPlatformAndroid),
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, moved)
	keys, err := q.ListWriteKeys(ctx, target.ID)
	require.NoError(t, err)
	got := map[uuid.UUID]*sqlc.AppPlatform{}
	for _, k := range keys {
		got[k.ID] = k.Platform
	}
	require.Equal(t, map[uuid.UUID]*sqlc.AppPlatform{
		anyKey.ID: ptr(sqlc.AppPlatformAndroid), // без платформы — единственная платформа источника
		webKey.ID: ptr(sqlc.AppPlatformWeb),     // своя платформа сохраняется
	}, got)

	// Порядок enum: ios раньше web, поэтому platforms[1] и platform цели становятся ios.
	earlier := target.CreatedAt.Add(-30 * 24 * time.Hour)
	now := target.CreatedAt.Add(time.Hour)
	absorbed, err := q.AbsorbAppForMerge(ctx, sqlc.AbsorbAppForMergeParams{
		TargetID: target.ID, SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb, sqlc.AppPlatformIos},
		SourceCreatedAt: earlier, Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformWeb}, absorbed.Platforms)
	require.Equal(t, sqlc.AppPlatformIos, absorbed.Platform)
	require.True(t, absorbed.CreatedAt.Equal(earlier), "created_at — самый ранний из двух")
	require.True(t, absorbed.UpdatedAt.Equal(now))

	// Более поздний created_at источника не сдвигает цель.
	again, err := q.AbsorbAppForMerge(ctx, sqlc.AbsorbAppForMergeParams{
		TargetID: target.ID, SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos},
		SourceCreatedAt: now, Now: now,
	})
	require.NoError(t, err)
	require.True(t, again.CreatedAt.Equal(earlier))
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformWeb}, again.Platforms)
}

func TestReopenClosedDays(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	app := createApp(t, store, "A")
	other := createApp(t, store, "B")
	day := func(d int) pgtype.Date {
		return pgtype.Date{Time: time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	for _, d := range []int{20, 31} {
		for _, id := range []uuid.UUID{app.ID, other.ID} {
			require.NoError(t, q.MarkDayClosed(ctx, sqlc.MarkDayClosedParams{AppID: id, Day: day(d), Now: now}))
		}
	}

	n, err := q.ReopenClosedDays(ctx, sqlc.ReopenClosedDaysParams{AppID: app.ID, FromDay: day(21)})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	last, err := q.GetLastClosedDay(ctx, app.ID)
	require.NoError(t, err)
	require.Equal(t, day(20).Time, last.Time)
	last, err = q.GetLastClosedDay(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, day(31).Time, last.Time, "другие приложения не затронуты")
}

// Заявки на переоткрытие дней (смена часового пояса, удаление данных человека): повторная заявка сдвигает
// from_day к раннему дню, применение удаляет closed_days с from_day и саму заявку одной командой. Приложение с
// незавершённым удалением человека или установки ждёт: иначе close_day закрыл бы дни заново до удаления данных.
func TestReopenRequests(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	app := createApp(t, store, "A")
	waiting := createApp(t, store, "B")
	day := func(d int) pgtype.Date {
		return pgtype.Date{Time: time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	for _, d := range []int{10, 20, 31} {
		for _, id := range []uuid.UUID{app.ID, waiting.ID} {
			require.NoError(t, q.MarkDayClosed(ctx, sqlc.MarkDayClosedParams{AppID: id, Day: day(d), Now: now}))
		}
	}
	for _, r := range []sqlc.RequestReopenClosedDaysParams{
		{AppID: app.ID, FromDay: day(25), Now: now},
		{AppID: app.ID, FromDay: day(15), Now: now},
		{AppID: app.ID, FromDay: day(28), Now: now},
		{AppID: waiting.ID, FromDay: day(1), Now: now},
	} {
		require.NoError(t, q.RequestReopenClosedDays(ctx, r))
	}
	_, err := store.Pool().Exec(ctx, `INSERT INTO deletion_requests (app_id, subject_type, subject_value, source, status)
		VALUES ($1, 'user', 'u1', 'api', 'running'), ($2, 'user', 'u2', 'api', 'done')`, waiting.ID, app.ID)
	require.NoError(t, err)

	applied, err := q.ApplyReopenRequests(ctx)
	require.NoError(t, err)
	require.Equal(t, []sqlc.ApplyReopenRequestsRow{{AppID: app.ID, FromDay: day(15), Days: 2}}, applied)
	last, err := q.GetLastClosedDay(ctx, app.ID)
	require.NoError(t, err)
	require.Equal(t, day(10).Time, last.Time)
	last, err = q.GetLastClosedDay(ctx, waiting.ID)
	require.NoError(t, err)
	require.Equal(t, day(31).Time, last.Time, "заявка ждёт конца удаления")

	again, err := q.ApplyReopenRequests(ctx)
	require.NoError(t, err)
	require.Empty(t, again, "применённая заявка удалена, ожидающая — нет")

	_, err = store.Pool().Exec(ctx, `UPDATE deletion_requests SET status = 'done' WHERE app_id = $1`, waiting.ID)
	require.NoError(t, err)
	applied, err = q.ApplyReopenRequests(ctx)
	require.NoError(t, err)
	require.Equal(t, []sqlc.ApplyReopenRequestsRow{{AppID: waiting.ID, FromDay: day(1), Days: 3}}, applied)
}

func TestMigration00003Down(t *testing.T) {
	ctx := t.Context()
	db := testutil.NewPostgresAt(t, 3)

	require.NoError(t, testutil.MigrateTo(ctx, db, 2))

	require.Equal(t, []string{""}, queryStrings(t, db, `SELECT coalesce(to_regclass('closed_days_reopen')::text, '')`))
}

func TestAppMergeLifecycle(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)
	q := store.Queries()
	target := createApp(t, store, "T")
	source := createApp(t, store, "S")
	other := createApp(t, store, "O")
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	m, err := q.CreateAppMerge(ctx, sqlc.CreateAppMergeParams{
		TargetAppID: target.ID, SourceAppID: &source.ID, SourceName: "S",
		SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformWeb}, SourceTimezone: "UTC", TotalEvents: 800,
	})
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusPending, m.Status)
	require.Equal(t, sqlc.AppMergeStepMoveKeys, m.Step)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformWeb}, m.SourcePlatforms)
	require.Equal(t, []int32{}, m.CopiedPartitions)

	for _, id := range []uuid.UUID{target.ID, source.ID} {
		active, err := q.ActiveMergeForApp(ctx, id)
		require.NoError(t, err)
		require.Equal(t, m.ID, active.ID)
	}
	_, err = q.ActiveMergeForApp(ctx, other.ID)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	// Второе незавершённое слияние в той же роли запрещено индексами.
	_, err = q.CreateAppMerge(ctx, sqlc.CreateAppMergeParams{
		TargetAppID: target.ID, SourceAppID: &other.ID, SourceName: "O",
		SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}, SourceTimezone: "UTC",
	})
	requirePgError(t, err, pgUniqueViolation, "app_merges_active_target_uq")
	_, err = q.CreateAppMerge(ctx, sqlc.CreateAppMergeParams{
		TargetAppID: other.ID, SourceAppID: &source.ID, SourceName: "S",
		SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}, SourceTimezone: "UTC",
	})
	requirePgError(t, err, pgUniqueViolation, "app_merges_active_source_uq")
	_, err = q.CreateAppMerge(ctx, sqlc.CreateAppMergeParams{
		TargetAppID: other.ID, SourceAppID: &other.ID, SourceName: "O",
		SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}, SourceTimezone: "UTC",
	})
	requirePgError(t, err, pgCheckViolation, "app_merges_distinct_apps_chk")

	claimed, err := q.ClaimNextAppMerge(ctx)
	require.NoError(t, err)
	require.Equal(t, m.ID, claimed.ID)

	firstDay := pgtype.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	require.NoError(t, q.SetAppMergeStep(ctx, sqlc.SetAppMergeStepParams{
		ID: m.ID, Step: sqlc.AppMergeStepCopy, Now: now, KeysMovedAt: &now, FirstSourceDay: firstDay,
	}))
	// Повтор шага без новых значений не стирает записанные.
	require.NoError(t, q.SetAppMergeStep(ctx, sqlc.SetAppMergeStepParams{ID: m.ID, Step: sqlc.AppMergeStepCopy, Now: now.Add(time.Hour)}))
	for range 2 {
		require.NoError(t, q.AddCopiedPartition(ctx, sqlc.AddCopiedPartitionParams{ID: m.ID, Partition: 202609, Rows: 500}))
	}
	require.NoError(t, q.AddCopiedPartition(ctx, sqlc.AddCopiedPartitionParams{ID: m.ID, Partition: 202610, Rows: 300}))

	got, err := q.GetAppMerge(ctx, sqlc.GetAppMergeParams{ID: m.ID, TargetAppID: target.ID})
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusRunning, got.Status)
	require.Equal(t, sqlc.AppMergeStepCopy, got.Step)
	require.True(t, got.StartedAt.Equal(now))
	require.True(t, got.KeysMovedAt.Equal(now))
	require.Equal(t, firstDay.Time, got.FirstSourceDay.Time)
	require.Equal(t, []int32{202609, 202610}, got.CopiedPartitions, "партиция учитывается один раз")
	require.EqualValues(t, 800, got.CopiedEvents)

	_, err = q.GetAppMerge(ctx, sqlc.GetAppMergeParams{ID: m.ID, TargetAppID: source.ID})
	require.ErrorIs(t, err, pgx.ErrNoRows, "слияние видно только у своей цели")

	// failed продолжает блокировать приложения; повтор возможен только из failed.
	_, err = q.RetryAppMerge(ctx, sqlc.RetryAppMergeParams{ID: m.ID, TargetAppID: target.ID})
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.NoError(t, q.FailAppMerge(ctx, sqlc.FailAppMergeParams{ID: m.ID, Error: "clickhouse: timeout"}))
	_, err = q.ClaimNextAppMerge(ctx)
	require.ErrorIs(t, err, pgx.ErrNoRows, "failed не забирается job-ом")
	active, err := q.ActiveMergeForApp(ctx, source.ID)
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusFailed, active.Status)
	require.Equal(t, ptr("clickhouse: timeout"), active.Error)

	retried, err := q.RetryAppMerge(ctx, sqlc.RetryAppMergeParams{ID: m.ID, TargetAppID: target.ID})
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusPending, retried.Status)
	require.Equal(t, sqlc.AppMergeStepCopy, retried.Step, "повтор продолжает с упавшего шага")
	require.EqualValues(t, 1, retried.Attempts)
	require.Nil(t, retried.Error)

	finished := now.Add(2 * time.Hour)
	require.NoError(t, q.FinishAppMerge(ctx, sqlc.FinishAppMergeParams{ID: m.ID, Now: finished}))
	_, err = q.ActiveMergeForApp(ctx, target.ID)
	require.ErrorIs(t, err, pgx.ErrNoRows, "завершённое слияние ничего не блокирует")

	// Удаление источника обнуляет source_app_id, запись журнала остаётся.
	_, err = q.DeleteApp(ctx, source.ID)
	require.NoError(t, err)
	list, err := q.ListAppMerges(ctx, target.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, sqlc.AppMergeStatusDone, list[0].Status)
	require.Equal(t, sqlc.AppMergeStepDone, list[0].Step)
	require.True(t, list[0].FinishedAt.Equal(finished))
	require.Nil(t, list[0].SourceAppID)
	require.Equal(t, "S", list[0].SourceName)
}
