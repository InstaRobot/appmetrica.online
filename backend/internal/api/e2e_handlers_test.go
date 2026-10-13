//go:build integration

package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func newE2EServer(t *testing.T) *authServer {
	t.Helper()
	pg, err := pgstore.Open(t.Context(), testutil.NewPostgresDB(t, true))
	require.NoError(t, err)
	t.Cleanup(pg.Close)

	cfg := config.API{
		Common:            config.Common{AppEnv: config.EnvE2E},
		PublicBaseURL:     "http://localhost:3000",
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "appmetrica.online",
		WebAuthnRPOrigins: []string{"http://localhost:3000"},
		SessionTTL:        720 * time.Hour,
		SessionIdleTTL:    168 * time.Hour,
	}
	logger := slog.New(slog.DiscardHandler)
	svc, err := auth.NewService(pg, clock.NewFixed(time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)), logger, cfg)
	require.NoError(t, err)

	mux := http.NewServeMux()
	api.NewServer(svc, logger, cfg).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &authServer{srv: srv, svc: svc, cfg: cfg, pg: pg, rpID: "localhost"}
}

func (s *authServer) e2eLink(t *testing.T) string {
	t.Helper()
	resp, body := s.do(t, http.MethodPost, "/internal/e2e/passkey-link", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	var out struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	token, ok := strings.CutPrefix(out.URL, "http://localhost:3000/setup/")
	require.True(t, ok, out.URL)
	return token
}

func (s *authServer) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, s.pg.Pool().QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
	return n
}

func TestE2ERoutes_PasskeyLinkStoresOnlyHash(t *testing.T) {
	s := newE2EServer(t)

	token := s.e2eLink(t)
	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	require.Len(t, raw, 32)

	var hash []byte
	require.NoError(t, s.pg.Pool().QueryRow(t.Context(), "SELECT token_hash FROM passkey_setup_tokens").Scan(&hash))
	sum := sha256.Sum256(raw)
	require.Equal(t, sum[:], hash)

	// Ссылка рабочая: по ней начинается церемония регистрации.
	c := s.begin(t, token)
	require.NotEmpty(t, c.CeremonyID)
}

func TestE2ERoutes_ResetAuthEmptiesOwnerState(t *testing.T) {
	s := newE2EServer(t)

	token := s.e2eLink(t)
	c := s.begin(t, token)
	a := testutil.NewAuthenticator(t, "localhost", "http://localhost:3000")
	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", map[string]any{
		"ceremony_id": c.CeremonyID,
		"credential":  json.RawMessage(a.Register(t, c.Options)),
		"name":        "MacBook",
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	cookie := sessionCookie(t, resp)
	_ = s.e2eLink(t) // неиспользованная ссылка тоже удаляется
	resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	resp, body = s.do(t, http.MethodPost, "/internal/e2e/reset-auth", nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	require.Empty(t, body)

	for _, table := range []string{"owner", "passkeys", "auth_sessions", "passkey_setup_tokens", "webauthn_ceremonies"} {
		require.Zero(t, s.count(t, table), table)
	}
	resp, body = s.do(t, http.MethodGet, "/admin/v1/auth/state", nil, cookie)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"authenticated":false,"setup_required":true}`, string(body))

	// Повторный сброс пустой базы — не ошибка; после сброса снова можно пройти первичную настройку.
	resp, _ = s.do(t, http.MethodPost, "/internal/e2e/reset-auth", nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	token = s.e2eLink(t)
	c = s.begin(t, token)
	resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/setup/"+token+"/finish", map[string]any{
		"ceremony_id": c.CeremonyID,
		"credential":  json.RawMessage(testutil.NewAuthenticator(t, "localhost", "http://localhost:3000").Register(t, c.Options)),
		"name":        "MacBook",
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))
	require.Equal(t, 1, s.count(t, "passkeys"))
}

// Лимит попыток входа общий на процесс: без сброса повторный прогон Playwright в ту же минуту получал бы 429.
func TestE2ERoutes_ResetAuthRefillsAttemptLimit(t *testing.T) {
	s := newE2EServer(t)

	for range 20 {
		resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	}
	resp, body := s.do(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
	requireError(t, resp, body, http.StatusTooManyRequests, "rate_limited")

	resp, _ = s.do(t, http.MethodPost, "/internal/e2e/reset-auth", nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp, body = s.do(t, http.MethodPost, "/admin/v1/auth/login/begin", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
}
