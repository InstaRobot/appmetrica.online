package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

func f64(v float64) *float64 { return &v }

// active_delta_pct = round((a − b) / b × 100, 1), null при b = 0 (02-tech-spec.md FR-012).
func TestDeltaPct(t *testing.T) {
	for _, tc := range []struct {
		a, b uint64
		want *float64
	}{
		{a: 12480, b: 11988, want: f64(4.1)},
		{a: 3, b: 2, want: f64(50)},
		{a: 2, b: 3, want: f64(-33.3)},
		{a: 0, b: 7, want: f64(-100)},
		{a: 5, b: 5, want: f64(0)},
		{a: 1, b: 8, want: f64(-87.5)},
		{a: 7, b: 0, want: nil},
		{a: 0, b: 0, want: nil},
	} {
		got := deltaPct(tc.a, tc.b)
		if tc.want == nil {
			require.Nil(t, got, "%d/%d", tc.a, tc.b)
			continue
		}
		require.NotNil(t, got, "%d/%d", tc.a, tc.b)
		require.InDelta(t, *tc.want, *got, 1e-9, "%d/%d", tc.a, tc.b)
	}
}

// retention_d1/d7 — доля когорты, null для пустой когорты.
func TestRatio(t *testing.T) {
	require.Nil(t, ratio(0, 0))
	require.Nil(t, ratio(3, 0))
	require.Equal(t, f64(0.5), ratio(1, 2))
	require.Equal(t, f64(0), ratio(0, 4))
	require.Equal(t, f64(1), ratio(3, 3))
	require.Equal(t, f64(2.0/3), ratio(2, 3))
}

func TestSparkline_ClosedThenLive(t *testing.T) {
	from := date(t, "2026-09-13")
	closed := map[string]uint64{"2026-09-13": 5, "2026-09-14": 999, "2026-09-20": 7}
	live := map[string]uint64{"2026-09-14": 3, "2026-09-15": 4, "2026-10-12": 9, "2026-10-13": 100, "2026-09-12": 100}

	got := sparkline(from, closed, live)
	require.Len(t, got, 30)
	require.Equal(t, SparklinePoint{Day: "2026-09-13", Active: 5}, got[0])
	require.Equal(t, SparklinePoint{Day: "2026-09-14", Active: 999}, got[1], "закрытый день важнее live")
	require.Equal(t, SparklinePoint{Day: "2026-09-15", Active: 4}, got[2])
	require.Equal(t, SparklinePoint{Day: "2026-09-16", Active: 0}, got[3], "дня нет ни там, ни там — 0")
	require.Equal(t, SparklinePoint{Day: "2026-09-20", Active: 7}, got[7])
	require.Equal(t, SparklinePoint{Day: "2026-10-12", Active: 9}, got[29], "дни вне окна не попадают")
	for i, p := range got {
		require.Equal(t, from.AddDate(0, 0, i).Format(time.DateOnly), p.Day)
	}
}

// Окно sparkline через переход DST: 30 календарных дней, без пропусков и повторов.
func TestSparkline_CalendarDaysAcrossDST(t *testing.T) {
	got := sparkline(date(t, "2026-10-14"), nil, nil)
	require.Len(t, got, 30)
	require.Equal(t, "2026-10-14", got[0].Day)
	require.Equal(t, "2026-11-01", got[18].Day)
	require.Equal(t, "2026-11-12", got[29].Day)
}

// date по умолчанию — вчера в settings.owner_timezone (06-api-contracts.md §2.4.3).
func TestPortfolio_DefaultDate(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	la, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)

	// 2026-10-13 05:00 UTC: в Москве 08:00 13-го, в Лос-Анджелесе 22:00 12-го.
	svc := NewService(nil, clock.NewFixed(time.Date(2026, 10, 13, 5, 0, 0, 0, time.UTC)))
	for _, tc := range []struct {
		loc  *time.Location
		want string
	}{
		{msk, "2026-10-12"},
		{la, "2026-10-11"},
		{nil, "2026-10-12"},
	} {
		got, err := svc.Portfolio(t.Context(), PortfolioQuery{OwnerLocation: tc.loc, Env: EnvProduction, Unit: identity.Persons})
		require.NoError(t, err)
		require.Equal(t, Portfolio{Date: tc.want, Environment: EnvProduction, Unit: identity.Persons, Rows: []PortfolioRow{}}, got)
	}

	// Явная дата не зависит от пояса владельца; время и пояс значения игнорируются.
	explicit := time.Date(2026, 10, 1, 23, 30, 0, 0, la)
	got, err := svc.Portfolio(t.Context(), PortfolioQuery{Date: explicit, OwnerLocation: msk, Env: EnvDevelopment, Unit: identity.Installs})
	require.NoError(t, err)
	require.Equal(t, Portfolio{Date: "2026-10-01", Environment: EnvDevelopment, Unit: identity.Installs, Rows: []PortfolioRow{}}, got)
}

// Равные active — в порядке enum Platform, неизвестные платформы — после известных по алфавиту.
func TestSortPlatformActive(t *testing.T) {
	got := []PlatformActive{
		{Platform: "web", Active: 1},
		{Platform: "zz", Active: 1},
		{Platform: "android", Active: 1},
		{Platform: "other", Active: 1},
		{Platform: "aa", Active: 1},
		{Platform: "macos", Active: 3},
		{Platform: "ios", Active: 2},
	}
	sortPlatformActive(got)
	require.Equal(t, []PlatformActive{
		{Platform: "macos", Active: 3},
		{Platform: "ios", Active: 2},
		{Platform: "android", Active: 1},
		{Platform: "web", Active: 1},
		{Platform: "other", Active: 1},
		{Platform: "aa", Active: 1},
		{Platform: "zz", Active: 1},
	}, got)

	sortPlatformActive(nil)
}

// platformOrder повторяет enum app_platform (и Platform в api.yaml): порядок долей портфеля — порядок enum.
func TestPlatformOrderMatchesEnum(t *testing.T) {
	want := make([]string, 0, len(platformOrder))
	for _, p := range sqlc.AllAppPlatformValues() {
		want = append(want, string(p))
	}
	require.Equal(t, want, platformOrder)
}
