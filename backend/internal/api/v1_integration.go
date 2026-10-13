package api

import (
	"context"
	"errors"

	gen "appmetrica.online/backend/internal/gen/api"
)

var errIntegrationNotImplemented = errors.New("api: здоровье интеграции не реализовано")

// GetIntegrationHealth — здоровье интеграции приложения (FR-017).
func (s *Server) GetIntegrationHealth(_ context.Context, _ gen.GetIntegrationHealthRequestObject) (gen.GetIntegrationHealthResponseObject, error) {
	return nil, errIntegrationNotImplemented
}

// parseIntegrationParams — hours 1..168, по умолчанию 24.
func parseIntegrationParams(_ gen.GetIntegrationHealthParams) (int, *paramError) {
	return 0, nil
}
