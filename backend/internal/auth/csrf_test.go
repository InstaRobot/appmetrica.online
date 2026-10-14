package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/httpx"
)

const csrfBaseURL = "https://appmetrica.online"

type csrfRequest struct {
	method, path string
	header       map[string]string
}

// serveCSRF прогоняет запрос с cookie am_session через auth.CSRF; true — запрос дошёл до обработчика.
func serveCSRF(t *testing.T, in csrfRequest) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	h := auth.CSRF(csrfBaseURL, func(w http.ResponseWriter, r *http.Request, status int, code, message string) {
		httpx.WriteError(w, r, status, code, message, nil)
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequestWithContext(t.Context(), in.method, in.path, nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
	for k, v := range in.header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached
}

func requireCSRFRejected(t *testing.T, in csrfRequest) {
	t.Helper()
	rec, reached := serveCSRF(t, in)
	require.False(t, reached, "%s %s дошёл до обработчика", in.method, in.path)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"csrf_check_failed"`)
}

func requireCSRFPassed(t *testing.T, in csrfRequest) {
	t.Helper()
	rec, reached := serveCSRF(t, in)
	require.True(t, reached, "%s %s: %d %s", in.method, in.path, rec.Code, rec.Body)
}

func TestCSRF_AdminMutations(t *testing.T) {
	web := map[string]string{"X-Requested-With": "appmetrica-web"}

	requireCSRFRejected(t, csrfRequest{method: http.MethodPost, path: "/admin/v1/apps"})
	requireCSRFRejected(t, csrfRequest{method: http.MethodPost, path: "/admin/v1/apps", header: map[string]string{
		"X-Requested-With": "appmetrica-web", "Origin": "https://evil.example",
	}})
	requireCSRFPassed(t, csrfRequest{method: http.MethodGet, path: "/admin/v1/apps"})
	requireCSRFPassed(t, csrfRequest{method: http.MethodPost, path: "/admin/v1/apps", header: web})
	requireCSRFPassed(t, csrfRequest{method: http.MethodPost, path: "/admin/v1/apps", header: map[string]string{
		"X-Requested-With": "appmetrica-web", "Origin": csrfBaseURL,
	}})
}

func TestCSRF_Rules(t *testing.T) {
	rejected := map[string]csrfRequest{
		"patch without header":  {method: http.MethodPatch, path: "/admin/v1/passkeys/1"},
		"delete without header": {method: http.MethodDelete, path: "/admin/v1/passkeys/1"},
		"put without header":    {method: http.MethodPut, path: "/admin/v1/settings"},
		"login without header":  {method: http.MethodPost, path: "/admin/v1/auth/login/begin"},
		"logout without header": {method: http.MethodPost, path: "/admin/v1/auth/logout"},
		"wrong header value": {method: http.MethodPost, path: "/admin/v1/apps", header: map[string]string{
			"X-Requested-With": "XMLHttpRequest",
		}},
		"null origin": {method: http.MethodPost, path: "/admin/v1/apps", header: map[string]string{
			"X-Requested-With": "appmetrica-web", "Origin": "null",
		}},
		"origin with other scheme": {method: http.MethodPost, path: "/admin/v1/apps", header: map[string]string{
			"X-Requested-With": "appmetrica-web", "Origin": "http://appmetrica.online",
		}},
		"cookie deletion request without header": {method: http.MethodPost, path: "/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f/deletion-requests"},
	}
	for name, in := range rejected {
		t.Run(name, func(t *testing.T) { requireCSRFRejected(t, in) })
	}

	passed := map[string]csrfRequest{
		"head":         {method: http.MethodHead, path: "/admin/v1/apps"},
		"options":      {method: http.MethodOptions, path: "/admin/v1/apps"},
		"read api get": {method: http.MethodGet, path: "/v1/apps"},
		"health":       {method: http.MethodPost, path: "/health"},
		"bearer deletion request": {method: http.MethodPost, path: "/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f/deletion-requests", header: map[string]string{
			"Authorization": "Bearer sk_test",
		}},
	}
	for name, in := range passed {
		t.Run(name, func(t *testing.T) { requireCSRFPassed(t, in) })
	}
}

// Два заголовка Origin, один из них свой, — отказ: принимается ровно одно значение.
func TestCSRF_MultipleOriginHeaders(t *testing.T) {
	reached := false
	h := auth.CSRF(csrfBaseURL, func(w http.ResponseWriter, r *http.Request, status int, code, message string) {
		httpx.WriteError(w, r, status, code, message, nil)
	})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/v1/apps", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
	req.Header.Set("X-Requested-With", "appmetrica-web")
	req.Header.Add("Origin", csrfBaseURL)
	req.Header.Add("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.False(t, reached)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"csrf_check_failed"`)
}
