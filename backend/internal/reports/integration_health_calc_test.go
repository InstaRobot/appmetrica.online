package reports

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
)

func TestIntegrationHours(t *testing.T) {
	load := func(t *testing.T, name string) *time.Location {
		t.Helper()
		loc, err := time.LoadLocation(name)
		require.NoError(t, err)
		return loc
	}
	now := time.Date(2026, 9, 10, 12, 20, 30, 500, time.UTC)

	t.Run("half_hour_offset", func(t *testing.T) {
		kolkata := load(t, "Asia/Kolkata")
		// 17:50:30 IST — текущий час начался в 17:00 IST = 11:30 UTC.
		last := time.Date(2026, 9, 10, 11, 30, 0, 0, time.UTC)
		first := last.Add(-23 * time.Hour)
		got := integrationHours(now, kolkata, 24,
			map[int64]uint64{
				first.Unix():                       3,
				last.Unix():                        4,
				first.Add(-time.Hour).Unix():       9, // до окна
				first.Add(30 * time.Minute).Unix(): 8, // не начало часа в TZ приложения
			},
			map[int64]uint64{first.Add(time.Hour).Unix(): 2, last.Unix(): 1},
		)
		require.Len(t, got, 24)
		for i, h := range got {
			require.True(t, first.Add(time.Duration(i)*time.Hour).Equal(h.Hour), "час %d", i)
			require.Equal(t, kolkata, h.Hour.Location())
			switch i {
			case 0:
				require.Equal(t, IntegrationHour{Hour: h.Hour, Accepted: 3}, h)
			case 1:
				require.Equal(t, IntegrationHour{Hour: h.Hour, Rejected: 2}, h)
			case 23:
				require.Equal(t, IntegrationHour{Hour: h.Hour, Accepted: 4, Rejected: 1}, h)
			default:
				require.Zero(t, h.Accepted, "час %d", i)
				require.Zero(t, h.Rejected, "час %d", i)
			}
		}
		require.Equal(t, "2026-09-09T18:00:00+05:30", got[0].Hour.Format(time.RFC3339))
		require.Equal(t, "2026-09-10T17:00:00+05:30", got[23].Hour.Format(time.RFC3339))
	})

	t.Run("single_hour", func(t *testing.T) {
		got := integrationHours(now, load(t, "Europe/Moscow"), 1, nil, nil)
		require.Len(t, got, 1)
		require.Equal(t, "2026-09-10T15:00:00+03:00", got[0].Hour.Format(time.RFC3339))
	})

	t.Run("max_hours_utc", func(t *testing.T) {
		got := integrationHours(now, nil, MaxIntegrationHours, nil, nil)
		require.Len(t, got, MaxIntegrationHours)
		require.Equal(t, time.UTC, got[0].Hour.Location())
		require.True(t, time.Date(2026, 9, 3, 13, 0, 0, 0, time.UTC).Equal(got[0].Hour))
		require.True(t, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC).Equal(got[MaxIntegrationHours-1].Hour))
	})

	t.Run("hour_boundary", func(t *testing.T) {
		at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
		got := integrationHours(at, nil, 2, nil, nil)
		require.Len(t, got, 2)
		require.True(t, at.Equal(got[1].Hour), "час, начавшийся ровно сейчас, — последняя точка")
	})

	t.Run("dst_fall_back", func(t *testing.T) {
		// 2026-10-25 в Европе/Берлине: в 03:00 CEST часы переводятся на 02:00 CET, час 02:00 повторяется.
		at := time.Date(2026, 10, 25, 3, 10, 0, 0, time.UTC) // 04:10 CET
		got := integrationHours(at, load(t, "Europe/Berlin"), 4, nil, nil)
		formatted := make([]string, 0, len(got))
		for _, h := range got {
			formatted = append(formatted, h.Hour.Format(time.RFC3339))
		}
		require.Equal(t, []string{
			"2026-10-25T02:00:00+02:00", "2026-10-25T02:00:00+01:00",
			"2026-10-25T03:00:00+01:00", "2026-10-25T04:00:00+01:00",
		}, formatted)
	})
}

func TestIntegrationWindowStart(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	now := time.Date(2026, 9, 10, 12, 20, 30, 0, time.UTC)
	require.True(t, time.Date(2026, 9, 9, 13, 0, 0, 0, time.UTC).Equal(integrationWindowStart(now, msk, 24)))
	require.True(t, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC).Equal(integrationWindowStart(now, msk, 1)))
	require.True(t, time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC).Equal(integrationWindowStart(now, nil, 1)))
}

func TestIntegrationHealth_HoursOutOfRange(t *testing.T) {
	svc := NewService(nil, clock.NewFixed(time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)))
	for _, h := range []int{0, -1, MaxIntegrationHours + 1} {
		_, err := svc.IntegrationHealth(t.Context(), Params{}, h)
		require.ErrorIs(t, err, ErrUnsupported, "hours=%d", h)
	}
}

func TestIntegrationVersions(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	msk := time.FixedZone("MSK", 3*3600)
	h1, h2 := at.Add(-2*time.Hour), at.Add(-time.Hour)

	got := integrationVersions(
		[]hourVersionRow{
			{Hour: h1, AppVersion: "1.0", N: 2, LastSeen: at.Add(-90 * time.Minute)},
			{Hour: h2, AppVersion: "1.0", N: 3, LastSeen: at.Add(-30 * time.Minute).In(msk)},
			{Hour: h2, AppVersion: "2.0", N: 1, LastSeen: at.Add(-10 * time.Minute)},
		},
		[]hourVersionRow{
			{Hour: h1, AppVersion: "1.0", N: 4, LastSeen: at.Add(-100 * time.Minute)},
			{Hour: h2, AppVersion: "", N: 1, LastSeen: at.Add(-30 * time.Minute)},
			{Hour: h2, AppVersion: "0.9", N: 5, LastSeen: at.Add(-5 * time.Minute)},
		},
	)
	require.Equal(t, []IntegrationVersion{
		{AppVersion: "0.9", Rejected: 5, LastSeen: at.Add(-5 * time.Minute)},
		{AppVersion: "2.0", Accepted: 1, LastSeen: at.Add(-10 * time.Minute)},
		{AppVersion: "", Rejected: 1, LastSeen: at.Add(-30 * time.Minute)},
		{AppVersion: "1.0", Accepted: 5, Rejected: 4, LastSeen: at.Add(-30 * time.Minute)},
	}, got, "по убыванию last_seen, при равенстве — по версии; last_seen в UTC")

	require.NotNil(t, integrationVersions(nil, nil))
	require.Empty(t, integrationVersions(nil, nil))

	var many []hourVersionRow
	for i := range IntegrationVersionsMax + 10 {
		many = append(many, hourVersionRow{Hour: h2, AppVersion: fmt.Sprintf("1.%03d", i), N: 1, LastSeen: at.Add(time.Duration(i) * time.Second)})
	}
	got = integrationVersions(many, nil)
	require.Len(t, got, IntegrationVersionsMax)
	require.Equal(t, fmt.Sprintf("1.%03d", IntegrationVersionsMax+9), got[0].AppVersion, "остаются самые свежие версии")
	require.Equal(t, "1.010", got[IntegrationVersionsMax-1].AppVersion)
}

func TestByHour(t *testing.T) {
	h := time.Date(2026, 9, 10, 11, 0, 0, 0, time.UTC)
	require.Equal(t, map[int64]uint64{h.Unix(): 5, h.Add(time.Hour).Unix(): 1}, byHour([]hourVersionRow{
		{Hour: h, AppVersion: "1.0", N: 2},
		{Hour: h.In(time.FixedZone("MSK", 3*3600)), AppVersion: "2.0", N: 3},
		{Hour: h.Add(time.Hour), AppVersion: "1.0", N: 1},
	}))
}
