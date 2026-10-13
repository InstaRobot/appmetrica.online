// Package api — HTTP-слой Read API (/v1) и admin API (/admin/v1) поверх strict-сервера oapi-codegen:
// преобразует доменные ответы и ошибки в модели и коды из contracts/openapi/api.yaml.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/config"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
)

// Server реализует операции api.yaml. Операции других тикетов приходят из встроенного
// gen.StrictServerInterface (nil) и недостижимы: Register подключает только реализованные маршруты.
type Server struct {
	gen.StrictServerInterface

	auth          *auth.Service
	logger        *slog.Logger
	secureCookies bool
	sessionTTL    time.Duration
}

func NewServer(authSvc *auth.Service, logger *slog.Logger, cfg config.API) *Server {
	return &Server{
		auth:          authSvc,
		logger:        logger,
		secureCookies: strings.HasPrefix(cfg.PublicBaseURL, "https://"),
		sessionTTL:    cfg.SessionTTL,
	}
}

// Register подключает реализованные операции к mux. Пути и методы — как в api.yaml.
func (s *Server) Register(mux *http.ServeMux) {
	si := gen.NewStrictHandlerWithOptions(s, []gen.StrictMiddlewareFunc{withRequestMeta}, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  invalidRequest,
		ResponseErrorHandlerFunc: s.internalError,
	})
	w := &gen.ServerInterfaceWrapper{Handler: si, ErrorHandlerFunc: invalidRequest}

	mux.HandleFunc("GET /admin/v1/auth/state", w.GetAuthState)
	mux.HandleFunc("POST /admin/v1/auth/setup/{token}/begin", w.BeginSetup)
	mux.HandleFunc("POST /admin/v1/auth/setup/{token}/finish", w.FinishSetup)
}

// requestMeta — данные запроса, которых нет в RequestObject strict-сервера.
type requestMeta struct {
	sessionToken string
	userAgent    string
}

type requestMetaKey struct{}

func withRequestMeta(f gen.StrictHandlerFunc, _ string) gen.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		m := requestMeta{userAgent: r.UserAgent()}
		if c, err := r.Cookie(auth.SessionCookie); err == nil {
			m.sessionToken = c.Value
		}
		return f(context.WithValue(ctx, requestMetaKey{}, m), w, r, request)
	}
}

func metaFrom(ctx context.Context) requestMeta {
	m, _ := ctx.Value(requestMetaKey{}).(requestMeta)
	return m
}

// invalidRequest — тело или параметры не разобрались по схеме. Текст ошибки разбора не возвращается:
// он может процитировать присланные значения.
func invalidRequest(w http.ResponseWriter, r *http.Request, _ error) {
	httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Некорректный запрос", nil)
}

// internalError — обработчик вернул ошибку вместо ответа из контракта.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.ErrorContext(r.Context(), "request failed",
		slog.String("request_id", httpx.RequestIDFrom(r.Context())), slog.Any("error", err))
	httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка. Повторите запрос позже.", nil)
}

// apiError — конверт ошибки {"error":{code,message,request_id,details}}.
func apiError(ctx context.Context, code, message string, details map[string]any) gen.Error {
	var e gen.Error
	e.Error.Code = code
	e.Error.Message = message
	e.Error.RequestId = httpx.RequestIDFrom(ctx)
	if details != nil {
		e.Error.Details = &details
	}
	return e
}
