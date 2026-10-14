package api

import (
	"context"
	"errors"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/deletion"
	gen "appmetrica.online/backend/internal/gen/api"
)

const msgDeletionRequestNotFound = "Запрос на удаление не найден"

// CreateDeletionRequest — source api и api_key_id для Bearer-ключа, source dashboard для cookie владельца.
func (s *Server) CreateDeletionRequest(ctx context.Context, req gen.CreateDeletionRequestRequestObject) (gen.CreateDeletionRequestResponseObject, error) {
	if req.Body == nil {
		return gen.CreateDeletionRequest400JSONResponse{BadRequestJSONResponse: invalidBody(ctx)}, nil
	}
	in := deletion.CreateInput{
		AppID:        req.AppId,
		SubjectType:  string(req.Body.SubjectType),
		SubjectValue: req.Body.SubjectValue,
	}
	if id, ok := auth.APIKeyIDFrom(ctx); ok {
		in.APIKeyID = &id
	}
	dr, err := s.deletion.Create(ctx, in)
	var verr *deletion.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.CreateDeletionRequest400JSONResponse{
			BadRequestJSONResponse: invalidParam(verr.Field, verr.Message).response(ctx),
		}, nil
	case errors.Is(err, deletion.ErrAppNotFound):
		return gen.CreateDeletionRequest404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.CreateDeletionRequest202JSONResponse(deletionRequestModel(dr)), nil
}

func (s *Server) GetDeletionRequest(ctx context.Context, req gen.GetDeletionRequestRequestObject) (gen.GetDeletionRequestResponseObject, error) {
	dr, err := s.deletion.Get(ctx, req.AppId, req.RequestId)
	switch {
	case errors.Is(err, deletion.ErrAppNotFound):
		return gen.GetDeletionRequest404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, deletion.ErrNotFound):
		return gen.GetDeletionRequest404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgDeletionRequestNotFound, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.GetDeletionRequest200JSONResponse(deletionRequestModel(dr)), nil
}

func (s *Server) ListDeletionRequests(ctx context.Context, req gen.ListDeletionRequestsRequestObject) (gen.ListDeletionRequestsResponseObject, error) {
	list, err := s.deletion.List(ctx, req.Params.AppId)
	if err != nil {
		return nil, err
	}
	items := make([]gen.DeletionRequest, 0, len(list))
	for _, dr := range list {
		items = append(items, deletionRequestModel(dr))
	}
	return gen.ListDeletionRequests200JSONResponse{Items: items}, nil
}
