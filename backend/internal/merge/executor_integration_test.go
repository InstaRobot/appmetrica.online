//go:build integration

package merge_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/pgstore/sqlc"
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
	onFailed := func(_ context.Context, m sqlc.AppMerge, cause string) {
		*failed = append(*failed, failedMerge{m.ID, cause})
	}
	return merge.NewExecutor(e.PG, clk, slog.New(slog.DiscardHandler), onFailed, opts...)
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

	clk.Set(t0.Add(deletion.AppPurgeDelay))
	res, err = exec.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStepCopy, res.Step)
	require.Equal(t, sqlc.AppMergeStepCopy, e.getMerge(t, target, m.ID).Step)
	require.Empty(t, failed)

	// Слияние ещё активно: разархивировать источник нельзя.
	requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/unarchive", nil), http.StatusConflict, "merge_in_progress")
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
