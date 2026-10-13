package ingest_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/stretchr/testify/require"
)

var ingestSpecPath = filepath.Join("..", "..", "..", "contracts", "openapi", "ingest.yaml")

// ingestSpec — ingest.yaml, единственный источник контракта POST /v1/events.
func ingestSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(ingestSpecPath)
	require.NoError(t, err)
	require.NoError(t, doc.Validate(t.Context()))
	return doc
}

// requireMatchesSpec проверяет ответ на /v1/events против ingest.yaml: статус описан у операции, тело и
// заголовки соответствуют схемам (kin-openapi). Маршрутизатор не нужен: путь у ingest один.
func requireMatchesSpec(t *testing.T, doc *openapi3.T, method string, rec *httptest.ResponseRecorder) {
	t.Helper()
	pathItem := doc.Paths.Find("/v1/events")
	require.NotNil(t, pathItem)
	op := pathItem.GetOperation(method)
	require.NotNil(t, op, "операция %s /v1/events", method)

	req := httptest.NewRequestWithContext(t.Context(), method, "https://i.appmetrica.online/v1/events", nil)
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: req,
			Route:   &routers.Route{Spec: doc, Path: "/v1/events", PathItem: pathItem, Method: method, Operation: op},
		},
		Status:  rec.Code,
		Header:  rec.Header(),
		Body:    io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true},
	}
	require.NoError(t, openapi3filter.ValidateResponse(t.Context(), in), "ответ %d: %s", rec.Code, rec.Body.String())

	if rec.Code != http.StatusNoContent {
		require.NotEmpty(t, rec.Header().Get("X-Request-Id"))
	}
	if rec.Code == http.StatusTooManyRequests || rec.Code == http.StatusServiceUnavailable {
		require.NotEmpty(t, rec.Header().Get("Retry-After"), "Retry-After обязателен на 429 и 503")
	}
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

// TestIngestResponses_MatchOpenAPI — все векторы I-01…I-38 через собранный конвейер: каждый ответ
// совпадает с ожидаемым и валиден против ingest.yaml (AC-014.6).
func TestIngestResponses_MatchOpenAPI(t *testing.T) {
	doc := ingestSpec(t)
	vectors := ingestVectors()
	require.Len(t, vectors, 38, "векторы I-01…I-38 из 06-api-contracts.md §2.4.1")
	for _, v := range vectors {
		t.Run(v.id, func(t *testing.T) {
			_, recs := runVector(t, v)
			reqs := v.requests(t)
			for i, rec := range recs {
				method := reqs[i].method
				if method == "" {
					method = http.MethodPost
				}
				requireMatchesSpec(t, doc, method, rec)
			}
		})
	}
}

// TestIngestResponses_InternalErrorMatchesSpec — 500 internal_error тоже соответствует контракту.
func TestIngestResponses_InternalErrorMatchesSpec(t *testing.T) {
	doc := ingestSpec(t)
	h := newHarness(t)
	h.proc.err = errStorageDown
	rec := h.do(t, request{contentType: "application/json", header: testKey, body: []byte(batchI01)})
	requireError(t, rec, http.StatusInternalServerError, "internal_error")
	rec.Header().Set("Access-Control-Allow-Origin", "*") // harness без CORS
	requireMatchesSpec(t, doc, http.MethodPost, rec)
}
