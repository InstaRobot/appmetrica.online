// Package api — HTTP-слой Read API (/v1) и admin API (/admin/v1) поверх strict-сервера oapi-codegen.
package api

import (
	"log/slog"
	"net/http"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/config"
)

type Server struct{}

func NewServer(_ *auth.Service, _ *slog.Logger, _ config.API) *Server { return &Server{} }

func (s *Server) Register(*http.ServeMux) {}
