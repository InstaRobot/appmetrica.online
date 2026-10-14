package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/config"
)

// Тело любого запроса API ограничено api.MaxBodyBytes: без лимита неаутентифицированный
// POST /admin/v1/auth/login/finish читал бы JSON произвольного размера в память до проверки credential.
func TestBodyLimit_RejectsOversizedBody(t *testing.T) {
	mux := newMux(t, config.EnvProduction)

	oversized := `{"ceremony_id":"` + strings.Repeat("a", int(api.MaxBodyBytes)) + `"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/v1/auth/login/finish", strings.NewReader(oversized))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "appmetrica-web")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "payload_too_large", body.Error.Code)
	require.NotContains(t, body.Error.Message, "aaaa", "текст ошибки не цитирует тело")
}

// Тело в пределах лимита доходит до разбора: ответ — обычный 400 invalid_request по схеме, а не 413.
func TestBodyLimit_AllowsBodyWithinLimit(t *testing.T) {
	mux := newMux(t, config.EnvProduction)

	small := []byte(`{"ceremony_id":"not-a-uuid","credential":{}}`)
	require.Less(t, int64(len(small)), api.MaxBodyBytes)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/v1/auth/login/finish", bytes.NewReader(small))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "appmetrica-web")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}
