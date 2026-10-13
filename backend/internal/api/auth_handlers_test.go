//go:build integration

package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

type authServer struct {
	srv  *httptest.Server
	svc  *auth.Service
	cfg  config.API
	pg   *pgstore.Store
	rpID string
}

func newAuthServer(t *testing.T, baseURL, rpID string) *authServer {
	t.Helper()
	pg, err := pgstore.Open(t.Context(), testutil.NewPostgresDB(t, true))
	require.NoError(t, err)
	t.Cleanup(pg.Close)

	cfg := config.API{
		PublicBaseURL:     baseURL,
		WebAuthnRPID:      rpID,
		WebAuthnRPName:    "appmetrica.online",
		WebAuthnRPOrigins: []string{baseURL},
		SessionTTL:        720 * time.Hour,
		SessionIdleTTL:    168 * time.Hour,
	}
	logger := slog.New(slog.DiscardHandler)
	c := clock.NewFixed(time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC))
	svc, err := auth.NewService(pg, c, logger, cfg)
	require.NoError(t, err)

	mux := http.NewServeMux()
	api.NewServer(svc, nil, logger, cfg).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &authServer{srv: srv, svc: svc, cfg: cfg, pg: pg, rpID: rpID}
}

func (s *authServer) token(t *testing.T) string {
	t.Helper()
	link, err := s.svc.CreatePasskeyLink(t.Context())
	require.NoError(t, err)
	token, ok := strings.CutPrefix(link, s.cfg.PublicBaseURL+"/setup/")
	require.True(t, ok, link)
	return token
}

// response — прочитанный ответ: тело закрыто внутри do.
type response struct {
	StatusCode int
	Header     http.Header
	cookies    []*http.Cookie
}

func (r *response) Cookies() []*http.Cookie { return r.cookies }

func (s *authServer) do(t *testing.T, method, path string, body any, cookies ...*http.Cookie) (*response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, ok := body.([]byte)
		if !ok {
			var err error
			b, err = json.Marshal(body)
			require.NoError(t, err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, s.srv.URL+path, r)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Requested-With", "appmetrica-web")
	req.Header.Set("User-Agent", "Mozilla/5.0 e2e")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := s.srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return &response{StatusCode: resp.StatusCode, Header: resp.Header, cookies: resp.Cookies()}, out
}

type ceremonyResp struct {
	CeremonyID string          `json:"ceremony_id"`
	Options    json.RawMessage `json:"options"`
}

func (s *authServer) begin(t *testing.T, token string) ceremonyResp {
	t.Helper()
	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/begin", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var c ceremonyResp
	require.NoError(t, json.Unmarshal(body, &c))
	return c
}

func requireError(t *testing.T, resp *response, body []byte, status int, code string) map[string]any {
	t.Helper()
	require.Equal(t, status, resp.StatusCode, string(body))
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &env), string(body))
	require.Equal(t, code, env.Error.Code)
	require.NotEmpty(t, env.Error.Message)
	return env.Error.Details
}

func sessionCookie(t *testing.T, resp *response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == "am_session" {
			return c
		}
	}
	t.Fatalf("нет cookie am_session: %v", resp.Header.Values("Set-Cookie"))
	return nil
}

func TestAdminAuth_SetupFlow(t *testing.T) {
	s := newAuthServer(t, "http://localhost:3000", "localhost")

	resp, body := s.do(t, http.MethodGet, "/admin/v1/auth/state", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"authenticated":false,"setup_required":true}`, string(body))

	token := s.token(t)
	c := s.begin(t, token)
	require.Len(t, c.CeremonyID, 36)
	var opts map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(c.Options, &opts))
	require.Contains(t, opts, "publicKey")

	a := testutil.NewAuthenticator(t, "localhost", "http://localhost:3000")
	resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", map[string]any{
		"ceremony_id": c.CeremonyID,
		"credential":  json.RawMessage(a.Register(t, c.Options)),
		"name":        "MacBook",
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	cookie := sessionCookie(t, resp)
	require.Len(t, cookie.Value, 43)
	require.Equal(t, "/", cookie.Path)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.Equal(t, 2592000, cookie.MaxAge)
	require.False(t, cookie.Secure, "http-окружение разработки")

	var ua string
	require.NoError(t, s.pg.Pool().QueryRow(t.Context(), "SELECT user_agent FROM auth_sessions").Scan(&ua))
	require.Equal(t, "Mozilla/5.0 e2e", ua)

	resp, body = s.do(t, http.MethodGet, "/admin/v1/auth/state", nil, cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"authenticated":true,"setup_required":false}`, string(body))

	resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/begin", nil)
	requireError(t, resp, body, http.StatusGone, "setup_token_expired")
}

func TestAdminAuth_SecureCookieOverHTTPS(t *testing.T) {
	s := newAuthServer(t, "https://appmetrica.online", "appmetrica.online")
	token := s.token(t)
	c := s.begin(t, token)

	a := testutil.NewAuthenticator(t, "appmetrica.online", "https://appmetrica.online")
	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", map[string]any{
		"ceremony_id": c.CeremonyID,
		"credential":  json.RawMessage(a.Register(t, c.Options)),
		"name":        "MacBook",
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	require.True(t, sessionCookie(t, resp).Secure)
}

func TestAdminAuth_SetupErrors(t *testing.T) {
	s := newAuthServer(t, "http://localhost:3000", "localhost")
	a := testutil.NewAuthenticator(t, "localhost", "http://localhost:3000")

	t.Run("unknown token", func(t *testing.T) {
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+strings.Repeat("A", 43)+"/begin", nil)
		requireError(t, resp, body, http.StatusNotFound, "not_found")
		resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+strings.Repeat("A", 43)+"/finish", map[string]any{
			"ceremony_id": "0199d6b0-0c11-7a2b-9c3d-4e5f6a7b8c9d", "credential": map[string]any{}, "name": "MacBook",
		})
		requireError(t, resp, body, http.StatusNotFound, "not_found")
	})

	t.Run("name too long", func(t *testing.T) {
		token := s.token(t)
		c := s.begin(t, token)
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", map[string]any{
			"ceremony_id": c.CeremonyID, "credential": json.RawMessage(a.Register(t, c.Options)), "name": strings.Repeat("x", 65),
		})
		details := requireError(t, resp, body, http.StatusBadRequest, "invalid_request")
		require.Contains(t, details["fields"], "name")
	})

	t.Run("malformed body", func(t *testing.T) {
		token := s.token(t)
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", []byte(`{"ceremony_id":`))
		requireError(t, resp, body, http.StatusBadRequest, "invalid_request")
	})

	t.Run("foreign ceremony", func(t *testing.T) {
		tokenA, tokenB := s.token(t), s.token(t)
		c := s.begin(t, tokenA)
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+tokenB+"/finish", map[string]any{
			"ceremony_id": c.CeremonyID, "credential": json.RawMessage(a.Register(t, c.Options)), "name": "MacBook",
		})
		requireError(t, resp, body, http.StatusUnprocessableEntity, "webauthn_verification_failed")
		require.Empty(t, resp.Header.Values("Set-Cookie"))
	})

	t.Run("expired token", func(t *testing.T) {
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+s.tokenUsed(t)+"/begin", nil)
		requireError(t, resp, body, http.StatusGone, "setup_token_expired")
	})
}

// tokenUsed выпускает ссылку и помечает её использованной.
func (s *authServer) tokenUsed(t *testing.T) string {
	t.Helper()
	token := s.token(t)
	_, err := s.pg.Pool().Exec(t.Context(), "UPDATE passkey_setup_tokens SET used_at = created_at WHERE used_at IS NULL")
	require.NoError(t, err)
	return token
}

// login проходит beginLogin → assertion → finishLogin по HTTP.
func (s *authServer) login(t *testing.T, a *testutil.Authenticator) (*response, []byte) {
	t.Helper()
	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var c ceremonyResp
	require.NoError(t, json.Unmarshal(body, &c))
	return s.do(t, http.MethodPost, "/admin/v1/auth/login/finish", map[string]any{
		"ceremony_id": c.CeremonyID,
		"credential":  json.RawMessage(a.Login(t, c.Options)),
	})
}

func (s *authServer) setupOwner(t *testing.T) (*testutil.Authenticator, *http.Cookie) {
	t.Helper()
	token := s.token(t)
	c := s.begin(t, token)
	a := testutil.NewAuthenticator(t, s.rpID, s.cfg.PublicBaseURL)
	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", map[string]any{
		"ceremony_id": c.CeremonyID, "credential": json.RawMessage(a.Register(t, c.Options)), "name": "MacBook",
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	return a, sessionCookie(t, resp)
}

func TestAdminAuth_LoginLogoutFlow(t *testing.T) {
	s := newAuthServer(t, "http://localhost:3000", "localhost")
	a, setupCookie := s.setupOwner(t)

	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/logout", nil, setupCookie)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	cleared := sessionCookie(t, resp)
	require.Empty(t, cleared.Value)
	require.Equal(t, -1, cleared.MaxAge)

	resp, body = s.do(t, http.MethodGet, "/admin/v1/auth/state", nil, setupCookie)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"authenticated":false,"setup_required":false}`, string(body))

	resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var c struct {
		CeremonyID string `json:"ceremony_id"`
		Options    struct {
			PublicKey map[string]any `json:"publicKey"`
			Mediation string         `json:"mediation"`
		} `json:"options"`
	}
	require.NoError(t, json.Unmarshal(body, &c))
	require.Equal(t, "conditional", c.Options.Mediation)
	require.Equal(t, "localhost", c.Options.PublicKey["rpId"])

	resp, body = s.login(t, a)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	cookie := sessionCookie(t, resp)
	require.Len(t, cookie.Value, 43)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, 2592000, cookie.MaxAge)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)

	resp, body = s.do(t, http.MethodGet, "/admin/v1/auth/state", nil, cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"authenticated":true,"setup_required":false}`, string(body))
}

func TestAdminAuth_LoginErrors(t *testing.T) {
	s := newAuthServer(t, "http://localhost:3000", "localhost")
	a, _ := s.setupOwner(t)

	t.Run("unknown ceremony", func(t *testing.T) {
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/login/finish", map[string]any{
			"ceremony_id": "0199d6b0-0c11-7a2b-9c3d-4e5f6a7b8c9d", "credential": map[string]any{},
		})
		requireError(t, resp, body, http.StatusNotFound, "not_found")
	})

	t.Run("unknown credential", func(t *testing.T) {
		stranger := testutil.NewAuthenticator(t, "localhost", "http://localhost:3000")
		stranger.UserHandle = a.UserHandle
		resp, body := s.login(t, stranger)
		requireError(t, resp, body, http.StatusUnprocessableEntity, "webauthn_verification_failed")
		require.Empty(t, resp.Header.Values("Set-Cookie"))
	})

	t.Run("malformed body", func(t *testing.T) {
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/login/finish", []byte(`{"ceremony_id":`))
		requireError(t, resp, body, http.StatusBadRequest, "invalid_request")
	})
}

// doRaw — запрос без заголовка X-Requested-With, с произвольными заголовками.
func (s *authServer) doRaw(t *testing.T, method, path string, header map[string]string, cookies ...*http.Cookie) (*response, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, s.srv.URL+path, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := s.srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return &response{StatusCode: resp.StatusCode, Header: resp.Header, cookies: resp.Cookies()}, out
}

func TestAdminAuth_CSRF(t *testing.T) {
	s := newAuthServer(t, "http://localhost:3000", "localhost")
	_, cookie := s.setupOwner(t)

	resp, body := s.doRaw(t, http.MethodPost, "/admin/v1/auth/logout", nil, cookie)
	requireError(t, resp, body, http.StatusForbidden, "csrf_check_failed")
	resp, body = s.doRaw(t, http.MethodPost, "/admin/v1/auth/logout", map[string]string{
		"X-Requested-With": "appmetrica-web", "Origin": "https://evil.example",
	}, cookie)
	requireError(t, resp, body, http.StatusForbidden, "csrf_check_failed")
	resp, body = s.doRaw(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
	requireError(t, resp, body, http.StatusForbidden, "csrf_check_failed")

	resp, body = s.doRaw(t, http.MethodGet, "/admin/v1/auth/state", nil, cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"authenticated":true,"setup_required":false}`, string(body), "выход без заголовка не сработал")

	resp, body = s.doRaw(t, http.MethodPost, "/admin/v1/auth/logout", map[string]string{
		"X-Requested-With": "appmetrica-web", "Origin": "http://localhost:3000",
	}, cookie)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
}
