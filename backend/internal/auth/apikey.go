package auth

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

// Scopes ключей Read API.
const (
	ScopeRead   = "read"
	ScopeDelete = "delete"
)

// RequireAPIAccess — заглушка до реализации T-021.
func (s *Service) RequireAPIAccess(writeError ErrorWriter, scopes ...string) func(http.Handler) http.Handler {
	return s.RequireSession(writeError)
}

// RegisterMetrics — заглушка до реализации T-021.
func (s *Service) RegisterMetrics(reg prometheus.Registerer) error { return nil }
