//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

// audienceParams — параметры отчёта приложения сценария, как их собирает Read API.
func audienceParams(t *testing.T, app testutil.ScenarioApp, from, to string) reports.Params {
	t.Helper()
	loc, err := time.LoadLocation(app.Timezone)
	require.NoError(t, err)
	parse := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		require.NoError(t, err)
		return d
	}
	return reports.Params{
		AppID: app.ID, Location: loc, Env: reports.EnvProduction, Unit: identity.Persons,
		Period: reports.Period{From: parse(from), To: parse(to)},
	}
}

func audiencePath(app testutil.ScenarioApp, report, query string) string {
	return "/v1/apps/" + app.ID.String() + "/audience/" + report + "?" + query
}

// requireJSON — ответ 200 совпадает с JSON отчёта reports.
func requireJSON(t *testing.T, want any, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	b, err := json.Marshal(want)
	require.NoError(t, err)
	require.JSONEq(t, string(b), rec.Body.String())
}

type apiErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details"`
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder, status int) apiErrorBody {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	var body struct {
		Error apiErrorBody `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Error
}

func TestGetAudience_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	msk, nyc, tyo := testutil.ScenarioApps[0], testutil.ScenarioApps[1], testutil.ScenarioApps[2]

	t.Run("summary_defaults", func(t *testing.T) {
		want, err := e.reports.AudienceSummary(t.Context(), audienceParams(t, msk, "2026-10-06", "2026-10-12"), true)
		require.NoError(t, err)
		require.NotNil(t, want.Compare, "compare=previous_period по умолчанию")
		requireJSON(t, want, e.get(t, audiencePath(msk, "summary", "from=2026-10-06&to=2026-10-12"), e.all))
	})

	t.Run("summary_params", func(t *testing.T) {
		p := audienceParams(t, msk, "2026-10-01", "2026-10-12")
		p.Env, p.Unit = reports.EnvDevelopment, identity.Installs
		p.Filters = reports.Filters{Platform: []string{"ios", "android"}, Country: []string{"RU"}}
		want, err := e.reports.AudienceSummary(t.Context(), p, false)
		require.NoError(t, err)
		rec := e.get(t, audiencePath(msk, "summary",
			"from=2026-10-01&to=2026-10-12&compare=none&environment=development&unit=installs&platform=ios&platform=android&country=RU"), e.all)
		requireJSON(t, want, rec)
		require.NotContains(t, rec.Body.String(), `"compare"`)
	})

	t.Run("timeseries", func(t *testing.T) {
		p := audienceParams(t, nyc, "2026-10-31", "2026-11-01")
		want, err := e.reports.AudienceTimeseries(t.Context(), p, reports.TimeseriesQuery{
			Granularity: reports.GranularityHour, Metric: reports.MetricSessions,
		})
		require.NoError(t, err)
		require.Len(t, want.Points, 49, "24 + 25 часов перехода")
		requireJSON(t, want, e.get(t, audiencePath(nyc, "timeseries",
			"from=2026-10-31&to=2026-11-01&granularity=hour&metric=sessions&compare=none"), e.all))

		p = audienceParams(t, msk, "2026-10-01", "2026-10-12")
		want, err = e.reports.AudienceTimeseries(t.Context(), p, reports.TimeseriesQuery{
			Granularity: reports.GranularityDay, Metric: reports.MetricActive, Compare: true,
		})
		require.NoError(t, err)
		requireJSON(t, want, e.get(t, audiencePath(msk, "timeseries", "from=2026-10-01&to=2026-10-12"), e.all))

		want, err = e.reports.AudienceTimeseries(t.Context(), p, reports.TimeseriesQuery{
			Granularity: reports.GranularityWeek, Metric: reports.MetricAvgDuration, Compare: true,
		})
		require.NoError(t, err)
		requireJSON(t, want, e.get(t, audiencePath(msk, "timeseries",
			"from=2026-10-01&to=2026-10-12&granularity=week&metric=avg_duration_ms&compare=previous_period"), e.all))
	})

	// AC-6.1: split=platform — ряды по платформам рядом с итогом, compare_points нет даже при compare=previous_period.
	t.Run("timeseries_split", func(t *testing.T) {
		p := audienceParams(t, msk, "2026-10-01", "2026-10-12")
		want, err := e.reports.AudienceTimeseries(t.Context(), p, reports.TimeseriesQuery{
			Granularity: reports.GranularityDay, Metric: reports.MetricActive, Compare: true, SplitByPlatform: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, want.Splits)
		require.Nil(t, want.ComparePoints)
		rec := e.get(t, audiencePath(msk, "timeseries", "from=2026-10-01&to=2026-10-12&split=platform"), e.all)
		requireJSON(t, want, rec)
		require.NotContains(t, rec.Body.String(), `"compare_points"`)

		want, err = e.reports.AudienceTimeseries(t.Context(), p, reports.TimeseriesQuery{
			Granularity: reports.GranularityDay, Metric: reports.MetricActive, Compare: true,
		})
		require.NoError(t, err)
		rec = e.get(t, audiencePath(msk, "timeseries", "from=2026-10-01&to=2026-10-12&split=none"), e.all)
		requireJSON(t, want, rec)
		require.NotContains(t, rec.Body.String(), `"splits"`)
	})

	t.Run("rolling_active", func(t *testing.T) {
		want, err := e.reports.RollingActive(t.Context(), audienceParams(t, msk, "2026-09-13", "2026-10-12"))
		require.NoError(t, err)
		require.Len(t, want.Points, 30)
		requireJSON(t, want, e.get(t, audiencePath(msk, "rolling-active", "from=2026-09-13&to=2026-10-12"), e.all))
	})

	t.Run("breakdown", func(t *testing.T) {
		p := audienceParams(t, msk, "2026-10-01", "2026-10-12")
		want, err := e.reports.AudienceBreakdown(t.Context(), p, reports.BreakdownQuery{
			Dimension: reports.DimensionCountry, Metric: reports.MetricActive, Limit: 50,
		})
		require.NoError(t, err)
		require.NotEmpty(t, want.Rows)
		requireJSON(t, want, e.get(t, audiencePath(msk, "breakdown", "from=2026-10-01&to=2026-10-12&dimension=country"), e.all))

		want, err = e.reports.AudienceBreakdown(t.Context(), p, reports.BreakdownQuery{
			Dimension: reports.DimensionAppVersion, Metric: reports.MetricNew, Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, want.Rows, 1)
		requireJSON(t, want, e.get(t, audiencePath(msk, "breakdown",
			"from=2026-10-01&to=2026-10-12&dimension=app_version&metric=new&limit=1"), e.all))
	})

	// Отчёты архивного приложения доступны; ключ, ограниченный приложением, видит только его.
	t.Run("archived_and_restricted", func(t *testing.T) {
		require.Equal(t, http.StatusOK, e.get(t, audiencePath(tyo, "summary", "from=2026-10-01&to=2026-10-12"), e.all).Code)
		require.Equal(t, http.StatusOK, e.get(t, audiencePath(nyc, "summary", "from=2026-10-01&to=2026-10-12"), e.restricted).Code)
		body := errorBody(t, e.get(t, audiencePath(msk, "summary", "from=2026-10-01&to=2026-10-12"), e.restricted), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})

	t.Run("app_not_found", func(t *testing.T) {
		unknown := testutil.ScenarioApp{ID: uuid.MustParse("5ce0a000-0000-4000-8000-0000000000ff")}
		for _, report := range []string{"summary", "timeseries", "rolling-active", "breakdown"} {
			query := "from=2026-10-01&to=2026-10-12&dimension=country"
			body := errorBody(t, e.get(t, audiencePath(unknown, report, query), e.all), http.StatusNotFound)
			require.Equal(t, "app_not_found", body.Code, report)
		}
	})
}

// Тест-векторы 06-api-contracts.md §2.4.4: hour_range, range_366 (400 даёт 367-й день: 366 дней включительно
// допустимы, OQ-042), from_after_to, bad_dimension и проверки остальных параметров — 400 с details.field,
// присланное значение не цитируется.
func TestGetAudience_InvalidParams(t *testing.T) {
	e := newPortfolioEnv(t)
	msk := testutil.ScenarioApps[0]

	for _, tc := range []struct {
		name, report, query string
		code, field         string
		maxDays             float64
	}{
		{"range_367", "summary", "from=2025-10-11&to=2026-10-12", "range_too_large", "to", 366},
		{"rolling_range", "rolling-active", "from=2025-10-11&to=2026-10-12", "range_too_large", "to", 366},
		{"hour_range", "timeseries", "from=2026-10-01&to=2026-10-08&granularity=hour", "range_too_large", "to", 7},
		{"from_after_to", "summary", "from=2026-10-12&to=2026-10-06", "invalid_request", "from", 0},
		{"from_after_to_breakdown", "breakdown", "from=2026-10-12&to=2026-10-06&dimension=country", "invalid_request", "from", 0},
		{"bad_dimension", "breakdown", "from=2026-10-06&to=2026-10-12&dimension=ip", "invalid_request", "dimension", 0},
		{"missing_dimension", "breakdown", "from=2026-10-06&to=2026-10-12", "invalid_request", "dimension", 0},
		{"breakdown_metric", "breakdown", "from=2026-10-06&to=2026-10-12&dimension=country&metric=avg_duration_ms", "invalid_request", "metric", 0},
		{"limit_zero", "breakdown", "from=2026-10-06&to=2026-10-12&dimension=country&limit=0", "invalid_request", "limit", 0},
		{"limit_201", "breakdown", "from=2026-10-06&to=2026-10-12&dimension=country&limit=201", "invalid_request", "limit", 0},
		{"limit_not_int", "breakdown", "from=2026-10-06&to=2026-10-12&dimension=country&limit=ten", "invalid_request", "limit", 0},
		{"timeseries_metric", "timeseries", "from=2026-10-06&to=2026-10-12&metric=retention", "invalid_request", "metric", 0},
		{"granularity", "timeseries", "from=2026-10-06&to=2026-10-12&granularity=quarter", "invalid_request", "granularity", 0},
		{"summary_compare", "summary", "from=2026-10-06&to=2026-10-12&compare=yes", "invalid_request", "compare", 0},
		{"timeseries_compare", "timeseries", "from=2026-10-06&to=2026-10-12&compare=", "invalid_request", "compare", 0},
		{"timeseries_split", "timeseries", "from=2026-10-06&to=2026-10-12&split=os", "invalid_request", "split", 0},
		{"unit", "rolling-active", "from=2026-10-06&to=2026-10-12&unit=people", "invalid_request", "unit", 0},
		{"environment", "summary", "from=2026-10-06&to=2026-10-12&environment=staging", "invalid_request", "environment", 0},
		{"country", "breakdown", "from=2026-10-06&to=2026-10-12&dimension=city&country=ru", "invalid_request", "country", 0},
		{"missing_to", "summary", "from=2026-10-06", "invalid_request", "to", 0},
		{"bad_date", "rolling-active", "from=2026-13-01&to=2026-10-12", "invalid_request", "from", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.get(t, audiencePath(msk, tc.report, tc.query), e.all)
			body := errorBody(t, rec, http.StatusBadRequest)
			require.Equal(t, tc.code, body.Code)
			require.Equal(t, tc.field, body.Details["field"])
			if tc.maxDays > 0 {
				require.InDelta(t, tc.maxDays, body.Details["max_days"], 0)
			}
			for _, quoted := range []string{"quarter", "retention", "ten", "staging", "people", "yes", `"ip"`} {
				require.NotContains(t, rec.Body.String(), quoted)
			}
		})
	}
}
