//go:build integration

package deletion_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

var now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type harness struct {
	env    *testutil.Env
	clock  *clock.Fixed
	logger *slog.Logger
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{
		env:    testutil.Start(t),
		clock:  clock.NewFixed(now),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func (h *harness) createApp(t *testing.T, name string) uuid.UUID {
	t.Helper()
	app, err := h.env.PG.Queries().CreateApp(t.Context(), sqlc.CreateAppParams{
		Name: name, Platform: sqlc.AppPlatformIos, Timezone: "Europe/Moscow",
	})
	require.NoError(t, err)
	return app.ID
}

func (h *harness) request(t *testing.T, appID uuid.UUID, subject, value string) uuid.UUID {
	t.Helper()
	dr, err := deletion.NewService(h.env.PG, h.clock, h.logger).Create(t.Context(), deletion.CreateInput{
		AppID: appID, SubjectType: subject, SubjectValue: value,
	})
	require.NoError(t, err)
	return dr.ID
}

func (h *harness) jobRun(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := h.env.PG.Queries().StartJobRun(t.Context(), sqlc.StartJobRunParams{Job: sqlc.JobNameDeletion, Now: h.clock.Now()})
	require.NoError(t, err)
	return id
}

func (h *harness) executor(purger deletion.Purger) *deletion.Executor {
	if purger == nil {
		purger = deletion.NewClickHousePurger(h.env.CH.Conn())
	}
	return deletion.NewExecutor(h.env.PG, purger, h.clock, h.logger, nil)
}

func (h *harness) status(t *testing.T, appID, id uuid.UUID) sqlc.DeletionRequest {
	t.Helper()
	dr, err := h.env.PG.Queries().GetDeletionRequest(t.Context(), sqlc.GetDeletionRequestParams{ID: id, AppID: appID})
	require.NoError(t, err)
	return dr
}

// event — session_start установки install в день day (UTC) с user_id user.
func event(appID, install uuid.UUID, user string, at time.Time) chstore.EventRow {
	return chstore.EventRow{
		AppID: appID, EventDate: at.Truncate(24 * time.Hour), EventTime: at, ClientTime: at, SentAt: at, ReceivedAt: at,
		InsertID: uuid.New(), Name: "session_start", InstallID: install, UserID: user, SessionID: uuid.New(),
		Environment: "production", Platform: "ios", OSName: "iOS", AppVersion: "1.0.0", Country: "RU",
	}
}

func rejected(appID uuid.UUID, payload string) chstore.RejectedRow {
	return chstore.RejectedRow{
		ReceivedAt: now.Add(-time.Hour), AppID: appID, KeyPrefix: "pk_test", Code: "invalid_event",
		Field: "events[0].timestamp", Message: "bad", EventName: "session_start", Platform: "ios", PayloadSample: payload,
	}
}

func insertDailyStats(t *testing.T, env *testutil.Env, appID uuid.UUID) {
	t.Helper()
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(clickhouse.Parameters{"app_id": appID.String()}))
	require.NoError(t, env.CH.Conn().Exec(ctx, `INSERT INTO daily_stats
SELECT {app_id:UUID}, toDate('2026-10-05'), env, unit, 10, 2, 12, 11, 600000, now64(3)
FROM (SELECT arrayJoin(['production', 'development']) AS env) AS e
CROSS JOIN (SELECT arrayJoin(['persons', 'installs']) AS unit) AS u`))
}

// count — число строк таблицы table, удовлетворяющих where (параметры — params).
func count(t *testing.T, env *testutil.Env, table, where string, params clickhouse.Parameters) uint64 {
	t.Helper()
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(params))
	var n uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT count() FROM "+table+" WHERE "+where).Scan(&n))
	return n
}

func dailyStats(t *testing.T, env *testutil.Env, appID uuid.UUID) string {
	t.Helper()
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(clickhouse.Parameters{"app_id": appID.String()}))
	var out string
	require.NoError(t, env.CH.Conn().QueryRow(ctx, `SELECT toString(groupArray((environment, unit, day, active, new, sessions)))
FROM (SELECT * FROM daily_stats FINAL WHERE app_id = {app_id:UUID} ORDER BY environment, unit, day)`).Scan(&out))
	return out
}

// AC-046.1 / AC-6.2.1: удаление пользователя U убирает его установки (связанные с ним как с первым владельцем)
// и все события с user_id = U; данные других людей и daily_stats не меняются.
func TestDeletion_UserRemovesAllLinkedData(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	appID := h.createApp(t, "App A")
	otherApp := h.createApp(t, "App B")
	const user = `o'brien\x "42"`
	i1, i2, i3, shared := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	day := now.Add(-72 * time.Hour)

	var rows []chstore.EventRow
	for d := range 3 {
		at := day.Add(time.Duration(d) * 24 * time.Hour)
		rows = append(rows,
			event(appID, i1, "", at),                    // анонимная сессия до входа
			event(appID, i1, user, at.Add(time.Hour)),   // I1 связана с U
			event(appID, i2, user, at.Add(2*time.Hour)), // I2 — явный user_id U
			event(appID, i3, "u2", at.Add(3*time.Hour)), // посторонний u2
			event(appID, i3, "", at.Add(4*time.Hour)),
			event(appID, shared, "u2", at.Add(5*time.Hour)), // общее устройство: первый владелец u2
			event(appID, shared, user, at.Add(6*time.Hour)), // … затем на нём входил U
			event(otherApp, i1, user, at.Add(7*time.Hour)),  // тот же U в другом приложении
		)
	}
	require.NoError(t, h.env.CH.InsertEvents(ctx, rows))
	require.NoError(t, h.env.CH.InsertRejected(ctx, []chstore.RejectedRow{
		rejected(appID, `{"install_id":"`+i1.String()+`","name":"session_start"}`),
		rejected(appID, `{"install_id":"`+uuidUpper(i2)+`"}`),
		rejected(appID, `{"user_id":"o'brien\\x \"42\"","install_id":"`+i3.String()+`"}`),
		rejected(appID, `{"install_id":"`+i3.String()+`","user_id":"u2"}`),
		rejected(otherApp, `{"install_id":"`+i1.String()+`"}`),
	}))
	insertDailyStats(t, h.env, appID)
	statsBefore := dailyStats(t, h.env, appID)

	id := h.request(t, appID, "user", user)
	res, err := h.executor(nil).RunOnce(ctx, h.jobRun(t))
	require.NoError(t, err)
	require.Equal(t, deletion.Result{Done: 1}, res)

	dr := h.status(t, appID, id)
	require.Equal(t, sqlc.DeletionStatusDone, dr.Status)
	require.Equal(t, now, dr.CompletedAt.UTC())
	require.Nil(t, dr.Error)

	p := clickhouse.Parameters{
		// user_id сравнивается через hex: тест не зависит от экранирования параметров String.
		"app": appID.String(), "user_hex": strings.ToUpper(hex.EncodeToString([]byte(user))), "i1": i1.String(), "i2": i2.String(), "i3": i3.String(),
		"shared": shared.String(), "other": otherApp.String(),
	}
	mine := "app_id = {app:UUID} AND install_id IN ({i1:UUID}, {i2:UUID})"
	require.Zero(t, count(t, h.env, "events", "app_id = {app:UUID} AND (hex(user_id) = {user_hex:String} OR install_id IN ({i1:UUID}, {i2:UUID}))", p))
	require.Zero(t, count(t, h.env, "install_first_seen", mine, p))
	require.Zero(t, count(t, h.env, "identity_links", mine, p))
	require.Zero(t, count(t, h.env, "user_first_seen", "app_id = {app:UUID} AND hex(user_id) = {user_hex:String}", p))

	// u2 и общее устройство (его первый владелец — u2) на месте; из общего устройства ушли только события U.
	require.Equal(t, uint64(6), count(t, h.env, "events", "app_id = {app:UUID} AND install_id = {i3:UUID}", p))
	require.Equal(t, uint64(3), count(t, h.env, "events", "app_id = {app:UUID} AND install_id = {shared:UUID}", p))
	require.Equal(t, uint64(2), count(t, h.env, "install_first_seen", "app_id = {app:UUID} AND install_id IN ({i3:UUID}, {shared:UUID})", p))
	require.Equal(t, uint64(2), count(t, h.env, "identity_links", "app_id = {app:UUID} AND install_id IN ({i3:UUID}, {shared:UUID})", p))
	require.Equal(t, uint64(1), count(t, h.env, "user_first_seen", "app_id = {app:UUID} AND user_id = 'u2'", p))

	// rejected_events: упоминания I1, I2 (в любом регистре) и U удалены, остальное на месте.
	require.Equal(t, uint64(1), count(t, h.env, "rejected_events", "app_id = {app:UUID}", p))
	require.Equal(t, uint64(1), count(t, h.env, "rejected_events", "app_id = {app:UUID} AND position(payload_sample, '\"u2\"') > 0", p))

	// Другое приложение не тронуто.
	require.Equal(t, uint64(3), count(t, h.env, "events", "app_id = {other:UUID}", p))
	require.Equal(t, uint64(1), count(t, h.env, "user_first_seen", "app_id = {other:UUID}", p))
	require.Equal(t, uint64(1), count(t, h.env, "rejected_events", "app_id = {other:UUID}", p))

	// daily_stats — агрегаты без персональных данных — не меняются.
	require.Equal(t, statsBefore, dailyStats(t, h.env, appID))
}

func TestDeletion_InstallRemovesInstallData(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	appID := h.createApp(t, "App A")
	target, keep := uuid.New(), uuid.New()
	require.NoError(t, h.env.CH.InsertEvents(ctx, []chstore.EventRow{
		event(appID, target, "u1", now.Add(-2*time.Hour)),
		event(appID, target, "", now.Add(-time.Hour)),
		event(appID, keep, "u1", now.Add(-time.Hour)),
	}))
	require.NoError(t, h.env.CH.InsertRejected(ctx, []chstore.RejectedRow{
		rejected(appID, `{"install_id":"`+target.String()+`"}`),
		rejected(appID, `{"install_id":"`+keep.String()+`"}`),
	}))

	id := h.request(t, appID, "install", target.String())
	res, err := h.executor(nil).RunOnce(ctx, h.jobRun(t))
	require.NoError(t, err)
	require.Equal(t, deletion.Result{Done: 1}, res)
	require.Equal(t, sqlc.DeletionStatusDone, h.status(t, appID, id).Status)

	p := clickhouse.Parameters{"app": appID.String(), "target": target.String(), "keep": keep.String()}
	for _, table := range []string{"events", "install_first_seen", "identity_links"} {
		require.Zero(t, count(t, h.env, table, "app_id = {app:UUID} AND install_id = {target:UUID}", p), table)
	}
	require.Equal(t, uint64(1), count(t, h.env, "events", "app_id = {app:UUID} AND install_id = {keep:UUID}", p))
	require.Equal(t, uint64(1), count(t, h.env, "rejected_events", "app_id = {app:UUID}", p))
	require.Equal(t, uint64(1), count(t, h.env, "rejected_events", "app_id = {app:UUID} AND position(payload_sample, {keep:String}) > 0", p))
	// user_first_seen по install не чистится: u1 остался на другой установке.
	require.Equal(t, uint64(1), count(t, h.env, "user_first_seen", "app_id = {app:UUID} AND user_id = 'u1'", p))
}

// AC-046.2 / AC-6.2.4: удаление приложения очищает все таблицы ClickHouse по app_id, удаляет строку apps
// (каскадом — ключи и запросы) и сохраняет копию запроса в job_runs.details цикла.
func TestDeletion_AppRemovesEverything(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	appB := h.createApp(t, "App B")
	keep := h.createApp(t, "Keep")
	for _, appID := range []uuid.UUID{appB, keep} {
		install := uuid.New()
		require.NoError(t, h.env.CH.InsertEvents(ctx, []chstore.EventRow{
			event(appID, install, "u1", now.Add(-48*time.Hour)),
			event(appID, install, "", now.Add(-24*time.Hour)),
		}))
		require.NoError(t, h.env.CH.InsertRejected(ctx, []chstore.RejectedRow{rejected(appID, `{}`)}))
		insertDailyStats(t, h.env, appID)
	}
	userReq := h.request(t, appB, "user", "u1") // снимается каскадом вместе с приложением
	dr, err := apps.NewService(h.env.PG, h.clock, h.logger).Delete(ctx, appB, "App B")
	require.NoError(t, err)

	runID := h.jobRun(t)
	res, err := h.executor(nil).RunOnce(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, deletion.Result{Done: 2}, res)

	q := h.env.PG.Queries()
	_, err = q.GetApp(ctx, appB)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	_, err = q.GetDeletionRequest(ctx, sqlc.GetDeletionRequestParams{ID: userReq, AppID: appB})
	require.ErrorIs(t, err, pgx.ErrNoRows)
	_, err = q.GetApp(ctx, keep)
	require.NoError(t, err)

	p := clickhouse.Parameters{"app": appB.String(), "keep": keep.String()}
	tables := []string{"events", "install_first_seen", "user_first_seen", "identity_links", "rejected_events", "daily_stats"}
	for _, table := range tables {
		require.Zero(t, count(t, h.env, table, "app_id = {app:UUID}", p), table)
		require.NotZero(t, count(t, h.env, table, "app_id = {keep:UUID}", p), table)
	}

	// Цикл завершается FinishJobRun со своими счётчиками — копия запроса из deleteApp не теряется.
	require.NoError(t, q.FinishJobRun(ctx, sqlc.FinishJobRunParams{
		Now: new(h.clock.Now()), Status: sqlc.JobStatusOk, Details: json.RawMessage(`{"done":1}`), ID: runID,
	}))
	runs, err := q.ListLatestJobRuns(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	var details struct {
		Done        int `json:"done"`
		DeletedApps []struct {
			ID           uuid.UUID `json:"id"`
			AppID        uuid.UUID `json:"app_id"`
			SubjectType  string    `json:"subject_type"`
			SubjectValue string    `json:"subject_value"`
			Source       string    `json:"source"`
			Status       string    `json:"status"`
			Attempts     int       `json:"attempts"`
			RequestedAt  time.Time `json:"requested_at"`
			CompletedAt  time.Time `json:"completed_at"`
		} `json:"deleted_apps"`
	}
	require.NoError(t, json.Unmarshal(runs[0].Details, &details))
	require.Equal(t, 1, details.Done)
	require.Len(t, details.DeletedApps, 1)
	got := details.DeletedApps[0]
	require.Equal(t, dr.ID, got.ID)
	require.Equal(t, appB, got.AppID)
	require.Equal(t, "app", got.SubjectType)
	require.Equal(t, appB.String(), got.SubjectValue)
	require.Equal(t, "dashboard", got.Source)
	require.Equal(t, "done", got.Status)
	require.Equal(t, 1, got.Attempts)
	require.True(t, now.Equal(got.CompletedAt))
}

type failingPurger struct{ deletion.Purger }

func (failingPurger) Ping(context.Context) error { return nil }

func (failingPurger) PurgeUser(context.Context, uuid.UUID, string) error {
	return errors.New("code: 159, timeout exceeded")
}

// Семантика FailDeletionRequest и CHECK attempts ≤ 5 в настоящем Postgres: 5 неудачных циклов — failed.
func TestDeletion_FailsAfterFiveAttemptsInPostgres(t *testing.T) {
	h := newHarness(t)
	appID := h.createApp(t, "App A")
	id := h.request(t, appID, "user", "u1")
	ex := h.executor(failingPurger{})

	for cycle := 1; cycle <= 5; cycle++ {
		_, err := ex.RunOnce(t.Context(), h.jobRun(t))
		require.Error(t, err)
		h.clock.Advance(time.Minute)
	}
	dr := h.status(t, appID, id)
	require.Equal(t, sqlc.DeletionStatusFailed, dr.Status)
	require.Equal(t, int32(5), dr.Attempts)
	require.NotNil(t, dr.Error)
	require.Contains(t, *dr.Error, "timeout exceeded")

	res, err := ex.RunOnce(t.Context(), h.jobRun(t))
	require.NoError(t, err)
	require.Equal(t, deletion.Result{}, res)
}

type stallingPurger struct{ deletion.Purger }

func (stallingPurger) Ping(context.Context) error { return nil }

// Зависший running с исчерпанными попытками помечается failed, а не возвращается в pending:
// следующий claim нарушил бы CHECK attempts ≤ 5.
func TestDeletion_StuckRequestWithLastAttemptFails(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	appID := h.createApp(t, "App A")
	id := h.request(t, appID, "user", "u1")
	_, err := h.env.PG.Pool().Exec(ctx,
		`UPDATE deletion_requests SET status = 'running', attempts = 5, started_at = $2 WHERE id = $1`, id, now.Add(-2*time.Hour))
	require.NoError(t, err)

	res, err := h.executor(stallingPurger{}).RunOnce(ctx, h.jobRun(t))
	require.NoError(t, err)
	require.Equal(t, deletion.Result{Requeued: 1}, res)
	dr := h.status(t, appID, id)
	require.Equal(t, sqlc.DeletionStatusFailed, dr.Status)
	require.NotNil(t, dr.Error)
}

func uuidUpper(id uuid.UUID) string {
	b := []byte(id.String())
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}
