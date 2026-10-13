package reports

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dates(t testing.TB, days ...string) []time.Time {
	t.Helper()
	out := make([]time.Time, 0, len(days))
	for _, d := range days {
		out = append(out, date(t, d))
	}
	return out
}

func returned(t testing.TB, cohort string, period int) cohortPeriod {
	t.Helper()
	return cohortPeriod{cohort: date(t, cohort).Unix(), period: period}
}

// Когорты по дням — каждый день from..to, по неделям — понедельники ISO-недель, содержащих from и to.
func TestCohortStarts(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		g              Granularity
		want           []string
	}{
		{"day", "2026-10-10", "2026-10-12", GranularityDay, []string{"2026-10-10", "2026-10-11", "2026-10-12"}},
		{"one_day", "2026-10-12", "2026-10-12", GranularityDay, []string{"2026-10-12"}},
		{"leap_day", "2024-02-28", "2024-03-01", GranularityDay, []string{"2024-02-28", "2024-02-29", "2024-03-01"}},
		{"week_aligned", "2026-09-28", "2026-10-12", GranularityWeek, []string{"2026-09-28", "2026-10-05", "2026-10-12"}},
		{"week_mid", "2026-10-01", "2026-10-13", GranularityWeek, []string{"2026-09-28", "2026-10-05", "2026-10-12"}},
		{"week_sunday", "2026-10-11", "2026-10-11", GranularityWeek, []string{"2026-10-05"}},
		{"iso_year", "2026-12-30", "2027-01-05", GranularityWeek, []string{"2026-12-28", "2027-01-04"}},
		{"unknown", "2026-10-10", "2026-10-12", GranularityMonth, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, d := range cohortStarts(Period{From: date(t, tc.from), To: date(t, tc.to)}, tc.g) {
				got = append(got, d.Format(time.DateOnly))
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// Классический day-N: values[N] = returned_N / size, D0 = 1, null — период не завершился к сегодняшнему дню
// или когорта пуста; average — взвешенное по размеру среди когорт с завершённым периодом.
func TestRetentionMatrix_Day(t *testing.T) {
	counts := map[cohortPeriod]uint64{
		returned(t, "2026-10-10", 0): 2,
		returned(t, "2026-10-10", 2): 2,
		returned(t, "2026-10-10", 1): 1,
		returned(t, "2026-10-12", 0): 1,
	}
	got := retentionMatrix(GranularityDay, 4, dates(t, "2026-10-10", "2026-10-11", "2026-10-12"), counts,
		date(t, "2026-10-13"))

	require.Equal(t, Retention{
		CohortGranularity: GranularityDay,
		Periods:           4,
		Cohorts: []RetentionCohort{
			{CohortStart: "2026-10-10", Size: 2, Values: []*float64{f64(1), f64(0.5), f64(1), nil, nil}},
			{CohortStart: "2026-10-11", Size: 0, Values: []*float64{nil, nil, nil, nil, nil}},
			{CohortStart: "2026-10-12", Size: 1, Values: []*float64{f64(1), nil, nil, nil, nil}},
		},
		Average: []*float64{f64(1), f64(0.5), f64(1), nil, nil},
	}, got)
}

// Тест-вектор average_weighted (06-api-contracts.md §2.4.5): когорты 10 и 30, D1 0.5 и 0.1 → average[1] = 0.2.
func TestRetention_WeightedAverage(t *testing.T) {
	counts := map[cohortPeriod]uint64{
		returned(t, "2026-10-01", 0): 10,
		returned(t, "2026-10-01", 1): 5,
		returned(t, "2026-10-02", 0): 30,
		returned(t, "2026-10-02", 1): 3,
	}
	got := retentionMatrix(GranularityDay, 1, dates(t, "2026-10-01", "2026-10-02"), counts, date(t, "2026-10-13"))
	require.Equal(t, []*float64{f64(1), f64(0.2)}, got.Average)
	require.Equal(t, []*float64{f64(1), f64(0.5)}, got.Cohorts[0].Values)
	require.Equal(t, []*float64{f64(1), f64(0.1)}, got.Cohorts[1].Values)
}

// Недельный период N завершён, когда закончилась вся ISO-неделя cohort_start + N.
func TestRetentionMatrix_Week(t *testing.T) {
	counts := map[cohortPeriod]uint64{
		returned(t, "2026-09-28", 0): 4,
		returned(t, "2026-09-28", 1): 2,
		returned(t, "2026-09-28", 2): 1, // неделя 10-12…10-18 идёт: null, хотя события уже есть
		returned(t, "2026-10-05", 0): 2,
		returned(t, "2026-10-05", 1): 1,
		returned(t, "2026-10-12", 0): 3,
	}
	got := retentionMatrix(GranularityWeek, 3, dates(t, "2026-09-28", "2026-10-05", "2026-10-12"), counts,
		date(t, "2026-10-13"))

	require.Equal(t, []RetentionCohort{
		{CohortStart: "2026-09-28", Size: 4, Values: []*float64{f64(1), f64(0.5), nil, nil}},
		{CohortStart: "2026-10-05", Size: 2, Values: []*float64{f64(1), nil, nil, nil}},
		{CohortStart: "2026-10-12", Size: 3, Values: []*float64{nil, nil, nil, nil}},
	}, got.Cohorts)
	require.Equal(t, []*float64{f64(1), f64(0.5), nil, nil}, got.Average)
	require.Equal(t, GranularityWeek, got.CohortGranularity)
	require.Equal(t, 3, got.Periods)

	// Воскресенье — последний день недели: неделя 10-05 завершается только с наступлением понедельника 10-12.
	sunday := retentionMatrix(GranularityWeek, 1, dates(t, "2026-10-05"), counts, date(t, "2026-10-11"))
	require.Equal(t, []*float64{nil, nil}, sunday.Cohorts[0].Values)
	monday := retentionMatrix(GranularityWeek, 1, dates(t, "2026-10-05"), counts, date(t, "2026-10-12"))
	require.Equal(t, []*float64{f64(1), nil}, monday.Cohorts[0].Values)
}

// Нет когорт — пустой список и average из null; когорты без данных — size 0 и null.
func TestRetentionMatrix_Empty(t *testing.T) {
	got := retentionMatrix(GranularityDay, 2, nil, nil, date(t, "2026-10-13"))
	require.Equal(t, []RetentionCohort{}, got.Cohorts)
	require.Equal(t, []*float64{nil, nil, nil}, got.Average)

	got = retentionMatrix(GranularityDay, 1, dates(t, "2026-10-01", "2026-10-20"), nil, date(t, "2026-10-13"))
	require.Equal(t, []RetentionCohort{
		{CohortStart: "2026-10-01", Size: 0, Values: []*float64{nil, nil}},
		{CohortStart: "2026-10-20", Size: 0, Values: []*float64{nil, nil}},
	}, got.Cohorts)
	require.Equal(t, []*float64{nil, nil}, got.Average)
}

// Granularity и periods вне whitelist — ErrUnsupported до обращения к ClickHouse.
func TestRetention_UnsupportedQuery(t *testing.T) {
	svc := NewService(nil, nil)
	p := Params{Period: Period{From: date(t, "2026-10-01"), To: date(t, "2026-10-07")}}
	for _, q := range []RetentionQuery{
		{Granularity: GranularityMonth},
		{Granularity: GranularityHour, Periods: 5},
		{Granularity: GranularityDay, Periods: MaxDayPeriods + 1},
		{Granularity: GranularityWeek, Periods: MaxWeekPeriods + 1},
	} {
		_, err := svc.Retention(context.Background(), p, q)
		require.ErrorIs(t, err, ErrUnsupported, "%+v", q)
	}
}

// Умолчания periods: 30 для дней, 12 для недель.
func TestRetentionQuery_Defaults(t *testing.T) {
	require.Equal(t, RetentionQuery{Granularity: GranularityDay, Periods: DefaultDayPeriods},
		RetentionQuery{}.withDefaults())
	require.Equal(t, RetentionQuery{Granularity: GranularityWeek, Periods: DefaultWeekPeriods},
		RetentionQuery{Granularity: GranularityWeek}.withDefaults())
	require.Equal(t, RetentionQuery{Granularity: GranularityWeek, Periods: 4},
		RetentionQuery{Granularity: GranularityWeek, Periods: 4}.withDefaults())
}
