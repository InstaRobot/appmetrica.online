//go:build integration

package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/deletion"
)

var secretRe = regexp.MustCompile(`^sk_[A-Za-z0-9]{40}$`)

// keysEnv — admin API и Read API поверх harness: владелец с сессией, метрики api_* в своём реестре.
type keysEnv struct {
	h     *harness
	mux   *http.ServeMux
	token string
	reg   *prometheus.Registry
	// deletion — тот же mux: createDeletionRequest с проверками api (CSRF для cookie, scope delete).
	deletion http.Handler
}

func (h *harness) keysEnv(t *testing.T) *keysEnv {
	t.Helper()
	sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
	require.NoError(t, err)
	reg := prometheus.NewRegistry()
	require.NoError(t, h.svc.RegisterMetrics(reg))

	logger := slog.New(slog.DiscardHandler)
	mux := http.NewServeMux()
	api.NewServer(h.svc, apps.NewService(h.pg, h.clock, logger), logger, testConfig()).
		WithDeletion(deletion.NewService(h.pg, h.clock, logger)).
		Register(mux)
	return &keysEnv{h: h, mux: mux, token: sess.Token, reg: reg, deletion: mux}
}

// do выполняет запрос на handler. body — nil, сырая строка или значение для JSON.
func do(t *testing.T, handler http.Handler, method, path string, body any, prepare func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var raw string
	switch b := body.(type) {
	case nil:
	case string:
		raw = b
	default:
		enc, err := json.Marshal(b)
		require.NoError(t, err)
		raw = string(enc)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	prepare(req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// admin — запрос дашборда: cookie сессии и X-Requested-With.
func (e *keysEnv) admin(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, e.mux, method, path, body, e.asOwner)
}

func (e *keysEnv) asOwner(r *http.Request) {
	r.Header.Set("X-Requested-With", auth.RequestedWith)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token})
}

// bearer — запрос скрипта: только Authorization: Bearer <secret>, без cookie и X-Requested-With.
func (e *keysEnv) bearer(t *testing.T, method, path, secret string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, e.mux, method, path, nil, withBearer(secret))
}

func withBearer(secret string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+secret) }
}

type apiKeyJSON struct {
	ID         string     `json:"id"`
	KeyPrefix  string     `json:"key_prefix"`
	Label      string     `json:"label"`
	Scopes     []string   `json:"scopes"`
	AppID      *string    `json:"app_id"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

// createKey выпускает ключ через POST /admin/v1/api-keys и возвращает его с секретом.
func (e *keysEnv) createKey(t *testing.T, body map[string]any) (apiKeyJSON, string) {
	t.Helper()
	rec := e.admin(t, http.MethodPost, "/admin/v1/api-keys", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	out := decodeJSON[struct {
		APIKey apiKeyJSON `json:"api_key"`
		Secret string     `json:"secret"`
	}](t, rec)
	return out.APIKey, out.Secret
}

func (e *keysEnv) readKey(t *testing.T) (apiKeyJSON, string) {
	t.Helper()
	return e.createKey(t, map[string]any{"label": "telegram-digest-script", "scopes": []string{"read"}})
}

func (e *keysEnv) listKeys(t *testing.T) []apiKeyJSON {
	t.Helper()
	rec := e.admin(t, http.MethodGet, "/admin/v1/api-keys", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[struct {
		Items []apiKeyJSON `json:"items"`
	}](t, rec).Items
}

// createApp создаёт приложение через admin API и возвращает его id.
func (e *keysEnv) createApp(t *testing.T, name string) string {
	t.Helper()
	rec := e.admin(t, http.MethodPost, "/admin/v1/apps", map[string]any{"name": name, "platform": "ios", "timezone": "Europe/Moscow"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	return decodeJSON[struct {
		App struct {
			ID string `json:"id"`
		} `json:"app"`
	}](t, rec).App.ID
}

// appNames — имена из ответа GET /v1/apps.
func appNames(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	list := decodeJSON[struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}](t, rec)
	names := make([]string, 0, len(list.Items))
	for _, a := range list.Items {
		names = append(names, a.Name)
	}
	return names
}

// metric — значение счётчика name с метками labels (0, если серии ещё нет).
func (e *keysEnv) metric(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := e.reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
	series:
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if labels[l.GetName()] != l.GetValue() {
					continue series
				}
			}
			return m.GetCounter().GetValue()
		}
	}
	return 0
}

func requireBearerChallenge(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	requireAPIError(t, rec, http.StatusUnauthorized, "unauthenticated")
	require.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
}

// randomSecret — синтаксически верный, но никогда не выпускавшийся ключ.
func randomSecret() string {
	return "sk_" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")[:40]
}

func TestAPIKey_CreateAndAuthenticate(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	e.createApp(t, "Notes iOS")

	key, secret := e.readKey(t)
	require.Regexp(t, secretRe, secret)
	require.Equal(t, secret[:11], key.KeyPrefix)
	require.Equal(t, "telegram-digest-script", key.Label)
	require.Equal(t, []string{"read"}, key.Scopes)
	require.Nil(t, key.AppID)
	require.False(t, key.CreatedAt.IsZero())
	require.Nil(t, key.RevokedAt)
	require.Nil(t, key.LastUsedAt)

	require.Equal(t, []string{"Notes iOS"}, appNames(t, e.bearer(t, http.MethodGet, "/v1/apps", secret)))

	// В api_keys только key_prefix и HMAC-SHA256(KEY_HASH_PEPPER, secret); секрета нет ни в одной колонке.
	var (
		prefix string
		hash   []byte
	)
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(), "SELECT key_prefix, key_hash FROM api_keys").Scan(&prefix, &hash))
	require.Equal(t, key.KeyPrefix, prefix)
	mac := hmac.New(sha256.New, testPepper)
	mac.Write([]byte(secret))
	require.Equal(t, mac.Sum(nil), hash)
	for _, part := range []string{secret, secret[11:]} {
		var n int
		require.NoError(t, h.pg.Pool().QueryRow(t.Context(),
			"SELECT count(*) FROM api_keys WHERE strpos(row_to_json(api_keys)::text, $1) > 0", part).Scan(&n))
		require.Zero(t, n)
	}

	// Список ключей секрет не возвращает; использование отмечено в last_used_at.
	rec := e.admin(t, http.MethodGet, "/admin/v1/api-keys", nil)
	require.NotContains(t, rec.Body.String(), secret[11:])
	list := e.listKeys(t)
	require.Len(t, list, 1)
	require.Equal(t, key.ID, list[0].ID)
	require.NotNil(t, list[0].LastUsedAt)
	require.True(t, list[0].LastUsedAt.Equal(t0), list[0].LastUsedAt)
}

func TestAPIKey_LastUsedAtAtMostOncePerMinute(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	_, secret := e.readKey(t)

	lastUsed := func() time.Time {
		t.Helper()
		list := e.listKeys(t)
		require.Len(t, list, 1)
		require.NotNil(t, list[0].LastUsedAt)
		return *list[0].LastUsedAt
	}
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", secret).Code)
	require.True(t, lastUsed().Equal(t0))

	h.clock.Advance(30 * time.Second)
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", secret).Code)
	require.True(t, lastUsed().Equal(t0), "обновление чаще раза в минуту")

	h.clock.Advance(2 * time.Minute)
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", secret).Code)
	require.True(t, lastUsed().Equal(h.clock.Now()))
}

func TestAPIKey_ScopeEnforced(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	appPath := "/v1/apps/" + e.createApp(t, "Notes iOS") + "/deletion-requests"
	body := map[string]any{"subject_type": "install", "subject_value": uuid.NewString()}

	_, read := e.readKey(t)
	_, del := e.createKey(t, map[string]any{"label": "gdpr-backend", "scopes": []string{"delete"}})
	_, both := e.createKey(t, map[string]any{"label": "all", "scopes": []string{"read", "delete"}})

	t.Run("read key cannot create deletion requests", func(t *testing.T) {
		rec := do(t, e.deletion, http.MethodPost, appPath, body, withBearer(read))
		requireAPIError(t, rec, http.StatusForbidden, "insufficient_scope")
	})
	t.Run("delete key passes without CSRF header", func(t *testing.T) {
		rec := do(t, e.deletion, http.MethodPost, appPath, body, withBearer(del))
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	})
	t.Run("delete-only key cannot read", func(t *testing.T) {
		requireAPIError(t, e.bearer(t, http.MethodGet, "/v1/apps", del), http.StatusForbidden, "insufficient_scope")
	})
	t.Run("key with both scopes reads and deletes", func(t *testing.T) {
		require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", both).Code)
		require.Equal(t, http.StatusAccepted, do(t, e.deletion, http.MethodPost, appPath, body, withBearer(both)).Code)
	})
	t.Run("owner session has every scope", func(t *testing.T) {
		require.Equal(t, http.StatusAccepted, do(t, e.deletion, http.MethodPost, appPath, body, e.asOwner).Code)
	})
	t.Run("cookie request still needs CSRF header", func(t *testing.T) {
		rec := do(t, e.deletion, http.MethodPost, appPath, body, func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token})
		})
		requireAPIError(t, rec, http.StatusForbidden, "csrf_check_failed")
	})
	require.InDelta(t, 2.0, e.metric(t, "api_auth_failures_total", map[string]string{"reason": "insufficient_scope"}), 0)
}

func TestAPIKey_AppRestriction(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	appA := e.createApp(t, "Notes iOS")
	appB := e.createApp(t, "Notes Android")

	key, secret := e.createKey(t, map[string]any{"label": "notes-ios-bot", "scopes": []string{"read"}, "app_id": appA})
	require.NotNil(t, key.AppID)
	require.Equal(t, appA, *key.AppID)

	require.Equal(t, []string{"Notes iOS"}, appNames(t, e.bearer(t, http.MethodGet, "/v1/apps", secret)))
	require.Equal(t, []string{"Notes iOS"}, appNames(t, e.bearer(t, http.MethodGet, "/v1/apps?include_archived=true", secret)))
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps/"+appA, secret).Code)
	requireAPIError(t, e.bearer(t, http.MethodGet, "/v1/apps/"+appB, secret), http.StatusNotFound, "app_not_found")
	requireAPIError(t, e.bearer(t, http.MethodGet, "/v1/apps/"+uuid.NewString(), secret), http.StatusNotFound, "app_not_found")

	// Ограничение распространяется на любой маршрут с {app_id}.
	rec := do(t, e.deletion, http.MethodPost, "/v1/apps/"+appB+"/deletion-requests", nil, withBearer(secret))
	requireAPIError(t, rec, http.StatusNotFound, "app_not_found")

	// Ключ без app_id видит всё.
	_, all := e.readKey(t)
	require.Equal(t, []string{"Notes Android", "Notes iOS"}, appNames(t, e.bearer(t, http.MethodGet, "/v1/apps", all)))

	t.Run("unknown app_id", func(t *testing.T) {
		rec := e.admin(t, http.MethodPost, "/admin/v1/api-keys",
			map[string]any{"label": "x", "scopes": []string{"read"}, "app_id": uuid.NewString()})
		requireAPIError(t, rec, http.StatusNotFound, "app_not_found")
	})
}

func TestAPIKey_RevokedAndUnknown(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	key, secret := e.readKey(t)
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", secret).Code)

	rec := e.admin(t, http.MethodPost, "/admin/v1/api-keys/"+key.ID+"/revoke", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	requireBearerChallenge(t, e.bearer(t, http.MethodGet, "/v1/apps", secret))
	requireBearerChallenge(t, e.bearer(t, http.MethodGet, "/v1/apps", randomSecret()))
	require.InDelta(t, 2.0, e.metric(t, "api_auth_failures_total", map[string]string{"reason": "invalid_key"}), 0)

	// Повторный отзыв — 204, revoked_at не меняется.
	h.clock.Advance(time.Hour)
	require.Equal(t, http.StatusNoContent, e.admin(t, http.MethodPost, "/admin/v1/api-keys/"+key.ID+"/revoke", nil).Code)
	list := e.listKeys(t)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].RevokedAt)
	require.True(t, list[0].RevokedAt.Equal(t0), list[0].RevokedAt)

	t.Run("unknown key id", func(t *testing.T) {
		rec := e.admin(t, http.MethodPost, "/admin/v1/api-keys/"+uuid.NewString()+"/revoke", nil)
		requireAPIError(t, rec, http.StatusNotFound, "not_found")
	})
	t.Run("invalid key id", func(t *testing.T) {
		rec := e.admin(t, http.MethodPost, "/admin/v1/api-keys/not-a-uuid/revoke", nil)
		requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	})
}

func TestAPIKey_RateLimited(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	_, first := e.readKey(t)
	_, second := e.readKey(t)

	for i := range 30 {
		require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", first).Code, "request %d", i+1)
	}
	rec := e.bearer(t, http.MethodGet, "/v1/apps", first)
	requireAPIError(t, rec, http.StatusTooManyRequests, "rate_limited")
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	require.InDelta(t, 1.0, e.metric(t, "api_rate_limited_total", nil), 0)

	// Лимит на ключ: другой ключ не затронут; через 100 мс у первого появляется один токен.
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", second).Code)
	h.clock.Advance(100 * time.Millisecond)
	require.Equal(t, http.StatusOK, e.bearer(t, http.MethodGet, "/v1/apps", first).Code)
	require.Equal(t, http.StatusTooManyRequests, e.bearer(t, http.MethodGet, "/v1/apps", first).Code)
}

func TestAPIKey_CreateValidation(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)

	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"empty scopes", map[string]any{"label": "x", "scopes": []string{}}, "scopes"},
		{"missing scopes", map[string]any{"label": "x"}, "scopes"},
		{"duplicate scopes", map[string]any{"label": "x", "scopes": []string{"read", "read"}}, "scopes"},
		{"unknown scope", map[string]any{"label": "x", "scopes": []string{"write"}}, "scopes"},
		{"empty label", map[string]any{"label": "", "scopes": []string{"read"}}, "label"},
		{"blank label", map[string]any{"label": "   ", "scopes": []string{"read"}}, "label"},
		{"label of 65 chars", map[string]any{"label": strings.Repeat("ж", 65), "scopes": []string{"read"}}, "label"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.admin(t, http.MethodPost, "/admin/v1/api-keys", tc.body)
			details := requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
			fields, ok := details["fields"].(map[string]any)
			require.True(t, ok, rec.Body.String())
			require.Contains(t, fields, tc.field)
		})
	}
	t.Run("broken JSON", func(t *testing.T) {
		requireAPIError(t, e.admin(t, http.MethodPost, "/admin/v1/api-keys", `{"label":`), http.StatusBadRequest, "invalid_request")
	})
	require.Empty(t, e.listKeys(t))

	key, _ := e.createKey(t, map[string]any{"label": "  " + strings.Repeat("ж", 64) + " ", "scopes": []string{"delete", "read"}})
	require.Equal(t, strings.Repeat("ж", 64), key.Label)
	require.ElementsMatch(t, []string{"read", "delete"}, key.Scopes)
}

func TestAPIKey_MalformedAuthorization(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	_, secret := e.readKey(t)

	for _, header := range []string{
		"Basic " + secret,
		"Bearer",
		"Bearer ",
		"Bearer pk_" + strings.Repeat("a", 32),
		"Bearer " + secret + "x",
		"Bearer " + secret[:len(secret)-1],
		"Bearer  " + secret,
	} {
		t.Run(header, func(t *testing.T) {
			rec := do(t, e.mux, http.MethodGet, "/v1/apps", nil, func(r *http.Request) { r.Header.Set("Authorization", header) })
			requireBearerChallenge(t, rec)
		})
	}
	t.Run("invalid Authorization is not rescued by a valid cookie", func(t *testing.T) {
		rec := do(t, e.mux, http.MethodGet, "/v1/apps", nil, func(r *http.Request) {
			e.asOwner(r)
			r.Header.Set("Authorization", "Bearer "+randomSecret())
		})
		requireBearerChallenge(t, rec)
	})
	t.Run("no credentials at all", func(t *testing.T) {
		requireBearerChallenge(t, do(t, e.mux, http.MethodGet, "/v1/apps", nil, func(*http.Request) {}))
	})
}

func TestAPIKey_BearerDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.PublicBearerEnabled = false
	h := newHarnessWithConfig(t, cfg)
	e := h.keysEnv(t)
	_, secret := e.readKey(t)

	requireBearerChallenge(t, e.bearer(t, http.MethodGet, "/v1/apps", secret))
	require.InDelta(t, 1.0, e.metric(t, "api_auth_failures_total", map[string]string{"reason": "bearer_disabled"}), 0)
	require.Equal(t, http.StatusOK, e.admin(t, http.MethodGet, "/v1/apps", nil).Code, "дашборд по cookie работает")
}

func TestAPIKey_AdminEndpointsRequireSession(t *testing.T) {
	h := newHarness(t)
	e := h.keysEnv(t)
	key, secret := e.readKey(t)

	t.Run("bearer is not accepted by admin API", func(t *testing.T) {
		requireAPIError(t, e.bearer(t, http.MethodGet, "/admin/v1/api-keys", secret), http.StatusUnauthorized, "unauthenticated")
		rec := do(t, e.mux, http.MethodPost, "/admin/v1/api-keys/"+key.ID+"/revoke", nil, func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+secret)
			r.Header.Set("X-Requested-With", auth.RequestedWith)
		})
		requireAPIError(t, rec, http.StatusUnauthorized, "unauthenticated")
	})
	t.Run("mutations need CSRF header", func(t *testing.T) {
		rec := do(t, e.mux, http.MethodPost, "/admin/v1/api-keys", map[string]any{"label": "x", "scopes": []string{"read"}},
			func(r *http.Request) { r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token}) })
		requireAPIError(t, rec, http.StatusForbidden, "csrf_check_failed")
	})
	require.Len(t, e.listKeys(t), 1)
	require.Nil(t, e.listKeys(t)[0].RevokedAt)
}
