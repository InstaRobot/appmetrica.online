//go:build integration

package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/identity"
)

func withContext(r chstore.EventRow, platform, appVersion, osName, country string) chstore.EventRow {
	r.Platform, r.AppVersion, r.OSName, r.Country = platform, appVersion, osName, country
	return r
}

// AC-027.5 (AC-5.2.4) на ClickHouse: OR внутри параметра, AND между параметрами, пустой — без фильтра.
func TestFilters_OrWithinAndAcross_ClickHouse(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		withContext(session(t, 1, "", "2026-10-05T12:00:00+03:00"), "ios", "2.4.0", "iOS", "RU"),
		withContext(session(t, 2, "", "2026-10-05T12:00:00+03:00"), "ios", "2.5.0", "iOS", "US"),
		withContext(session(t, 3, "", "2026-10-05T12:00:00+03:00"), "android", "2.4.0", "Android", "RU"),
		withContext(session(t, 4, "", "2026-10-05T12:00:00+03:00"), "web", "", "", ""),
	})

	for _, tc := range []struct {
		name    string
		filters Filters
		want    []string
	}{
		{"none", Filters{}, []string{installKey(1), installKey(2), installKey(3), installKey(4)}},
		{"platform_and_versions", Filters{Platform: []string{"ios"}, AppVersion: []string{"2.4.0", "2.5.0"}},
			[]string{installKey(1), installKey(2)}},
		{"platforms_and_country", Filters{Platform: []string{"ios", "android"}, Country: []string{"RU"}},
			[]string{installKey(1), installKey(3)}},
		{"os_name", Filters{OSName: []string{"Android"}}, []string{installKey(3)}},
		{"empty_value", Filters{AppVersion: []string{""}}, []string{installKey(4)}},
		{"no_match", Filters{Country: []string{"DE"}}, nil},
		{"and_excludes", Filters{Platform: []string{"android"}, Country: []string{"US"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := period(t, identity.Installs, "2026-10-05", "2026-10-05")
			p.Filters = tc.filters
			got := personDays(t, env, p)["2026-10-05"]
			require.Equal(t, tc.want, got)
		})
	}
}

// Значения фильтров доходят до ClickHouse без изменений, включая кавычки, обратные слэши и управляющие символы.
func TestFilters_ParameterRoundTrip(t *testing.T) {
	env := startWith(t)
	values := []string{"", "2.4.0", "it's", `a\b`, `\'`, "x'] OR 1=1 --", "Россия 東京", "a\tb\nc\rd", "{f_country:Array(String)}"}
	p := period(t, identity.Persons, "2026-10-05", "2026-10-05")
	p.Filters = Filters{AppVersion: values}

	var got []string
	require.NoError(t, env.CH.Conn().QueryRow(p.Bind(t.Context()), `SELECT {f_app_version:Array(String)}`).Scan(&got))
	require.Equal(t, values, got)
}

// Период — сутки в TZ приложения: событие 01:30 MSK 10-05 (22:30 UTC 10-04, партиция 10-04) относится к 10-05.
func TestParams_PeriodInAppTimezone(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-04T20:30:00Z"), // 23:30 MSK 10-04
		session(t, 2, "", "2026-10-04T22:30:00Z"), // 01:30 MSK 10-05
		session(t, 3, "", "2026-10-05T20:59:59.999Z"),
		session(t, 4, "", "2026-10-05T21:00:00Z"), // 00:00 MSK 10-06
	})

	require.Equal(t, map[string][]string{"2026-10-04": {installKey(1)}},
		personDays(t, env, period(t, identity.Installs, "2026-10-04", "2026-10-04")))
	require.Equal(t, map[string][]string{"2026-10-05": {installKey(2), installKey(3)}},
		personDays(t, env, period(t, identity.Installs, "2026-10-05", "2026-10-05")))
	require.Equal(t, map[string][]string{"2026-10-06": {installKey(4)}},
		personDays(t, env, period(t, identity.Installs, "2026-10-06", "2026-10-06")))

	utc := period(t, identity.Installs, "2026-10-04", "2026-10-04")
	utc.Location = time.UTC
	require.Equal(t, map[string][]string{"2026-10-04": {installKey(1), installKey(2)}}, personDays(t, env, utc))
}
