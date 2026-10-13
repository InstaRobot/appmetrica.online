package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
)

// newMux собирает маршруты API без Postgres: запросы в этом файле не доходят до хранилища.
func newMux(t *testing.T, env config.Env) *http.ServeMux {
	t.Helper()
	cfg := config.API{
		Common:            config.Common{AppEnv: env},
		PublicBaseURL:     "http://localhost:3000",
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "appmetrica.online",
		WebAuthnRPOrigins: []string{"http://localhost:3000"},
		SessionTTL:        720 * time.Hour,
		SessionIdleTTL:    168 * time.Hour,
	}
	logger := slog.New(slog.DiscardHandler)
	svc, err := auth.NewService(nil, clock.NewFixed(time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)), logger, cfg)
	require.NoError(t, err)
	mux := http.NewServeMux()
	api.NewServer(svc, logger, cfg).Register(mux)
	return mux
}

func serve(mux http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

var e2eRoutes = []string{"/internal/e2e/passkey-link", "/internal/e2e/reset-auth"}

func TestE2ERoutes_DisabledOutsideE2E(t *testing.T) {
	for _, env := range []config.Env{config.EnvProduction, config.EnvDevelopment} {
		t.Run(string(env), func(t *testing.T) {
			mux := newMux(t, env)
			for _, path := range e2eRoutes {
				for _, method := range []string{http.MethodPost, http.MethodGet} {
					require.Equal(t, http.StatusNotFound, serve(mux, method, path).Code, "%s %s", method, path)
				}
			}
		})
	}

	// В e2e маршруты зарегистрированы только на POST: GET получает 405 от mux, не доходя до Postgres.
	t.Run("e2e", func(t *testing.T) {
		mux := newMux(t, config.Env("e2e"))
		for _, path := range e2eRoutes {
			rec := serve(mux, http.MethodGet, path)
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code, path)
			require.Equal(t, http.MethodPost, rec.Header().Get("Allow"), path)
		}
	})
}
