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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
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
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewReader(r.body))
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
	families, err := reg.Gather()
	require.NoError(t, err)
	out := map[string]float64{}
	for _, mf := range families {
		if mf.GetName() != "ingest_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "status" {
					out[l.GetValue()] = m.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

func mustField(t *testing.T, raw json.RawMessage, name string) string {
	t.Helper()
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	v, ok := obj[name]
	require.True(t, ok, "нет поля %q", name)
	return string(v)
}
