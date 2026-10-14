// Package api — HTTP-слой Read API (/v1) и admin API (/admin/v1) поверх strict-сервера oapi-codegen:
// преобразует доменные ответы и ошибки в модели и коды из contracts/openapi/api.yaml.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/deletion"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/reports"
)

// MaxBodyBytes — лимит тела любого запроса API (http.MaxBytesReader в Register). Самое большое легитимное
// тело — attestation WebAuthn при регистрации passkey (единицы КБ); превышение → 413 payload_too_large.
const MaxBodyBytes int64 = 64 << 10

// Server реализует операции api.yaml. Операции других тикетов приходят из встроенного
// gen.StrictServerInterface (nil) и недостижимы: Register подключает только реализованные маршруты.
type Server struct {
	gen.StrictServerInterface

	auth          *auth.Service
	apps          *apps.Service
	reports       *reports.Service  // nil — отчёты не подключены (WithReports)
	deletion      *deletion.Service // nil — запросы на удаление не подключены (WithDeletion)
	merges        *merge.Service    // nil — слияние приложений не подключено (WithMerges)
	settings      SettingsReader
	settingsStore SettingsStore  // nil — настройки не подключены (WithSettings)
	telegram      TelegramSender // nil — TELEGRAM_BOT_TOKEN не задан
	system        *SystemSources // nil — статус системы не подключён (WithSystemStatus)
	clock         clock.Clock
	ready         []httpx.ReadyCheck
	metrics       *metrics // nil — метрики не подключены (RegisterMetrics)
	logger        *slog.Logger
	appEnv        config.Env
	publicBaseURL string
	secureCookies bool
	sessionTTL    time.Duration
}

func NewServer(authSvc *auth.Service, appsSvc *apps.Service, logger *slog.Logger, cfg config.API) *Server {
	return &Server{
		auth:          authSvc,
		apps:          appsSvc,
		logger:        logger,
		appEnv:        cfg.AppEnv,
		publicBaseURL: cfg.PublicBaseURL,
		secureCookies: strings.HasPrefix(cfg.PublicBaseURL, "https://"),
		sessionTTL:    cfg.SessionTTL,
	}
}

// WithDeletion подключает запросы на удаление данных: createDeletionRequest, getDeletionRequest,
// listDeletionRequests.
func (s *Server) WithDeletion(svc *deletion.Service) *Server {
	s.deletion = svc
	return s
}

// WithReadyChecks задаёт проверки хранилищ для GET /ready.
func (s *Server) WithReadyChecks(checks ...httpx.ReadyCheck) *Server {
	s.ready = checks
	return s
}

// ApiReady — GET /ready: 200 ready, если все проверки прошли, иначе 503 not_ready.
//
//nolint:revive // имя метода задаёт gen.StrictServerInterface (operationId apiReady).
func (s *Server) ApiReady(ctx context.Context, _ gen.ApiReadyRequestObject) (gen.ApiReadyResponseObject, error) {
	ready, results := httpx.RunReadyChecks(ctx, s.ready)
	st := gen.ReadyStatus{Status: gen.ReadyStatusStatusReady, Checks: make(map[string]gen.ReadyStatusChecks, len(results))}
	for name, ok := range results {
		st.Checks[name] = gen.ReadyStatusChecksFail
		if ok {
			st.Checks[name] = gen.ReadyStatusChecksOk
		}
	}
	if !ready {
		st.Status = gen.ReadyStatusStatusNotReady
		return gen.ApiReady503JSONResponse(st), nil
	}
	return gen.ApiReady200JSONResponse(st), nil
}

// Register подключает реализованные операции к mux. Пути и методы — как в api.yaml.
// CSRF стоит перед разбором параметров: запрос без X-Requested-With получает 403 csrf_check_failed, а не 400.
func (s *Server) Register(mux *http.ServeMux) {
	si := gen.NewStrictHandlerWithOptions(s, []gen.StrictMiddlewareFunc{withRequestMeta, withOperation}, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  invalidRequest,
		ResponseErrorHandlerFunc: s.responseError,
	})
	w := &gen.ServerInterfaceWrapper{
		Handler:            si,
		HandlerMiddlewares: []gen.MiddlewareFunc{s.metrics.observe},
		ErrorHandlerFunc:   invalidRequest,
	}
	body := httpx.BodyLimit(MaxBodyBytes)
	csrf := auth.CSRF(s.publicBaseURL, writeError)
	limit := s.auth.LimitAttempts(writeError)
	session := s.auth.RequireSession(writeError)

	mux.HandleFunc("GET /ready", w.ApiReady)

	public := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, body(csrf(h))) }
	attempt := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, body(csrf(limit(h)))) }
	owner := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, body(csrf(session(h)))) }

	public("GET /admin/v1/auth/state", w.GetAuthState)
	attempt("POST /admin/v1/auth/login/begin", w.BeginLogin)
	attempt("POST /admin/v1/auth/login/finish", w.FinishLogin)
	attempt("POST /admin/v1/auth/setup/{token}/begin", w.BeginSetup)
	attempt("POST /admin/v1/auth/setup/{token}/finish", w.FinishSetup)
	public("POST /admin/v1/auth/logout", w.Logout)

	owner("GET /admin/v1/passkeys", w.ListPasskeys)
	owner("POST /admin/v1/passkeys/register/begin", w.BeginPasskeyRegistration)
	owner("POST /admin/v1/passkeys/register/finish", w.FinishPasskeyRegistration)
	owner("PATCH /admin/v1/passkeys/{passkey_id}", w.RenamePasskey)
	owner("DELETE /admin/v1/passkeys/{passkey_id}", w.DeletePasskey)

	// Read API: cookie владельца или Bearer sk_ со scope read; удаление данных — со scope delete,
	// статус удаления — с любым из двух.
	apiAccess := func(scopes ...string) func(string, http.HandlerFunc) {
		access := s.auth.RequireAPIAccess(writeError, scopes...)
		return func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, body(csrf(access(h)))) }
	}
	reader := apiAccess(auth.ScopeRead)

	reader("GET /v1/apps", w.ListApps)
	reader("GET /v1/apps/{app_id}", w.GetApp)
	if s.reports != nil {
		reader("GET /v1/portfolio", w.GetPortfolio)
		reader("GET /v1/apps/{app_id}/audience/summary", w.GetAudienceSummary)
		reader("GET /v1/apps/{app_id}/audience/timeseries", w.GetAudienceTimeseries)
		reader("GET /v1/apps/{app_id}/audience/rolling-active", w.GetRollingActive)
		reader("GET /v1/apps/{app_id}/audience/breakdown", w.GetAudienceBreakdown)
		reader("GET /v1/apps/{app_id}/retention", w.GetRetention)
		reader("GET /v1/apps/{app_id}/realtime", w.GetRealtime)
		reader("GET /v1/apps/{app_id}/profiles", w.SearchProfiles)
		reader("GET /v1/apps/{app_id}/profiles/{person_key}", w.GetProfile)
		reader("GET /v1/apps/{app_id}/integration/health", w.GetIntegrationHealth)
	}
	if s.deletion != nil {
		apiAccess(auth.ScopeDelete)("POST /v1/apps/{app_id}/deletion-requests", w.CreateDeletionRequest)
		apiAccess(auth.ScopeRead, auth.ScopeDelete)("GET /v1/apps/{app_id}/deletion-requests/{request_id}", w.GetDeletionRequest)
		owner("GET /admin/v1/deletion-requests", w.ListDeletionRequests)
	}

	if s.settingsStore != nil {
		owner("GET /admin/v1/settings", w.GetSettings)
		owner("PATCH /admin/v1/settings", w.UpdateSettings)
		owner("POST /admin/v1/settings/telegram/test", w.SendTelegramTest)
	}
	if s.system != nil {
		owner("GET /admin/v1/system/status", w.GetSystemStatus)
	}

	owner("POST /admin/v1/apps", w.CreateApp)
	owner("PATCH /admin/v1/apps/{app_id}", w.UpdateApp)
	owner("DELETE /admin/v1/apps/{app_id}", w.DeleteApp)
	owner("POST /admin/v1/apps/{app_id}/archive", w.ArchiveApp)
	owner("POST /admin/v1/apps/{app_id}/unarchive", w.UnarchiveApp)

	owner("GET /admin/v1/apps/{app_id}/write-keys", w.ListWriteKeys)
	owner("POST /admin/v1/apps/{app_id}/write-keys", w.CreateWriteKey)
	owner("POST /admin/v1/apps/{app_id}/write-keys/{key_id}/revoke", w.RevokeWriteKey)
	if s.merges != nil {
		owner("GET /admin/v1/apps/{app_id}/merge-preview", w.GetMergePreview)
		owner("GET /admin/v1/apps/{app_id}/merges", w.ListAppMerges)
		owner("POST /admin/v1/apps/{app_id}/merges", w.CreateAppMerge)
		owner("POST /admin/v1/apps/{app_id}/merges/{merge_id}/retry", w.RetryAppMerge)
	}
	owner("GET /admin/v1/api-keys", w.ListApiKeys)
	owner("POST /admin/v1/api-keys", w.CreateApiKey)
	owner("POST /admin/v1/api-keys/{key_id}/revoke", w.RevokeApiKey)

	if s.appEnv == config.EnvE2E {
		s.registerE2E(mux)
	}
}

// writeError — auth.ErrorWriter поверх конверта ошибки httpx.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	httpx.WriteError(w, r, status, code, message, nil)
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
// он может процитировать присланные значения; для параметра в details.field — его имя.
// Тело больше MaxBodyBytes (ошибка http.MaxBytesReader при декодировании) — 413 payload_too_large.
func invalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large",
			fmt.Sprintf("Тело запроса больше %d байт.", MaxBodyBytes), nil)
		return
	}
	if name := paramName(err); name != "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Некорректный параметр "+name+".",
			map[string]any{"field": name})
		return
	}
	httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request", "Некорректный запрос", nil)
}

// paramName — имя параметра из ошибки разбора обёртки oapi-codegen; "" — ошибка не про параметр.
func paramName(err error) string {
	var (
		format   *gen.InvalidParamFormatError
		required *gen.RequiredParamError
		many     *gen.TooManyValuesForParamError
		unmarsh  *gen.UnmarshalingParamError
	)
	switch {
	case errors.As(err, &format):
		return format.ParamName
	case errors.As(err, &required):
		return required.ParamName
	case errors.As(err, &many):
		return many.ParamName
	case errors.As(err, &unmarsh):
		return unmarsh.ParamName
	default:
		return ""
	}
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
