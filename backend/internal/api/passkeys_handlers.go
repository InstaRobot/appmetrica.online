package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oapi-codegen/nullable"

	"appmetrica.online/backend/internal/auth"
	gen "appmetrica.online/backend/internal/gen/api"
)

const (
	msgKeyNotFound         = "Passkey не найден"
	msgLastKey             = "Нельзя удалить последний passkey: без него войти в дашборд не получится"
	msgRegistrationExpired = "Попытка добавить passkey устарела. Попробуйте ещё раз"
)

func (s *Server) ListPasskeys(ctx context.Context, _ gen.ListPasskeysRequestObject) (gen.ListPasskeysResponseObject, error) {
	list, err := s.auth.ListPasskeys(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]gen.Passkey, 0, len(list))
	for _, p := range list {
		items = append(items, passkeyModel(p))
	}
	return gen.ListPasskeys200JSONResponse{Items: items}, nil
}

func (s *Server) BeginPasskeyRegistration(ctx context.Context, _ gen.BeginPasskeyRegistrationRequestObject) (gen.BeginPasskeyRegistrationResponseObject, error) {
	c, err := s.auth.BeginPasskeyRegistration(ctx)
	if err != nil {
		return nil, err
	}
	options, err := ceremonyOptions(c)
	if err != nil {
		return nil, err
	}
	return gen.BeginPasskeyRegistration200JSONResponse{CeremonyId: c.ID, Options: options}, nil
}

func (s *Server) FinishPasskeyRegistration(ctx context.Context, req gen.FinishPasskeyRegistrationRequestObject) (gen.FinishPasskeyRegistrationResponseObject, error) {
	if req.Body == nil {
		return gen.FinishPasskeyRegistration400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", "Некорректный запрос", nil))}, nil
	}
	credential, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, fmt.Errorf("encode credential: %w", err)
	}
	p, err := s.auth.FinishPasskeyRegistration(ctx, auth.FinishPasskeyRegistrationInput{
		CeremonyID: req.Body.CeremonyId,
		Credential: credential,
		Name:       req.Body.Name,
	})
	var verr *auth.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.FinishPasskeyRegistration400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", verr.Message, fieldDetails(verr)))}, nil
	case errors.Is(err, auth.ErrNotFound):
		return gen.FinishPasskeyRegistration404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgRegistrationExpired, nil))}, nil
	case errors.Is(err, auth.ErrVerificationFailed):
		return gen.FinishPasskeyRegistration422JSONResponse{WebAuthnFailedJSONResponse: gen.WebAuthnFailedJSONResponse(
			apiError(ctx, "webauthn_verification_failed", msgWebAuthnFailed, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.FinishPasskeyRegistration201JSONResponse(passkeyModel(p)), nil
}

func (s *Server) RenamePasskey(ctx context.Context, req gen.RenamePasskeyRequestObject) (gen.RenamePasskeyResponseObject, error) {
	if req.Body == nil {
		return gen.RenamePasskey400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", "Некорректный запрос", nil))}, nil
	}
	p, err := s.auth.RenamePasskey(ctx, req.PasskeyId, req.Body.Name)
	var verr *auth.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.RenamePasskey400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", verr.Message, fieldDetails(verr)))}, nil
	case errors.Is(err, auth.ErrNotFound):
		return gen.RenamePasskey404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgKeyNotFound, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.RenamePasskey200JSONResponse(passkeyModel(p)), nil
}

func (s *Server) DeletePasskey(ctx context.Context, req gen.DeletePasskeyRequestObject) (gen.DeletePasskeyResponseObject, error) {
	err := s.auth.DeletePasskey(ctx, req.PasskeyId)
	switch {
	case errors.Is(err, auth.ErrNotFound):
		return gen.DeletePasskey404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgKeyNotFound, nil))}, nil
	case errors.Is(err, auth.ErrLastPasskey):
		return gen.DeletePasskey409JSONResponse{ConflictJSONResponse: gen.ConflictJSONResponse(
			apiError(ctx, "last_passkey", msgLastKey, nil))}, nil
	case err != nil:
		return nil, err
	}
	return gen.DeletePasskey204Response{}, nil
}

// passkeyModel — Passkey из api.yaml. Транспорты вне enum контракта (браузер может прислать новые) отбрасываются.
func passkeyModel(p auth.Passkey) gen.Passkey {
	transports := make([]gen.PasskeyTransports, 0, len(p.Transports))
	for _, t := range p.Transports {
		if v := gen.PasskeyTransports(t); v.Valid() {
			transports = append(transports, v)
		}
	}
	lastUsed := nullable.NewNullNullable[time.Time]()
	if p.LastUsedAt != nil {
		lastUsed = nullable.NewNullableWithValue(*p.LastUsedAt)
	}
	return gen.Passkey{
		Id:             p.ID,
		Name:           p.Name,
		CreatedAt:      p.CreatedAt,
		LastUsedAt:     lastUsed,
		BackupEligible: p.BackupEligible,
		BackupState:    p.BackupState,
		Transports:     transports,
	}
}
