package api

import (
	"context"
	"errors"

	gen "appmetrica.online/backend/internal/gen/api"
)

var errProfilesNotImplemented = errors.New("api: профили не реализованы")

// SearchProfiles — поиск профилей по точному install_id или user_id (FR-016).
func (s *Server) SearchProfiles(_ context.Context, _ gen.SearchProfilesRequestObject) (gen.SearchProfilesResponseObject, error) {
	return nil, errProfilesNotImplemented
}

// GetProfile — карточка профиля (FR-016).
func (s *Server) GetProfile(_ context.Context, _ gen.GetProfileRequestObject) (gen.GetProfileResponseObject, error) {
	return nil, errProfilesNotImplemented
}
