package api

import (
	"context"
	"errors"

	gen "appmetrica.online/backend/internal/gen/api"
)

// GetRealtime — срез «прямо сейчас» приложения (FR-015).
func (s *Server) GetRealtime(ctx context.Context, req gen.GetRealtimeRequestObject) (gen.GetRealtimeResponseObject, error) {
	return nil, errors.New("api: realtime: not implemented")
}
