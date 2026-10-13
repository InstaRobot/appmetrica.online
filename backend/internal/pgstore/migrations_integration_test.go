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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

// openStore открывает Store на свежей базе; migrate=true применяет все миграции.
func openStore(t *testing.T, migrate bool) *pgstore.Store {
	t.Helper()
	ctx := t.Context()
	store, err := pgstore.Open(ctx, testutil.NewPostgresDB(t, false))
	require.NoError(t, err)
	t.Cleanup(store.Close)

	if migrate {
		p, err := pgstore.NewMigrationProvider(store.Pool())
		require.NoError(t, err)
		defer func() { require.NoError(t, p.Close()) }()
		_, err = p.Up(ctx)
		require.NoError(t, err)
	}
	return store
}

var (
	wantTables = []string{
		"alert_states", "api_keys", "apps", "auth_sessions", "closed_days", "deletion_requests", "job_runs",
		"owner", "passkey_setup_tokens", "passkeys", "settings", "webauthn_ceremonies", "write_keys",
	}
	wantEnums = []string{
		"app_platform", "app_status", "ceremony_kind", "deletion_source", "deletion_status",
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

	_, err = p.Down(ctx)
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
	require.Equal(t, int64(1), v)

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
		Name:     name,
		Platform: sqlc.AppPlatformIos,
		Timezone: "Europe/Moscow",
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
	require.Equal(t, sqlc.AppPlatformIos, rows[0].AppPlatform)

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
		Name: "notes", Platform: sqlc.AppPlatformAndroid, Timezone: "UTC",
	})
	requirePgError(t, err, pgUniqueViolation, "apps_name_lower_uq")
}

func ptr[T any](v T) *T { return &v }

var errBoom = errors.New("boom")

func TestInTx_CommitsAndRollsBack(t *testing.T) {
	ctx := t.Context()
	store := openStore(t, true)

	err := store.InTx(ctx, func(q *sqlc.Queries) error {
		_, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "committed", Platform: sqlc.AppPlatformWeb, Timezone: "UTC"})
		return err
	})
	require.NoError(t, err)

	err = store.InTx(ctx, func(q *sqlc.Queries) error {
		if _, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "rolled-back", Platform: sqlc.AppPlatformWeb, Timezone: "UTC"}); err != nil {
			return err
		}
		return errBoom
	})
	require.ErrorIs(t, err, errBoom)

	require.Panics(t, func() {
		_ = store.InTx(ctx, func(q *sqlc.Queries) error {
			if _, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "panicked", Platform: sqlc.AppPlatformWeb, Timezone: "UTC"}); err != nil {
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
