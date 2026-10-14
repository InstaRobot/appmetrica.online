//go:build integration

package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
	"appmetrica.online/backend/internal/worker"
)

// closeNow — 2026-10-21 03:31 в Москве (UTC+3): закрывается день 2026-10-13.
var closeNow = time.Date(2026, 10, 21, 0, 31, 0, 0, time.UTC)

// flakyCloser — reports.Service, у которого первые failures вызовов CloseDay падают, как при недоступном ClickHouse.
type flakyCloser struct {
	*reports.Service
	mu       sync.Mutex
	failures int
}

func (f *flakyCloser) CloseDay(ctx context.Context, appID uuid.UUID, loc *time.Location, day time.Time) error {
	f.mu.Lock()
	fail := f.failures > 0
	f.failures--
	f.mu.Unlock()
	if fail {
		return errors.New("clickhouse: dial tcp: connection refused")
	}
	return f.Service.CloseDay(ctx, appID, loc, day)
}

type harness struct {
	env    *testutil.Env
	clock  *clock.Fixed
	stats  *flakyCloser
	sched  *worker.Scheduler
	logger *slog.Logger
}

func newHarness(t *testing.T, maxDays int) *harness {
	t.Helper()
	env := testutil.Start(t)
	env.LoadNDJSON(t, testutil.ScenarioPath())
	clk := clock.NewFixed(closeNow)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stats := &flakyCloser{Service: reports.NewService(env.CH.Conn(), clk)}
	return &harness{
		env: env, clock: clk, stats: stats, logger: logger,
		sched: worker.NewScheduler(env.PG.Queries(), clk, logger, []string{"close_day"},
			worker.CloseDayJob(env.PG.Queries(), stats, env.PG, clk, maxDays)),
	}
}

// addApp — приложение сценария в Postgres, созданное в полночь createdDay по своей TZ.
func (h *harness) addApp(t *testing.T, app testutil.ScenarioApp, createdDay string, archived bool) {
	t.Helper()
	loc, err := time.LoadLocation(app.Timezone)
	require.NoError(t, err)
	d, err := time.ParseInLocation(time.DateOnly, createdDay, loc)
	require.NoError(t, err)
	status, archivedAt := "active", (*time.Time)(nil)
	if archived {
		status, archivedAt = "archived", &d
	}
	_, err = h.env.PG.Pool().Exec(t.Context(),
		`INSERT INTO apps (id, name, platform, timezone, status, created_at, archived_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		app.ID, app.Name, app.Platform, app.Timezone, status, d, archivedAt)
	require.NoError(t, err)
}

func (h *harness) run(t *testing.T) error {
	t.Helper()
	return h.sched.RunOnce(t.Context(), "close_day")
}

func (h *harness) closedDays(t *testing.T, appID uuid.UUID) []string {
	t.Helper()
	rows, err := h.env.PG.Pool().Query(t.Context(), `SELECT day::text FROM closed_days WHERE app_id = $1 ORDER BY day`, appID)
	require.NoError(t, err)
	days, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	return days
}

type jobRun struct {
	Status  string
	Details map[string]any
}

func (h *harness) jobRuns(t *testing.T) []jobRun {
	t.Helper()
	rows, err := h.env.PG.Pool().Query(t.Context(),
		`SELECT status::text, details FROM job_runs WHERE job = 'close_day' ORDER BY started_at, id`)
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (jobRun, error) {
		var (
			jr  jobRun
			raw []byte
		)
		if err := r.Scan(&jr.Status, &raw); err != nil {
			return jr, err
		}
		return jr, json.Unmarshal(raw, &jr.Details)
	})
	require.NoError(t, err)
	return out
}

type statRow struct {
	Env, Unit                                                  string
	Active, New, Sessions, SessionsWithDuration, DurationMsSum uint64
}

// dailyStats — строки daily_stats дня после FINAL.
func (h *harness) dailyStats(t *testing.T, appID uuid.UUID, day string) []statRow {
	t.Helper()
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(clickhouse.Parameters{"app_id": appID.String(), "day": day}))
	rows, err := h.env.CH.Conn().Query(ctx, `SELECT environment, unit, active, new, sessions, sessions_with_duration, duration_ms_sum
FROM daily_stats FINAL WHERE app_id = {app_id:UUID} AND day = {day:Date} ORDER BY environment, unit`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var out []statRow
	for rows.Next() {
		var r statRow
		require.NoError(t, rows.Scan(&r.Env, &r.Unit, &r.Active, &r.New, &r.Sessions, &r.SessionsWithDuration, &r.DurationMsSum))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// physicalRows — строки daily_stats приложения после OPTIMIZE FINAL (без FINAL в запросе).
func (h *harness) physicalRows(t *testing.T, appID uuid.UUID) uint64 {
	t.Helper()
	require.NoError(t, h.env.CH.Conn().Exec(t.Context(), "OPTIMIZE TABLE daily_stats FINAL"))
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(clickhouse.Parameters{"app_id": appID.String()}))
	var n uint64
	require.NoError(t, h.env.CH.Conn().QueryRow(ctx, `SELECT count() FROM daily_stats WHERE app_id = {app_id:UUID}`).Scan(&n))
	return n
}

// AC-050.1 (AC-8.3.1): в 03:31 по Москве закрывается 2026-10-13 — 4 строки daily_stats, значения равны
// live-расчёту getAudienceSummary за этот день, строка closed_days.
func TestCloseDay_WritesFourRows(t *testing.T) {
	h := newHarness(t, worker.DefaultCloseMaxDays)
	msk := testutil.ScenarioApps[0]
	h.addApp(t, msk, "2026-10-13", false)

	require.NoError(t, h.run(t))

	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, msk.ID))
	got := h.dailyStats(t, msk.ID, "2026-10-13")
	require.Len(t, got, 4)
	loc, err := time.LoadLocation(msk.Timezone)
	require.NoError(t, err)
	svc := reports.NewService(h.env.CH.Conn(), h.clock)
	i := 0
	for _, env := range []string{reports.EnvDevelopment, reports.EnvProduction} {
		for _, unit := range []identity.Unit{identity.Installs, identity.Persons} {
			live, err := svc.AudienceSummary(t.Context(), reports.Params{
				AppID: msk.ID, Location: loc, Env: env, Unit: unit,
				Period: reports.Period{From: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)},
			}, false)
			require.NoError(t, err)
			row := got[i]
			i++
			require.Equal(t, env, row.Env)
			require.Equal(t, string(unit), row.Unit)
			require.Equal(t, []uint64{live.Active, live.New, live.Sessions}, []uint64{row.Active, row.New, row.Sessions}, "%s %s", env, unit)
		}
	}
	require.Positive(t, got[3].Active, "production persons за 2026-10-13 в сценарии не пустой")

	runs := h.jobRuns(t)
	require.Len(t, runs, 1)
	require.Equal(t, "ok", runs[0].Status)
	require.InDelta(t, 1, runs[0].Details["closed_days"], 0)
	require.Equal(t, []any{map[string]any{"app_id": msk.ID.String(), "from": "2026-10-13", "to": "2026-10-13"}}, runs[0].Details["apps"])
}

// AC-050.2 (AC-8.3.2): в 03:31 по Москве (20:31 накануне в Нью-Йорке) из двух приложений закрыто только
// московское; архивное приложение (Токио, 09:31) тоже закрывается; Нью-Йорк закрывается после 03:30 по своей TZ.
func TestCloseDay_RespectsAppTimezone(t *testing.T) {
	h := newHarness(t, worker.DefaultCloseMaxDays)
	msk, nyc, tyo := testutil.ScenarioApps[0], testutil.ScenarioApps[1], testutil.ScenarioApps[2]
	h.addApp(t, msk, "2026-10-13", false)
	h.addApp(t, nyc, "2026-10-13", false)
	h.addApp(t, tyo, "2026-10-13", true)

	h.clock.Set(closeNow.Add(-2 * time.Minute)) // 03:29 по Москве
	require.NoError(t, h.run(t))
	require.Empty(t, h.closedDays(t, msk.ID))
	require.Empty(t, h.closedDays(t, nyc.ID))
	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, tyo.ID))

	h.clock.Set(closeNow)
	require.NoError(t, h.run(t))
	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, msk.ID))
	require.Empty(t, h.closedDays(t, nyc.ID), "в Нью-Йорке ещё 2026-10-20")
	require.Len(t, h.dailyStats(t, msk.ID, "2026-10-13"), 4)
	require.Empty(t, h.dailyStats(t, nyc.ID, "2026-10-13"))

	h.clock.Set(time.Date(2026, 10, 21, 7, 30, 0, 0, time.UTC)) // 03:30 в Нью-Йорке
	require.NoError(t, h.run(t))
	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, nyc.ID))
	require.Len(t, h.dailyStats(t, nyc.ID, "2026-10-13"), 4)
}

// AC-050.3 (AC-8.3.3): повторный запуск по закрытому дню не добавляет строк closed_days, в daily_stats после
// FINAL — 4 строки; сбой между записью daily_stats и closed_days тоже не плодит строк.
func TestCloseDay_Idempotent(t *testing.T) {
	h := newHarness(t, worker.DefaultCloseMaxDays)
	msk := testutil.ScenarioApps[0]
	h.addApp(t, msk, "2026-10-13", false)

	require.NoError(t, h.run(t))
	want := h.dailyStats(t, msk.ID, "2026-10-13")
	h.clock.Advance(15 * time.Minute)
	require.NoError(t, h.run(t))
	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, msk.ID))
	require.Equal(t, uint64(4), h.physicalRows(t, msk.ID))

	// daily_stats записан, closed_days — нет (worker упал между ними): следующий запуск закрывает день заново.
	_, err := h.env.PG.Pool().Exec(t.Context(), `DELETE FROM closed_days WHERE app_id = $1`, msk.ID)
	require.NoError(t, err)
	h.clock.Advance(15 * time.Minute)
	require.NoError(t, h.run(t))
	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, msk.ID))
	require.Equal(t, uint64(4), h.physicalRows(t, msk.ID))
	require.Equal(t, want, h.dailyStats(t, msk.ID, "2026-10-13"))
}

// AC-050.4 (AC-8.3.4): первая попытка упала на ClickHouse — closed_days не записан, job_runs failed; следующий
// цикл закрывает день, в job_runs — failed и ok.
func TestCloseDay_RetriesAfterFailure(t *testing.T) {
	h := newHarness(t, worker.DefaultCloseMaxDays)
	msk := testutil.ScenarioApps[0]
	h.addApp(t, msk, "2026-10-13", false)
	h.stats.failures = 1

	require.Error(t, h.run(t))
	require.Empty(t, h.closedDays(t, msk.ID))

	h.clock.Advance(15 * time.Minute)
	require.NoError(t, h.run(t))
	require.Equal(t, []string{"2026-10-13"}, h.closedDays(t, msk.ID))
	require.Len(t, h.dailyStats(t, msk.ID, "2026-10-13"), 4)

	runs := h.jobRuns(t)
	require.Len(t, runs, 2)
	require.Equal(t, "failed", runs[0].Status)
	require.Contains(t, runs[0].Details["error"], "connection refused")
	require.Equal(t, "ok", runs[1].Status)
}

// FR-022: первый запуск закрывает дни с apps.created_at (в TZ приложения), не больше лимита за проход;
// следующие запуски продолжают с последнего закрытого дня.
func TestCloseDay_CatchesUpFromCreatedAt(t *testing.T) {
	h := newHarness(t, 4)
	msk := testutil.ScenarioApps[0]
	h.addApp(t, msk, "2026-10-05", false)

	require.NoError(t, h.run(t))
	require.Equal(t, []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08"}, h.closedDays(t, msk.ID))
	require.NoError(t, h.run(t))
	require.NoError(t, h.run(t))
	require.NoError(t, h.run(t))
	days := h.closedDays(t, msk.ID)
	require.Len(t, days, 9)
	require.Equal(t, "2026-10-13", days[8])
	for _, d := range days {
		require.Len(t, h.dailyStats(t, msk.ID, d), 4, d)
	}
	require.Empty(t, h.dailyStats(t, msk.ID, "2026-10-14"), "открытые дни не закрываются")
}
