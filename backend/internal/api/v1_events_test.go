//go:build integration

package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

const eventsQuery = "from=2026-10-04&to=2026-11-02"

// newEventsEnv — Read API поверх сценария и своих событий (custom_events.ndjson): msk и nyc шлют свои события.
func newEventsEnv(t *testing.T) *portfolioEnv {
	t.Helper()
	e := newPortfolioEnv(t)
	e.env.LoadNDJSON(t, testutil.CustomEventsPath())
	return e
}

func eventsPath(app testutil.ScenarioApp, tail, query string) string {
	return "/v1/apps/" + app.ID.String() + "/events" + tail + "?" + query
}

func eventsParams(t *testing.T, app testutil.ScenarioApp) reports.Params {
	t.Helper()
	return audienceParams(t, app, "2026-10-04", "2026-11-02")
}

// AC-006.1: тела HTTP равны прямому вызову reports; cookie и Bearer дают одно тело.
func TestEvents_MatchesReports(t *testing.T) {
	e := newEventsEnv(t)
	msk, nyc := testutil.ScenarioApps[0], testutil.ScenarioApps[1]

	t.Run("list", func(t *testing.T) {
		want, err := e.reports.EventsList(t.Context(), eventsParams(t, msk), true)
		require.NoError(t, err)
		require.NotEmpty(t, want.Events)
		require.NotNil(t, want.Events[0].PrevEvents, "compare=previous_period по умолчанию")
		requireJSON(t, want, e.get(t, eventsPath(msk, "", eventsQuery), e.all))

		want, err = e.reports.EventsList(t.Context(), eventsParams(t, nyc), false)
		require.NoError(t, err)
		rec := e.get(t, eventsPath(nyc, "", eventsQuery+"&compare=none"), e.all)
		requireJSON(t, want, rec)
		require.NotContains(t, rec.Body.String(), "prev_")
	})

	t.Run("list_params", func(t *testing.T) {
		p := eventsParams(t, msk)
		p.Env, p.Unit = reports.EnvDevelopment, identity.Installs
		p.Filters = reports.Filters{Platform: []string{"ios"}, Country: []string{"RU"}}
		want, err := e.reports.EventsList(t.Context(), p, true)
		require.NoError(t, err)
		requireJSON(t, want, e.get(t, eventsPath(msk, "",
			eventsQuery+"&environment=development&unit=installs&platform=ios&country=RU"), e.all))
	})

	t.Run("report", func(t *testing.T) {
		p := eventsParams(t, msk)
		want, err := e.reports.EventReport(t.Context(), p, reports.EventReportQuery{
			Name: "note_created", Granularity: reports.GranularityDay, Compare: true,
		})
		require.NoError(t, err)
		require.NotZero(t, want.Summary.Events)
		require.NotEmpty(t, want.PropertyKeys)
		requireJSON(t, want, e.get(t, eventsPath(msk, "/note_created", eventsQuery), e.all))

		want, err = e.reports.EventReport(t.Context(), p, reports.EventReportQuery{
			Name: "purchase", Granularity: reports.GranularityWeek,
		})
		require.NoError(t, err)
		rec := e.get(t, eventsPath(msk, "/purchase", eventsQuery+"&granularity=week&compare=none"), e.all)
		requireJSON(t, want, rec)
		require.NotContains(t, rec.Body.String(), "prev_")
	})

	// Имя без событий за период — 200 с нулями, не 404.
	t.Run("report_no_events", func(t *testing.T) {
		want, err := e.reports.EventReport(t.Context(), eventsParams(t, msk), reports.EventReportQuery{
			Name: "never_sent", Granularity: reports.GranularityDay, Compare: true,
		})
		require.NoError(t, err)
		require.Zero(t, want.Summary.Events)
		requireJSON(t, want, e.get(t, eventsPath(msk, "/never_sent", eventsQuery), e.all))
	})

	t.Run("breakdown", func(t *testing.T) {
		p := eventsParams(t, msk)
		want, err := e.reports.EventBreakdown(t.Context(), p, reports.EventBreakdownQuery{Name: "note_created", Property: "source"})
		require.NoError(t, err)
		require.NotNil(t, want.Other, "25 значений при limit=20")
		requireJSON(t, want, e.get(t, eventsPath(msk, "/note_created/breakdown", eventsQuery+"&property=source"), e.all))

		want, err = e.reports.EventBreakdown(t.Context(), p, reports.EventBreakdownQuery{Name: "purchase", Property: "plan", Limit: 100})
		require.NoError(t, err)
		rec := e.get(t, eventsPath(msk, "/purchase/breakdown", eventsQuery+"&property=plan&limit=100"), e.all)
		requireJSON(t, want, rec)
		require.NotContains(t, rec.Body.String(), `"other"`)
	})

	t.Run("cookie_and_bearer", func(t *testing.T) {
		cookie := ownerCookie(t, e)
		for _, path := range []string{
			eventsPath(msk, "", eventsQuery),
			eventsPath(msk, "/purchase", eventsQuery),
			eventsPath(msk, "/purchase/breakdown", eventsQuery+"&property=plan"),
		} {
			byBearer := e.get(t, path, e.all)
			require.Equal(t, http.StatusOK, byBearer.Code, byBearer.Body.String())
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			req.AddCookie(cookie)
			byCookie := serveRequest(t, e.handler, req)
			require.Equal(t, http.StatusOK, byCookie.Code, byCookie.Body.String())
			require.Equal(t, byBearer.Body.String(), byCookie.Body.String(), path)
		}
	})

	t.Run("restricted_key", func(t *testing.T) {
		require.Equal(t, http.StatusOK, e.get(t, eventsPath(nyc, "", eventsQuery), e.restricted).Code)
		body := errorBody(t, e.get(t, eventsPath(msk, "", eventsQuery), e.restricted), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})
}

// AC-006.2: неверные параметры — 400 с details.field, системное имя — 404 event_not_found.
func TestEvents_InvalidParams(t *testing.T) {
	e := newEventsEnv(t)
	msk := testutil.ScenarioApps[0]

	for _, tc := range []struct {
		name, tail, query string
		status            int
		code, field       string
		maxDays           float64
	}{
		{"property_pattern", "/purchase/breakdown", eventsQuery + "&property=Bad", 400, "invalid_request", "property", 0},
		{"property_dash", "/purchase/breakdown", eventsQuery + "&property=a-b", 400, "invalid_request", "property", 0},
		{"property_missing", "/purchase/breakdown", eventsQuery, 400, "invalid_request", "property", 0},
		{"property_empty", "/purchase/breakdown", eventsQuery + "&property=", 400, "invalid_request", "property", 0},
		{"limit_zero", "/purchase/breakdown", eventsQuery + "&property=plan&limit=0", 400, "invalid_request", "limit", 0},
		{"limit_101", "/purchase/breakdown", eventsQuery + "&property=plan&limit=101", 400, "invalid_request", "limit", 0},
		{"limit_not_int", "/purchase/breakdown", eventsQuery + "&property=plan&limit=ten", 400, "invalid_request", "limit", 0},
		{"event_name_pattern", "/Purchase", eventsQuery, 400, "invalid_request", "event_name", 0},
		{"event_name_breakdown", "/9lives/breakdown", eventsQuery + "&property=plan", 400, "invalid_request", "event_name", 0},
		{"range_400_days", "", "from=2025-09-29&to=2026-11-02", 400, "range_too_large", "to", 366},
		{"range_report", "/purchase", "from=2025-09-29&to=2026-11-02", 400, "range_too_large", "to", 366},
		{"range_breakdown", "/purchase/breakdown", "from=2025-09-29&to=2026-11-02&property=plan", 400, "range_too_large", "to", 366},
		{"hour_range", "/purchase", "from=2026-10-01&to=2026-10-08&granularity=hour", 400, "range_too_large", "to", 7},
		{"granularity", "/purchase", eventsQuery + "&granularity=quarter", 400, "invalid_request", "granularity", 0},
		{"compare", "", eventsQuery + "&compare=yes", 400, "invalid_request", "compare", 0},
		{"from_after_to", "", "from=2026-11-02&to=2026-10-04", 400, "invalid_request", "from", 0},
		{"unit", "", eventsQuery + "&unit=people", 400, "invalid_request", "unit", 0},
		{"system_report", "/session_start", eventsQuery, 404, "event_not_found", "", 0},
		{"system_end_report", "/session_end", eventsQuery, 404, "event_not_found", "", 0},
		{"system_breakdown", "/session_start/breakdown", eventsQuery + "&property=plan", 404, "event_not_found", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.get(t, eventsPath(msk, tc.tail, tc.query), e.all)
			body := errorBody(t, rec, tc.status)
			require.Equal(t, tc.code, body.Code)
			if tc.field != "" {
				require.Equal(t, tc.field, body.Details["field"])
			}
			if tc.maxDays > 0 {
				require.InDelta(t, tc.maxDays, body.Details["max_days"], 0)
			}
			for _, quoted := range []string{"quarter", "ten", "people", "Bad", "a-b"} {
				require.NotContains(t, rec.Body.String(), quoted)
			}
		})
	}

	t.Run("app_not_found", func(t *testing.T) {
		unknown := testutil.ScenarioApp{ID: testutil.ScenarioApps[0].ID}
		unknown.ID[15] ^= 0xff
		for _, tail := range []string{"", "/purchase", "/purchase/breakdown"} {
			body := errorBody(t, e.get(t, eventsPath(unknown, tail, eventsQuery+"&property=plan"), e.all), http.StatusNotFound)
			require.Equal(t, "app_not_found", body.Code, tail)
		}
	})

	// Имя в пути кодируется как есть: процентное кодирование не обходит шаблон.
	t.Run("encoded_name", func(t *testing.T) {
		rec := e.get(t, eventsPath(msk, "/"+url.PathEscape("note created"), eventsQuery), e.all)
		require.Equal(t, "invalid_request", errorBody(t, rec, http.StatusBadRequest).Code)
	})
}
