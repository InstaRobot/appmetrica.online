//go:build integration

package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

type portfolioEnv struct {
	env        *testutil.Env
	mux        *http.ServeMux
	reports    *reports.Service
	all        string // sk_ read на все приложения
	restricted string // sk_ read только на nyc
}

// newPortfolioEnv — Read API поверх сценария фикстур: msk и nyc активны, tyo в архиве;
// сейчас 2026-10-13 05:00 UTC (08:00 в Москве — owner_timezone по умолчанию).
func newPortfolioEnv(t *testing.T) *portfolioEnv {
	t.Helper()
	env := testutil.Start(t)
	env.LoadNDJSON(t, testutil.ScenarioPath())
	for _, app := range testutil.ScenarioApps {
		_, err := env.PG.Pool().Exec(t.Context(),
			`INSERT INTO apps (id, name, platform, timezone) VALUES ($1, $2, $3, $4)`,
			app.ID, app.Name, app.Platform, app.Timezone)
		require.NoError(t, err)
	}
	_, err := env.PG.Pool().Exec(t.Context(),
		`UPDATE apps SET status = 'archived', archived_at = now() WHERE id = $1`, testutil.ScenarioApps[2].ID)
	require.NoError(t, err)

	cfg := config.API{
		PublicBaseURL:       "http://localhost:3000",
		WebAuthnRPID:        "localhost",
		WebAuthnRPName:      "appmetrica.online",
		WebAuthnRPOrigins:   []string{"http://localhost:3000"},
		SessionTTL:          720 * time.Hour,
		SessionIdleTTL:      168 * time.Hour,
		KeyHashPepper:       []byte("test-pepper-0123456789abcdef-32b"),
		PublicBearerEnabled: true,
	}
	logger := slog.New(slog.DiscardHandler)
	clk := clock.NewFixed(time.Date(2026, 10, 13, 5, 0, 0, 0, time.UTC))
	authSvc, err := auth.NewService(env.PG, clk, logger, cfg)
	require.NoError(t, err)
	_, all, err := authSvc.CreateAPIKey(t.Context(), auth.CreateAPIKeyInput{Label: "all", Scopes: []string{auth.ScopeRead}})
	require.NoError(t, err)
	nyc := testutil.ScenarioApps[1].ID
	_, restricted, err := authSvc.CreateAPIKey(t.Context(), auth.CreateAPIKeyInput{
		Label: "nyc", Scopes: []string{auth.ScopeRead}, AppID: &nyc,
	})
	require.NoError(t, err)

	rep := reports.NewService(env.CH.Conn(), clk)
	mux := http.NewServeMux()
	api.NewServer(authSvc, apps.NewService(env.PG, clk, logger), logger, cfg).
		WithReports(rep, env.PG.Queries()).
		Register(mux)
	return &portfolioEnv{env: env, mux: mux, reports: rep, all: all, restricted: restricted}
}

func (e *portfolioEnv) get(t *testing.T, path, secret string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func (e *portfolioEnv) portfolio(t *testing.T, path, secret string) gen.Portfolio {
	t.Helper()
	rec := e.get(t, path, secret)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got gen.Portfolio
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

func appIDs(p gen.Portfolio) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(p.Rows))
	for _, r := range p.Rows {
		ids = append(ids, r.App.Id)
	}
	return ids
}

// expectedRows — строки отчёта reports по приложениям сценария с теми же параметрами, по app_id.
func (e *portfolioEnv) expectedRows(t *testing.T, day string, env string, unit identity.Unit) map[uuid.UUID]reports.PortfolioRow {
	t.Helper()
	d, err := time.Parse(time.DateOnly, day)
	require.NoError(t, err)
	q := reports.PortfolioQuery{Date: d, Env: env, Unit: unit}
	for _, app := range testutil.ScenarioApps {
		loc, err := time.LoadLocation(app.Timezone)
		require.NoError(t, err)
		q.Apps = append(q.Apps, reports.PortfolioApp{ID: app.ID, Location: loc})
	}
	p, err := e.reports.Portfolio(t.Context(), q)
	require.NoError(t, err)
	out := map[uuid.UUID]reports.PortfolioRow{}
	for _, r := range p.Rows {
		out[r.AppID] = r
	}
	return out
}

func requireRowMatches(t *testing.T, want reports.PortfolioRow, got gen.PortfolioRow) {
	t.Helper()
	require.Equal(t, want.AppID, got.App.Id)
	require.Equal(t, int64(want.ActiveToday), got.ActiveToday)
	require.Equal(t, int64(want.ActiveDate), got.ActiveDate)
	require.Equal(t, int64(want.ActiveSameDayPrevWeek), got.ActiveSameDayPrevWeek)
	require.Equal(t, int64(want.NewDate), got.NewDate)
	require.Equal(t, int64(want.SessionsDate), got.SessionsDate)
	for _, pair := range []struct {
		want *float64
		got  interface {
			IsNull() bool
			Get() (float32, error)
		}
	}{{want.ActiveDeltaPct, got.ActiveDeltaPct}, {want.RetentionD1, got.RetentionD1}, {want.RetentionD7, got.RetentionD7}} {
		if pair.want == nil {
			require.True(t, pair.got.IsNull())
			continue
		}
		v, err := pair.got.Get()
		require.NoError(t, err)
		require.InDelta(t, *pair.want, float64(v), 1e-6)
	}
	require.Len(t, got.Sparkline, 30)
	for i, p := range got.Sparkline {
		require.Equal(t, want.Sparkline[i].Day, p.Day.Format(time.DateOnly))
		require.Equal(t, int64(want.Sparkline[i].Active), p.Active)
	}
}

func TestGetPortfolio_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	msk, nyc, tyo := testutil.ScenarioApps[0], testutil.ScenarioApps[1], testutil.ScenarioApps[2]

	t.Run("defaults", func(t *testing.T) {
		got := e.portfolio(t, "/v1/portfolio", e.all)
		require.Equal(t, "2026-10-12", got.Date.Format(time.DateOnly), "вчера в owner_timezone (Europe/Moscow)")
		require.Equal(t, gen.EnvironmentProduction, got.Environment)
		require.Equal(t, gen.UnitPersons, got.Unit)
		require.ElementsMatch(t, []uuid.UUID{msk.ID, nyc.ID}, appIDs(got), "архивное скрыто")

		want := e.expectedRows(t, "2026-10-12", reports.EnvProduction, identity.Persons)
		for i, row := range got.Rows {
			requireRowMatches(t, want[row.App.Id], row)
			if i > 0 {
				require.GreaterOrEqual(t, got.Rows[i-1].ActiveDate, row.ActiveDate, "сортировка по active_date DESC")
			}
		}
		require.Equal(t, gen.AppSummary{Id: msk.ID, Name: msk.Name, Platform: gen.PlatformIos, Status: gen.AppStatusActive},
			got.Rows[slices.Index(appIDs(got), msk.ID)].App)
	})

	t.Run("explicit_params_and_archived", func(t *testing.T) {
		got := e.portfolio(t, "/v1/portfolio?date=2026-10-05&environment=development&unit=installs&include_archived=true", e.all)
		require.Equal(t, "2026-10-05", got.Date.Format(time.DateOnly))
		require.Equal(t, gen.EnvironmentDevelopment, got.Environment)
		require.Equal(t, gen.UnitInstalls, got.Unit)
		require.ElementsMatch(t, []uuid.UUID{msk.ID, nyc.ID, tyo.ID}, appIDs(got))
		want := e.expectedRows(t, "2026-10-05", reports.EnvDevelopment, identity.Installs)
		for _, row := range got.Rows {
			requireRowMatches(t, want[row.App.Id], row)
			if row.App.Id == tyo.ID {
				require.Equal(t, gen.AppStatusArchived, row.App.Status)
			}
		}
	})

	t.Run("restricted_key", func(t *testing.T) {
		got := e.portfolio(t, "/v1/portfolio?include_archived=true", e.restricted)
		require.Equal(t, []uuid.UUID{nyc.ID}, appIDs(got))
	})

	t.Run("invalid_params", func(t *testing.T) {
		for query, field := range map[string]string{
			"date=2026-13-01":     "date",
			"environment=staging": "environment",
			"unit=people":         "unit",
		} {
			rec := e.get(t, "/v1/portfolio?"+query, e.all)
			require.Equal(t, http.StatusBadRequest, rec.Code, query)
			require.JSONEq(t, `{"field":"`+field+`"}`, string(mustDetails(t, rec)), query)
		}
		require.Equal(t, http.StatusUnauthorized, e.get(t, "/v1/portfolio", "sk_wrong").Code)
	})

	// Дата по умолчанию следует owner_timezone: в Лос-Анджелесе ещё 12-е, вчера — 11-е.
	t.Run("owner_timezone", func(t *testing.T) {
		_, err := e.env.PG.Pool().Exec(t.Context(), `UPDATE settings SET owner_timezone = 'America/Los_Angeles'`)
		require.NoError(t, err)
		got := e.portfolio(t, "/v1/portfolio", e.all)
		require.Equal(t, "2026-10-11", got.Date.Format(time.DateOnly))
	})
}

func mustDetails(t *testing.T, rec *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	var body struct {
		Error struct {
			Code    string          `json:"code"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "invalid_request", body.Error.Code)
	return body.Error.Details
}
