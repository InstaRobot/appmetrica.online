//go:build integration

package reports

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// portfolioNow — «сейчас» golden-теста: 2026-10-13 во всех трёх поясах сценария.
var portfolioNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func location(t testing.TB, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func scenarioPortfolioApps(t testing.TB) []PortfolioApp {
	t.Helper()
	out := make([]PortfolioApp, 0, len(testutil.ScenarioApps))
	for _, app := range testutil.ScenarioApps {
		out = append(out, PortfolioApp{ID: app.ID, Location: location(t, app.Timezone)})
	}
	return out
}

func portfolio(t *testing.T, svc *Service, q PortfolioQuery) Portfolio {
	t.Helper()
	got, err := svc.Portfolio(t.Context(), q)
	require.NoError(t, err)
	require.Len(t, got.Rows, len(q.Apps))
	for i, row := range got.Rows {
		require.Equal(t, q.Apps[i].ID, row.AppID, "строки в порядке приложений запроса")
		require.Len(t, row.Sparkline, 30)
	}
	return got
}

func identityPortfolio(t *testing.T, svc *Service, env string, unit identity.Unit, day string) PortfolioRow {
	t.Helper()
	got := portfolio(t, svc, PortfolioQuery{
		Date: date(t, day), Env: env, Unit: unit,
		Apps: []PortfolioApp{{ID: identityApp, Location: location(t, "Europe/Moscow")}},
	})
	return got.Rows[0]
}

// AC-028.1: портфель сценария на 2026-10-12 совпадает с golden для persons и installs; persons сверяется
// с эталонной моделью фикстур (testutil.ScenarioStats), installs — не меньше persons.
func TestPortfolio_Golden(t *testing.T) {
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, testutil.ScenarioPath())
	svc := NewService(env.CH.Conn(), clock.NewFixed(portfolioNow))

	got := map[identity.Unit]Portfolio{}
	for _, unit := range []identity.Unit{identity.Persons, identity.Installs} {
		got[unit] = portfolio(t, svc, PortfolioQuery{
			Date: date(t, "2026-10-12"), Env: EnvProduction, Unit: unit, Apps: scenarioPortfolioApps(t),
		})
		require.Equal(t, "2026-10-12", got[unit].Date)
		require.Equal(t, EnvProduction, got[unit].Environment)
		require.Equal(t, unit, got[unit].Unit)
	}
	testutil.AssertGolden(t, "portfolio.golden.json", got)

	for i, app := range testutil.ScenarioApps {
		t.Run(app.Code, func(t *testing.T) {
			stats, err := testutil.ScenarioStats(rows, app)
			require.NoError(t, err)
			model := map[string]testutil.ScenarioDay{}
			for _, d := range stats {
				model[d.Day] = d
			}

			row := got[identity.Persons].Rows[i]
			require.Equal(t, uint64(model["2026-10-13"].Active), row.ActiveToday)
			require.Equal(t, uint64(model["2026-10-12"].Active), row.ActiveDate)
			require.Equal(t, uint64(model["2026-10-05"].Active), row.ActiveSameDayPrevWeek)
			require.Equal(t, uint64(model["2026-10-12"].New), row.NewDate)
			require.Equal(t, ratio(uint64(model["2026-10-11"].D1Returned), uint64(model["2026-10-11"].New)), row.RetentionD1)
			require.Equal(t, deltaPct(row.ActiveDate, row.ActiveSameDayPrevWeek), row.ActiveDeltaPct)
			require.Positive(t, row.ActiveDate)
			require.Positive(t, row.SessionsDate)
			for _, p := range row.Sparkline {
				require.Equal(t, uint64(model[p.Day].Active), p.Active, "sparkline %s", p.Day)
			}
			require.Equal(t, "2026-09-13", row.Sparkline[0].Day)
			require.Equal(t, "2026-10-12", row.Sparkline[29].Day)

			inst := got[identity.Installs].Rows[i]
			require.GreaterOrEqual(t, inst.ActiveDate, row.ActiveDate)
			require.GreaterOrEqual(t, inst.NewDate, row.NewDate)
			require.Equal(t, row.SessionsDate, inst.SessionsDate, "сессии не зависят от unit")
		})
	}
}

// AC-028.2 (AC-5.1.2): приложение появилось за 3 дня до даты — нет базы прошлой недели и когорты D7.
func TestPortfolio_NoBaseline(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-10T12:00:00+03:00"),
		session(t, 1, "", "2026-10-12T12:00:00+03:00"),
		session(t, 2, "", "2026-10-11T12:00:00+03:00"),
		session(t, 2, "", "2026-10-12T18:00:00+03:00"),
		session(t, 3, "", "2026-10-11T13:00:00+03:00"),
	})
	svc := NewService(env.CH.Conn(), clock.NewFixed(portfolioNow))
	empty := uuid.MustParse("1de0a000-0000-4000-8000-0000000000ff")

	got := portfolio(t, svc, PortfolioQuery{
		Date: date(t, "2026-10-12"), Env: EnvProduction, Unit: identity.Persons,
		Apps: []PortfolioApp{
			{ID: identityApp, Location: location(t, "Europe/Moscow")},
			{ID: empty, Location: location(t, "Asia/Tokyo")},
		},
	})

	row := got.Rows[0]
	require.Equal(t, uint64(2), row.ActiveDate)
	require.Equal(t, uint64(0), row.ActiveSameDayPrevWeek)
	require.Nil(t, row.ActiveDeltaPct)
	require.Nil(t, row.RetentionD7)
	require.Equal(t, f64(0.5), row.RetentionD1, "когорта 10-11 — установки 2 и 3, вернулась 2")
	require.Equal(t, uint64(0), row.NewDate, "установка 1 новая 10-10")
	require.Equal(t, uint64(2), row.SessionsDate)
	for _, p := range row.Sparkline[:27] {
		require.Zero(t, p.Active, p.Day)
	}
	require.Equal(t, []SparklinePoint{
		{Day: "2026-10-10", Active: 1}, {Day: "2026-10-11", Active: 2}, {Day: "2026-10-12", Active: 2},
	}, row.Sparkline[27:])

	// Приложение без единого события — строка с нулями.
	zero := got.Rows[1]
	require.Equal(t, empty, zero.AppID)
	require.Zero(t, zero.ActiveToday+zero.ActiveDate+zero.ActiveSameDayPrevWeek+zero.NewDate+zero.SessionsDate)
	require.Nil(t, zero.ActiveDeltaPct)
	require.Nil(t, zero.RetentionD1)
	require.Nil(t, zero.RetentionD7)
	for _, p := range zero.Sparkline {
		require.Zero(t, p.Active, p.Day)
	}
}

// Дельта к тому же дню прошлой недели, D1 и D7 считаются по когортам с активностью в день когорты.
func TestPortfolio_DeltaAndRetention(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		// 10-05: установки 1, 2 новые (когорта D7), 3 — старая.
		session(t, 3, "", "2026-09-01T12:00:00+03:00"),
		session(t, 1, "", "2026-10-05T10:00:00+03:00"),
		session(t, 2, "", "2026-10-05T11:00:00+03:00"),
		session(t, 3, "", "2026-10-05T12:00:00+03:00"),
		// 10-11: установки 4, 5, 6 новые (когорта D1).
		session(t, 4, "", "2026-10-11T10:00:00+03:00"),
		session(t, 5, "", "2026-10-11T11:00:00+03:00"),
		session(t, 6, "", "2026-10-11T12:00:00+03:00"),
		// 10-12: вернулись 1 из D7 и 4, 5 из D1; 3 — старая, 7 — новая.
		session(t, 1, "", "2026-10-12T10:00:00+03:00"),
		session(t, 3, "", "2026-10-12T11:00:00+03:00"),
		session(t, 4, "", "2026-10-12T12:00:00+03:00"),
		session(t, 5, "", "2026-10-12T13:00:00+03:00"),
		session(t, 7, "", "2026-10-12T14:00:00+03:00"),
		// 10-13 — сегодня.
		session(t, 7, "", "2026-10-13T09:00:00+03:00"),
	})
	svc := NewService(env.CH.Conn(), clock.NewFixed(portfolioNow))

	row := identityPortfolio(t, svc, EnvProduction, identity.Persons, "2026-10-12")
	require.Equal(t, uint64(1), row.ActiveToday)
	require.Equal(t, uint64(5), row.ActiveDate)
	require.Equal(t, uint64(3), row.ActiveSameDayPrevWeek)
	require.Equal(t, f64(66.7), row.ActiveDeltaPct)
	require.Equal(t, uint64(1), row.NewDate)
	require.Equal(t, uint64(5), row.SessionsDate)
	require.Equal(t, ratio(2, 3), row.RetentionD1)
	require.Equal(t, f64(0.5), row.RetentionD7)
}

// Сутки — в TZ приложения: 23:30 UTC 11.10 у приложения Europe/Moscow — это 02:30 12.10.
func TestPortfolio_AppTimezoneBoundary(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-11T23:30:00Z"),
		session(t, 2, "", "2026-10-11T20:59:59Z"),
	})
	svc := NewService(env.CH.Conn(), clock.NewFixed(portfolioNow))

	row := identityPortfolio(t, svc, EnvProduction, identity.Persons, "2026-10-12")
	require.Equal(t, uint64(1), row.ActiveDate)
	require.Equal(t, uint64(1), row.NewDate)
	require.Equal(t, uint64(1), row.SessionsDate)
	require.Equal(t, f64(0), row.RetentionD1, "установка 2 — когорта 11.10 по Москве, 12.10 не вернулась")
	require.Equal(t, SparklinePoint{Day: "2026-10-11", Active: 1}, row.Sparkline[28])
	require.Equal(t, SparklinePoint{Day: "2026-10-12", Active: 1}, row.Sparkline[29])
}

// unit=installs: человек с двумя устройствами — на одну единицу больше; environment разделяет трафик.
func TestPortfolio_UnitAndEnvironment(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "u-1", "2026-10-12T09:00:00+03:00"),
		session(t, 2, "u-1", "2026-10-12T18:00:00+03:00"),
		dev(session(t, 3, "", "2026-10-12T12:00:00+03:00")),
		dev(session(t, 3, "", "2026-10-13T12:00:00+03:00")),
	})
	svc := NewService(env.CH.Conn(), clock.NewFixed(portfolioNow))

	persons := identityPortfolio(t, svc, EnvProduction, identity.Persons, "2026-10-12")
	installs := identityPortfolio(t, svc, EnvProduction, identity.Installs, "2026-10-12")
	require.Equal(t, uint64(1), persons.ActiveDate)
	require.Equal(t, uint64(2), installs.ActiveDate)
	require.Equal(t, uint64(1), persons.NewDate)
	require.Equal(t, uint64(2), installs.NewDate)
	require.Equal(t, uint64(2), persons.SessionsDate)
	require.Equal(t, uint64(2), installs.SessionsDate)
	require.Zero(t, persons.ActiveToday)

	development := identityPortfolio(t, svc, EnvDevelopment, identity.Persons, "2026-10-12")
	require.Equal(t, uint64(1), development.ActiveDate)
	require.Equal(t, uint64(1), development.ActiveToday)
	require.Equal(t, uint64(1), development.SessionsDate)
}

type dailyStat struct {
	app      uuid.UUID
	day      string
	env      string
	unit     identity.Unit
	active   uint64
	closedAt time.Time
}

func insertDailyStats(t *testing.T, env *testutil.Env, stats ...dailyStat) {
	t.Helper()
	batch, err := env.CH.Conn().PrepareBatch(t.Context(),
		"INSERT INTO daily_stats (app_id, day, environment, unit, active, new, sessions, sessions_with_duration, duration_ms_sum, closed_at)")
	require.NoError(t, err)
	for _, s := range stats {
		require.NoError(t, batch.Append(s.app, date(t, s.day), s.env, string(s.unit), s.active,
			uint64(0), uint64(0), uint64(0), uint64(0), s.closedAt))
	}
	require.NoError(t, batch.Send())
}

// AC-028.3 (AC-5.1.3): закрытые дни (≤ сегодня − 8) читаются из daily_stats, открытые — из events;
// честно закрытые дни совпадают с live-расчётом, подменённый — виден, пропуск закрытия закрывается live.
func TestPortfolio_UsesDailyStatsForClosedDays(t *testing.T) {
	env := testutil.Start(t)
	env.LoadNDJSON(t, testutil.ScenarioPath())
	// Сегодня 2026-10-23 во всех поясах; закрыты дни ≤ 10-15, sparkline 09-23 … 10-22.
	svc := NewService(env.CH.Conn(), clock.NewFixed(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)))
	msk := testutil.ScenarioApps[0]
	q := PortfolioQuery{
		Date: date(t, "2026-10-22"), Env: EnvProduction, Unit: identity.Persons,
		Apps: []PortfolioApp{{ID: msk.ID, Location: location(t, msk.Timezone)}},
	}
	live := portfolio(t, svc, q).Rows[0]

	old := time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC)
	closed := old.Add(time.Hour)
	var stats []dailyStat
	for _, p := range live.Sparkline {
		if p.Day > "2026-10-15" || p.Day == "2026-10-10" { // 10-10 не закрыт: пропуск закрытия
			continue
		}
		active := p.Active
		if p.Day == "2026-10-05" {
			active = 999
		}
		stats = append(stats, dailyStat{msk.ID, p.Day, EnvProduction, identity.Persons, active, closed})
	}
	stats = append(stats,
		dailyStat{msk.ID, "2026-10-06", EnvProduction, identity.Persons, 555, old},                         // старая версия строки
		dailyStat{msk.ID, "2026-10-18", EnvProduction, identity.Persons, 999, closed},                      // открытый день
		dailyStat{msk.ID, "2026-10-07", EnvProduction, identity.Installs, 777, closed},                     // другой unit
		dailyStat{msk.ID, "2026-10-08", EnvDevelopment, identity.Persons, 777, closed},                     // другой environment
		dailyStat{testutil.ScenarioApps[1].ID, "2026-10-09", EnvProduction, identity.Persons, 777, closed}, // другое приложение
	)
	insertDailyStats(t, env, stats...)

	got := portfolio(t, svc, q).Rows[0]
	for i, p := range got.Sparkline {
		want := live.Sparkline[i]
		if p.Day == "2026-10-05" {
			want.Active = 999
		}
		require.Equal(t, want, p, "sparkline %s", p.Day)
	}
	require.Positive(t, live.Sparkline[12].Active, "10-05 в сценарии не пустой")

	// Метрики даты считаются по events независимо от daily_stats.
	got.Sparkline, live.Sparkline = nil, nil
	require.Equal(t, live, got)
}
