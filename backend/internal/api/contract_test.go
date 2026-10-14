//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/testutil"
)

// pendingOperations — операции /v1 из api.yaml, которые ещё не реализованы: удаление данных (T-045).
var pendingOperations = []string{"createDeletionRequest", "getDeletionRequest"}

func loadAPIDoc(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "contracts", "openapi", "api.yaml"))
	require.NoError(t, err)
	require.NoError(t, doc.Validate(t.Context()))
	// Маршрут ищется по пути и методу: хост httptest не совпадает ни с одним servers[].url.
	doc.Servers = nil
	return doc
}

// validateResponse проверяет ответ rec на запрос req против api.yaml: статус описан у операции, тело и
// заголовки (Retry-After) соответствуют схемам, форматы date/date-time/uuid проверяются. Возвращает operationId.
func validateResponse(t *testing.T, doc *openapi3.T, req *http.Request, rec *httptest.ResponseRecorder) string {
	t.Helper()
	router, err := legacy.NewRouter(doc)
	require.NoError(t, err)
	route, params, err := router.FindRoute(req)
	require.NoError(t, err, "%s %s", req.Method, req.URL)
	opts := &openapi3filter.Options{
		IncludeResponseStatus:   true,
		MultiError:              true,
		SchemaValidationOptions: []openapi3.SchemaValidationOption{openapi3.EnableFormatValidation()},
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: opts},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Options:                opts,
	}
	in.SetBodyBytes(rec.Body.Bytes())
	require.NoError(t, openapi3filter.ValidateResponse(t.Context(), in),
		"%s %s → %d %s", req.Method, req.URL, rec.Code, rec.Body.String())
	return route.Operation.OperationID
}

// readOperations — GET-операции Read API (/v1) и health из api.yaml.
func readOperations(doc *openapi3.T) []string {
	var ids []string
	for path, item := range doc.Paths.Map() {
		if !strings.HasPrefix(path, "/v1/") && path != "/ready" {
			continue
		}
		for _, op := range item.Operations() {
			if !slices.Contains(pendingOperations, op.OperationID) {
				ids = append(ids, op.OperationID)
			}
		}
	}
	slices.Sort(ids)
	return ids
}

// newKey — отдельный ключ sk_ read: у каждого ключа свой лимит 10 rps (burst 30), а часы в тестах стоят.
func (e *portfolioEnv) newKey(t *testing.T) string {
	t.Helper()
	_, secret, err := e.auth.CreateAPIKey(t.Context(), auth.CreateAPIKeyInput{Label: "contract", Scopes: []string{auth.ScopeRead}})
	require.NoError(t, err)
	return secret
}

func TestReadAPI_ResponsesMatchOpenAPI(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	msk := testutil.ScenarioApps[0]
	app := "/v1/apps/" + msk.ID.String()
	const period = "from=2026-10-01&to=2026-10-12"

	// person_key реального профиля сценария — через поиск по его установке.
	rec := e.get(t, app+"/profiles?q="+testutil.ScenarioInstallID(msk, 0).String(), e.all)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var found gen.ProfileSearchResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &found))
	require.NotEmpty(t, found.Items)
	profile := app + "/profiles/" + url.PathEscape(found.Items[0].PersonKey)

	cases := []struct {
		path   string
		status int
	}{
		{"/ready", http.StatusOK},
		{"/v1/apps", http.StatusOK},
		{"/v1/apps?include_archived=true", http.StatusOK},
		{app, http.StatusOK},
		{"/v1/portfolio", http.StatusOK},
		{"/v1/portfolio?date=2026-10-12&unit=installs&include_archived=true", http.StatusOK},
		{"/v1/portfolio?date=2026-10-12&environment=development", http.StatusOK},
		{app + "/audience/summary?" + period, http.StatusOK},
		{app + "/audience/summary?" + period + "&compare=previous_period&platform=ios&country=RU", http.StatusOK},
		{app + "/audience/timeseries?" + period + "&granularity=day&metric=active&compare=none", http.StatusOK},
		{app + "/audience/timeseries?from=2026-10-10&to=2026-10-12&granularity=hour&metric=avg_duration_ms", http.StatusOK},
		{app + "/audience/timeseries?from=2026-07-01&to=2026-10-12&granularity=month&metric=new", http.StatusOK},
		{app + "/audience/rolling-active?" + period, http.StatusOK},
		{app + "/audience/breakdown?" + period + "&dimension=country", http.StatusOK},
		{app + "/audience/breakdown?" + period + "&dimension=app_version&metric=sessions&limit=5", http.StatusOK},
		{app + "/retention?from=2026-09-14&to=2026-10-12", http.StatusOK},
		{app + "/retention?from=2026-08-03&to=2026-10-11&cohort_granularity=week&periods=8", http.StatusOK},
		{app + "/realtime", http.StatusOK},
		{app + "/profiles?q=" + testutil.ScenarioInstallID(msk, 0).String(), http.StatusOK},
		{app + "/profiles?q=nobody", http.StatusOK},
		{profile, http.StatusOK},
		{profile + "?sessions_limit=1", http.StatusOK},
		{app + "/integration/health", http.StatusOK},
		{app + "/integration/health?hours=168", http.StatusOK},

		// Ошибки — тот же конверт Error, статусы из операций api.yaml.
		{"/v1/apps/" + uuid.NewString(), http.StatusNotFound},
		{"/v1/apps/not-a-uuid", http.StatusBadRequest},
		{"/v1/portfolio?date=yesterday", http.StatusBadRequest},
		{app + "/audience/summary?from=2024-01-01&to=2026-10-12", http.StatusBadRequest},
		{app + "/audience/timeseries?from=2026-09-01&to=2026-10-12&granularity=hour", http.StatusBadRequest},
		{app + "/audience/breakdown?" + period, http.StatusBadRequest},
		{app + "/retention?from=2026-10-12&to=2026-10-01", http.StatusBadRequest},
		{app + "/profiles", http.StatusBadRequest},
		{app + "/profiles/" + url.PathEscape("u:nobody"), http.StatusNotFound},
		{app + "/integration/health?hours=169", http.StatusBadRequest},
		{"/v1/apps/" + uuid.NewString() + "/realtime", http.StatusNotFound},
	}

	called := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+e.newKey(t))
			rec := serveRequest(t, e.handler, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.True(t, httpx.ValidULID(rec.Header().Get(httpx.HeaderRequestID)))
			op := validateResponse(t, doc, req, rec)
			if rec.Code == http.StatusOK {
				called[op] = true
			}
		})
	}

	// Каждая реализованная операция /v1 проверена хотя бы одним успешным ответом.
	var missing []string
	for _, op := range readOperations(doc) {
		if !called[op] {
			missing = append(missing, op)
		}
	}
	require.Empty(t, missing, "операции без успешного ответа в тесте")
}
