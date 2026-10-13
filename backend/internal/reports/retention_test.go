//go:build integration

package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// retentionNow — «сейчас» golden-тестов retention: 2026-11-03, следующий день после окна сценария во всех трёх
// поясах; часть периодов поздних когорт ещё не завершилась.
var retentionNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func retention(t *testing.T, svc *Service, p Params, q RetentionQuery) Retention {
	t.Helper()
	got, err := svc.Retention(t.Context(), p, q)
	require.NoError(t, err)
	q = q.withDefaults()
	require.Equal(t, q.Granularity, got.CohortGranularity)
	require.Equal(t, q.Periods, got.Periods)
	require.Len(t, got.Average, q.Periods+1)
	for _, c := range got.Cohorts {
		require.Len(t, c.Values, q.Periods+1, c.CohortStart)
	}
	return got
}

// retentionSnapshot — матрицы приложения для unit=persons и unit=installs (сверка с AppMetrica).
type retentionSnapshot struct {
	Persons  Retention `json:"persons"`
	Installs Retention `json:"installs"`
}

func retentionReport(t *testing.T, svc *Service, p Params, q RetentionQuery) retentionSnapshot {
	t.Helper()
	installs := p
	installs.Unit = identity.Installs
	return retentionSnapshot{Persons: retention(t, svc, p, q), Installs: retention(t, svc, installs, q)}
}

func scenarioModel(t *testing.T, rows []chstore.EventRow, app testutil.ScenarioApp) map[string]testutil.ScenarioDay {
	t.Helper()
	stats, err := testutil.ScenarioStats(rows, app)
	require.NoError(t, err)
	out := map[string]testutil.ScenarioDay{}
	for _, d := range stats {
		out[d.Day] = d
	}
	return out
}

// AC-030.1 (AC-5.3.1): дневные когорты сценария 2026-09-28…2026-10-12, periods=30, совпадают с golden; размер
// когорты и D1 — с эталонной моделью фикстур, D0 = 1, периоды с 2026-11-03 — null.
func TestRetention_DayGolden(t *testing.T) {
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, testutil.ScenarioPath())
	svc := NewService(env.CH.Conn(), clock.NewFixed(retentionNow))
	q := RetentionQuery{Granularity: GranularityDay, Periods: 30}

	got := map[string]retentionSnapshot{}
	for _, app := range testutil.ScenarioApps {
		got[app.Code] = retentionReport(t, svc, scenarioParams(t, app, "2026-09-28", "2026-10-12"), q)
	}
	testutil.AssertGolden(t, "retention.golden.json", got)

	today := date(t, "2026-11-03")
	for _, app := range testutil.ScenarioApps {
		t.Run(app.Code, func(t *testing.T) {
			model := scenarioModel(t, rows, app)
			m := got[app.Code].Persons
			require.Len(t, m.Cohorts, 15)
			var sizes, d1 uint64
			for i, c := range m.Cohorts {
				day := date(t, "2026-09-28").AddDate(0, 0, i)
				require.Equal(t, day.Format(time.DateOnly), c.CohortStart)
				st := model[c.CohortStart]
				require.Equal(t, uint64(st.New), c.Size, "размер когорты %s", c.CohortStart)
				sizes += c.Size
				for n, v := range c.Values {
					if c.Size == 0 || !day.AddDate(0, 0, n).Before(today) {
						require.Nil(t, v, "%s D%d", c.CohortStart, n)
						continue
					}
					require.NotNil(t, v, "%s D%d", c.CohortStart, n)
				}
				if c.Size == 0 {
					continue
				}
				require.Equal(t, f64(1), c.Values[0])
				require.Equal(t, f64(float64(st.D1Returned)/float64(st.New)), c.Values[1], "D1 %s", c.CohortStart)
				d1 += uint64(st.D1Returned)
			}
			require.Positive(t, sizes)
			require.Equal(t, f64(1), m.Average[0])
			require.Equal(t, f64(float64(d1)/float64(sizes)), m.Average[1], "average D1 взвешен по размеру когорт")

			inst := got[app.Code].Installs
			var instSizes uint64
			for _, c := range inst.Cohorts {
				instSizes += c.Size
			}
			require.GreaterOrEqual(t, instSizes, sizes, "установок в когортах не меньше, чем людей")
		})
	}
}

// AC-030.2 (AC-5.3.2): недельные когорты (ISO, с понедельника) 2026-09-28…2026-11-02, periods=12, совпадают с
// golden; размер недельной когорты — сумма дневных новых, W0 = 1, незавершённые недели — null.
func TestRetention_WeekGolden(t *testing.T) {
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, testutil.ScenarioPath())
	svc := NewService(env.CH.Conn(), clock.NewFixed(retentionNow))
	q := RetentionQuery{Granularity: GranularityWeek, Periods: 12}

	got := map[string]retentionSnapshot{}
	for _, app := range testutil.ScenarioApps {
		got[app.Code] = retentionReport(t, svc, scenarioParams(t, app, "2026-10-01", "2026-11-02"), q)
	}
	testutil.AssertGolden(t, "retention_week.golden.json", got)

	today := date(t, "2026-11-03")
	for _, app := range testutil.ScenarioApps {
		t.Run(app.Code, func(t *testing.T) {
			model := scenarioModel(t, rows, app)
			m := got[app.Code].Persons
			require.Len(t, m.Cohorts, 6, "недели с понедельников 09-28…11-02")
			for i, c := range m.Cohorts {
				monday := date(t, "2026-09-28").AddDate(0, 0, 7*i)
				require.Equal(t, monday.Format(time.DateOnly), c.CohortStart)
				require.Equal(t, time.Monday, monday.Weekday())
				var size int
				for d := range 7 {
					size += model[monday.AddDate(0, 0, d).Format(time.DateOnly)].New
				}
				require.Equal(t, uint64(size), c.Size, "неделя %s — вся ISO-неделя, а не только дни from…to", c.CohortStart)
				for n, v := range c.Values {
					done := !monday.AddDate(0, 0, 7*(n+1)).After(today)
					if c.Size == 0 || !done {
						require.Nil(t, v, "%s W%d", c.CohortStart, n)
						continue
					}
					require.NotNil(t, v, "%s W%d", c.CohortStart, n)
				}
				if c.Size > 0 {
					require.Equal(t, f64(1), c.Values[0])
				}
			}
			require.NotNil(t, m.Cohorts[0].Values[4], "W4 когорты 09-28 — неделя 10-26…11-01 завершилась")
			require.Nil(t, m.Cohorts[0].Values[5])
		})
	}
}

// retentionRows — установки 1 и 2 впервые 10-10 (1 возвращается 10-11 и 10-12, 2 — только 10-12), 10-11 новых
// нет, установка 3 впервые 10-12; все трое активны сегодня (10-13).
func retentionRows(t *testing.T) []chstore.EventRow {
	t.Helper()
	return concat(
		visit(t, 1, "", "2026-10-10T10:00:00+03:00", time.Minute),
		visit(t, 2, "", "2026-10-10T11:00:00+03:00", 0),
		visit(t, 1, "", "2026-10-11T10:00:00+03:00", time.Minute),
		visit(t, 1, "", "2026-10-12T10:00:00+03:00", time.Minute),
		visit(t, 2, "", "2026-10-12T12:00:00+03:00", time.Minute),
		visit(t, 3, "", "2026-10-12T23:30:00+03:00", time.Minute),
		visit(t, 1, "", "2026-10-13T09:00:00+03:00", time.Minute),
		visit(t, 2, "", "2026-10-13T09:00:00+03:00", time.Minute),
		visit(t, 3, "", "2026-10-13T09:00:00+03:00", time.Minute),
	)
}

// AC-030.3 (AC-5.3.3): сегодня 2026-10-13 — у вчерашней когорты D0 заполнен, D1…D30 null; когорта без новых —
// size 0 и все null; average игнорирует null. Возврат только на D2 засчитан в D2, не в D1 (classic_not_rolling).
func TestRetention_NullsForFutureAndEmpty(t *testing.T) {
	svc := newService(startWith(t, retentionRows(t)))
	got := retention(t, svc, period(t, identity.Persons, "2026-10-10", "2026-10-12"), RetentionQuery{})

	nulls := func(n int) []*float64 { return make([]*float64, n) }
	require.Len(t, got.Cohorts, 3)
	require.Equal(t, RetentionCohort{
		CohortStart: "2026-10-10", Size: 2,
		Values: append([]*float64{f64(1), f64(0.5), f64(1)}, nulls(28)...),
	}, got.Cohorts[0])
	require.Equal(t, RetentionCohort{CohortStart: "2026-10-11", Size: 0, Values: nulls(31)}, got.Cohorts[1])
	require.Equal(t, RetentionCohort{
		CohortStart: "2026-10-12", Size: 1, Values: append([]*float64{f64(1)}, nulls(30)...),
	}, got.Cohorts[2], "вчерашняя когорта: D1 = сегодня, ещё не завершился")
	require.Equal(t, append([]*float64{f64(1), f64(0.5), f64(1)}, nulls(28)...), got.Average)

	t.Run("today_and_future_cohorts", func(t *testing.T) {
		got := retention(t, svc, period(t, identity.Persons, "2026-10-13", "2026-10-15"), RetentionQuery{Periods: 2})
		require.Equal(t, []RetentionCohort{
			{CohortStart: "2026-10-13", Size: 0, Values: nulls(3)},
			{CohortStart: "2026-10-14", Size: 0, Values: nulls(3)},
			{CohortStart: "2026-10-15", Size: 0, Values: nulls(3)},
		}, got.Cohorts)
		require.Equal(t, nulls(3), got.Average)
	})

	t.Run("current_week", func(t *testing.T) {
		got := retention(t, svc, period(t, identity.Persons, "2026-10-10", "2026-10-13"),
			RetentionQuery{Granularity: GranularityWeek, Periods: 2})
		require.Equal(t, []RetentionCohort{
			{CohortStart: "2026-10-05", Size: 2, Values: []*float64{f64(1), nil, nil}},
			{CohortStart: "2026-10-12", Size: 1, Values: nulls(3)},
		}, got.Cohorts, "W1 когорты 10-05 — неделя 10-12…10-18 ещё идёт")
	})
}

// Тест-вектор retro_merge: анонимная установка в когорте 10-10, через 2 дня вход — человек остаётся в когорте
// своего первого дня, D2 засчитан. Фильтр применяется и к событию когорты, и к возвратам; environment разделяет
// когорты; при unit=installs вторая установка того же пользователя — отдельная когорта.
func TestRetention_RetroMergeFiltersAndUnit(t *testing.T) {
	rows := concat(
		visit(t, 1, "", "2026-10-10T10:00:00+03:00", time.Minute),
		visit(t, 1, "u-1", "2026-10-12T10:00:00+03:00", time.Minute),
		visit(t, 2, "u-1", "2026-10-11T10:00:00+03:00", time.Minute), // второе устройство u-1
		with(visit(t, 3, "", "2026-10-10T12:00:00+03:00", time.Minute), func(r *chstore.EventRow) { r.Platform = "android" }),
		with(visit(t, 3, "", "2026-10-11T12:00:00+03:00", time.Minute), func(r *chstore.EventRow) { r.Platform = "android" }),
		with(visit(t, 4, "", "2026-10-10T13:00:00+03:00", time.Minute), func(r *chstore.EventRow) { r.Environment = EnvDevelopment }),
		with(visit(t, 4, "", "2026-10-11T13:00:00+03:00", time.Minute), func(r *chstore.EventRow) { r.Environment = EnvDevelopment }),
	)
	svc := newService(startWith(t, rows))
	q := RetentionQuery{Periods: 2}

	p := period(t, identity.Persons, "2026-10-10", "2026-10-11")
	got := retention(t, svc, p, q)
	require.Equal(t, []RetentionCohort{
		{CohortStart: "2026-10-10", Size: 2, Values: []*float64{f64(1), f64(1), f64(0.5)}},
		{CohortStart: "2026-10-11", Size: 0, Values: []*float64{nil, nil, nil}},
	}, got.Cohorts, "u:u-1 (установки 1 и 2) — когорта 10-10: D1 вторым устройством, D2 входом; i:3 — D1")

	p.Filters = Filters{Platform: []string{"ios"}}
	got = retention(t, svc, p, q)
	require.Equal(t, RetentionCohort{CohortStart: "2026-10-10", Size: 1, Values: []*float64{f64(1), f64(1), f64(1)}},
		got.Cohorts[0], "android-установка 3 отфильтрована")

	p.Filters = Filters{Platform: []string{"android"}}
	got = retention(t, svc, p, q)
	require.Equal(t, RetentionCohort{CohortStart: "2026-10-10", Size: 1, Values: []*float64{f64(1), f64(1), f64(0)}},
		got.Cohorts[0])

	p.Filters = Filters{}
	p.Env = EnvDevelopment
	got = retention(t, svc, p, q)
	require.Equal(t, RetentionCohort{CohortStart: "2026-10-10", Size: 1, Values: []*float64{f64(1), f64(1), f64(0)}},
		got.Cohorts[0], "development: только установка 4")

	installs := period(t, identity.Installs, "2026-10-10", "2026-10-11")
	got = retention(t, svc, installs, q)
	require.Equal(t, []RetentionCohort{
		{CohortStart: "2026-10-10", Size: 2, Values: []*float64{f64(1), f64(0.5), f64(0.5)}},
		{CohortStart: "2026-10-11", Size: 1, Values: []*float64{f64(1), f64(0), nil}},
	}, got.Cohorts, "установки 1 и 3 — когорта 10-10, установка 2 — 10-11")
}
