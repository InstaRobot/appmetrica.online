package api

import (
	"context"
	"errors"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	msgMergeNotFound = "Слияние не найдено"
	msgAppArchived   = "Целевое приложение в архиве — сначала верните его"
)

// WithMerges подключает слияние приложений: getMergePreview, listAppMerges, createAppMerge, retryAppMerge.
func (s *Server) WithMerges(svc *merge.Service) *Server {
	s.merges = svc
	return s
}

func (s *Server) GetMergePreview(ctx context.Context, req gen.GetMergePreviewRequestObject) (gen.GetMergePreviewResponseObject, error) {
	p, err := s.merges.Preview(ctx, req.AppId, req.Params.SourceAppId)
	var verr *merge.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.GetMergePreview400JSONResponse{BadRequestJSONResponse: mergeValidation(ctx, verr)}, nil
	case errors.Is(err, merge.ErrAppNotFound):
		return gen.GetMergePreview404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.GetMergePreview200JSONResponse{
		Source:          appSummaryModel(p.Source),
		Events:          int64(p.Events), //nolint:gosec // число событий одного приложения далеко от 2^63
		FirstDay:        nullableDate(p.FirstDay),
		LastDay:         nullableDate(p.LastDay),
		WriteKeys:       p.WriteKeys,
		TimezoneDiffers: p.Source.Timezone != p.Target.Timezone,
		TargetTimezone:  p.Target.Timezone,
		SourceTimezone:  p.Source.Timezone,
	}, nil
}

func (s *Server) ListAppMerges(ctx context.Context, req gen.ListAppMergesRequestObject) (gen.ListAppMergesResponseObject, error) {
	list, err := s.merges.List(ctx, req.AppId)
	switch {
	case errors.Is(err, merge.ErrAppNotFound):
		return gen.ListAppMerges404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	items := make([]gen.AppMerge, 0, len(list))
	for _, m := range list {
		items = append(items, appMergeModel(m))
	}
	return gen.ListAppMerges200JSONResponse{Items: items}, nil
}

func (s *Server) CreateAppMerge(ctx context.Context, req gen.CreateAppMergeRequestObject) (gen.CreateAppMergeResponseObject, error) {
	if req.Body == nil {
		return gen.CreateAppMerge400JSONResponse{BadRequestJSONResponse: invalidBody(ctx)}, nil
	}
	m, err := s.merges.Create(ctx, req.AppId, merge.CreateInput{
		SourceAppID: req.Body.SourceAppId,
		ConfirmName: req.Body.ConfirmName,
	})
	var verr *merge.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.CreateAppMerge400JSONResponse{BadRequestJSONResponse: mergeValidation(ctx, verr)}, nil
	case errors.Is(err, merge.ErrAppNotFound):
		return gen.CreateAppMerge404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, merge.ErrConfirmNameMismatch):
		return gen.CreateAppMerge409JSONResponse{ConflictJSONResponse: gen.ConflictJSONResponse(
			apiError(ctx, "confirm_name_mismatch", msgConfirmNameMismatch, nil))}, nil
	case errors.Is(err, merge.ErrDeletionPending):
		return gen.CreateAppMerge409JSONResponse{ConflictJSONResponse: deletionPending(ctx)}, nil
	case errors.Is(err, merge.ErrAppArchived):
		return gen.CreateAppMerge409JSONResponse{ConflictJSONResponse: gen.ConflictJSONResponse(
			apiError(ctx, "app_archived", msgAppArchived, nil))}, nil
	case errors.Is(err, merge.ErrMergeInProgress):
		return gen.CreateAppMerge409JSONResponse{ConflictJSONResponse: mergeInProgress(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.CreateAppMerge202JSONResponse(appMergeModel(m)), nil
}

func (s *Server) RetryAppMerge(ctx context.Context, req gen.RetryAppMergeRequestObject) (gen.RetryAppMergeResponseObject, error) {
	m, err := s.merges.Retry(ctx, req.AppId, req.MergeId)
	switch {
	case errors.Is(err, merge.ErrAppNotFound):
		return gen.RetryAppMerge404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, merge.ErrNotFound):
		return gen.RetryAppMerge404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgMergeNotFound, nil))}, nil
	case errors.Is(err, merge.ErrMergeInProgress):
		return gen.RetryAppMerge409JSONResponse{ConflictJSONResponse: mergeInProgress(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.RetryAppMerge200JSONResponse(appMergeModel(m)), nil
}

// mergeValidation — 400 invalid_request; ошибка поля попадает в details.fields, как у приложений.
func mergeValidation(ctx context.Context, e *merge.ValidationError) gen.BadRequestJSONResponse {
	return gen.BadRequestJSONResponse(apiError(ctx, "invalid_request", e.Message, map[string]any{
		"fields": map[string]any{e.Field: e.Message},
	}))
}

func appMergeModel(m sqlc.AppMerge) gen.AppMerge {
	return gen.AppMerge{
		Id:              m.ID,
		TargetAppId:     m.TargetAppID,
		SourceAppId:     nullableValue(m.SourceAppID),
		SourceName:      m.SourceName,
		SourcePlatforms: platformListModel(m.SourcePlatforms),
		Status:          gen.AppMergeStatus(m.Status),
		Step:            gen.AppMergeStep(m.Step),
		CopiedEvents:    m.CopiedEvents,
		TotalEvents:     m.TotalEvents,
		Error:           nullableValue(m.Error),
		CreatedAt:       m.CreatedAt,
		StartedAt:       nullableValue(m.StartedAt),
		FinishedAt:      nullableValue(m.FinishedAt),
	}
}

// nullableDate — день без времени; nil пишется как null.
func nullableDate(t *time.Time) nullable.Nullable[openapi_types.Date] {
	if t == nil {
		return nullable.NewNullNullable[openapi_types.Date]()
	}
	return nullable.NewNullableWithValue(openapi_types.Date{Time: *t})
}
