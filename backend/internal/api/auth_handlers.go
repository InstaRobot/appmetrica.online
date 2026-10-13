package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"appmetrica.online/backend/internal/auth"
	gen "appmetrica.online/backend/internal/gen/api"
)

const (
	msgSetupLinkUnknown  = "Ссылка установки passkey не найдена"
	msgSetupLinkExpired  = "Ссылка недействительна или устарела. Выполните appmetrica admin passkey-link ещё раз"
	msgWebAuthnFailed    = "Не удалось подтвердить passkey. Попробуйте ещё раз"
	msgLoginFailed       = "Не удалось подтвердить passkey. Попробуйте ещё раз или используйте другой ключ"
	msgLoginCeremonyGone = "Попытка входа устарела. Попробуйте ещё раз"
)

func (s *Server) GetAuthState(ctx context.Context, _ gen.GetAuthStateRequestObject) (gen.GetAuthStateResponseObject, error) {
	st, err := s.auth.State(ctx, metaFrom(ctx).sessionToken)
	if err != nil {
		return nil, err
	}
	return gen.GetAuthState200JSONResponse{Authenticated: st.Authenticated, SetupRequired: st.SetupRequired}, nil
}

func (s *Server) BeginLogin(ctx context.Context, _ gen.BeginLoginRequestObject) (gen.BeginLoginResponseObject, error) {
	c, err := s.auth.BeginLogin(ctx)
	if err != nil {
		return nil, err
	}
	options, err := ceremonyOptions(c)
	if err != nil {
		return nil, err
	}
	return gen.BeginLogin200JSONResponse{CeremonyId: c.ID, Options: options}, nil
}

func (s *Server) FinishLogin(ctx context.Context, req gen.FinishLoginRequestObject) (gen.FinishLoginResponseObject, error) {
	if req.Body == nil {
		return gen.FinishLogin400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", "Некорректный запрос", nil))}, nil
	}
	credential, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, fmt.Errorf("encode credential: %w", err)
	}
	sess, err := s.auth.FinishLogin(ctx, auth.FinishLoginInput{
		CeremonyID: req.Body.CeremonyId,
		Credential: credential,
		UserAgent:  metaFrom(ctx).userAgent,
	})
	switch {
	case errors.Is(err, auth.ErrNotFound):
		return gen.FinishLogin404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgLoginCeremonyGone, nil))}, nil
	case errors.Is(err, auth.ErrVerificationFailed):
		return gen.FinishLogin422JSONResponse{WebAuthnFailedJSONResponse: gen.WebAuthnFailedJSONResponse(
			apiError(ctx, "webauthn_verification_failed", msgLoginFailed, nil))}, nil
	case err != nil:
		return nil, err
	}
	cookie := s.sessionCookie(sess)
	return gen.FinishLogin204Response{Headers: gen.FinishLogin204ResponseHeaders{SetCookie: &cookie}}, nil
}

// Logout отзывает сессию cookie и очищает её; без cookie или с недействительной — тоже 204.
func (s *Server) Logout(ctx context.Context, _ gen.LogoutRequestObject) (gen.LogoutResponseObject, error) {
	if err := s.auth.Logout(ctx, metaFrom(ctx).sessionToken); err != nil {
		return nil, err
	}
	cookie := s.clearSessionCookie()
	return gen.Logout204Response{Headers: gen.Logout204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s *Server) BeginSetup(ctx context.Context, req gen.BeginSetupRequestObject) (gen.BeginSetupResponseObject, error) {
	c, err := s.auth.BeginSetup(ctx, req.Token)
	switch {
	case errors.Is(err, auth.ErrNotFound):
		return gen.BeginSetup404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgSetupLinkUnknown, nil))}, nil
	case errors.Is(err, auth.ErrSetupTokenExpired):
		return gen.BeginSetup410JSONResponse{GoneJSONResponse: gen.GoneJSONResponse(
			apiError(ctx, "setup_token_expired", msgSetupLinkExpired, nil))}, nil
	case err != nil:
		return nil, err
	}
	options, err := ceremonyOptions(c)
	if err != nil {
		return nil, err
	}
	return gen.BeginSetup200JSONResponse{CeremonyId: c.ID, Options: options}, nil
}

func (s *Server) FinishSetup(ctx context.Context, req gen.FinishSetupRequestObject) (gen.FinishSetupResponseObject, error) {
	if req.Body == nil {
		return gen.FinishSetup400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", "Некорректный запрос", nil))}, nil
	}
	credential, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, fmt.Errorf("encode credential: %w", err)
	}
	sess, err := s.auth.FinishSetup(ctx, auth.FinishSetupInput{
		Token:      req.Token,
		CeremonyID: req.Body.CeremonyId,
		Credential: credential,
		Name:       req.Body.Name,
		UserAgent:  metaFrom(ctx).userAgent,
	})
	var verr *auth.ValidationError
	switch {
	case errors.As(err, &verr):
		return gen.FinishSetup400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", verr.Message, fieldDetails(verr)))}, nil
	case errors.Is(err, auth.ErrNotFound):
		return gen.FinishSetup404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(
			apiError(ctx, "not_found", msgSetupLinkUnknown, nil))}, nil
	case errors.Is(err, auth.ErrSetupTokenExpired):
		return gen.FinishSetup410JSONResponse{GoneJSONResponse: gen.GoneJSONResponse(
			apiError(ctx, "setup_token_expired", msgSetupLinkExpired, nil))}, nil
	case errors.Is(err, auth.ErrVerificationFailed):
		return gen.FinishSetup422JSONResponse{WebAuthnFailedJSONResponse: gen.WebAuthnFailedJSONResponse(
			apiError(ctx, "webauthn_verification_failed", msgWebAuthnFailed, nil))}, nil
	case err != nil:
		return nil, err
	}
	cookie := s.sessionCookie(sess)
	return gen.FinishSetup204Response{Headers: gen.FinishSetup204ResponseHeaders{SetCookie: &cookie}}, nil
}

// sessionCookie — am_session=<token>; Path=/; Max-Age=SESSION_TTL; HttpOnly; Secure (https); SameSite=Lax.
func (s *Server) sessionCookie(sess auth.Session) string {
	return s.cookie(sess.Token, int(s.sessionTTL.Seconds()))
}

// clearSessionCookie — am_session=; Path=/; Max-Age=0; HttpOnly; Secure (https); SameSite=Lax.
func (s *Server) clearSessionCookie() string {
	return s.cookie("", -1) // MaxAge < 0 — net/http пишет Max-Age=0
}

func (s *Server) cookie(value string, maxAge int) string {
	c := &http.Cookie{ //nolint:gosec // G124: Secure только для https PUBLIC_BASE_URL, в разработке — http://localhost
		Name:     auth.SessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	return c.String()
}

// ceremonyOptions переводит опции go-webauthn в свободный объект options из WebAuthnCeremonyOptions.
func ceremonyOptions(c auth.Ceremony) (map[string]any, error) {
	var options map[string]any
	if err := json.Unmarshal(c.Options, &options); err != nil {
		return nil, fmt.Errorf("decode ceremony options: %w", err)
	}
	return options, nil
}

func fieldDetails(e *auth.ValidationError) map[string]any {
	return map[string]any{"fields": map[string]any{e.Field: e.Message}}
}
