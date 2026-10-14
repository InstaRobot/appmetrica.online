//go:build integration

package merge_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/reports"
)

var (
	aug1 = time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	sep1 = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
)

type failedMerge struct {
	ID    uuid.UUID
	Cause string
}

// executor — исполнитель слияний на часах clk; упавшие слияния копятся в failed.
func (e *env) executor(clk clock.Clock, failed *[]failedMerge, opts ...merge.Option) *merge.Executor {
	return e.executorOn(e.CH.Conn(), clk, failed, opts...)
}

// executorOn — исполнитель поверх соединения ClickHouse conn.
func (e *env) executorOn(conn driver.Conn, clk clock.Clock, failed *[]failedMerge, opts ...merge.Option) *merge.Executor {
	onFailed := func(_ context.Context, m sqlc.AppMerge, cause string) {
		*failed = append(*failed, failedMerge{m.ID, cause})
	}
	return merge.NewExecutor(e.PG, conn, clk, slog.New(slog.DiscardHandler), onFailed, opts...)
}

func (e *env) getMerge(t *testing.T, target appJSON, id string) sqlc.AppMerge {
	t.Helper()
	m, err := e.PG.Queries().GetAppMerge(t.Context(), sqlc.GetAppMergeParams{ID: uuid.MustParse(id), TargetAppID: uuid.MustParse(target.ID)})
	require.NoError(t, err)
	return m
}

func (e *env) getApp(t *testing.T, app appJSON) sqlc.App {
	t.Helper()
	a, err := e.PG.Queries().GetApp(t.Context(), uuid.MustParse(app.ID))
	require.NoError(t, err)
	return a
}

func (e *env) setCreatedAt(t *testing.T, app appJSON, at time.Time) {
	t.Helper()
	_, err := e.PG.Pool().Exec(t.Context(), "UPDATE apps SET created_at = $1 WHERE id = $2", at, app.ID)
	require.NoError(t, err)
}

// AC-008.1: move_keys переносит ключи источника (ключ без платформы получает android), архивирует источник,
// расширяет platforms цели и берёт самый ранний created_at; затем слияние ждёт кэш ключей ingest.
func TestMoveKeysStep(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	e.setCreatedAt(t, target, sep1)
	e.setCreatedAt(t, source, aug1)
	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/write-keys", map[string]any{"label": "Любая"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	sourceKeys, err := e.PG.Queries().ListWriteKeys(t.Context(), uuid.MustParse(source.ID))
	require.NoError(t, err)
	require.Len(t, sourceKeys, 2)
	targetKeys, err := e.PG.Queries().ListWriteKeys(t.Context(), uuid.MustParse(target.ID))
	require.NoError(t, err)

	rec = e.createMerge(t, target, source)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m := decode[mergeJSON](t, rec)

	clk := clock.NewFixed(t0)
	var failed []failedMerge
	exec := e.executor(clk, &failed)
	res, err := exec.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, merge.Result{MergeID: uuid.MustParse(m.ID), Status: sqlc.AppMergeStatusRunning, Step: sqlc.AppMergeStepWaitKeys}, res)

	keys, err := e.PG.Queries().ListWriteKeys(t.Context(), uuid.MustParse(target.ID))
	require.NoError(t, err)
	require.Len(t, keys, len(targetKeys)+2)
	moved := map[uuid.UUID]bool{}
	for _, k := range sourceKeys {
		moved[k.ID] = true
	}
	for _, k := range keys {
		if moved[k.ID] {
			require.NotNil(t, k.Platform, "ключ %s без платформы", k.Label)
			require.Equal(t, sqlc.AppPlatformAndroid, *k.Platform, k.Label)
		}
	}
	left, err := e.PG.Queries().ListWriteKeys(t.Context(), uuid.MustParse(source.ID))
	require.NoError(t, err)
	require.Empty(t, left)

	src := e.getApp(t, source)
	require.Equal(t, sqlc.AppStatusArchived, src.Status)
	tgt := e.getApp(t, target)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid}, tgt.Platforms)
	require.Equal(t, sqlc.AppPlatformIos, tgt.Platform)
	require.True(t, aug1.Equal(tgt.CreatedAt), "created_at цели %s", tgt.CreatedAt)
	require.Equal(t, sqlc.AppStatusActive, tgt.Status)

	got := e.getMerge(t, target, m.ID)
	require.Equal(t, sqlc.AppMergeStatusRunning, got.Status)
	require.Equal(t, sqlc.AppMergeStepWaitKeys, got.Step)
	require.NotNil(t, got.KeysMovedAt)
	require.True(t, t0.Equal(*got.KeysMovedAt))
	require.NotNil(t, got.StartedAt)
	require.True(t, t0.Equal(*got.StartedAt))

	// Пока ingest может держать старый снимок ключей, шаг не меняется.
	clk.Set(t0.Add(5 * time.Minute))
	_, err = exec.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStepWaitKeys, e.getMerge(t, target, m.ID).Step)

	// Слияние ещё активно: разархивировать источник нельзя.
	requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/unarchive", nil), http.StatusConflict, "merge_in_progress")

	// Ожидание прошло: событий у источника нет, слияние доходит до конца в том же запуске.
	clk.Set(t0.Add(deletion.AppPurgeDelay))
	res, err = exec.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	require.Equal(t, sqlc.AppMergeStepDone, e.getMerge(t, target, m.ID).Step)
	require.Empty(t, failed)
}

// Ключ без платформы у источника с несколькими платформами остаётся без платформы.
func TestMoveKeysStepMultiPlatformSource(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "UTC")
	source := e.create(t, "Notes Other", []string{"android", "web"}, "UTC")
	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/write-keys", map[string]any{"label": "Любая"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	anyKey := decode[struct {
		ID string `json:"id"`
	}](t, rec)
	require.Equal(t, http.StatusAccepted, e.createMerge(t, target, source).Code)

	var failed []failedMerge
	_, err := e.executor(clock.NewFixed(t0), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)

	keys, err := e.PG.Queries().ListWriteKeys(t.Context(), uuid.MustParse(target.ID))
	require.NoError(t, err)
	require.Len(t, keys, 4, "ключ цели, ключи android и web и ключ без платформы")
	for _, k := range keys {
		if k.ID.String() == anyKey.ID {
			require.Nil(t, k.Platform)
		}
	}
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid, sqlc.AppPlatformWeb}, e.getApp(t, target).Platforms)
}

// AC-008.3: reclose удаляет closed_days цели с first_source_day и завершает слияние.
func TestRecloseStep(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	targetID := uuid.MustParse(target.ID)
	for d := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC); !d.After(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		_, err := e.PG.Pool().Exec(t.Context(), "INSERT INTO closed_days (app_id, day, closed_at) VALUES ($1, $2, $3)", targetID, d, t0)
		require.NoError(t, err)
	}
	rec := e.createMerge(t, target, source)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m := decode[mergeJSON](t, rec)
	e.setMerge(t, m.ID, "status = 'running', step = 'reclose', started_at = now(), first_source_day = DATE '2026-09-01'")

	clk := clock.NewFixed(t0.Add(time.Hour))
	var failed []failedMerge
	res, err := e.executor(clk, &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)

	var n int
	var last time.Time
	require.NoError(t, e.PG.Pool().QueryRow(t.Context(),
		"SELECT count(*), max(day) FROM closed_days WHERE app_id = $1", targetID).Scan(&n, &last))
	require.Equal(t, 12, n, "2026-08-20…2026-08-31")
	require.Equal(t, "2026-08-31", last.Format(time.DateOnly))

	got := e.getMerge(t, target, m.ID)
	require.Equal(t, sqlc.AppMergeStatusDone, got.Status)
	require.Equal(t, sqlc.AppMergeStepDone, got.Step)
	require.NotNil(t, got.FinishedAt)
	require.True(t, clk.Now().Equal(*got.FinishedAt))
	require.Nil(t, got.Error)
	require.Empty(t, failed)

	// Слияние завершено: операции с целью снова доступны, повторный запуск ничего не делает.
	res, err = e.executor(clk, &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, merge.Result{}, res)
	require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, "/admin/v1/apps/"+target.ID+"/archive", nil).Code)
}

// Источник без событий (first_source_day NULL): reclose ничего не переоткрывает.
func TestRecloseStepWithoutSourceEvents(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "UTC")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	_, err := e.PG.Pool().Exec(t.Context(), "INSERT INTO closed_days (app_id, day, closed_at) VALUES ($1, DATE '2026-09-01', $2)", target.ID, t0)
	require.NoError(t, err)
	m := decode[mergeJSON](t, e.createMerge(t, target, source))
	e.setMerge(t, m.ID, "status = 'running', step = 'reclose', started_at = now()")

	var failed []failedMerge
	res, err := e.executor(clock.NewFixed(t0), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	require.Equal(t, 1, e.count(t, "closed_days"))
}

// Ошибка шага: слияние failed с текстом, алерт один раз; «Повторить» продолжает с того же шага.
func TestMoveKeysStepFailsWithoutSource(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "UTC")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	m := decode[mergeJSON](t, e.createMerge(t, target, source))
	// Строка источника исчезла в обход API (ON DELETE SET NULL): переносить нечего.
	_, err := e.PG.Pool().Exec(t.Context(), "DELETE FROM apps WHERE id = $1", source.ID)
	require.NoError(t, err)

	var failed []failedMerge
	exec := e.executor(clock.NewFixed(t0), &failed)
	res, err := exec.RunOnce(t.Context(), uuid.New())
	require.Error(t, err)
	require.Equal(t, sqlc.AppMergeStatusFailed, res.Status)

	got := e.getMerge(t, target, m.ID)
	require.Equal(t, sqlc.AppMergeStatusFailed, got.Status)
	require.Equal(t, sqlc.AppMergeStepMoveKeys, got.Step)
	require.NotNil(t, got.Error)
	require.NotEmpty(t, *got.Error)
	require.Len(t, failed, 1)
	require.Equal(t, uuid.MustParse(m.ID), failed[0].ID)
	require.Equal(t, *got.Error, failed[0].Cause)

	// Упавшее слияние worker не берёт, пока владелец не нажмёт «Повторить».
	res, err = exec.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, merge.Result{}, res)
	require.Len(t, failed, 1)
}

// faultyConn — соединение ClickHouse со сбоем: закрывается после closeAfter-й копии партиции в events
// или отказывает в запросах, начинающихся с failPrefix.
type faultyConn struct {
	driver.Conn
	closeAfter int
	failPrefix string
	copies     int
}

func (c *faultyConn) Exec(ctx context.Context, query string, args ...any) error {
	q := strings.TrimSpace(query)
	if c.failPrefix != "" && strings.HasPrefix(q, c.failPrefix) {
		return errors.New("clickhouse: injected failure: " + c.failPrefix)
	}
	if err := c.Conn.Exec(ctx, query, args...); err != nil {
		return err
	}
	if strings.HasPrefix(q, "INSERT INTO events") {
		c.copies++
		if c.copies == c.closeAfter {
			return c.Close()
		}
	}
	return nil
}

// openCH — отдельное соединение ClickHouse с базой теста (его можно закрыть, не ломая e.CH).
func (e *env) openCH(t *testing.T) driver.Conn {
	t.Helper()
	store, err := chstore.Open(t.Context(), e.ClickHouse)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store.Conn()
}

func (e *env) chCount(t *testing.T, query string, params clickhouse.Parameters) uint64 {
	t.Helper()
	var n uint64
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(params))
	require.NoError(t, e.CH.Conn().QueryRow(ctx, query).Scan(&n), query)
	return n
}

// appRows — строки таблицы ClickHouse по приложению.
func (e *env) appRows(t *testing.T, table string, app appJSON) uint64 {
	t.Helper()
	return e.chCount(t, "SELECT count() FROM "+table+" WHERE app_id = {app:UUID}", clickhouse.Parameters{"app": app.ID})
}

func (e *env) closeDays(t *testing.T, app appJSON, from, to time.Time) {
	t.Helper()
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		_, err := e.PG.Pool().Exec(t.Context(), "INSERT INTO closed_days (app_id, day, closed_at) VALUES ($1, $2, $3)", app.ID, d, t0)
		require.NoError(t, err)
	}
}

// runToDone — запуски job app_merge, пока слияние не завершится (не больше 5).
func runToDone(t *testing.T, exec *merge.Executor, runID uuid.UUID) merge.Result {
	t.Helper()
	var res merge.Result
	for range 5 {
		var err error
		res, err = exec.RunOnce(t.Context(), runID)
		require.NoError(t, err)
		if res.Status == sqlc.AppMergeStatusDone {
			return res
		}
	}
	t.Fatalf("слияние не завершилось: %+v", res)
	return res
}

func rejectedRow(app appJSON) chstore.RejectedRow {
	return chstore.RejectedRow{
		// TTL rejected_events — 7 дней от времени сервера: строка должна быть свежей по настоящим часам.
		ReceivedAt: time.Now().UTC().Truncate(time.Millisecond), AppID: uuid.MustParse(app.ID), KeyPrefix: "wk_live_a1b2",
		Code: "invalid_field", Field: "name", Message: "Пустое имя события", EventName: "", AppVersion: "3.4.0",
		Platform: "android", PayloadSample: `{"name":""}`,
	}
}

// AC-009.1, AC-8.1: полный цикл T←S — события и журнал отказов источника у цели, агрегаты цели пересобраны
// materialized views, источник удалён из ClickHouse и Postgres с записью в job_runs, дни цели переоткрыты.
func TestMergeEndToEnd(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	e.setCreatedAt(t, target, sep1)
	e.setCreatedAt(t, source, aug1)
	require.NoError(t, e.CH.InsertEvents(ctx, events(target, "ios", "u1", 1000, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Hour)))
	e.seedSource(t, source)
	require.NoError(t, e.CH.InsertRejected(ctx, []chstore.RejectedRow{rejectedRow(source)}))
	e.closeDays(t, target, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	// Ключ Read API, привязанный к источнику, и заявки на удаление его людей: после слияния они у цели.
	sourceKey := e.createAPIKey(t, source, "Источник")
	sourceUser := e.createDeletionRequest(t, source, sqlc.DeletionSubjectUser, "u1")
	sourceInstall := e.createDeletionRequest(t, source, sqlc.DeletionSubjectInstall, "i-1")
	// У цели уже есть активная заявка на ту же установку: заявка источника не переезжает, а уходит с ним.
	targetInstall := e.createDeletionRequest(t, target, sqlc.DeletionSubjectInstall, "i-1")

	rec := e.createMerge(t, target, source)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m := decode[mergeJSON](t, rec)
	require.Equal(t, int64(800), m.TotalEvents)

	runID, err := e.PG.Queries().StartJobRun(ctx, sqlc.StartJobRunParams{Job: sqlc.JobNameAppMerge, Now: t0})
	require.NoError(t, err)
	var failed []failedMerge
	res := runToDone(t, e.executor(clock.NewFixed(t0), &failed, merge.WithKeysWait(0)), runID)
	require.Equal(t, sqlc.AppMergeStepDone, res.Step)
	require.Empty(t, failed)

	key, err := e.PG.Queries().GetApiKey(ctx, sourceKey)
	require.NoError(t, err, "ключ Read API источника не удалён каскадом")
	require.Equal(t, uuid.MustParse(target.ID), *key.AppID)
	user, err := e.PG.Queries().GetDeletionRequest(ctx, sqlc.GetDeletionRequestParams{ID: sourceUser, AppID: uuid.MustParse(target.ID)})
	require.NoError(t, err, "заявка на удаление человека выполнится в цели")
	require.Equal(t, sqlc.DeletionStatusPending, user.Status)
	_, err = e.PG.Queries().GetDeletionRequest(ctx, sqlc.GetDeletionRequestParams{ID: targetInstall, AppID: uuid.MustParse(target.ID)})
	require.NoError(t, err)
	require.Equal(t, 0, e.countWhere(t, "deletion_requests", sourceInstall.String()), "дубликат активной заявки цели ушёл с источником")

	// События и отказы источника — у цели.
	require.Equal(t, uint64(1800), e.appRows(t, "events", target))
	require.Equal(t, uint64(0), e.appRows(t, "events", source))
	require.Equal(t, uint64(1), e.appRows(t, "rejected_events", target))
	require.Equal(t, uint64(0), e.appRows(t, "rejected_events", source))
	// Materialized views пересобрали агрегаты цели; агрегатов источника не осталось.
	targetParams := clickhouse.Parameters{"app": target.ID}
	require.Equal(t, uint64(1800), e.chCount(t, "SELECT uniqExact(install_id) FROM install_first_seen WHERE app_id = {app:UUID}", targetParams))
	require.Equal(t, uint64(1800), e.chCount(t, "SELECT uniqExact(install_id) FROM identity_links WHERE app_id = {app:UUID}", targetParams))
	require.Equal(t, uint64(1), e.chCount(t, "SELECT uniqExact(user_id) FROM user_first_seen WHERE app_id = {app:UUID}", targetParams))
	var firstSeen time.Time
	require.NoError(t, e.CH.Conn().QueryRow(clickhouse.Context(ctx, clickhouse.WithParameters(targetParams)),
		"SELECT min(first_seen) FROM user_first_seen WHERE app_id = {app:UUID} AND user_id = 'u1'").Scan(&firstSeen))
	require.Equal(t, time.Date(2026, 8, 10, 0, 30, 0, 0, time.UTC), firstSeen.UTC(), "u1 впервые — в источнике")
	for _, table := range []string{"install_first_seen", "user_first_seen", "identity_links"} {
		require.Equal(t, uint64(0), e.appRows(t, table, source), table)
	}

	// Люди сквозь платформы: u1 с 1 800 установок iOS и Android — один человек.
	moscow, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	period := reports.Params{
		AppID: uuid.MustParse(target.ID), Location: moscow,
		Period: reports.Period{From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)},
	}
	stats := reports.NewService(e.CH.Conn(), clock.NewFixed(t0))
	summary, err := stats.AudienceSummary(ctx, period, false)
	require.NoError(t, err)
	require.Equal(t, uint64(1), summary.Active)
	period.Unit = identity.Installs
	summary, err = stats.AudienceSummary(ctx, period, false)
	require.NoError(t, err)
	require.Equal(t, uint64(1800), summary.Active)

	// Строки источника нет, слияние ссылается на него только снимком имени.
	require.Equal(t, 0, e.countWhere(t, "apps", source.ID))
	got := e.getMerge(t, target, m.ID)
	require.Nil(t, got.SourceAppID)
	require.Equal(t, sqlc.AppMergeStatusDone, got.Status)
	require.Equal(t, []int32{202608, 202609}, got.CopiedPartitions)
	require.Equal(t, int64(800), got.CopiedEvents)
	require.True(t, got.FirstSourceDay.Valid)
	require.Equal(t, "2026-08-10", got.FirstSourceDay.Time.Format(time.DateOnly))
	list := decode[struct {
		Items []mergeJSON `json:"items"`
	}](t, e.call(t, http.MethodGet, "/admin/v1/apps/"+target.ID+"/merges", nil))
	require.Len(t, list.Items, 1)
	require.Nil(t, list.Items[0].SourceAppID)
	require.Equal(t, "done", list.Items[0].Status)

	// Копия слияния в журнале запуска job.
	var raw []byte
	require.NoError(t, e.PG.Pool().QueryRow(ctx, "SELECT details -> 'merged_apps' FROM job_runs WHERE id = $1", runID).Scan(&raw))
	var merged []map[string]any
	require.NoError(t, json.Unmarshal(raw, &merged), string(raw))
	require.Len(t, merged, 1)
	require.Equal(t, m.ID, merged[0]["merge_id"])
	require.Equal(t, source.ID, merged[0]["source_app_id"])
	require.Equal(t, target.ID, merged[0]["target_app_id"])
	require.Equal(t, "Notes Android", merged[0]["source_name"])

	// closed_days цели с первого дня данных источника (2026-08-10 в Москве) переоткрыты.
	var n int
	var last time.Time
	require.NoError(t, e.PG.Pool().QueryRow(ctx, "SELECT count(*), max(day) FROM closed_days WHERE app_id = $1", target.ID).Scan(&n, &last))
	require.Equal(t, 9, n, "2026-08-01…2026-08-09")
	require.Equal(t, "2026-08-09", last.Format(time.DateOnly))
}

// first_source_day — в часовом поясе цели: 2026-09-01T22:30Z в Москве — уже 2 сентября.
func TestMergeFirstSourceDayInTargetTimezone(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	require.NoError(t, e.CH.InsertEvents(t.Context(), events(source, "android", "u1", 1, time.Date(2026, 9, 1, 22, 30, 0, 0, time.UTC), time.Hour)))
	e.closeDays(t, target, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	m := decode[mergeJSON](t, e.createMerge(t, target, source))

	var failed []failedMerge
	runToDone(t, e.executor(clock.NewFixed(t0), &failed, merge.WithKeysWait(0)), uuid.New())
	got := e.getMerge(t, target, m.ID)
	require.Equal(t, "2026-09-02", got.FirstSourceDay.Time.Format(time.DateOnly))
	var last time.Time
	require.NoError(t, e.PG.Pool().QueryRow(t.Context(), "SELECT max(day) FROM closed_days WHERE app_id = $1", target.ID).Scan(&last))
	require.Equal(t, "2026-09-01", last.Format(time.DateOnly))
}

// AC-009.2, AC-8.5: ClickHouse закрывается после первой партиции — слияние failed с первой партицией
// в copied_partitions; «Повторить» докопирует остальное без дублей.
func TestMergeResumesAfterFailure(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	target := e.create(t, "Notes iOS", []string{"ios"}, "UTC")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	require.NoError(t, e.CH.InsertEvents(ctx, events(target, "ios", "u1", 100, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Hour)))
	e.seedSource(t, source)
	m := decode[mergeJSON](t, e.createMerge(t, target, source))

	clk := clock.NewFixed(t0)
	var failed []failedMerge
	conn := &faultyConn{Conn: e.openCH(t), closeAfter: 1}
	res, err := e.executorOn(conn, clk, &failed, merge.WithKeysWait(0)).RunOnce(ctx, uuid.New())
	require.Error(t, err)
	require.Equal(t, sqlc.AppMergeStatusFailed, res.Status)
	require.Equal(t, sqlc.AppMergeStepCopy, res.Step)
	got := e.getMerge(t, target, m.ID)
	require.Equal(t, []int32{202608}, got.CopiedPartitions)
	require.Equal(t, int64(400), got.CopiedEvents)
	require.NotNil(t, got.Error)
	require.NotEmpty(t, *got.Error)
	require.Len(t, failed, 1)
	// Источник цел, пока события не скопированы полностью.
	require.Equal(t, uint64(800), e.appRows(t, "events", source))
	require.Equal(t, 1, e.countWhere(t, "apps", source.ID))

	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+target.ID+"/merges/"+m.ID+"/retry", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	runToDone(t, e.executor(clk, &failed), uuid.New())
	require.Len(t, failed, 1)
	got = e.getMerge(t, target, m.ID)
	require.Equal(t, []int32{202608, 202609}, got.CopiedPartitions)
	require.Equal(t, int64(800), got.CopiedEvents)

	require.NoError(t, e.CH.Conn().Exec(ctx, "OPTIMIZE TABLE events FINAL"))
	params := clickhouse.Parameters{"app": target.ID}
	require.Equal(t, uint64(900), e.appRows(t, "events", target))
	require.Equal(t, uint64(900), e.chCount(t,
		"SELECT uniqExact(app_id, event_date, install_id, insert_id) FROM events WHERE app_id = {app:UUID}", params))
	require.Equal(t, uint64(0), e.appRows(t, "events", source))
}

// Сбой purge_source оставляет источник в Postgres; повтор шага copy целиком (все партиции скопированы) не
// дублирует ни события, ни журнал отказов.
func TestMergeRetriesAfterPurgeFailure(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	target := e.create(t, "Notes iOS", []string{"ios"}, "UTC")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	e.seedSource(t, source)
	require.NoError(t, e.CH.InsertRejected(ctx, []chstore.RejectedRow{rejectedRow(source)}))
	m := decode[mergeJSON](t, e.createMerge(t, target, source))

	clk := clock.NewFixed(t0)
	var failed []failedMerge
	conn := &faultyConn{Conn: e.CH.Conn(), failPrefix: "SYSTEM FLUSH ASYNC INSERT QUEUE"}
	res, err := e.executorOn(conn, clk, &failed, merge.WithKeysWait(0)).RunOnce(ctx, uuid.New())
	require.ErrorContains(t, err, "injected failure")
	require.Equal(t, sqlc.AppMergeStepPurgeSource, res.Step)
	require.Equal(t, sqlc.AppMergeStatusFailed, res.Status)
	require.Equal(t, 1, e.countWhere(t, "apps", source.ID))
	require.NotNil(t, e.getMerge(t, target, m.ID).SourceAppID)

	e.setMerge(t, m.ID, "status = 'failed', step = 'copy'")
	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+target.ID+"/merges/"+m.ID+"/retry", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	runToDone(t, e.executor(clk, &failed), uuid.New())

	require.Equal(t, uint64(800), e.appRows(t, "events", target))
	require.Equal(t, uint64(1), e.appRows(t, "rejected_events", target))
	require.Equal(t, uint64(0), e.appRows(t, "rejected_events", source))
	require.Equal(t, 0, e.countWhere(t, "apps", source.ID))
	require.Equal(t, int64(800), e.getMerge(t, target, m.ID).CopiedEvents)
}

func (e *env) countWhere(t *testing.T, table, id string) int {
	t.Helper()
	var n int
	require.NoError(t, e.PG.Pool().QueryRow(t.Context(), "SELECT count(*) FROM "+table+" WHERE id = $1", id).Scan(&n))
	return n
}

// createAPIKey — ключ Read API со scope read, привязанный к приложению app.
func (e *env) createAPIKey(t *testing.T, app appJSON, label string) uuid.UUID {
	t.Helper()
	hash := sha256.Sum256([]byte(label + app.ID))
	appID := uuid.MustParse(app.ID)
	row, err := e.PG.Queries().CreateApiKey(t.Context(), sqlc.CreateApiKeyParams{
		KeyPrefix: "sk_" + strings.ToLower(app.ID[:8]), KeyHash: hash[:], Label: label, Scopes: []string{"read"}, AppID: &appID,
	})
	require.NoError(t, err)
	return row.ID
}

// createDeletionRequest — заявка pending на удаление subject из приложения app.
func (e *env) createDeletionRequest(t *testing.T, app appJSON, subject sqlc.DeletionSubject, value string) uuid.UUID {
	t.Helper()
	req, err := e.PG.Queries().CreateDeletionRequest(t.Context(), sqlc.CreateDeletionRequestParams{
		AppID: uuid.MustParse(app.ID), SubjectType: subject, SubjectValue: value, Source: sqlc.DeletionSourceDashboard, Now: t0,
	})
	require.NoError(t, err)
	return req.ID
}
