package api_test

import (
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
	"appmetrica.online/backend/internal/merge"
)

// TestMergeEndpointsRequireSession — операции слияния только для владельца: без cookie 401, мутации без
// X-Requested-With 403. Запросы отклоняются до хранилищ, поэтому сервисы без Postgres и ClickHouse.
func TestMergeEndpointsRequireSession(t *testing.T) {
	cfg := config.API{
		PublicBaseURL:     "http://localhost:3000",
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "appmetrica.online",
		WebAuthnRPOrigins: []string{"http://localhost:3000"},
		SessionTTL:        720 * time.Hour,
		SessionIdleTTL:    168 * time.Hour,
	}
	logger := slog.New(slog.DiscardHandler)
	clk := clock.NewFixed(time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC))
	authSvc, err := auth.NewService(nil, clk, logger, cfg)
	require.NoError(t, err)
	mux := http.NewServeMux()
	api.NewServer(authSvc, nil, logger, cfg).WithMerges(merge.NewService(nil, nil, clk, logger)).Register(mux)

	const app = "/admin/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f"
	const body = `{"source_app_id":"0199d5f0-2b3c-7d4e-9f50-6b7c8d9e0f1a","confirm_name":"Notes Android"}`
	cases := []struct {
		method, path, body string
		csrf               bool
		status             int
		code               string
	}{
		{http.MethodGet, app + "/merge-preview?source_app_id=0199d5f0-2b3c-7d4e-9f50-6b7c8d9e0f1a", "", false, http.StatusUnauthorized, "unauthenticated"},
		{http.MethodGet, app + "/merges", "", false, http.StatusUnauthorized, "unauthenticated"},
		{http.MethodPost, app + "/merges", body, true, http.StatusUnauthorized, "unauthenticated"},
		{http.MethodPost, app + "/merges", body, false, http.StatusForbidden, "csrf_check_failed"},
		{http.MethodPost, app + "/merges/0199d5f0-3c4d-7e5f-8a60-7c8d9e0f1a2b/retry", "", true, http.StatusUnauthorized, "unauthenticated"},
		{http.MethodPost, app + "/merges/0199d5f0-3c4d-7e5f-8a60-7c8d9e0f1a2b/retry", "", false, http.StatusForbidden, "csrf_check_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.csrf {
				req.Header.Set("X-Requested-With", auth.RequestedWith)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"code":"`+tc.code+`"`)
		})
	}
}
