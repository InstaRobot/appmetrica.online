package api

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	gen "appmetrica.online/backend/internal/gen/api"
)

const (
	msgWriteKeyNotFound = "Write-ключ не найден"
	msgAPIKeyNotFound   = "API-ключ не найден" //nolint:gosec // G101: текст ошибки, а не секрет
)

func (s *Server) ListWriteKeys(ctx context.Context, req gen.ListWriteKeysRequestObject) (gen.ListWriteKeysResponseObject, error) {
	keys, err := s.apps.ListWriteKeys(ctx, req.AppId)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return gen.ListWriteKeys404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	items := make([]gen.WriteKey, 0, len(keys))
	for _, k := range keys {
		items = append(items, writeKeyModel(k))
	}
	return gen.ListWriteKeys200JSONResponse{Items: items}, nil
}

func (s *Server) CreateWriteKey(ctx context.Context, req gen.CreateWriteKeyRequestObject) (gen.CreateWriteKeyResponseObject, error) {
	var label string
	if req.Body != nil && req.Body.Label != nil {
		label = *req.Body.Label
	}
	key, err := s.apps.CreateWriteKey(ctx, req.AppId, label)
	var verr *apps.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.CreateWriteKey400JSONResponse{BadRequestJSONResponse: appValidation(ctx, verr)}, nil
	case errors.Is(err, apps.ErrNotFound):
		return gen.CreateWriteKey404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.CreateWriteKey201JSONResponse(writeKeyModel(key)), nil
}

func (s *Server) RevokeWriteKey(ctx context.Context, req gen.RevokeWriteKeyRequestObject) (gen.RevokeWriteKeyResponseObject, error) {
	err := s.apps.RevokeWriteKey(ctx, req.AppId, req.KeyId)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return gen.RevokeWriteKey404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, apps.ErrWriteKeyNotFound):
		return gen.RevokeWriteKey404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgWriteKeyNotFound, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.RevokeWriteKey204Response{}, nil
}

//nolint:revive // имя задаёт gen.StrictServerInterface
func (s *Server) ListApiKeys(ctx context.Context, _ gen.ListApiKeysRequestObject) (gen.ListApiKeysResponseObject, error) {
	keys, err := s.auth.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]gen.ApiKey, 0, len(keys))
	for _, k := range keys {
		items = append(items, apiKeyModel(k))
	}
	return gen.ListApiKeys200JSONResponse{Items: items}, nil
}

//nolint:revive // имя задаёт gen.StrictServerInterface
func (s *Server) CreateApiKey(ctx context.Context, req gen.CreateApiKeyRequestObject) (gen.CreateApiKeyResponseObject, error) {
	if req.Body == nil {
		return gen.CreateApiKey400JSONResponse{BadRequestJSONResponse: invalidBody(ctx)}, nil
	}
	in := auth.CreateAPIKeyInput{Label: req.Body.Label, Scopes: make([]string, 0, len(req.Body.Scopes))}
	for _, sc := range req.Body.Scopes {
		in.Scopes = append(in.Scopes, string(sc))
	}
	if appID, err := req.Body.AppId.Get(); err == nil {
		in.AppID = &appID
	}
	key, secret, err := s.auth.CreateAPIKey(ctx, in)
	var verr *auth.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.CreateApiKey400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", verr.Message, fieldDetails(verr)))}, nil
	case errors.Is(err, auth.ErrAppNotFound):
		return gen.CreateApiKey404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.CreateApiKey201JSONResponse{ApiKey: apiKeyModel(key), Secret: secret}, nil
}

//nolint:revive // имя задаёт gen.StrictServerInterface
func (s *Server) RevokeApiKey(ctx context.Context, req gen.RevokeApiKeyRequestObject) (gen.RevokeApiKeyResponseObject, error) {
	err := s.auth.RevokeAPIKey(ctx, req.KeyId)
	switch {
	case errors.Is(err, auth.ErrNotFound):
		return gen.RevokeApiKey404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgAPIKeyNotFound, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.RevokeApiKey204Response{}, nil
}

func apiKeyModel(k auth.APIKey) gen.ApiKey {
	scopes := make([]gen.ApiKeyScope, 0, len(k.Scopes))
	for _, sc := range k.Scopes {
		scopes = append(scopes, gen.ApiKeyScope(sc))
	}
	return gen.ApiKey{
		Id:         k.ID,
		KeyPrefix:  k.KeyPrefix,
		Label:      k.Label,
		Scopes:     scopes,
		AppId:      nullableValue[uuid.UUID](k.AppID),
		CreatedAt:  k.CreatedAt,
		RevokedAt:  nullableValue(k.RevokedAt),
		LastUsedAt: nullableValue(k.LastUsedAt),
	}
}
