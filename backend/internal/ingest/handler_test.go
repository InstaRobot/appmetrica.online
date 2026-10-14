package ingest_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/ratelimit"
)

const (
	testKey  = "pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2"
	otherKey = "pk_0000000000000000000000000000000Z"
)

// receivedAt — момент приёма из векторов 06-api-contracts.md §2.4.1.
var receivedAt = time.Date(2026, 10, 13, 9, 15, 2, 0, time.UTC)

// batchI01 — батч I-01: session_start + session_end iOS-приложения.
const batchI01 = `{
  "sent_at": "2026-10-13T09:15:02.120Z",
  "context": {"platform": "ios", "os_name": "iOS", "os_version": "26.0.1", "app_version": "3.4.0", "app_build": "341",
              "device_model": "iPhone17,3", "device_type": "phone", "locale": "ru-RU", "environment": "production"},
  "events": [
    {"insert_id": "0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61", "name": "session_start", "timestamp": "2026-10-13T09:11:58.004Z",
     "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21", "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f",
     "user_id": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90"},
    {"insert_id": "0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b", "name": "session_end", "timestamp": "2026-10-13T09:15:01.990Z",
     "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21", "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f",
     "user_id": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90", "duration_ms": 183986}
  ]
}`

// batchI18 — батч I-18: sendBeacon из браузера, ключ в теле.
const batchI18 = `{
  "app_key": "` + testKey + `",
  "sent_at": "2026-10-13T09:20:00.500+03:00",
  "context": {"platform": "web", "os_name": "macOS", "device_type": "desktop", "locale": "ru-RU"},
  "events": [
    {"insert_id": "0199d6b9-0a0b-7c0d-8e0f-101112131415", "name": "session_end", "timestamp": "2026-10-13T09:20:00.480+03:00",
     "install_id": "9b2e4c61-0f3a-4d8b-9e71-5c2a6d8f0b13", "session_id": "0199d6b0-1f2e-7d3c-8b4a-596877665544",
     "duration_ms": 412300}
  ]
}`

const event = `{"insert_id":"0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61","name":"session_start","timestamp":"2026-10-13T09:11:58.004Z",` +
	`"install_id":"6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21","session_id":"0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f"}`

func batchWithEvents(n int) string {
	return `{"sent_at":"2026-10-13T09:15:02.120Z","events":[` + strings.TrimSuffix(strings.Repeat(event+",", n), ",") + `]}`
}

// paddedBatch — валидный батч из одного события, дополненный неизвестным полем до size байт.
func paddedBatch(t *testing.T, size int) []byte {
	t.Helper()
	head := `{"sent_at":"2026-10-13T09:15:02.120Z","events":[` + event + `],"pad":"`
	tail := `"}`
	pad := size - len(head) - len(tail)
	require.Positive(t, pad)
	b := []byte(head + strings.Repeat("a", pad) + tail)
	require.Len(t, b, size)
	return b
}

func gzipped(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(b)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// fakeProcessor запоминает батчи и отвечает accepted = len(events) либо заданной ошибкой.
type fakeProcessor struct {
	mu      sync.Mutex
	batches []*ingest.Batch
	result  *ingestapi.IngestResult
	err     error
}

func (p *fakeProcessor) Process(_ context.Context, b *ingest.Batch) (ingestapi.IngestResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batches = append(p.batches, b)
	if p.err != nil {
		return ingestapi.IngestResult{}, p.err
	}
	if p.result != nil {
		return *p.result, nil
	}
	return ingestapi.IngestResult{
		Accepted:   len(b.Events),
		Rejected:   []ingestapi.Rejection{},
		Warnings:   []ingestapi.IngestWarning{},
		ServerTime: b.ReceivedAt,
	}, nil
}

func (p *fakeProcessor) calls() []*ingest.Batch {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.batches
}

type harness struct {
	handler http.Handler
	proc    *fakeProcessor
	reg     *prometheus.Registry
	logs    *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics, err := ingest.NewMetrics(reg)
	require.NoError(t, err)
	h := &harness{proc: &fakeProcessor{}, reg: reg, logs: &bytes.Buffer{}}
	c := clock.NewFixed(receivedAt)
	h.handler = httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: h.proc,
		Clock:     c,
		Logger:    slog.New(slog.NewJSONHandler(h.logs, nil)),
		Metrics:   metrics,
	}), httpx.RequestID(c))
	return h
}

type request struct {
	method      string // "" — POST
	headers     map[string]string
	contentType string
	encoding    string
	header      string // X-App-Key
	query       string
	body        []byte
}

func (h *harness) do(t *testing.T, r request) *httptest.ResponseRecorder {
	t.Helper()
	target := "/v1/events"
	if r.query != "" {
		target += "?" + r.query
	}
	method := r.method
	if method == "" {
		method = http.MethodPost
	}
	req := httptest.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(r.body))
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	if r.contentType != "" {
		req.Header.Set("Content-Type", r.contentType)
	}
	if r.encoding != "" {
		req.Header.Set("Content-Encoding", r.encoding)
	}
	if r.header != "" {
		req.Header.Set("X-App-Key", r.header)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

type errorEnvelope struct {
	Error struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		RequestID string         `json:"request_id"`
		Details   map[string]any `json:"details"`
	} `json:"error"`
}

// requireError проверяет статус, код и конверт ошибки по контракту Error из ingest.yaml.
func requireError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) errorEnvelope {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	var env errorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	require.Equal(t, code, env.Error.Code)
	require.NotEmpty(t, env.Error.Message)
	require.NotEmpty(t, env.Error.RequestID)
	require.Equal(t, rec.Header().Get("X-Request-Id"), env.Error.RequestID)
	return env
}

func requireAccepted(t *testing.T, rec *httptest.ResponseRecorder, accepted int) {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	var res ingestapi.IngestResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Equal(t, accepted, res.Accepted)
}

func TestIngestHandler_Transport(t *testing.T) {
	t.Run("I-16 gzip batch I-01", func(t *testing.T) {
		h := newHarness(t)
		rec := h.do(t, request{contentType: "application/json", encoding: "gzip", header: testKey, body: gzipped(t, []byte(batchI01))})
		requireAccepted(t, rec, 2)

		calls := h.proc.calls()
		require.Len(t, calls, 1)
		b := calls[0]
		require.Equal(t, testKey, b.AppKey)
		require.Equal(t, time.Date(2026, 10, 13, 9, 15, 2, 120_000_000, time.UTC), b.SentAt.UTC())
		require.Equal(t, receivedAt, b.ReceivedAt)
		require.Len(t, b.Events, 2)
		require.JSONEq(t, `"session_start"`, mustField(t, b.Events[0], "name"))
		require.JSONEq(t, `183986`, mustField(t, b.Events[1], "duration_ms"))
		require.JSONEq(t, `"ios"`, mustField(t, b.Context, "platform"))
	})

	t.Run("I-18 text/plain with app_key in body", func(t *testing.T) {
		h := newHarness(t)
		rec := h.do(t, request{contentType: "text/plain;charset=UTF-8", body: []byte(batchI18)})
		requireAccepted(t, rec, 1)

		calls := h.proc.calls()
		require.Len(t, calls, 1)
		require.Equal(t, testKey, calls[0].AppKey)
		require.Equal(t, time.Date(2026, 10, 13, 6, 20, 0, 500_000_000, time.UTC), calls[0].SentAt.UTC())
		require.Len(t, calls[0].Events, 1)
	})

	ok := []struct {
		name        string
		contentType string
		encoding    string
		body        []byte
		accepted    int
	}{
		{"plain json", "application/json", "", []byte(batchI01), 2},
		{"json with charset", "Application/JSON; charset=utf-8", "", []byte(batchI01), 2},
		{"identity encoding", "application/json", "identity", []byte(batchI01), 2},
		{"exactly 500 events", "application/json", "", []byte(batchWithEvents(500)), 500},
		{"body exactly 1 MiB after gunzip", "application/json", "gzip", nil, 1},
		{"null context", "application/json", "", []byte(`{"sent_at":"2026-10-13T09:15:02Z","context":null,"events":[` + event + `]}`), 1},
		{"unknown top-level field ignored", "application/json", "", []byte(`{"sdk":"x","sent_at":"2026-10-13T09:15:02Z","events":[` + event + `]}`), 1},
	}
	for _, tt := range ok {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			if body == nil {
				body = gzipped(t, paddedBatch(t, ingest.MaxBodyBytes))
			}
			h := newHarness(t)
			rec := h.do(t, request{contentType: tt.contentType, encoding: tt.encoding, header: testKey, body: body})
			requireAccepted(t, rec, tt.accepted)
			require.Len(t, h.proc.calls(), 1)
		})
	}

	fail := []struct {
		name        string
		contentType string
		encoding    string
		body        []byte
		status      int
		code        string
		field       string
	}{
		{"I-17 brotli", "application/json", "br", []byte(batchI01), http.StatusBadRequest, "unsupported_content_encoding", ""},
		{"I-27 form urlencoded", "application/x-www-form-urlencoded", "", []byte(batchI01), http.StatusUnsupportedMediaType, "unsupported_media_type", ""},
		{"no content type", "", "", []byte(batchI01), http.StatusUnsupportedMediaType, "unsupported_media_type", ""},
		{"malformed content type", "application/json; =", "", []byte(batchI01), http.StatusUnsupportedMediaType, "unsupported_media_type", ""},
		{"text/html", "text/html", "", []byte(batchI01), http.StatusUnsupportedMediaType, "unsupported_media_type", ""},
		{"I-25 truncated json", "application/json", "", []byte(`{"sent_at":`), http.StatusBadRequest, "invalid_json", ""},
		{"empty body", "application/json", "", nil, http.StatusBadRequest, "invalid_json", ""},
		{"trailing garbage", "application/json", "", []byte(batchI01 + `}`), http.StatusBadRequest, "invalid_json", ""},
		{"second json value", "application/json", "", []byte(batchI01 + ` {}`), http.StatusBadRequest, "invalid_json", ""},
		{"corrupt gzip header", "application/json", "gzip", []byte(batchI01), http.StatusBadRequest, "invalid_json", ""},
		{"I-23 501 events", "application/json", "", []byte(batchWithEvents(501)), http.StatusBadRequest, "invalid_batch", "events"},
		{"zero events", "application/json", "", []byte(`{"sent_at":"2026-10-13T09:15:02Z","events":[]}`), http.StatusBadRequest, "invalid_batch", "events"},
		{"events missing", "application/json", "", []byte(`{"sent_at":"2026-10-13T09:15:02Z"}`), http.StatusBadRequest, "invalid_batch", "events"},
		{"events null", "application/json", "", []byte(`{"sent_at":"2026-10-13T09:15:02Z","events":null}`), http.StatusBadRequest, "invalid_batch", "events"},
		{"events not array", "application/json", "", []byte(`{"sent_at":"2026-10-13T09:15:02Z","events":{"0":{}}}`), http.StatusBadRequest, "invalid_batch", "events"},
		{"I-26 sent_at missing", "application/json", "", []byte(`{"events":[` + event + `]}`), http.StatusBadRequest, "invalid_batch", "sent_at"},
		{"sent_at null", "application/json", "", []byte(`{"sent_at":null,"events":[` + event + `]}`), http.StatusBadRequest, "invalid_batch", "sent_at"},
		{"sent_at not a string", "application/json", "", []byte(`{"sent_at":1760346902,"events":[` + event + `]}`), http.StatusBadRequest, "invalid_batch", "sent_at"},
		{"sent_at without offset", "application/json", "", []byte(`{"sent_at":"2026-10-13T09:15:02","events":[` + event + `]}`), http.StatusBadRequest, "invalid_batch", "sent_at"},
		{"top-level array", "application/json", "", []byte(`[` + event + `]`), http.StatusBadRequest, "invalid_batch", ""},
		{"top-level null", "application/json", "", []byte(`null`), http.StatusBadRequest, "invalid_batch", ""},
		{"I-24 body 1 MiB + 1 byte after gunzip", "application/json", "gzip", nil, http.StatusRequestEntityTooLarge, "payload_too_large", ""},
	}
	for _, tt := range fail {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			if tt.code == "payload_too_large" {
				body = gzipped(t, paddedBatch(t, ingest.MaxBodyBytes+1))
				require.Less(t, len(body), 20*1024, "лимит считается по распакованному телу")
			}
			h := newHarness(t)
			rec := h.do(t, request{contentType: tt.contentType, encoding: tt.encoding, header: testKey, body: body})
			env := requireError(t, rec, tt.status, tt.code)
			if tt.field != "" {
				require.Equal(t, tt.field, env.Error.Details["field"])
				require.NotEmpty(t, env.Error.Details["issue"])
			}
			require.Empty(t, h.proc.calls(), "батч с ошибкой транспорта не обрабатывается")
		})
	}

	t.Run("truncated gzip stream", func(t *testing.T) {
		z := gzipped(t, []byte(batchI01))
		h := newHarness(t)
		rec := h.do(t, request{contentType: "application/json", encoding: "gzip", header: testKey, body: z[:len(z)/2]})
		requireError(t, rec, http.StatusBadRequest, "invalid_json")
		require.Empty(t, h.proc.calls())
	})

	t.Run("invalid_batch issues", func(t *testing.T) {
		cases := map[string]string{
			batchWithEvents(501):                               "max_items",
			`{"sent_at":"2026-10-13T09:15:02Z","events":[]}`:   "min_items",
			`{"sent_at":"2026-10-13T09:15:02Z"}`:               "required",
			`{"events":[` + event + `]}`:                       "required",
			`{"sent_at":"yesterday","events":[` + event + `]}`: "format",
		}
		for body, issue := range cases {
			h := newHarness(t)
			env := requireError(t, h.do(t, request{contentType: "application/json", header: testKey, body: []byte(body)}), http.StatusBadRequest, "invalid_batch")
			require.Equal(t, issue, env.Error.Details["issue"], body[:min(len(body), 60)])
		}
	})
}

func TestIngestHandler_AuthErrors(t *testing.T) {
	keyInBody := func(key string) []byte {
		return []byte(`{"app_key":"` + key + `","sent_at":"2026-10-13T09:15:02Z","events":[` + event + `]}`)
	}
	tests := []struct {
		name   string
		header string
		query  string
		body   []byte
		status int
		code   string
		field  string
		key    string
	}{
		{"I-20 no header and no app_key", "", "", []byte(batchI01), http.StatusUnauthorized, "missing_app_key", "", ""},
		{"key only in query string", "", "app_key=" + testKey, []byte(batchI01), http.StatusUnauthorized, "missing_app_key", "", ""},
		{"key in query and in header", testKey, "app_key=" + otherKey, []byte(batchI01), http.StatusOK, "", "", testKey},
		{"empty app_key in body", "", "", keyInBody(""), http.StatusUnauthorized, "missing_app_key", "", ""},
		{"null app_key in body", "", "", []byte(`{"app_key":null,"sent_at":"2026-10-13T09:15:02Z","events":[` + event + `]}`), http.StatusUnauthorized, "missing_app_key", "", ""},
		{"whitespace header", "   ", "", []byte(batchI01), http.StatusUnauthorized, "missing_app_key", "", ""},
		{"I-19 header and body differ", testKey, "", keyInBody(otherKey), http.StatusBadRequest, "invalid_batch", "app_key", ""},
		{"app_key not a string", "", "", []byte(`{"app_key":42,"sent_at":"2026-10-13T09:15:02Z","events":[` + event + `]}`), http.StatusBadRequest, "invalid_batch", "app_key", ""},
		{"header and body equal", testKey, "", keyInBody(testKey), http.StatusOK, "", "", testKey},
		{"header only", testKey, "", []byte(batchI01), http.StatusOK, "", "", testKey},
		{"body only", "", "", keyInBody(testKey), http.StatusOK, "", "", testKey},
		{"missing key checked before batch validation", "", "", []byte(batchWithEvents(501)), http.StatusUnauthorized, "missing_app_key", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			rec := h.do(t, request{contentType: "application/json", header: tt.header, query: tt.query, body: tt.body})
			if tt.status == http.StatusOK {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				calls := h.proc.calls()
				require.Len(t, calls, 1)
				require.Equal(t, tt.key, calls[0].AppKey)
				return
			}
			env := requireError(t, rec, tt.status, tt.code)
			if tt.field != "" {
				require.Equal(t, tt.field, env.Error.Details["field"])
			}
			require.Empty(t, h.proc.calls())
			require.NotContains(t, rec.Body.String(), testKey, "ключ не попадает в текст ошибки")
			require.NotContains(t, rec.Body.String(), otherKey)
		})
	}
}

func TestIngestHandler_ProcessorErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		code       string
		retryAfter string
	}{
		{
			"typed error with Retry-After",
			&ingest.Error{Status: http.StatusServiceUnavailable, Code: "storage_unavailable", Message: "Хранилище временно недоступно. Повторите весь батч.", RetryAfter: 5},
			http.StatusServiceUnavailable, "storage_unavailable", "5",
		},
		{
			"wrapped typed error",
			fmt.Errorf("auth: %w", &ingest.Error{Status: http.StatusUnauthorized, Code: "invalid_app_key", Message: "Ключ не найден или отозван."}),
			http.StatusUnauthorized, "invalid_app_key", "",
		},
		{"unknown error", errors.New("boom: connection reset"), http.StatusInternalServerError, "internal_error", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.proc.err = tt.err
			rec := h.do(t, request{contentType: "application/json", header: testKey, body: []byte(batchI01)})
			env := requireError(t, rec, tt.status, tt.code)
			require.Equal(t, tt.retryAfter, rec.Header().Get("Retry-After"))
			require.NotContains(t, env.Error.Message, "boom", "внутренние детали не уходят клиенту")
		})
	}

	t.Run("unknown error is logged without key and body", func(t *testing.T) {
		h := newHarness(t)
		h.proc.err = errors.New("boom")
		rec := h.do(t, request{contentType: "application/json", header: testKey, body: []byte(batchI01)})
		require.Equal(t, http.StatusInternalServerError, rec.Code)
		logs := h.logs.String()
		require.Contains(t, logs, "boom")
		require.Contains(t, logs, rec.Header().Get("X-Request-Id"))
		require.NotContains(t, logs, testKey)
		require.NotContains(t, logs, "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90", "user_id из тела не логируется")
	})
}

func TestIngestHandler_ResultArraysNeverNull(t *testing.T) {
	h := newHarness(t)
	h.proc.result = &ingestapi.IngestResult{Accepted: 2, ServerTime: receivedAt}
	rec := h.do(t, request{contentType: "application/json", header: testKey, body: []byte(batchI01)})
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"accepted":2,"rejected":[],"warnings":[],"server_time":"2026-10-13T09:15:02Z"}`, rec.Body.String())
}

func TestIngestHandler_RequestsMetric(t *testing.T) {
	h := newHarness(t)
	h.do(t, request{contentType: "application/json", header: testKey, body: []byte(batchI01)})
	h.do(t, request{contentType: "application/json", header: testKey, body: []byte(batchI01)})
	h.do(t, request{contentType: "text/html", header: testKey, body: []byte(batchI01)})
	h.do(t, request{contentType: "application/json", encoding: "br", header: testKey, body: []byte(batchI01)})
	h.do(t, request{contentType: "application/json", body: []byte(batchI01)})

	require.Equal(t, map[string]float64{"200": 2, "400": 1, "401": 1, "415": 1}, requestCounts(t, h.reg),
		"ошибка из Gunzip (400) тоже считается")

	_, err := ingest.NewMetrics(h.reg)
	require.Error(t, err, "повторная регистрация метрик в том же реестре — ошибка, а не паника")
}

// requestCounts — значения ingest_requests_total по метке status.
func requestCounts(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()
	return labelCounts(t, reg, "ingest_requests_total", "status")
}

// labelCounts — значения счётчика name по значению метки label.
func labelCounts(t *testing.T, reg *prometheus.Registry, name, label string) map[string]float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	out := map[string]float64{}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == label {
					out[l.GetValue()] = m.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

// newRateHarness — проверка ключа в хендлере, затем RateLimits.LimitApp → fakeProcessor; fakeProcessor стоит
// на месте записи в ClickHouse. Ключ testKey принадлежит appA, otherKey — appB.
func newRateHarness(t *testing.T, eventsPerSec float64, burst int) (*keyHarness, ingest.RateLimits) {
	t.Helper()
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive), keyRow(wk2, otherKey, appB, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))
	limits := ingest.RateLimits{
		Apps:     ratelimit.NewAppLimiter(eventsPerSec, burst, h.clock),
		Installs: ratelimit.NewInstallLimiter(100, 1000, h.clock),
		Metrics:  h.metrics,
	}
	h.setPipeline(limits.LimitApp(h.proc))
	return h, limits
}

func (h *keyHarness) postEvents(t *testing.T, key string, n int) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(t, request{contentType: "application/json", header: key, body: []byte(batchWithEvents(n))})
}

func TestIngestHandler_AppRateLimited(t *testing.T) {
	t.Run("burst 4000 exhausted (I-29)", func(t *testing.T) {
		h, limits := newRateHarness(t, 2000, 4000)
		ok, _ := limits.Apps.Allow(appA, 4000)
		require.True(t, ok)

		rec := h.postEvents(t, testKey, 10)
		env := requireError(t, rec, http.StatusTooManyRequests, "app_rate_limited")
		require.Equal(t, "Превышен лимит 2000 событий/с для приложения.", env.Error.Message)
		retryAfter, err := strconv.Atoi(rec.Header().Get("Retry-After"))
		require.NoError(t, err)
		require.GreaterOrEqual(t, retryAfter, 1)
		require.Empty(t, h.proc.calls(), "в CH ничего не записано")
		require.Equal(t, map[string]float64{"app": 1}, labelCounts(t, h.reg, "ingest_rate_limited_total", "scope"))
		require.Equal(t, map[string]float64{"429": 1}, requestCounts(t, h.reg))
	})

	t.Run("cost is the number of events (AC-1.5.4)", func(t *testing.T) {
		h, _ := newRateHarness(t, 10, 20)
		requireAccepted(t, h.postEvents(t, testKey, 15), 15)
		rec := h.postEvents(t, testKey, 15)
		requireError(t, rec, http.StatusTooManyRequests, "app_rate_limited")
		require.Equal(t, "1", rec.Header().Get("Retry-After"), "не хватает 10 событий при 10/с")
		require.Equal(t, map[string]float64{"app": 1}, labelCounts(t, h.reg, "ingest_rate_limited_total", "scope"))
		require.Len(t, h.proc.calls(), 1)

		h.clock.Advance(time.Second)
		requireAccepted(t, h.postEvents(t, testKey, 15), 15)
	})

	t.Run("Retry-After rounds up to whole seconds", func(t *testing.T) {
		h, _ := newRateHarness(t, 4, 20)
		requireAccepted(t, h.postEvents(t, testKey, 15), 15)
		rec := h.postEvents(t, testKey, 15)
		requireError(t, rec, http.StatusTooManyRequests, "app_rate_limited")
		require.Equal(t, "3", rec.Header().Get("Retry-After"), "10 событий при 4/с — 2,5 с")
		require.Equal(t, "Превышен лимит 4 событий/с для приложения.", mustErrorMessage(t, rec))
	})

	t.Run("sub-second wait is reported as 1", func(t *testing.T) {
		h, limits := newRateHarness(t, 2000, 4000)
		ok, _ := limits.Apps.Allow(appA, 3995)
		require.True(t, ok)
		rec := h.postEvents(t, testKey, 10)
		requireError(t, rec, http.StatusTooManyRequests, "app_rate_limited")
		require.Equal(t, "1", rec.Header().Get("Retry-After"))
	})

	t.Run("apps have independent buckets", func(t *testing.T) {
		h, _ := newRateHarness(t, 10, 20)
		requireAccepted(t, h.postEvents(t, testKey, 20), 20)
		requireError(t, h.postEvents(t, testKey, 1), http.StatusTooManyRequests, "app_rate_limited")
		requireAccepted(t, h.postEvents(t, otherKey, 20), 20)
	})

	t.Run("rejected keys do not spend the app budget", func(t *testing.T) {
		h, _ := newRateHarness(t, 10, 20)
		requireError(t, h.postEvents(t, "pk_unknownunknownunknownunknownun", 20), http.StatusUnauthorized, "invalid_app_key")
		requireAccepted(t, h.postEvents(t, testKey, 20), 20)
		require.Empty(t, labelCounts(t, h.reg, "ingest_rate_limited_total", "scope"))
	})
}

func mustErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env errorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Error.Message
}

func mustField(t *testing.T, raw json.RawMessage, name string) string {
	t.Helper()
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	v, ok := obj[name]
	require.True(t, ok, "нет поля %q", name)
	return string(v)
}

// TestIngestHandler_PartialSuccess — AC-014.2: событие с PII отклоняется, остальные записываются, отказ
// уходит в rejected_events (I-38).
func TestIngestHandler_PartialSuccess(t *testing.T) {
	h := newPipelineHarness(t)
	body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"app_version": "3.4.0", "platform": "ios"},
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "user_id": "ivan@example.com"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(2), "name": "session_end", "duration_ms": 4000}),
	)
	res := decodeResult(t, h.postBody(t, body))
	require.Equal(t, 2, res.Accepted)
	require.Len(t, res.Rejected, 1)
	rej := res.Rejected[0]
	require.Equal(t, 1, rej.Index)
	require.Equal(t, ingestapi.RejectionCodeUserIdLooksLikePii, rej.Code)
	require.False(t, rej.Retryable)
	require.Equal(t, numberedInsertID(1), rej.InsertId.MustGet())
	require.Empty(t, res.Warnings)

	rows := h.writer.written()
	require.Len(t, rows, 2)
	require.Equal(t, numberedInsertID(0), rows[0].InsertID.String())
	require.Equal(t, numberedInsertID(2), rows[1].InsertID.String())

	logged := h.sink.logged()
	require.Len(t, logged, 1, "в rejected_events одна строка")
	r := logged[0]
	require.Equal(t, appA, r.AppID)
	require.Equal(t, testKey[:11], r.KeyPrefix)
	require.Equal(t, "user_id_looks_like_pii", r.Code)
	require.Equal(t, "events[1].user_id", r.Field)
	require.Equal(t, rej.Message, r.Message)
	require.Equal(t, "session_start", r.EventName)
	require.Equal(t, "3.4.0", r.AppVersion)
	require.Equal(t, "ios", r.Platform)
	require.Equal(t, receivedAt, r.ReceivedAt)
	require.NotContains(t, r.PayloadSample, "ivan@example.com")
	require.NotContains(t, h.logs.String(), "ivan@example.com", "user_id не логируется")
}

// TestIngestHandler_DuplicateInBatch — AC-014.3 (I-12): второй insert_id пропускается с warning без rejection.
func TestIngestHandler_DuplicateInBatch(t *testing.T) {
	h := newPipelineHarness(t)
	body := batchBody(t, "2026-10-13T09:15:02.000Z", nil, eventJSON(t, nil), eventJSON(t, nil))
	res := decodeResult(t, h.postBody(t, body))
	require.Equal(t, 1, res.Accepted)
	require.Empty(t, res.Rejected)
	require.Len(t, res.Warnings, 1)
	require.Equal(t, 1, res.Warnings[0].Index)
	require.Equal(t, ingestapi.WarningCodeDuplicateInsertIdInBatch, res.Warnings[0].Code)
	require.Len(t, h.writer.written(), 1)
	require.Empty(t, h.sink.logged())

	t.Run("duplicate of a rejected event is processed", func(t *testing.T) {
		h := newPipelineHarness(t)
		body := batchBody(t, "2026-10-13T09:15:02.000Z", nil,
			eventJSON(t, map[string]any{"user_id": "ivan@example.com"}), eventJSON(t, nil))
		res := decodeResult(t, h.postBody(t, body))
		require.Equal(t, 1, res.Accepted)
		require.Len(t, res.Rejected, 1)
		require.Empty(t, res.Warnings)
	})

	t.Run("duplicates do not spend the install budget", func(t *testing.T) {
		h := newPipelineHarness(t)
		// 500 событий с одним insert_id: лимит установки списывает только первое.
		events := make([]json.RawMessage, 500)
		for i := range events {
			events[i] = eventJSON(t, nil)
		}
		res := decodeResult(t, h.postBody(t, batchBody(t, "2026-10-13T09:15:02.000Z", nil, events...)))
		require.Equal(t, 1, res.Accepted)
		require.Len(t, res.Warnings, 499)
		ok, _ := h.limits.Installs.Allow(installKey(appA, validInstallID), 999)
		require.True(t, ok, "в бакете установки осталось 999 токенов")
	})
}

// TestIngestHandler_StorageUnavailable — AC-014.4 (I-35): ошибка или таймаут вставки — 503 с Retry-After: 5,
// 200 не отправляется, отказы не пишутся (клиент повторит весь батч).
func TestIngestHandler_StorageUnavailable(t *testing.T) {
	t.Run("insert error", func(t *testing.T) {
		h := newPipelineHarness(t)
		h.writer.fail(errStorageDown)
		body := batchBody(t, "2026-10-13T09:15:02.000Z", nil,
			eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
			eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "user_id": "ivan@example.com"}))
		rec := h.postBody(t, body)
		env := requireError(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
		require.Equal(t, "5", rec.Header().Get("Retry-After"))
		require.NotContains(t, env.Error.Message, "dial tcp", "внутренние детали не уходят клиенту")
		require.Empty(t, h.sink.logged(), "отказы не пишутся, пока батч не принят")
		require.InDelta(t, 1.0, metricValue(t, h.reg, "ingest_clickhouse_insert_errors_total"), 0)
		require.Equal(t, map[string]float64{"503": 1}, requestCounts(t, h.reg))
		require.Empty(t, labelCounts(t, h.reg, "ingest_events_accepted_total", "app_id"))

		logs := h.logs.String()
		require.Contains(t, logs, "connection refused")
		require.Contains(t, logs, rec.Header().Get("X-Request-Id"))
		require.Contains(t, logs, appA.String())
		require.NotContains(t, logs, testKey)
	})

	t.Run("insert timeout", func(t *testing.T) {
		h := newPipelineHarness(t, func(c *ingest.PipelineConfig) { c.InsertTimeout = 20 * time.Millisecond })
		h.writer.mu.Lock()
		h.writer.block = true
		h.writer.mu.Unlock()
		rec := h.postBody(t, []byte(batchI01))
		requireError(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
		require.Equal(t, "5", rec.Header().Get("Retry-After"))
	})

	t.Run("recovers when ClickHouse is back", func(t *testing.T) {
		h := newPipelineHarness(t)
		h.writer.fail(errStorageDown)
		requireError(t, h.postBody(t, []byte(batchI01)), http.StatusServiceUnavailable, "storage_unavailable")
		h.writer.fail(nil)
		requireAccepted(t, h.postBody(t, []byte(batchI01)), 2)
	})

	t.Run("all events rejected — no insert", func(t *testing.T) {
		h := newPipelineHarness(t)
		h.writer.fail(errStorageDown)
		res := decodeResult(t, h.postBody(t, oneEvent(t, map[string]any{"name": "purchase"})))
		require.Zero(t, res.Accepted)
		require.Zero(t, h.writer.insertCalls(), "пустой батч в ClickHouse не отправляется")
		require.Len(t, h.sink.logged(), 1)
	})
}

// TestIngestHandler_MixedInstalls — лимит установки считается по каждому install_id отдельно.
func TestIngestHandler_MixedInstalls(t *testing.T) {
	h := newPipelineHarness(t)
	ok, _ := h.limits.Installs.Allow(installKey(appA, validInstallID), 1000)
	require.True(t, ok, "бакет первой установки исчерпан")
	other := "9b2e4c61-0f3a-4d8b-9e71-5c2a6d8f0b13"
	body := batchBody(t, "2026-10-13T09:15:02.000Z", nil,
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "install_id": other}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(2), "install_id": other}),
	)
	res := decodeResult(t, h.postBody(t, body))
	require.Equal(t, 2, res.Accepted)
	require.Len(t, res.Rejected, 1)
	require.Equal(t, 0, res.Rejected[0].Index)
	require.Equal(t, ingestapi.RejectionCodeInstallRateLimited, res.Rejected[0].Code)
	require.True(t, res.Rejected[0].Retryable)
	for _, r := range h.writer.written() {
		require.Equal(t, other, r.InstallID.String())
	}
	require.Equal(t, map[string]float64{"install": 1}, labelCounts(t, h.reg, "ingest_rate_limited_total", "scope"))
}

// TestIngestHandler_UnknownFieldsIgnored — неизвестные поля батча, события и context не мешают приёму.
func TestIngestHandler_UnknownFieldsIgnored(t *testing.T) {
	h := newPipelineHarness(t)
	body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"platform": "ios", "screen": "1170x2532"},
		eventJSON(t, map[string]any{"foo": 1, "context": map[string]any{"bar": true}}))
	body = bytes.Replace(body, []byte(`{`), []byte(`{"sdk_version":"x",`), 1)
	requireAccepted(t, h.postBody(t, body), 1)
	require.Len(t, h.writer.written(), 1)
}

func installKey(app uuid.UUID, installID string) ratelimit.InstallKey {
	return ratelimit.InstallKey{AppID: app, InstallID: uuid.MustParse(installID)}
}

// 06-api-contracts.md §2.4.1: ключ проверяется до полей батча — неизвестный ключ и архив получают 401/403,
// даже если батч невалиден; с рабочим ключом тот же батч — 400 invalid_batch.
func TestIngestHandler_KeyCheckedBeforeBatchFields(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive), keyRow(wk2, otherKey, appB, sqlc.AppStatusArchived))
	require.NoError(t, h.cache.Refresh(t.Context()))
	malformed := []byte(`{"events":[]}`)
	post := func(key string) *httptest.ResponseRecorder {
		return h.do(t, request{contentType: "application/json", header: key, body: malformed})
	}

	requireError(t, post("pk_unknownunknownunknownunknownun"), http.StatusUnauthorized, "invalid_app_key")
	requireError(t, post(otherKey), http.StatusForbidden, "app_archived")
	env := requireError(t, post(testKey), http.StatusBadRequest, "invalid_batch")
	require.Equal(t, "sent_at", env.Error.Details["field"])
	require.Empty(t, h.proc.calls())
}

// INGEST_MAX_EVENTS и INGEST_MAX_BODY_BYTES: лимиты хендлера и тексты ошибок берутся из конфига.
func TestIngestHandler_ConfiguredLimits(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := ingest.NewMetrics(reg)
	require.NoError(t, err)
	h := &harness{proc: &fakeProcessor{}, reg: reg, logs: &bytes.Buffer{}}
	c := clock.NewFixed(receivedAt)
	h.handler = httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: h.proc, Clock: c, Logger: slog.New(slog.NewJSONHandler(h.logs, nil)), Metrics: metrics,
		MaxEvents: 2, MaxBodyBytes: 2048,
	}), httpx.RequestID(c))
	post := func(body []byte) *httptest.ResponseRecorder {
		return h.do(t, request{contentType: "application/json", header: testKey, body: body})
	}

	requireAccepted(t, post([]byte(batchWithEvents(2))), 2)
	env := requireError(t, post([]byte(batchWithEvents(3))), http.StatusBadRequest, "invalid_batch")
	require.Equal(t, "max_items", env.Error.Details["issue"])
	require.Contains(t, env.Error.Message, "от 1 до 2 событий")

	env = requireError(t, post(paddedBatch(t, 2049)), http.StatusRequestEntityTooLarge, "payload_too_large")
	require.Contains(t, env.Error.Message, "больше 2 048 байт")
	require.Len(t, h.proc.calls(), 1)
}
