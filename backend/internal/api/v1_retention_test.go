//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

func retentionPath(app testutil.ScenarioApp, query string) string {
	return "/v1/apps/" + app.ID.String() + "/retention?" + query
}

// requireRetention — ответ 200 совпадает с матрицей reports: доли в api.yaml — float (float32), поэтому
// значения сравниваются с допуском, null — точно.
func requireRetention(t *testing.T, want reports.Retention, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got reports.Retention
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	requireShares := func(want, got []*float64, msg string) {
		t.Helper()
		require.Len(t, got, len(want), msg)
		for i := range want {
			if want[i] == nil {
				require.Nil(t, got[i], "%s[%d]", msg, i)
				continue
			}
			require.NotNil(t, got[i], "%s[%d]", msg, i)
			require.InDelta(t, *want[i], *got[i], 1e-6, "%s[%d]", msg, i)
		}
	}
	require.Equal(t, want.CohortGranularity, got.CohortGranularity)
	require.Equal(t, want.Periods, got.Periods)
	requireShares(want.Average, got.Average, "average")
	require.Len(t, got.Cohorts, len(want.Cohorts))
	for i, c := range want.Cohorts {
		require.Equal(t, c.CohortStart, got.Cohorts[i].CohortStart)
		require.Equal(t, c.Size, got.Cohorts[i].Size, c.CohortStart)
		requireShares(c.Values, got.Cohorts[i].Values, c.CohortStart)
	}
}

// getRetention отдаёт матрицу reports.Retention с умолчаниями periods (30 дней, 12 недель) и общими параметрами.
func TestGetRetention_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	msk, nyc, tyo := testutil.ScenarioApps[0], testutil.ScenarioApps[1], testutil.ScenarioApps[2]
	ctx := t.Context()

	t.Run("day_defaults", func(t *testing.T) {
		want, err := e.reports.Retention(ctx, audienceParams(t, msk, "2026-09-13", "2026-10-12"),
			reports.RetentionQuery{Granularity: reports.GranularityDay, Periods: reports.DefaultDayPeriods})
		require.NoError(t, err)
		require.Len(t, want.Cohorts, 30)
		requireRetention(t, want, e.get(t, retentionPath(msk, "from=2026-09-13&to=2026-10-12"), e.all))
	})

	t.Run("week_params", func(t *testing.T) {
		p := audienceParams(t, nyc, "2026-09-28", "2026-10-12")
		p.Unit = identity.Installs
		p.Filters = reports.Filters{Platform: []string{"android"}}
		want, err := e.reports.Retention(ctx, p, reports.RetentionQuery{Granularity: reports.GranularityWeek, Periods: 4})
		require.NoError(t, err)
		require.Len(t, want.Cohorts, 3)
		requireRetention(t, want, e.get(t, retentionPath(nyc,
			"from=2026-09-28&to=2026-10-12&cohort_granularity=week&periods=4&unit=installs&platform=android"), e.all))
	})

	t.Run("development", func(t *testing.T) {
		p := audienceParams(t, msk, "2026-10-01", "2026-10-12")
		p.Env = reports.EnvDevelopment
		want, err := e.reports.Retention(ctx, p, reports.RetentionQuery{Granularity: reports.GranularityWeek})
		require.NoError(t, err)
		requireRetention(t, want, e.get(t, retentionPath(msk,
			"from=2026-10-01&to=2026-10-12&cohort_granularity=week&environment=development"), e.all))
	})

	t.Run("archived_and_restricted", func(t *testing.T) {
		require.Equal(t, http.StatusOK, e.get(t, retentionPath(tyo, "from=2026-10-01&to=2026-10-12"), e.all).Code)
		require.Equal(t, http.StatusOK, e.get(t, retentionPath(nyc, "from=2026-10-01&to=2026-10-12"), e.restricted).Code)
		body := errorBody(t, e.get(t, retentionPath(msk, "from=2026-10-01&to=2026-10-12"), e.restricted), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})

	t.Run("app_not_found", func(t *testing.T) {
		unknown := testutil.ScenarioApp{ID: uuid.MustParse("5ce0a000-0000-4000-8000-0000000000ff")}
		body := errorBody(t, e.get(t, retentionPath(unknown, "from=2026-10-01&to=2026-10-12"), e.all), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})
}

// Тест-вектор periods_limit (06-api-contracts.md §2.4.5) и остальные параметры getRetention — 400 с details.field,
// присланное значение не цитируется.
func TestGetRetention_InvalidParams(t *testing.T) {
	e := newPortfolioEnv(t)
	msk := testutil.ScenarioApps[0]

	for _, tc := range []struct {
		name, query, code, field string
	}{
		{"periods_91", "from=2026-09-13&to=2026-10-12&periods=91", "invalid_request", "periods"},
		{"week_periods_53", "from=2026-09-13&to=2026-10-12&cohort_granularity=week&periods=53", "invalid_request", "periods"},
		{"periods_not_int", "from=2026-09-13&to=2026-10-12&periods=thirty", "invalid_request", "periods"},
		{"granularity", "from=2026-09-13&to=2026-10-12&cohort_granularity=quarter", "invalid_request", "cohort_granularity"},
		{"range_367", "from=2025-10-11&to=2026-10-12", "range_too_large", "to"},
		{"from_after_to", "from=2026-10-12&to=2026-10-06", "invalid_request", "from"},
		{"unit", "from=2026-10-06&to=2026-10-12&unit=people", "invalid_request", "unit"},
		{"country", "from=2026-10-06&to=2026-10-12&country=ru", "invalid_request", "country"},
		{"missing_to", "from=2026-10-06", "invalid_request", "to"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.get(t, retentionPath(msk, tc.query), e.all)
			body := errorBody(t, rec, http.StatusBadRequest)
			require.Equal(t, tc.code, body.Code)
			require.Equal(t, tc.field, body.Details["field"])
			for _, quoted := range []string{"quarter", "thirty", "people"} {
				require.NotContains(t, rec.Body.String(), quoted)
			}
		})
	}
}
