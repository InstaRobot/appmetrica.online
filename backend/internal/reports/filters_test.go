package reports

import (
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/identity"
)

// filterColumns — фильтр → колонка events, по которой он отбирает строки.
var filterColumns = map[string]string{
	"f_platform":    "e.platform",
	"f_app_version": "e.app_version",
	"f_os_name":     "e.os_name",
	"f_country":     "e.country",
}

// AC-027.5: значения внутри параметра — OR (IN по массиву), разные параметры — AND; значения уходят
// server-side параметрами {f_*:Array(String)}, в текст SQL не попадают.
func TestFilters_OrWithinAndAcross(t *testing.T) {
	p := Params{
		AppID:    uuid.MustParse("5ce0a000-0000-4000-8000-000000000001"),
		Location: time.UTC,
		Env:      EnvProduction,
		Unit:     identity.Persons,
		Period:   Period{From: date(t, "2026-10-01"), To: date(t, "2026-10-07")},
		Filters:  Filters{Platform: []string{"ios", "android"}, Country: []string{"RU"}},
	}
	got := p.Parameters()
	require.Equal(t, "['ios','android']", got["f_platform"])
	require.Equal(t, "['RU']", got["f_country"])
	require.Equal(t, "[]", got["f_app_version"], "пустой массив выключает фильтр")
	require.Equal(t, "[]", got["f_os_name"])

	// Каждый фильтр — отдельное условие AND: выключен пустым массивом, иначе IN по массиву (OR внутри).
	for name, col := range filterColumns {
		param := "{" + name + ":Array(String)}"
		cond := "AND (empty(" + param + ") OR " + col + " IN " + param + ")"
		require.Contains(t, eventsFilter, cond, "фильтр %s", name)
	}
	require.Contains(t, evCTE, eventsFilter)

	for _, v := range []string{"ios", "android", "'RU'"} {
		require.NotContains(t, eventsFilter, v, "значения фильтров не конкатенируются в SQL")
		require.NotContains(t, evCTE, v)
	}
}

func TestFilters_ArrayParameterEscaping(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want string
	}{
		{"nil", nil, "[]"},
		{"empty_slice", []string{}, "[]"},
		{"empty_value", []string{""}, "['']"},
		{"single", []string{"2.4.0"}, "['2.4.0']"},
		{"quote", []string{"it's"}, `['it\'s']`},
		{"backslash", []string{`a\b`}, `['a\\b']`},
		{"backslash_quote", []string{`\'`}, `['\\\'']`},
		{"injection", []string{"x'] OR 1=1 --"}, `['x\'] OR 1=1 --']`},
		{"unicode", []string{"Россия", "東京"}, "['Россия','東京']"},
		{"control", []string{"a\tb\nc"}, `['a\tb\nc']`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := clickhouse.Parameters{}
			Filters{AppVersion: tc.in}.bind(p)
			require.Equal(t, tc.want, p["f_app_version"])
		})
	}
}

func TestParams_Parameters(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	p := Params{
		AppID:    uuid.MustParse("5ce0a000-0000-4000-8000-000000000001"),
		Location: msk,
		Env:      EnvDevelopment,
		Unit:     identity.Installs,
		Period:   Period{From: date(t, "2026-10-01"), To: date(t, "2026-10-07")},
		Filters:  Filters{AppVersion: []string{"2.4.0", "2.5.0"}, OSName: []string{"iOS"}},
	}
	require.Equal(t, clickhouse.Parameters{
		"app_id":        "5ce0a000-0000-4000-8000-000000000001",
		"env":           "development",
		"unit":          "installs",
		"tz":            "Europe/Moscow",
		"start_utc":     "2026-09-30 21:00:00.000",
		"end_utc":       "2026-10-07 21:00:00.000",
		"d_lo":          "2026-09-30",
		"d_hi":          "2026-10-07",
		"f_platform":    "[]",
		"f_app_version": "['2.4.0','2.5.0']",
		"f_os_name":     "['iOS']",
		"f_country":     "[]",
	}, p.Parameters())
}

func TestParams_NilLocationIsUTC(t *testing.T) {
	p := Params{Env: EnvProduction, Unit: identity.Persons, Period: Period{From: date(t, "2026-10-01"), To: date(t, "2026-10-01")}}
	got := p.Parameters()
	require.Equal(t, "UTC", got["tz"])
	require.Equal(t, "2026-10-01 00:00:00.000", got["start_utc"])
	require.Equal(t, "2026-10-02 00:00:00.000", got["end_utc"])
}

func TestPeriod_Bounds(t *testing.T) {
	nyc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	tyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	for _, tc := range []struct {
		name       string
		loc        *time.Location
		from, to   string
		start, end string
		days       int
	}{
		{"utc_single_day", time.UTC, "2026-10-12", "2026-10-12", "2026-10-12T00:00:00Z", "2026-10-13T00:00:00Z", 1},
		{"east_of_utc", tyo, "2026-10-01", "2026-10-12", "2026-09-30T15:00:00Z", "2026-10-12T15:00:00Z", 12},
		// Сутки перехода на зимнее время в Нью-Йорке длятся 25 часов: EDT (−4) → EST (−5).
		{"dst_fall_back", nyc, "2026-11-01", "2026-11-01", "2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z", 1},
		// Весенний переход: 23 часа.
		{"dst_spring_forward", nyc, "2026-03-08", "2026-03-08", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z", 1},
		{"year_end", tyo, "2026-12-31", "2027-01-01", "2026-12-30T15:00:00Z", "2027-01-01T15:00:00Z", 2},
		{"leap_year", time.UTC, "2028-01-01", "2028-12-31", "2028-01-01T00:00:00Z", "2029-01-01T00:00:00Z", 366},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Period{From: date(t, tc.from), To: date(t, tc.to)}
			start, end := p.Bounds(tc.loc)
			require.Equal(t, tc.start, start.Format(time.RFC3339))
			require.Equal(t, tc.end, end.Format(time.RFC3339))
			require.Equal(t, time.UTC, start.Location())
			require.Equal(t, time.UTC, end.Location())
			require.Equal(t, tc.days, p.Days())
		})
	}
}

// Days и Bounds берут только календарную дату From/To: время и пояс значения не сдвигают день.
func TestPeriod_IgnoresClockOfDates(t *testing.T) {
	tyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	p := Period{
		From: time.Date(2026, time.October, 1, 23, 59, 0, 0, tyo),
		To:   time.Date(2026, time.October, 2, 0, 1, 0, 0, time.UTC),
	}
	require.Equal(t, 2, p.Days())
	start, end := p.Bounds(time.UTC)
	require.Equal(t, "2026-10-01T00:00:00Z", start.Format(time.RFC3339))
	require.Equal(t, "2026-10-03T00:00:00Z", end.Format(time.RFC3339))
}

func TestSQLFragments_NoDatabasePrefix(t *testing.T) {
	// Соединение открыто с базой CLICKHOUSE_DATABASE: в тестах это t_*, поэтому имя базы в тексте запроса
	// направило бы его в пустую appmetrica.
	for name, sql := range map[string]string{
		"links":         identity.LinksCTE,
		"person_key":    identity.PersonKeyExpr,
		"persons_first": identity.PersonsFirstCTE,
		"events_filter": eventsFilter,
		"ev":            evCTE,
	} {
		require.NotEmpty(t, strings.TrimSpace(sql), name)
		require.NotContains(t, sql, "appmetrica.", name)
	}
}

func date(t testing.TB, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	require.NoError(t, err)
	return d
}
