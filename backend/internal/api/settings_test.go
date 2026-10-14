//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/telegram"
)

const settingsPath = "/admin/v1/settings"

// settingsJSON — Settings в виде map: telegram_chat_id бывает null, а updated_at сравнивается отдельно.
type settingsJSON struct {
	OwnerTimezone  string    `json:"owner_timezone"`
	TelegramChatID *string   `json:"telegram_chat_id"`
	DigestEnabled  bool      `json:"digest_enabled"`
	DigestTime     string    `json:"digest_time"`
	AlertsEnabled  bool      `json:"alerts_enabled"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func ptr[T any](v T) *T { return &v }

// settingsCall — запрос владельца к настройкам с проверкой ответа по api.yaml.
func (e *portfolioEnv) settingsCall(t *testing.T, doc *openapi3.T, cookie *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := e.call(t, method, path, body, asOwner(cookie))
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	validateResponse(t, doc, req, rec)
	return rec
}

func decodeSettings(t *testing.T, rec *httptest.ResponseRecorder) settingsJSON {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var s settingsJSON
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	return s
}

func (e *portfolioEnv) advance(d time.Duration) { e.clock.(*clock.Fixed).Advance(d) }

func TestUpdateSettings_Persists(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	cookie := ownerCookie(t, e)

	initial := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodGet, settingsPath, nil))
	require.Equal(t, settingsJSON{
		OwnerTimezone: "Europe/Moscow", DigestTime: "09:00", AlertsEnabled: true, UpdatedAt: initial.UpdatedAt,
	}, initial)

	e.advance(time.Minute)
	want := settingsJSON{
		OwnerTimezone:  "Asia/Almaty",
		TelegramChatID: ptr("-1001234567890"),
		DigestEnabled:  true,
		DigestTime:     "08:30",
		AlertsEnabled:  false,
		UpdatedAt:      e.clock.Now(),
	}
	patched := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{
		"owner_timezone":   "Asia/Almaty",
		"telegram_chat_id": "-1001234567890",
		"digest_enabled":   true,
		"digest_time":      "08:30",
		"alerts_enabled":   false,
	}))
	require.Equal(t, want, normalize(patched))

	got := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodGet, settingsPath, nil))
	require.Equal(t, want, normalize(got))
	// Начальный updated_at — now() Postgres, а часы теста стоят: сравнивается только факт изменения.
	require.False(t, got.UpdatedAt.Equal(initial.UpdatedAt))

	// Частичный PATCH меняет только переданные поля.
	e.advance(time.Minute)
	partial := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"digest_time": "23:59"}))
	want.DigestTime = "23:59"
	want.UpdatedAt = e.clock.Now()
	require.Equal(t, want, normalize(partial))

	// PATCH идемпотентен: то же тело — тот же результат.
	again := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"digest_time": "23:59"}))
	require.Equal(t, want, normalize(again))
}

// normalize — updated_at в UTC: Postgres возвращает timestamptz в зоне сессии.
func normalize(s settingsJSON) settingsJSON {
	s.UpdatedAt = s.UpdatedAt.UTC()
	return s
}

// telegram_chat_id: null отключает дайджест (CHECK settings_digest_requires_chat_chk); алерты остаются как были.
func TestUpdateSettings_ClearChatDisablesDigest(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	cookie := ownerCookie(t, e)

	decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{
		"telegram_chat_id": "123456789", "digest_enabled": true,
	}))
	cleared := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"telegram_chat_id": nil}))
	require.Nil(t, cleared.TelegramChatID)
	require.False(t, cleared.DigestEnabled)
	require.True(t, cleared.AlertsEnabled)

	// Чат задаётся вместе с включением дайджеста одним запросом; без чата дайджест не включить.
	rec := e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"digest_enabled": true})
	requireFieldError(t, rec, "digest_enabled")
	rec = e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"telegram_chat_id": nil, "digest_enabled": true})
	requireFieldError(t, rec, "digest_enabled")
	decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{
		"telegram_chat_id": "42", "digest_enabled": true,
	}))
}

// requireFieldError — 400 invalid_request с details.field и тем же полем в details.fields.
func requireFieldError(t *testing.T, rec *httptest.ResponseRecorder, fields ...string) {
	t.Helper()
	requireEnvelope(t, rec, http.StatusBadRequest, "invalid_request")
	var body struct {
		Error struct {
			Details struct {
				Field  string            `json:"field"`
				Fields map[string]string `json:"fields"`
			} `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, fields[0], body.Error.Details.Field)
	require.Len(t, body.Error.Details.Fields, len(fields))
	for _, f := range fields {
		require.NotEmpty(t, body.Error.Details.Fields[f], f)
	}
}

func TestUpdateSettings_Validation(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	cookie := ownerCookie(t, e)
	before := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodGet, settingsPath, nil))
	e.advance(time.Minute)

	cases := []struct {
		name   string
		body   any
		fields []string // details.field — первое; пусто — ошибка без поля
	}{
		{"digest_time_25", map[string]any{"digest_time": "25:00"}, []string{"digest_time"}},
		{"digest_time_no_leading_zero", map[string]any{"digest_time": "8:30"}, []string{"digest_time"}},
		{"digest_time_seconds", map[string]any{"digest_time": "08:30:00"}, []string{"digest_time"}},
		{"digest_time_minutes_60", map[string]any{"digest_time": "08:60"}, []string{"digest_time"}},
		{"timezone_unknown", map[string]any{"owner_timezone": "Nope/Zone"}, []string{"owner_timezone"}},
		{"timezone_city", map[string]any{"owner_timezone": "Moscow"}, []string{"owner_timezone"}},
		{"timezone_empty", map[string]any{"owner_timezone": ""}, []string{"owner_timezone"}},
		{"timezone_local", map[string]any{"owner_timezone": "Local"}, []string{"owner_timezone"}},
		{"chat_letters", map[string]any{"telegram_chat_id": "abc"}, []string{"telegram_chat_id"}},
		{"chat_username", map[string]any{"telegram_chat_id": "@channel"}, []string{"telegram_chat_id"}},
		{"chat_too_long", map[string]any{"telegram_chat_id": "123456789012345678901"}, []string{"telegram_chat_id"}},
		{"chat_empty", map[string]any{"telegram_chat_id": ""}, []string{"telegram_chat_id"}},
		{
			"time_and_timezone",
			map[string]any{"digest_time": "25:00", "owner_timezone": "Nope/Zone"},
			[]string{"owner_timezone", "digest_time"},
		},
		{
			"all_three",
			map[string]any{"digest_time": "25:00", "owner_timezone": "Nope/Zone", "telegram_chat_id": "abc", "alerts_enabled": true},
			[]string{"owner_timezone", "telegram_chat_id", "digest_time"},
		},
		{"digest_without_chat", map[string]any{"digest_enabled": true}, []string{"digest_enabled"}},
		{"empty_object", map[string]any{}, nil},
		{"only_unknown_fields", map[string]any{"theme": "dark"}, nil},
		{"only_nulls", map[string]any{"digest_enabled": nil, "owner_timezone": nil}, nil},
		{"wrong_type", `{"digest_enabled":"yes"}`, nil},
		{"not_json", `{`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, tc.body)
			if tc.fields != nil {
				requireFieldError(t, rec, tc.fields...)
			} else {
				requireEnvelope(t, rec, http.StatusBadRequest, "invalid_request")
			}
		})
	}

	after := decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodGet, settingsPath, nil))
	require.Equal(t, before, after, "ошибка валидации не меняет настройки")
}

func TestSettings_RequireOwner(t *testing.T) {
	e := newPortfolioEnv(t)
	cookie := ownerCookie(t, e)
	body := map[string]any{"digest_time": "10:00"}

	noCSRF := func(r *http.Request) { r.AddCookie(cookie) }
	requireEnvelope(t, e.call(t, http.MethodPatch, settingsPath, body, noCSRF), http.StatusForbidden, "csrf_check_failed")
	requireEnvelope(t, e.call(t, http.MethodPost, settingsPath+"/telegram/test", nil, noCSRF), http.StatusForbidden, "csrf_check_failed")

	anonymous := func(r *http.Request) { r.Header.Set("X-Requested-With", auth.RequestedWith) }
	requireEnvelope(t, e.call(t, http.MethodGet, settingsPath, nil, anonymous), http.StatusUnauthorized, "unauthenticated")
	requireEnvelope(t, e.call(t, http.MethodPatch, settingsPath, body, anonymous), http.StatusUnauthorized, "unauthenticated")

	// Настройки — admin API: Bearer sk_ сюда не пускает.
	requireEnvelope(t, e.call(t, http.MethodGet, settingsPath, nil, bearer(e.all)), http.StatusUnauthorized, "unauthenticated")
}

// fakeTelegram — httptest вместо api.telegram.org: отвечает status/body и записывает тела запросов.
type fakeTelegram struct {
	*httptest.Server
	mu       sync.Mutex
	requests []map[string]any
}

func newFakeTelegram(t *testing.T, status int, body string) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

const testBotToken = "123456:test-token-SECRET"

func (e *portfolioEnv) useTelegram(t *testing.T, f *fakeTelegram) {
	t.Helper()
	e.telegram = telegram.New(testBotToken, telegram.WithBaseURL(f.URL),
		telegram.WithSleep(func(_ context.Context, _ time.Duration) error { return nil }))
	e.rebuild(t)
}

func telegramResult(t *testing.T, rec *httptest.ResponseRecorder) gen.TelegramTestResult {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var r gen.TelegramTestResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r))
	return r
}

func TestSendTelegramTest(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	cookie := ownerCookie(t, e)
	const testPath = settingsPath + "/telegram/test"
	setChat := func(t *testing.T) {
		t.Helper()
		decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"telegram_chat_id": "-100500"}))
	}

	t.Run("chat_not_found", func(t *testing.T) {
		f := newFakeTelegram(t, http.StatusBadRequest, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
		e.useTelegram(t, f)
		setChat(t)
		rec := e.settingsCall(t, doc, cookie, http.MethodPost, testPath, nil)
		require.JSONEq(t, `{"sent":false,"error":"Bad Request: chat not found"}`, rec.Body.String())
		require.Len(t, f.requests, 1)
	})

	t.Run("sent", func(t *testing.T) {
		f := newFakeTelegram(t, http.StatusOK, `{"ok":true,"result":{"message_id":4211}}`)
		e.useTelegram(t, f)
		setChat(t)
		rec := e.settingsCall(t, doc, cookie, http.MethodPost, testPath, nil)
		require.JSONEq(t, `{"sent":true,"telegram_message_id":4211}`, rec.Body.String())
		require.Len(t, f.requests, 1)
		require.Equal(t, "-100500", f.requests[0]["chat_id"])
		require.Equal(t, "Тестовое сообщение appmetrica.online", f.requests[0]["text"])
	})

	t.Run("telegram_unreachable", func(t *testing.T) {
		f := newFakeTelegram(t, http.StatusOK, `{}`)
		e.useTelegram(t, f)
		f.Close()
		setChat(t)
		r := telegramResult(t, e.settingsCall(t, doc, cookie, http.MethodPost, testPath, nil))
		require.False(t, r.Sent)
		require.NotNil(t, r.Error)
		require.NotEmpty(t, *r.Error)
		require.NotContains(t, *r.Error, testBotToken)
		require.Nil(t, r.TelegramMessageId)
	})

	t.Run("no_bot_token", func(t *testing.T) {
		e.telegram = nil
		e.rebuild(t)
		setChat(t)
		r := telegramResult(t, e.settingsCall(t, doc, cookie, http.MethodPost, testPath, nil))
		require.False(t, r.Sent)
		require.NotNil(t, r.Error)
		require.Contains(t, *r.Error, "TELEGRAM_BOT_TOKEN")
	})

	t.Run("no_chat_id", func(t *testing.T) {
		f := newFakeTelegram(t, http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
		e.useTelegram(t, f)
		decodeSettings(t, e.settingsCall(t, doc, cookie, http.MethodPatch, settingsPath, map[string]any{"telegram_chat_id": nil}))
		rec := e.settingsCall(t, doc, cookie, http.MethodPost, testPath, nil)
		requireEnvelope(t, rec, http.StatusBadRequest, "invalid_request")
		require.Empty(t, f.requests)
	})
}
