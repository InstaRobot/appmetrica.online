package api

import (
	"context"
	"errors"

	"github.com/oapi-codegen/nullable"

	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	msgAppNotFound         = "Приложение не найдено"
	msgAppNameTaken        = "Приложение с таким названием уже есть"
	msgConfirmNameMismatch = "Название не совпадает"
	msgDeletionPending     = "Приложение ожидает удаления"
)

// ListApps — для Bearer-ключа с app_id только его приложение; чужие app_id в остальных маршрутах
// отсекает auth.RequireAPIAccess (404 app_not_found).
func (s *Server) ListApps(ctx context.Context, req gen.ListAppsRequestObject) (gen.ListAppsResponseObject, error) {
	list, err := s.apps.List(ctx, req.Params.IncludeArchived != nil && *req.Params.IncludeArchived)
	if err != nil {
		return nil, err
	}
	only, restricted := auth.APIKeyAppFrom(ctx)
	items := make([]gen.App, 0, len(list))
	for _, a := range list {
		if !restricted || a.ID == only {
			items = append(items, appModel(a))
		}
	}
	return gen.ListApps200JSONResponse{Items: items}, nil
}

func (s *Server) GetApp(ctx context.Context, req gen.GetAppRequestObject) (gen.GetAppResponseObject, error) {
	app, err := s.apps.Get(ctx, req.AppId)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return gen.GetApp404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.GetApp200JSONResponse(appModel(app)), nil
}

func (s *Server) CreateApp(ctx context.Context, req gen.CreateAppRequestObject) (gen.CreateAppResponseObject, error) {
	if req.Body == nil {
		return gen.CreateApp400JSONResponse{BadRequestJSONResponse: invalidBody(ctx)}, nil
	}
	res, err := s.apps.Create(ctx, apps.CreateInput{
		Name:      req.Body.Name,
		Platforms: platformStrings(req.Body.Platforms),
		Timezone:  req.Body.Timezone,
	})
	var verr *apps.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.CreateApp400JSONResponse{BadRequestJSONResponse: appValidation(ctx, verr)}, nil
	case errors.Is(err, apps.ErrNameTaken):
		return gen.CreateApp409JSONResponse{ConflictJSONResponse: appNameTaken(ctx)}, nil
	case err != nil:
		return nil, err
	}
	keys := make([]gen.WriteKey, 0, len(res.WriteKeys))
	for _, k := range res.WriteKeys {
		keys = append(keys, writeKeyModel(k))
	}
	return gen.CreateApp201JSONResponse{App: appModel(res.App), WriteKeys: keys}, nil
}

func (s *Server) UpdateApp(ctx context.Context, req gen.UpdateAppRequestObject) (gen.UpdateAppResponseObject, error) {
	if req.Body == nil {
		return gen.UpdateApp400JSONResponse{BadRequestJSONResponse: invalidBody(ctx)}, nil
	}
	in := apps.UpdateInput{Name: req.Body.Name, Timezone: req.Body.Timezone}
	if req.Body.Platforms != nil {
		platforms := platformStrings(*req.Body.Platforms)
		in.Platforms = &platforms
	}
	app, err := s.apps.Update(ctx, req.AppId, in)
	var verr *apps.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.UpdateApp400JSONResponse{BadRequestJSONResponse: appValidation(ctx, verr)}, nil
	case errors.Is(err, apps.ErrNotFound):
		return gen.UpdateApp404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, apps.ErrNameTaken):
		return gen.UpdateApp409JSONResponse{ConflictJSONResponse: appNameTaken(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.UpdateApp200JSONResponse(appModel(app)), nil
}

func (s *Server) DeleteApp(ctx context.Context, req gen.DeleteAppRequestObject) (gen.DeleteAppResponseObject, error) {
	var confirm string
	if req.Body != nil {
		confirm = req.Body.ConfirmName
	}
	dr, err := s.apps.Delete(ctx, req.AppId, confirm)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return gen.DeleteApp404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, apps.ErrConfirmNameMismatch):
		return gen.DeleteApp409JSONResponse{ConflictJSONResponse: gen.ConflictJSONResponse(
			apiError(ctx, "confirm_name_mismatch", msgConfirmNameMismatch, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.DeleteApp202JSONResponse(deletionRequestModel(dr)), nil
}

func (s *Server) ArchiveApp(ctx context.Context, req gen.ArchiveAppRequestObject) (gen.ArchiveAppResponseObject, error) {
	err := s.apps.Archive(ctx, req.AppId)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return gen.ArchiveApp404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.ArchiveApp204Response{}, nil
}

func (s *Server) UnarchiveApp(ctx context.Context, req gen.UnarchiveAppRequestObject) (gen.UnarchiveAppResponseObject, error) {
	err := s.apps.Unarchive(ctx, req.AppId)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return gen.UnarchiveApp404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	case errors.Is(err, apps.ErrDeletionPending):
		return gen.UnarchiveApp409JSONResponse{ConflictJSONResponse: deletionPending(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return gen.UnarchiveApp204Response{}, nil
}

func appNotFound(ctx context.Context) gen.NotFoundJSONResponse {
	return gen.NotFoundJSONResponse(apiError(ctx, "app_not_found", msgAppNotFound, nil))
}

// deletionPending — 409 deletion_pending: у приложения невыполненная заявка на удаление (deleteApp).
func deletionPending(ctx context.Context) gen.ConflictJSONResponse {
	return gen.ConflictJSONResponse(apiError(ctx, "deletion_pending", msgDeletionPending, nil))
}

func appNameTaken(ctx context.Context) gen.ConflictJSONResponse {
	return gen.ConflictJSONResponse(apiError(ctx, "app_name_taken", msgAppNameTaken, map[string]any{
		"fields": map[string]any{"name": msgAppNameTaken},
	}))
}

func invalidBody(ctx context.Context) gen.BadRequestJSONResponse {
	return gen.BadRequestJSONResponse(apiError(ctx, "invalid_request", "Некорректный запрос", nil))
}

// appValidation — 400 invalid_request; ошибка поля попадает в details.fields.
func appValidation(ctx context.Context, e *apps.ValidationError) gen.BadRequestJSONResponse {
	var details map[string]any
	if e.Field != "" {
		details = map[string]any{"fields": map[string]any{e.Field: e.Message}}
	}
	return gen.BadRequestJSONResponse(apiError(ctx, "invalid_request", e.Message, details))
}

// platformStrings — платформы тела запроса как строки: значения проверяет apps.Service.
func platformStrings(in gen.PlatformList) []string {
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = string(p)
	}
	return out
}

// platformListModel — набор платформ приложения в порядке enum (как хранится в apps.platforms).
func platformListModel(in []sqlc.AppPlatform) gen.PlatformList {
	out := make(gen.PlatformList, len(in))
	for i, p := range in {
		out[i] = gen.Platform(p)
	}
	return out
}

func appModel(a sqlc.App) gen.App {
	return gen.App{
		Id:         a.ID,
		Name:       a.Name,
		Platform:   gen.Platform(a.Platform),
		Platforms:  platformListModel(a.Platforms),
		Timezone:   a.Timezone,
		Status:     gen.AppStatus(a.Status),
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
		ArchivedAt: nullableValue(a.ArchivedAt),
	}
}

func writeKeyModel(k sqlc.WriteKey) gen.WriteKey {
	return gen.WriteKey{
		Id:         k.ID,
		AppId:      k.AppID,
		Key:        k.Key,
		Label:      k.Label,
		Platform:   nullablePlatform(k.Platform),
		CreatedAt:  k.CreatedAt,
		RevokedAt:  nullableValue(k.RevokedAt),
		LastUsedAt: nullableValue(k.LastUsedAt),
	}
}

func deletionRequestModel(d sqlc.DeletionRequest) gen.DeletionRequest {
	return gen.DeletionRequest{
		Id:           d.ID,
		AppId:        d.AppID,
		SubjectType:  gen.DeletionSubjectType(d.SubjectType),
		SubjectValue: d.SubjectValue,
		Status:       gen.DeletionStatus(d.Status),
		Source:       gen.DeletionSource(d.Source),
		RequestedAt:  d.RequestedAt,
		StartedAt:    nullableValue(d.StartedAt),
		CompletedAt:  nullableValue(d.CompletedAt),
		Error:        nullableValue(d.Error),
	}
}

// nullablePlatform — платформа ключа; nil (любая платформа) пишется как null.
func nullablePlatform(p *sqlc.AppPlatform) nullable.Nullable[gen.Platform] {
	if p == nil {
		return nullable.NewNullNullable[gen.Platform]()
	}
	return nullable.NewNullableWithValue(gen.Platform(*p))
}

// nullableValue — nullable-поле контракта: nil пишется как явный null.
func nullableValue[T any](v *T) nullable.Nullable[T] {
	if v == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*v)
}
