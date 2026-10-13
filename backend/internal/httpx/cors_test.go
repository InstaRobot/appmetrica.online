package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/httpx"
)

func TestCORS_IngestPreflight(t *testing.T) {
	called := false
	h := httpx.CORS()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/v1/events", nil)
	req.Header.Set("Origin", "https://site.ru")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type, x-app-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.False(t, called, "preflight отвечает сам, обработчик не вызывается")
	require.Empty(t, rec.Body.String())
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	allow := rec.Header().Get("Access-Control-Allow-Headers")
	for _, name := range []string{"Content-Type", "Content-Encoding", "X-App-Key"} {
		require.Contains(t, allow, name)
	}
	require.Equal(t, "86400", rec.Header().Get("Access-Control-Max-Age"))
	require.NotContains(t, rec.Header(), "Access-Control-Allow-Credentials")

	t.Run("without Access-Control-Request-Method", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/v1/events", nil)
		req.Header.Set("Origin", "https://site.ru")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "86400", rec.Header().Get("Access-Control-Max-Age"))
	})
}

func TestCORS_ActualRequest(t *testing.T) {
	h := httpx.CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_json", "bad", nil)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", strings.NewReader("{"))
	req.Header.Set("Origin", "https://site.ru")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "обычный запрос доходит до обработчика")
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"), "браузер должен прочитать и ответ с ошибкой")
	expose := rec.Header().Get("Access-Control-Expose-Headers")
	require.Contains(t, expose, "X-Request-Id")
	require.Contains(t, expose, "Retry-After")
	require.NotContains(t, rec.Header(), "Access-Control-Allow-Credentials")
	require.Empty(t, rec.Header().Get("Access-Control-Max-Age"), "Max-Age только у preflight")
}
