package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"appmetrica.online/backend/internal/httpx"
)

// registerE2E подключает служебные маршруты стека Playwright (web/e2e/helpers/seed.ts). Вызывается только
// при APP_ENV=e2e: в production и development маршрутов нет, mux отвечает 404. Маршруты вне контракта api.yaml.
func (s *Server) registerE2E(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/e2e/passkey-link", s.e2ePasskeyLink)
	mux.HandleFunc("POST /internal/e2e/reset-auth", s.e2eResetAuth)
}

// e2ePasskeyLink — то же, что `appmetrica admin passkey-link`, но по HTTP: {"url": "<PUBLIC_BASE_URL>/setup/<token>"}.
func (s *Server) e2ePasskeyLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.auth.CreatePasskeyLink(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(map[string]string{"url": link}); err != nil {
		s.logger.WarnContext(r.Context(), "e2e passkey link: write response",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())), slog.Any("error", err))
	}
}

// e2eResetAuth — пустая auth-база перед сценариями первичной настройки и входа.
func (s *Server) e2eResetAuth(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.ResetForE2E(r.Context()); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
