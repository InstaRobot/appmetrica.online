//go:build integration

package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/testutil"
)

// registerOwner проходит первичную настройку: owner с одним passkey на аутентификаторе a.
func (h *harness) registerOwner(t *testing.T) *testutil.Authenticator {
	t.Helper()
	a := newAuthenticator(t)
	_, err := h.setup(t, h.newToken(t), a, "MacBook")
	require.NoError(t, err)
	return a
}

// login проходит beginLogin → assertion на аутентификаторе → finishLogin.
func (h *harness) login(t *testing.T, a *testutil.Authenticator) (auth.Session, error) {
	t.Helper()
	c, err := h.svc.BeginLogin(t.Context())
	require.NoError(t, err)
	return h.svc.FinishLogin(t.Context(), auth.FinishLoginInput{
		CeremonyID: c.ID, Credential: a.Login(t, c.Options), UserAgent: "Mozilla/5.0 login",
	})
}

func (h *harness) signCount(t *testing.T, a *testutil.Authenticator) int64 {
	t.Helper()
	var n int64
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(),
		"SELECT sign_count FROM passkeys WHERE credential_id = $1", a.CredentialID).Scan(&n))
	return n
}

func TestBeginLogin_DiscoverableConditional(t *testing.T) {
	h := newHarness(t)
	h.registerOwner(t)

	c, err := h.svc.BeginLogin(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, c.ID)
	o := testutil.ParseRequestOptions(t, c.Options)
	require.NotEmpty(t, o.PublicKey.Challenge)
	require.Equal(t, testRPID, o.PublicKey.RPID)
	require.Equal(t, "required", o.PublicKey.UserVerification)
	require.Empty(t, o.PublicKey.AllowCredentials, "discoverable login: браузер сам предлагает passkey")
	require.Equal(t, 300000, o.PublicKey.Timeout)
	require.Equal(t, "conditional", o.Mediation)

	var kind string
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(),
		"SELECT kind FROM webauthn_ceremonies WHERE id = $1 AND setup_token_hash IS NULL", c.ID).Scan(&kind))
	require.Equal(t, "login", kind)
}

func TestFinishLogin_CreatesSessionAndUpdatesPasskey(t *testing.T) {
	h := newHarness(t)
	a := h.registerOwner(t)
	h.clock.Advance(time.Hour)

	a.SignCount = 7
	sess, err := h.login(t, a)
	require.NoError(t, err)
	require.Len(t, sess.Token, 43)
	require.Equal(t, h.clock.Now().Add(720*time.Hour), sess.ExpiresAt)

	state, err := h.svc.State(t.Context(), sess.Token)
	require.NoError(t, err)
	require.Equal(t, auth.State{Authenticated: true}, state)

	var (
		count    int64
		lastUsed time.Time
	)
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(),
		"SELECT sign_count, last_used_at FROM passkeys WHERE credential_id = $1", a.CredentialID).Scan(&count, &lastUsed))
	require.EqualValues(t, 7, count)
	require.True(t, lastUsed.Equal(h.clock.Now()))

	var (
		hash []byte
		ua   string
	)
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(),
		"SELECT token_hash, user_agent FROM auth_sessions ORDER BY created_at DESC LIMIT 1").Scan(&hash, &ua))
	raw, err := base64.RawURLEncoding.DecodeString(sess.Token)
	require.NoError(t, err)
	require.Equal(t, sha(raw), hash, "в базе только sha256 токена")
	require.Equal(t, "Mozilla/5.0 login", ua)
	require.NotContains(t, h.logs.String(), sess.Token)
	require.Equal(t, 0, h.count(t, "webauthn_ceremonies"), "церемония одноразовая")
}

func TestFinishLogin_SyncedPasskeyWithZeroCounter(t *testing.T) {
	h := newHarness(t)
	a := h.registerOwner(t) // SignCount = 0, как у iCloud Keychain

	for range 2 {
		_, err := h.login(t, a)
		require.NoError(t, err)
	}
	require.Zero(t, h.signCount(t, a))
}

func TestFinishLogin_CloneWarningRejected(t *testing.T) {
	h := newHarness(t)
	a := h.registerOwner(t)
	_, err := h.pg.Pool().Exec(t.Context(), "UPDATE passkeys SET sign_count = 10")
	require.NoError(t, err)
	sessions := h.count(t, "auth_sessions")

	for _, n := range []uint32{5, 10} {
		a.SignCount = n
		_, err = h.login(t, a)
		require.ErrorIs(t, err, auth.ErrVerificationFailed, "sign_count=%d", n)
	}
	require.Equal(t, sessions, h.count(t, "auth_sessions"), "сессия не создана")
	require.EqualValues(t, 10, h.signCount(t, a), "счётчик не откатывается")
	require.Contains(t, h.logs.String(), "passkey_clone_warning")

	a.SignCount = 11
	_, err = h.login(t, a)
	require.NoError(t, err)
	require.EqualValues(t, 11, h.signCount(t, a))
}

func TestFinishLogin_CeremonyExpiredUnknownOrReused(t *testing.T) {
	h := newHarness(t)
	a := h.registerOwner(t)
	ctx := t.Context()

	c, err := h.svc.BeginLogin(ctx)
	require.NoError(t, err)
	h.clock.Advance(5*time.Minute + time.Second)
	_, err = h.svc.FinishLogin(ctx, auth.FinishLoginInput{CeremonyID: c.ID, Credential: a.Login(t, c.Options)})
	require.ErrorIs(t, err, auth.ErrNotFound, "церемония истекла")

	_, err = h.svc.FinishLogin(ctx, auth.FinishLoginInput{CeremonyID: uuid.New(), Credential: a.Login(t, c.Options)})
	require.ErrorIs(t, err, auth.ErrNotFound, "неизвестная церемония")

	c, err = h.svc.BeginLogin(ctx)
	require.NoError(t, err)
	in := auth.FinishLoginInput{CeremonyID: c.ID, Credential: a.Login(t, c.Options)}
	_, err = h.svc.FinishLogin(ctx, in)
	require.NoError(t, err)
	_, err = h.svc.FinishLogin(ctx, in)
	require.ErrorIs(t, err, auth.ErrNotFound, "повтор той же церемонии")

	c, err = h.svc.BeginLogin(ctx)
	require.NoError(t, err)
	_, err = h.svc.FinishLogin(ctx, auth.FinishLoginInput{CeremonyID: c.ID, Credential: []byte(`{`)})
	require.ErrorIs(t, err, auth.ErrVerificationFailed)
	_, err = h.svc.FinishLogin(ctx, auth.FinishLoginInput{CeremonyID: c.ID, Credential: a.Login(t, c.Options)})
	require.ErrorIs(t, err, auth.ErrNotFound, "церемония расходуется и при неудачной проверке")
}

func TestFinishLogin_VerificationFailures(t *testing.T) {
	h := newHarness(t)
	a := h.registerOwner(t)
	sessions := h.count(t, "auth_sessions")

	stranger := newAuthenticator(t) // passkey не зарегистрирован (например, удалён в настройках)
	stranger.UserHandle = a.UserHandle

	cases := map[string]func(t *testing.T, options []byte) []byte{
		"bad signature": func(t *testing.T, options []byte) []byte {
			return patchResponse(t, a.Login(t, options), "signature", func(v string) string {
				raw, err := base64.RawURLEncoding.DecodeString(v)
				require.NoError(t, err)
				raw[len(raw)-1] ^= 0xff
				return base64.RawURLEncoding.EncodeToString(raw)
			})
		},
		"unknown credential": func(t *testing.T, options []byte) []byte { return stranger.Login(t, options) },
		"foreign user handle": func(t *testing.T, options []byte) []byte {
			return patchResponse(t, a.Login(t, options), "userHandle", func(string) string {
				return base64.RawURLEncoding.EncodeToString([]byte("someone-else"))
			})
		},
		"empty user handle": func(t *testing.T, options []byte) []byte {
			return patchResponse(t, a.Login(t, options), "userHandle", func(string) string { return "" })
		},
		"wrong origin": func(t *testing.T, options []byte) []byte {
			evil := *a
			evil.Origin = "https://evil.example"
			return evil.Login(t, options)
		},
		"challenge of another ceremony": func(t *testing.T, _ []byte) []byte {
			other, err := h.svc.BeginLogin(t.Context())
			require.NoError(t, err)
			return a.Login(t, other.Options)
		},
		"not json": func(*testing.T, []byte) []byte { return []byte(`not json`) },
	}
	for name, credential := range cases {
		t.Run(name, func(t *testing.T) {
			c, err := h.svc.BeginLogin(t.Context())
			require.NoError(t, err)
			_, err = h.svc.FinishLogin(t.Context(), auth.FinishLoginInput{CeremonyID: c.ID, Credential: credential(t, c.Options)})
			require.ErrorIs(t, err, auth.ErrVerificationFailed)
		})
	}
	require.Equal(t, sessions, h.count(t, "auth_sessions"))
}

func TestFinishLogin_NoOwner(t *testing.T) {
	h := newHarness(t)
	a := newAuthenticator(t)
	a.UserHandle = []byte("unknown")

	c, err := h.svc.BeginLogin(t.Context())
	require.NoError(t, err)
	_, err = h.svc.FinishLogin(t.Context(), auth.FinishLoginInput{CeremonyID: c.ID, Credential: a.Login(t, c.Options)})
	require.ErrorIs(t, err, auth.ErrVerificationFailed)
}

// patchResponse заменяет поле response.<field> в AuthenticationResponseJSON.
func patchResponse(t *testing.T, credential []byte, field string, patch func(string) string) []byte {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(credential, &m))
	resp, ok := m["response"].(map[string]any)
	require.True(t, ok)
	v, _ := resp[field].(string)
	resp[field] = patch(v)
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return out
}

func TestAuthEndpoints_GlobalRateLimit(t *testing.T) {
	h := newHarness(t)
	mux := http.NewServeMux()
	api.NewServer(h.svc, slog.New(slog.DiscardHandler), testConfig()).Register(mux)

	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "appmetrica-web")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	unknownToken := "/admin/v1/auth/setup/" + strings.Repeat("A", 43)

	// Лимит общий для beginLogin, finishLogin, beginSetup и finishSetup: 20 попыток в минуту на процесс.
	for i := range 20 {
		var rec *httptest.ResponseRecorder
		switch i % 4 {
		case 0:
			rec = post(unknownToken + "/begin")
		case 1:
			rec = post(unknownToken + "/finish")
		case 2:
			rec = post("/admin/v1/auth/login/finish")
		default:
			rec = post("/admin/v1/auth/login/begin")
		}
		require.NotEqual(t, http.StatusTooManyRequests, rec.Code, "попытка %d: %s", i+1, rec.Body)
	}

	for _, path := range []string{"/admin/v1/auth/login/begin", "/admin/v1/auth/login/finish", unknownToken + "/begin"} {
		rec := post(path)
		require.Equal(t, http.StatusTooManyRequests, rec.Code, path)
		require.Contains(t, rec.Body.String(), `"code":"rate_limited"`)
		retry, err := strconv.Atoi(rec.Header().Get("Retry-After"))
		require.NoError(t, err)
		require.GreaterOrEqual(t, retry, 1)
	}

	// Состояние и выход не лимитируются.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin/v1/auth/state", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, http.StatusNoContent, post("/admin/v1/auth/logout").Code)

	h.clock.Advance(time.Minute)
	require.Equal(t, http.StatusOK, post("/admin/v1/auth/login/begin").Code)
}
