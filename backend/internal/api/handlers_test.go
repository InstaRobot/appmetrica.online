//go:build integration

package api_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
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

// AC-002.4: Read API отдаёт набор платформ и устаревшее platform = platforms[0].
func TestAppModelPlatforms(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	id := uuid.Must(uuid.NewV7())
	_, err := e.env.PG.Pool().Exec(t.Context(),
		`INSERT INTO apps (id, name, platform, platforms, timezone) VALUES ($1, 'Notes', 'ios', '{ios,web}', 'Europe/Moscow')`, id)
	require.NoError(t, err)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/apps/"+id.String(), nil)
	req.Header.Set("Authorization", "Bearer "+e.newKey(t))
	rec := serveRequest(t, e.handler, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "getApp", validateResponse(t, doc, req, rec))
	var app gen.App
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))
	require.Equal(t, gen.PlatformList{gen.PlatformIos, gen.PlatformWeb}, app.Platforms)
	require.Equal(t, gen.PlatformIos, app.Platform)

	// В списке приложений — те же поля.
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+e.newKey(t))
	rec = serveRequest(t, e.handler, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	validateResponse(t, doc, req, rec)
	var list gen.AppList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	var found bool
	for _, a := range list.Items {
		require.NotEmpty(t, a.Platforms, a.Name)
		require.Equal(t, a.Platforms[0], a.Platform, a.Name)
		if a.Id == id {
			found = true
			require.Equal(t, app, a)
		}
	}
	require.True(t, found)
}

// AC-003.2: неизвестная платформа ключа — 400 invalid_request с полем platform, ключ не создаётся.
func TestCreateWriteKeyRejectsUnknownPlatform(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	owner := asOwner(ownerCookie(t, e))
	path := "/admin/v1/apps/" + testutil.ScenarioApps[0].ID.String() + "/write-keys"

	rec := e.call(t, http.MethodPost, path, map[string]any{"label": "CI", "platform": "symbian"}, owner)
	body := errorBody(t, rec, http.StatusBadRequest)
	require.Equal(t, "invalid_request", body.Code)
	fields, ok := body.Details["fields"].(map[string]any)
	require.True(t, ok, rec.Body.String())
	require.Contains(t, fields, "platform")

	rec = e.call(t, http.MethodGet, path, nil, owner)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list gen.WriteKeyList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Empty(t, list.Items)

	// Известная платформа принимается, ответ валиден по контракту.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(`{"label":"CI","platform":"web"}`))
	req.Header.Set("Content-Type", "application/json")
	owner(req)
	rec = serveRequest(t, e.handler, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, "createWriteKey", validateResponse(t, doc, req, rec))
	var key gen.WriteKey
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &key))
	platform, err := key.Platform.Get()
	require.NoError(t, err)
	require.Equal(t, gen.PlatformWeb, platform)
}

// AC-1.4: набор платформ меняет только владелец — без cookie 401, без X-Requested-With 403; набор не меняется.
func TestUpdateAppPlatformsRequiresSession(t *testing.T) {
	e := newPortfolioEnv(t)
	cookie := ownerCookie(t, e)
	id := testutil.ScenarioApps[0].ID
	path := "/admin/v1/apps/" + id.String()
	body := map[string]any{"platforms": []string{"web"}}
	platforms := func() []string {
		var got []string
		require.NoError(t, e.env.PG.Pool().QueryRow(t.Context(),
			`SELECT platforms::text[] FROM apps WHERE id = $1`, id).Scan(&got))
		return got
	}
	before := platforms()
	require.NotEqual(t, []string{"web"}, before)

	anonymous := func(r *http.Request) { r.Header.Set("X-Requested-With", auth.RequestedWith) }
	requireEnvelope(t, e.call(t, http.MethodPatch, path, body, anonymous), http.StatusUnauthorized, "unauthenticated")
	noCSRF := func(r *http.Request) { r.AddCookie(cookie) }
	requireEnvelope(t, e.call(t, http.MethodPatch, path, body, noCSRF), http.StatusForbidden, "csrf_check_failed")
	require.Equal(t, before, platforms())

	// С сессией и заголовком тот же запрос проходит.
	rec := e.call(t, http.MethodPatch, path, body, asOwner(cookie))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{"web"}, platforms())
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
// в метрику не попадает. api_clickhouse_query_duration_seconds считает отчёты, дошедшие до ClickHouse:
// запрос с ошибкой параметров отклоняется раньше.
func TestReadAPI_RequestMetrics(t *testing.T) {
	e := newPortfolioEnv(t)
	msk := testutil.ScenarioApps[0]

	require.Equal(t, http.StatusOK, e.get(t, "/v1/portfolio?date=2026-10-12", e.all).Code)
	require.Equal(t, http.StatusOK, e.get(t, "/v1/portfolio?date=2026-10-12", e.all).Code)
	require.Equal(t, http.StatusOK, e.get(t, retentionPath(msk, "from=2026-10-01&to=2026-10-12"), e.all).Code)
	require.Equal(t, http.StatusBadRequest, e.get(t, retentionPath(msk, "from=2026-10-12&to=2026-10-01"), e.all).Code)
	require.Equal(t, http.StatusUnauthorized, e.get(t, "/v1/apps", "sk_unknown").Code)

	require.Equal(t, map[string]uint64{"getPortfolio/200": 2, "getRetention/200": 1, "getRetention/400": 1},
		histogramCounts(t, e.metrics, "api_request_duration_seconds", "operation", "status"))
	require.Equal(t, map[string]uint64{"portfolio": 2, "retention": 1},
		histogramCounts(t, e.metrics, "api_clickhouse_query_duration_seconds", "report"))
}

// histogramCounts — число наблюдений гистограммы name по значениям меток labels (через «/»).
func histogramCounts(t *testing.T, g prometheus.Gatherer, name string, labels ...string) map[string]uint64 {
	t.Helper()
	families, err := g.Gather()
	require.NoError(t, err)
	out := map[string]uint64{}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			values := map[string]string{}
			for _, l := range m.GetLabel() {
				values[l.GetName()] = l.GetValue()
			}
			key := make([]string, len(labels))
			for i, l := range labels {
				key[i] = values[l]
			}
			out[strings.Join(key, "/")] = m.GetHistogram().GetSampleCount()
		}
	}
	return out
}
