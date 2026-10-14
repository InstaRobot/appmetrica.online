//go:build integration

package reports

import (
	"math"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// closedRow — строка daily_stats после FINAL.
type closedRow struct {
	Env, Unit                                                  string
	Active, New, Sessions, SessionsWithDuration, DurationMsSum uint64
	ClosedAt                                                   time.Time
}

func closedRows(t *testing.T, env *testutil.Env, appID uuid.UUID, day string) []closedRow {
	t.Helper()
	ctx := clickhouse.Context(t.Context(), clickhouse.WithParameters(clickhouse.Parameters{"app_id": appID.String(), "day": day}))
	rows, err := env.CH.Conn().Query(ctx, `SELECT environment, unit, active, new, sessions, sessions_with_duration, duration_ms_sum, closed_at
FROM daily_stats FINAL WHERE app_id = {app_id:UUID} AND day = {day:Date} ORDER BY environment, unit`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var out []closedRow
	for rows.Next() {
		var r closedRow
		require.NoError(t, rows.Scan(&r.Env, &r.Unit, &r.Active, &r.New, &r.Sessions, &r.SessionsWithDuration, &r.DurationMsSum, &r.ClosedAt))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// AC-050.1 (AC-8.3.1): закрытый день — 4 строки (development/production × installs/persons), значения которых
// совпадают с getAudienceSummary за этот день: активные, новые, сессии и средняя длительность.
func TestCloseDay_MatchesAudienceSummary(t *testing.T) {
	env := testutil.Start(t)
	env.LoadNDJSON(t, testutil.ScenarioPath())
	closedAt := time.Date(2026, 10, 21, 0, 31, 0, 0, time.UTC)
	svc := NewService(env.CH.Conn(), clock.NewFixed(closedAt))

	var nonEmpty int
	for _, app := range testutil.ScenarioApps {
		loc := location(t, app.Timezone)
		for _, day := range []string{"2026-10-04", "2026-10-13", "2026-10-25"} { // 10-25 — переход на зимнее время в Европе
			require.NoError(t, svc.CloseDay(t.Context(), app.ID, loc, date(t, day)))
			rows := closedRows(t, env, app.ID, day)
			require.Len(t, rows, 4, "%s %s", app.Code, day)
			i := 0
			for _, e := range []string{EnvDevelopment, EnvProduction} {
				for _, u := range []identity.Unit{identity.Installs, identity.Persons} {
					got := rows[i]
					i++
					require.Equal(t, e, got.Env)
					require.Equal(t, string(u), got.Unit)
					require.Equal(t, closedAt, got.ClosedAt.UTC())

					p := Params{AppID: app.ID, Location: loc, Env: e, Unit: u, Period: Period{From: date(t, day), To: date(t, day)}}
					live, err := svc.AudienceSummary(t.Context(), p, false)
					require.NoError(t, err)
					msg := []any{"%s %s %s %s", app.Code, day, e, u}
					require.Equal(t, live.Active, got.Active, msg...)
					require.Equal(t, live.New, got.New, msg...)
					require.Equal(t, live.Sessions, got.Sessions, msg...)
					if got.SessionsWithDuration == 0 {
						require.Nil(t, live.AvgDurationMs, msg...)
						require.Zero(t, got.DurationMsSum, msg...)
					} else {
						require.NotNil(t, live.AvgDurationMs, msg...)
						require.InDelta(t, *live.AvgDurationMs,
							math.RoundToEven(float64(got.DurationMsSum)/float64(got.SessionsWithDuration)), 0, msg...) // round() ClickHouse — к чётному
					}
					if got.Active > 0 {
						nonEmpty++
					}
				}
			}
		}
	}
	require.Positive(t, nonEmpty, "в сценарии есть активные за проверяемые дни")
}

// AC-050.3: повторное закрытие перезаписывает строки дня — после FINAL их по-прежнему 4, closed_at — новый.
func TestCloseDay_RepeatReplacesRows(t *testing.T) {
	env := testutil.Start(t)
	env.LoadNDJSON(t, testutil.ScenarioPath())
	clk := clock.NewFixed(time.Date(2026, 10, 21, 0, 31, 0, 0, time.UTC))
	svc := NewService(env.CH.Conn(), clk)
	msk := testutil.ScenarioApps[0]
	loc := location(t, msk.Timezone)

	require.NoError(t, svc.CloseDay(t.Context(), msk.ID, loc, date(t, "2026-10-13")))
	first := closedRows(t, env, msk.ID, "2026-10-13")
	clk.Advance(15 * time.Minute)
	require.NoError(t, svc.CloseDay(t.Context(), msk.ID, loc, date(t, "2026-10-13")))

	require.NoError(t, env.CH.Conn().Exec(t.Context(), "OPTIMIZE TABLE daily_stats FINAL"))
	var n uint64
	require.NoError(t, env.CH.Conn().QueryRow(t.Context(), "SELECT count() FROM daily_stats").Scan(&n))
	require.Equal(t, uint64(4), n)
	second := closedRows(t, env, msk.ID, "2026-10-13")
	for i := range second {
		require.Equal(t, clk.Now(), second[i].ClosedAt.UTC())
		second[i].ClosedAt = first[i].ClosedAt
	}
	require.Equal(t, first, second)
}

func TestCloseDay_ClickHouseError(t *testing.T) {
	env := testutil.Start(t)
	svc := NewService(env.CH.Conn(), clock.NewFixed(time.Date(2026, 10, 21, 0, 31, 0, 0, time.UTC)))
	require.NoError(t, env.CH.Conn().Exec(t.Context(), "DROP TABLE daily_stats"))
	err := svc.CloseDay(t.Context(), testutil.ScenarioApps[0].ID, time.UTC, date(t, "2026-10-13"))
	require.ErrorContains(t, err, "reports: close day 2026-10-13")
}
