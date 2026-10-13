package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func loadLocation(t testing.TB, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

// Предыдущий период — той же длины, сразу перед from (02-tech-spec.md FR-013).
func TestPeriod_Previous(t *testing.T) {
	for _, tc := range []struct{ from, to, wantFrom, wantTo string }{
		{"2026-10-06", "2026-10-12", "2026-09-29", "2026-10-05"},
		{"2026-10-12", "2026-10-12", "2026-10-11", "2026-10-11"},
		{"2026-03-01", "2026-03-31", "2026-01-29", "2026-02-28"},
		{"2024-03-01", "2024-03-01", "2024-02-29", "2024-02-29"},
	} {
		got := Period{From: date(t, tc.from), To: date(t, tc.to)}.Previous()
		require.Equal(t, tc.wantFrom, got.From.Format(time.DateOnly), tc.from)
		require.Equal(t, tc.wantTo, got.To.Format(time.DateOnly), tc.from)
	}
}

func formatAll(ts []time.Time) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Format(time.RFC3339))
	}
	return out
}

// Корзины ряда — в TZ приложения: часы переходного дня (23/25), ISO-недели с понедельника, календарные месяцы.
func TestBucketStarts(t *testing.T) {
	nyc := loadLocation(t, "America/New_York")
	msk := loadLocation(t, "Europe/Moscow")

	t.Run("hour_fall_back", func(t *testing.T) {
		got := formatAll(bucketStarts(Period{From: date(t, "2026-11-01"), To: date(t, "2026-11-01")}, nyc, GranularityHour))
		require.Len(t, got, 25)
		require.Equal(t, []string{
			"2026-11-01T00:00:00-04:00", "2026-11-01T01:00:00-04:00", "2026-11-01T01:00:00-05:00", "2026-11-01T02:00:00-05:00",
		}, got[:4])
		require.Equal(t, "2026-11-01T23:00:00-05:00", got[24])
	})

	t.Run("hour_spring_forward", func(t *testing.T) {
		got := formatAll(bucketStarts(Period{From: date(t, "2026-03-08"), To: date(t, "2026-03-08")}, nyc, GranularityHour))
		require.Len(t, got, 23)
		require.Equal(t, []string{"2026-03-08T00:00:00-05:00", "2026-03-08T01:00:00-05:00", "2026-03-08T03:00:00-04:00"}, got[:3])
	})

	t.Run("hour_week", func(t *testing.T) {
		require.Len(t, bucketStarts(Period{From: date(t, "2026-10-06"), To: date(t, "2026-10-12")}, msk, GranularityHour), 7*24)
	})

	t.Run("day", func(t *testing.T) {
		got := formatAll(bucketStarts(Period{From: date(t, "2026-10-31"), To: date(t, "2026-11-02")}, nyc, GranularityDay))
		require.Equal(t, []string{"2026-10-31T00:00:00-04:00", "2026-11-01T00:00:00-04:00", "2026-11-02T00:00:00-05:00"}, got)
	})

	t.Run("week_iso_monday", func(t *testing.T) {
		// 2026-10-01 — четверг: первая корзина начинается в понедельник 09-28, последняя — 10-12.
		got := formatAll(bucketStarts(Period{From: date(t, "2026-10-01"), To: date(t, "2026-10-12")}, msk, GranularityWeek))
		require.Equal(t, []string{"2026-09-28T00:00:00+03:00", "2026-10-05T00:00:00+03:00", "2026-10-12T00:00:00+03:00"}, got)
		// Воскресенье — последний день ISO-недели.
		got = formatAll(bucketStarts(Period{From: date(t, "2026-10-11"), To: date(t, "2026-10-11")}, msk, GranularityWeek))
		require.Equal(t, []string{"2026-10-05T00:00:00+03:00"}, got)
	})

	t.Run("month", func(t *testing.T) {
		got := formatAll(bucketStarts(Period{From: date(t, "2026-09-15"), To: date(t, "2026-11-01")}, nyc, GranularityMonth))
		require.Equal(t, []string{"2026-09-01T00:00:00-04:00", "2026-10-01T00:00:00-04:00", "2026-11-01T00:00:00-04:00"}, got)
	})

	t.Run("nil_location_is_utc", func(t *testing.T) {
		got := formatAll(bucketStarts(Period{From: date(t, "2026-10-12"), To: date(t, "2026-10-12")}, nil, GranularityDay))
		require.Equal(t, []string{"2026-10-12T00:00:00Z"}, got)
	})

	t.Run("unknown_granularity", func(t *testing.T) {
		require.Empty(t, bucketStarts(Period{From: date(t, "2026-10-12"), To: date(t, "2026-10-12")}, msk, "quarter"))
	})
}

// delta_pct сравнения периодов — в процентах с двумя знаками (06-api-contracts.md §2.4.4), null при нулевой
// или отсутствующей базе.
func TestChangePct(t *testing.T) {
	for _, tc := range []struct {
		cur, base *float64
		want      *float64
	}{
		{f64(41230), f64(40102), f64(2.81)},
		{f64(8120), f64(7998), f64(1.53)},
		{f64(3), f64(2), f64(50)},
		{f64(0), f64(7), f64(-100)},
		{f64(2), f64(3), f64(-33.33)},
		{f64(5), f64(0), nil},
		{nil, f64(5), nil},
		{f64(5), nil, nil},
	} {
		got := changePct(tc.cur, tc.base)
		if tc.want == nil {
			require.Nil(t, got)
			continue
		}
		require.NotNil(t, got)
		require.InDelta(t, *tc.want, *got, 1e-9)
	}
}

// share = count / total с четырьмя знаками; пустой total — 0.
func TestShareOf(t *testing.T) {
	require.InDelta(t, 0.7303, shareOf(30110, 41230), 1e-9)
	require.InDelta(t, 0.2891, shareOf(11920, 41230), 1e-9)
	require.InDelta(t, 1.0, shareOf(5, 5), 1e-9)
	require.InDelta(t, 0.0, shareOf(0, 0), 1e-9)
	require.InDelta(t, 0.3333, shareOf(1, 3), 1e-9)
}
