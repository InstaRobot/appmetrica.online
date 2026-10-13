//go:build integration

package auth_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/httpx"
)

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	httpx.WriteError(w, r, status, code, message, nil)
}

// protected — маршрут за RequireSession: отвечает id владельца из контекста.
func (h *harness) protected() http.Handler {
	return h.svc.RequireSession(writeError)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.OwnerIDFrom(r.Context())
		if !ok {
			http.Error(w, "no owner in context", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(id.String()))
	}))
}

func (h *harness) get(t *testing.T, handler http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/apps", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func (h *harness) ownerID(t *testing.T) string {
	t.Helper()
	var id string
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(), "SELECT id::text FROM owner").Scan(&id))
	return id
}

func (h *harness) lastSeen(t *testing.T, token string) time.Time {
	t.Helper()
	var ts time.Time
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(),
		"SELECT last_seen_at FROM auth_sessions WHERE token_hash = sha256(decode(translate($1, '-_', '+/') || '=', 'base64'))",
		token).Scan(&ts))
	return ts
}

func requireUnauthenticated(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"code":"unauthenticated"`)
}

func TestSession_IdleSliding(t *testing.T) {
	t.Run("request 6 days after the last one extends the session", func(t *testing.T) {
		h := newHarness(t)
		sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
		require.NoError(t, err)
		handler := h.protected()

		h.clock.Advance(6 * 24 * time.Hour)
		rec := h.get(t, handler, sess.Token)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, h.ownerID(t), rec.Body.String())
		require.True(t, h.lastSeen(t, sess.Token).Equal(h.clock.Now()), "last_seen_at сдвинут")

		h.clock.Advance(6 * 24 * time.Hour)
		require.Equal(t, http.StatusOK, h.get(t, handler, sess.Token).Code, "idle отсчитывается от последнего запроса")
		state, err := h.svc.State(t.Context(), sess.Token)
		require.NoError(t, err)
		require.True(t, state.Authenticated)
	})

	t.Run("8 days without requests", func(t *testing.T) {
		h := newHarness(t)
		sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
		require.NoError(t, err)

		h.clock.Advance(8 * 24 * time.Hour)
		requireUnauthenticated(t, h.get(t, h.protected(), sess.Token))
		state, err := h.svc.State(t.Context(), sess.Token)
		require.NoError(t, err)
		require.False(t, state.Authenticated)
	})

	t.Run("created 31 days ago with regular activity", func(t *testing.T) {
		h := newHarness(t)
		sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
		require.NoError(t, err)
		handler := h.protected()

		for _, days := range []int{5, 5, 5, 5, 5, 4} {
			h.clock.Advance(time.Duration(days) * 24 * time.Hour)
			require.Equal(t, http.StatusOK, h.get(t, handler, sess.Token).Code, "%v", h.clock.Now())
		}
		h.clock.Advance(2 * 24 * time.Hour) // 31-й день: абсолютный срок 30 дней истёк, хотя idle не превышен
		requireUnauthenticated(t, h.get(t, handler, sess.Token))
	})
}

func TestSession_SlidingAtMostEveryFiveMinutes(t *testing.T) {
	h := newHarness(t)
	sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
	require.NoError(t, err)
	handler := h.protected()
	created := h.clock.Now()

	h.clock.Advance(5 * time.Minute)
	require.Equal(t, http.StatusOK, h.get(t, handler, sess.Token).Code)
	require.True(t, h.lastSeen(t, sess.Token).Equal(created), "ровно 5 минут — без записи")

	h.clock.Advance(time.Second)
	require.Equal(t, http.StatusOK, h.get(t, handler, sess.Token).Code)
	require.True(t, h.lastSeen(t, sess.Token).Equal(h.clock.Now()))
}

func TestSession_IdleTimeoutFromConfig(t *testing.T) {
	h := newHarness(t)
	cfg := testConfig()
	cfg.SessionIdleTTL = time.Hour
	svc, err := auth.NewService(h.pg, h.clock, slog.New(slog.DiscardHandler), cfg)
	require.NoError(t, err)
	h.svc = svc
	sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
	require.NoError(t, err)

	h.clock.Advance(59 * time.Minute)
	require.Equal(t, http.StatusOK, h.get(t, h.protected(), sess.Token).Code)
	h.clock.Advance(61 * time.Minute)
	requireUnauthenticated(t, h.get(t, h.protected(), sess.Token))
}

func TestRequireSession_MissingOrInvalidCookie(t *testing.T) {
	h := newHarness(t)
	_, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
	require.NoError(t, err)

	for name, token := range map[string]string{
		"no cookie": "",
		"garbage":   "not-a-token",
		"unknown":   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		t.Run(name, func(t *testing.T) {
			requireUnauthenticated(t, h.get(t, h.protected(), token))
		})
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	h := newHarness(t)
	sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
	require.NoError(t, err)
	mux := http.NewServeMux()
	api.NewServer(h.svc, slog.New(slog.DiscardHandler), testConfig()).Register(mux)

	logout := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/v1/auth/logout", nil)
		req.Header.Set("X-Requested-With", "appmetrica-web")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	h.clock.Advance(time.Minute)
	rec := logout(sess.Token)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	cookie := rec.Result().Cookies()
	require.Len(t, cookie, 1)
	require.Equal(t, auth.SessionCookie, cookie[0].Name)
	require.Empty(t, cookie[0].Value)
	require.Equal(t, -1, cookie[0].MaxAge, "Max-Age=0")
	require.True(t, cookie[0].HttpOnly)
	require.Equal(t, "/", cookie[0].Path)

	var revoked time.Time
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(), "SELECT revoked_at FROM auth_sessions").Scan(&revoked))
	require.True(t, revoked.Equal(h.clock.Now()))

	requireUnauthenticated(t, h.get(t, h.protected(), sess.Token))
	state, err := h.svc.State(t.Context(), sess.Token)
	require.NoError(t, err)
	require.False(t, state.Authenticated)

	// Повторный выход и выход без cookie идемпотентны: 204, revoked_at не меняется.
	h.clock.Advance(time.Minute)
	require.Equal(t, http.StatusNoContent, logout(sess.Token).Code)
	require.Equal(t, http.StatusNoContent, logout("").Code)
	require.Equal(t, http.StatusNoContent, logout("garbage").Code)
	var again time.Time
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(), "SELECT revoked_at FROM auth_sessions").Scan(&again))
	require.True(t, again.Equal(revoked))
}
