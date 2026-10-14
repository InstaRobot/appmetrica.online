//go:build integration

package api_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

// ownerCookie — сессия владельца, записанная прямо в Postgres: тестам Read API не нужна WebAuthn-церемония.
func ownerCookie(t *testing.T, e *portfolioEnv) *http.Cookie {
	t.Helper()
	handle := make([]byte, 64)
	raw := make([]byte, 32)
	_, _ = rand.Read(handle)
	_, _ = rand.Read(raw)
	var ownerID uuid.UUID
	require.NoError(t, e.env.PG.Pool().QueryRow(t.Context(),
		`INSERT INTO owner (webauthn_user_handle) VALUES ($1) RETURNING id`, handle).Scan(&ownerID))
	hash := sha256.Sum256(raw)
	now := e.clock.Now()
	_, err := e.env.PG.Pool().Exec(t.Context(),
		`INSERT INTO auth_sessions (token_hash, owner_id, created_at, last_seen_at, expires_at) VALUES ($1, $2, $3, $3, $4)`,
		hash[:], ownerID, now, now.Add(e.cfg.SessionTTL))
	require.NoError(t, err)
	return &http.Cookie{Name: auth.SessionCookie, Value: base64.RawURLEncoding.EncodeToString(raw)}
}

func serveRequest(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// requireEnvelope — конверт Error с кодом code и request_id, совпадающим с X-Request-Id.
func requireEnvelope(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	body := errorBody(t, rec, status)
	require.Equal(t, code, body.Code)
	require.NotEmpty(t, body.Message)
	require.True(t, httpx.ValidULID(body.RequestID), body.RequestID)
	require.Equal(t, rec.Header().Get(httpx.HeaderRequestID), body.RequestID)
}

func TestReadAPI_Unauthenticated(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	msk := testutil.ScenarioApps[0]

	cases := []struct {
		name   string
		path   string
		header map[string]string
		cookie *http.Cookie
	}{
		{name: "no_credentials", path: "/v1/apps"},
		{name: "unknown_bearer", path: "/v1/apps", header: map[string]string{"Authorization": "Bearer sk_" + "0123456789abcdefghij0123456789abcdefghij"}},
		{name: "not_bearer_scheme", path: "/v1/apps", header: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"}},
		{name: "empty_authorization", path: "/v1/apps", header: map[string]string{"Authorization": ""}},
		{name: "unknown_cookie", path: "/v1/apps", cookie: &http.Cookie{Name: auth.SessionCookie, Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}},
		{name: "malformed_cookie", path: "/v1/apps", cookie: &http.Cookie{Name: auth.SessionCookie, Value: "not-a-token"}},
		{name: "report_without_credentials", path: audiencePath(msk, "summary", "from=2026-10-01&to=2026-10-12")},
		{name: "portfolio_without_credentials", path: "/v1/portfolio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rec := serveRequest(t, e.handler, req)
			requireEnvelope(t, rec, http.StatusUnauthorized, "unauthenticated")
			validateResponse(t, doc, req, rec)
		})
	}
}

func TestReadAPI_SameResultForCookieAndBearer(t *testing.T) {
	e := newPortfolioEnv(t)
	cookie := ownerCookie(t, e)
	msk := testutil.ScenarioApps[0]

	for _, path := range []string{
		"/v1/portfolio?date=2026-10-12",
		"/v1/portfolio?date=2026-10-12&unit=installs&include_archived=true",
		"/v1/apps",
		audiencePath(msk, "summary", "from=2026-10-01&to=2026-10-12&compare=previous_period"),
	} {
		t.Run(path, func(t *testing.T) {
			byBearer := e.get(t, path, e.all)
			require.Equal(t, http.StatusOK, byBearer.Code, byBearer.Body.String())

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			req.AddCookie(cookie)
			byCookie := serveRequest(t, e.handler, req)
			require.Equal(t, http.StatusOK, byCookie.Code, byCookie.Body.String())

			require.Equal(t, byBearer.Body.String(), byCookie.Body.String())
			require.Equal(t, byBearer.Header().Get("Content-Type"), byCookie.Header().Get("Content-Type"))
		})
	}
}

func TestReadAPI_StorageUnavailable(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	down, err := chstore.Open(t.Context(), e.env.ClickHouse)
	require.NoError(t, err)
	require.NoError(t, down.Close())
	h := e.newHandler(t, reports.NewService(down.Conn(), e.clock), down, prometheus.NewRegistry())
	msk := testutil.ScenarioApps[0]

	for _, path := range []string{
		"/v1/portfolio",
		"/v1/portfolio?date=2026-10-12",
		audiencePath(msk, "summary", "from=2026-10-01&to=2026-10-12"),
		retentionPath(msk, "from=2026-09-14&to=2026-10-12"),
		realtimePath(msk, ""),
		integrationPath(msk.ID, ""),
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+e.all)
			rec := serveRequest(t, h, req)
			requireEnvelope(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
			require.Equal(t, "5", rec.Header().Get("Retry-After"))
			validateResponse(t, doc, req, rec)
		})
	}

	// Postgres жив: список приложений ClickHouse не читает.
	t.Run("apps_without_clickhouse", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/apps", nil)
		req.Header.Set("Authorization", "Bearer "+e.all)
		require.Equal(t, http.StatusOK, serveRequest(t, h, req).Code)
	})

	t.Run("ready", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", nil)
		rec := serveRequest(t, h, req)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.JSONEq(t, `{"status":"not_ready","checks":{"postgres":"ok","clickhouse":"fail"}}`, rec.Body.String())
		validateResponse(t, doc, req, rec)
	})
}

// api_request_duration_seconds считает операции по operationId и статусу; отказ аутентификации до операции
// в метрику не попадает.
func TestReadAPI_RequestMetrics(t *testing.T) {
	e := newPortfolioEnv(t)
	msk := testutil.ScenarioApps[0]

	require.Equal(t, http.StatusOK, e.get(t, "/v1/portfolio?date=2026-10-12", e.all).Code)
	require.Equal(t, http.StatusOK, e.get(t, "/v1/portfolio?date=2026-10-12", e.all).Code)
	require.Equal(t, http.StatusBadRequest, e.get(t, retentionPath(msk, "from=2026-10-12&to=2026-10-01"), e.all).Code)
	require.Equal(t, http.StatusUnauthorized, e.get(t, "/v1/apps", "sk_unknown").Code)

	families, err := e.metrics.Gather()
	require.NoError(t, err)
	counts := map[string]uint64{}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			counts[labels["operation"]+"/"+labels["status"]] = m.GetHistogram().GetSampleCount()
		}
	}
	require.Equal(t, map[string]uint64{"getPortfolio/200": 2, "getRetention/400": 1}, counts)
}
