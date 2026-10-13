package ingest_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/ratelimit"
)

// archivedKey — write-ключ архивного приложения appB в pipelineHarness.
const archivedKey = "pk_ArchArchArchArchArchArchArchArc"

// fakeWriter — ClickHouse events: принятые строки или ошибка вставки. block=true — InsertEvents ждёт отмены ctx
// (зависший ClickHouse).
type fakeWriter struct {
	mu    sync.Mutex
	rows  []chstore.EventRow
	calls int
	err   error
	block bool
}

func (w *fakeWriter) InsertEvents(ctx context.Context, rows []chstore.EventRow) error {
	w.mu.Lock()
	w.calls++
	err, block := w.err, w.block
	w.mu.Unlock()
	if block {
		<-ctx.Done()
		return fmt.Errorf("insert events: %w", ctx.Err())
	}
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rows = append(w.rows, rows...)
	return nil
}

func (w *fakeWriter) written() []chstore.EventRow {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.rows)
}

func (w *fakeWriter) insertCalls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func (w *fakeWriter) fail(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = err
}

// fakeSink — буфер rejected_events; capacity > 0 — сколько строк он примет всего, остальные отбрасываются.
type fakeSink struct {
	mu       sync.Mutex
	rows     []chstore.RejectedRow
	capacity int
}

func (s *fakeSink) Add(rows ...chstore.RejectedRow) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for _, r := range rows {
		if s.capacity > 0 && len(s.rows) >= s.capacity {
			dropped++
			continue
		}
		s.rows = append(s.rows, r)
	}
	return dropped
}

func (s *fakeSink) logged() []chstore.RejectedRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.rows)
}

// pipelineHarness — собранный ingest как в `appmetrica ingest`: CORS → RequestID → хендлер → Authenticate →
// лимит приложения → конвейер событий, ClickHouse заменён fakeWriter и fakeSink. testKey принадлежит
// активному appA (platform ios), archivedKey — архивному appB. Гео — синтетическая test-city.mmdb.
type pipelineHarness struct {
	*keyHarness
	writer *fakeWriter
	sink   *fakeSink
	limits ingest.RateLimits
}

func newPipelineHarness(t *testing.T, opts ...func(*ingest.PipelineConfig)) *pipelineHarness {
	t.Helper()
	kh := newKeyHarness(t, ingest.KeyCacheConfig{})
	kh.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive), keyRow(wk2, archivedKey, appB, sqlc.AppStatusArchived))
	require.NoError(t, kh.cache.Refresh(t.Context()))
	h := &pipelineHarness{keyHarness: kh, writer: &fakeWriter{}, sink: &fakeSink{}}
	h.limits = ingest.RateLimits{
		Apps:     ratelimit.NewAppLimiter(2000, 4000, kh.clock),
		Installs: ratelimit.NewInstallLimiter(100, 1000, kh.clock),
		Metrics:  kh.metrics,
	}
	cfg := ingest.PipelineConfig{
		Keys:     kh.cache,
		Limits:   h.limits,
		Enricher: ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true},
		Time:     ingest.DefaultTimeConfig,
		Writer:   h.writer,
		Rejected: ingest.RejectedLogger{Sink: h.sink, Keys: kh.cache, Metrics: kh.metrics},
		Clock:    kh.clock,
		Logger:   kh.logger,
		Metrics:  kh.metrics,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	h.handler = httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor:   ingest.NewPipeline(cfg),
		Clock:       kh.clock,
		Logger:      kh.logger,
		Metrics:     kh.metrics,
		BatchErrors: cfg.Rejected,
	}), httpx.CORS(), httpx.RequestID(kh.clock))
	return h
}

func (h *pipelineHarness) postBody(t *testing.T, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(t, request{contentType: "application/json", header: testKey, body: body})
}

// decodeResult разбирает 200 IngestResult.
func decodeResult(t *testing.T, rec *httptest.ResponseRecorder) ingestapi.IngestResult {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res ingestapi.IngestResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res), rec.Body.String())
	return res
}

// batchBody собирает EventBatch: ctx == nil — без context.
func batchBody(t *testing.T, sentAt string, ctx map[string]any, events ...json.RawMessage) []byte {
	t.Helper()
	b := map[string]any{"sent_at": sentAt, "events": events}
	if ctx != nil {
		b["context"] = ctx
	}
	raw, err := json.Marshal(b)
	require.NoError(t, err)
	return raw
}

// oneEvent — батч с sent_at = received_at и одним событием eventJSON(overrides).
func oneEvent(t *testing.T, overrides map[string]any) []byte {
	t.Helper()
	return batchBody(t, "2026-10-13T09:15:02.000Z", nil, eventJSON(t, overrides))
}

// numberedInsertID — уникальный insert_id события n.
func numberedInsertID(n int) string {
	return fmt.Sprintf("0199d6a1-0000-7000-8000-%012d", n)
}

// numberedEvents — события from..from+n-1 одной установки с уникальными insert_id.
func numberedEvents(t *testing.T, from, n int, overrides map[string]any) []json.RawMessage {
	t.Helper()
	events := make([]json.RawMessage, n)
	for i := range events {
		o := map[string]any{"insert_id": numberedInsertID(from + i)}
		for k, v := range overrides {
			o[k] = v
		}
		events[i] = eventJSON(t, o)
	}
	return events
}

// rejectionWant — ожидаемый элемент rejected[].
type rejectionWant struct {
	index     int
	code      ingestapi.RejectionCode
	field     string
	retryable bool
}

// warningWant — ожидаемый элемент warnings[].
type warningWant struct {
	index int
	code  ingestapi.WarningCode
}

// ingestVector — тест-вектор 06-api-contracts.md §2.4.1: запросы отправляются по порядку, status/code и
// accepted/rejected/warnings сверяются с последним ответом, check — с записанными строками.
type ingestVector struct {
	id       string
	setup    func(t *testing.T, h *pipelineHarness)
	requests func(t *testing.T) []request
	status   int
	code     string // error.code для ответа не 200
	accepted int
	rejected []rejectionWant
	warnings []warningWant
	check    func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder)
}

func single(r request) func(*testing.T) []request {
	return func(*testing.T) []request { return []request{r} }
}

func jsonRequest(body func(t *testing.T) []byte) func(*testing.T) []request {
	return func(t *testing.T) []request {
		return []request{{contentType: "application/json", header: testKey, body: body(t)}}
	}
}

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

// requireOneRow — ровно одна записанная строка events.
func requireOneRow(t *testing.T, h *pipelineHarness) chstore.EventRow {
	t.Helper()
	rows := h.writer.written()
	require.Len(t, rows, 1)
	return rows[0]
}

func requireNoRows(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
	t.Helper()
	require.Empty(t, h.writer.written(), "в CH ничего не записано")
}

// ingestVectors — все векторы I-01…I-38 при received_at = 2026-10-13T09:15:02Z, ключ testKey активен,
// приложение appA с platform ios.
func ingestVectors() []ingestVector {
	return []ingestVector{
		{
			id: "I-01 valid_batch", requests: jsonRequest(func(*testing.T) []byte { return []byte(batchI01) }),
			status: http.StatusOK, accepted: 2,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				rows := h.writer.written()
				require.Len(t, rows, 2)
				for _, r := range rows {
					require.Equal(t, "ios", r.Platform)
					require.Equal(t, "production", r.Environment)
					require.Equal(t, appA, r.AppID)
					require.Equal(t, wk1, r.WriteKeyID)
					require.Equal(t, receivedAt, r.ReceivedAt)
				}
				require.Equal(t, ts("2026-10-13T09:11:57.884Z"), rows[0].EventTime.UTC())
				require.Equal(t, uint32(183986), rows[1].DurationMs)
			},
		},
		{
			id: "I-02 clock_correction",
			requests: jsonRequest(func(t *testing.T) []byte {
				return batchBody(t, "2026-10-13T12:15:02.000Z", nil, eventJSON(t, map[string]any{"timestamp": "2026-10-13T12:14:02.000Z"}))
			}),
			status: http.StatusOK, accepted: 1,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				r := requireOneRow(t, h)
				require.Equal(t, ts("2026-10-13T09:14:02Z"), r.EventTime.UTC())
				require.Equal(t, ts("2026-10-13T12:14:02Z"), r.ClientTime.UTC())
				require.Equal(t, ts("2026-10-13T00:00:00Z"), r.EventDate)
			},
		},
		{
			id:       "I-03 future_clamp",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"timestamp": "2026-10-13T09:40:02.000Z"}) }),
			status:   http.StatusOK, accepted: 1,
			warnings: []warningWant{{0, ingestapi.WarningCodeTimestampInFutureClamped}},
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				require.Equal(t, receivedAt, requireOneRow(t, h).EventTime.UTC())
			},
		},
		{
			id:       "I-04 future_within_tolerance",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"timestamp": "2026-10-13T09:25:01.000Z"}) }),
			status:   http.StatusOK, accepted: 1,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				require.Equal(t, receivedAt.Add(9*time.Minute+59*time.Second), requireOneRow(t, h).EventTime.UTC())
			},
		},
		{
			id:       "I-05 too_old",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"timestamp": "2026-10-06T09:15:01.999Z"}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeEventTooOld, "events[0].timestamp", false}},
			check:    requireNoRows,
		},
		{
			id:       "I-06 exactly_7_days",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"timestamp": "2026-10-06T09:15:02.000Z"}) }),
			status:   http.StatusOK, accepted: 1,
		},
		{
			id:       "I-07 pii_email",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"user_id": "ivan@example.com"}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeUserIdLooksLikePii, "events[0].user_id", false}},
			check:    requireNoRows,
		},
		{
			id:       "I-08 pii_e164",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"user_id": "+79161234567"}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeUserIdLooksLikePii, "events[0].user_id", false}},
		},
		{
			id:       "I-09 pii_formatted_phone",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"user_id": "8 (916) 123-45-67"}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeUserIdLooksLikePii, "events[0].user_id", false}},
		},
		{
			id:       "I-10 digits_allowed",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"user_id": "123456789012"}) }),
			status:   http.StatusOK, accepted: 1,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				require.Equal(t, "123456789012", requireOneRow(t, h).UserID)
			},
		},
		{
			id:       "I-11 nil_install_id",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"install_id": nilUUID}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeInvalidField, "events[0].install_id", false}},
		},
		{
			id: "I-12 dup_insert_id",
			requests: jsonRequest(func(t *testing.T) []byte {
				return batchBody(t, "2026-10-13T09:15:02.000Z", nil, eventJSON(t, nil), eventJSON(t, map[string]any{"user_id": "other"}))
			}),
			status: http.StatusOK, accepted: 1,
			warnings: []warningWant{{1, ingestapi.WarningCodeDuplicateInsertIdInBatch}},
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				require.Equal(t, "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90", requireOneRow(t, h).UserID, "принимается первое событие")
				require.Empty(t, h.sink.logged(), "дубль не отклонение")
			},
		},
		{
			id: "I-13 end_without_duration",
			requests: jsonRequest(func(t *testing.T) []byte {
				return oneEvent(t, map[string]any{"name": "session_end"})
			}),
			status: http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeDurationRequired, "events[0].duration_ms", false}},
		},
		{
			id:       "I-14 unknown_name",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"name": "purchase"}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeUnknownEventName, "events[0].name", false}},
		},
		{
			id:       "I-15 missing_session_id",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"session_id": absent}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeMissingField, "events[0].session_id", false}},
		},
		{
			id: "I-16 gzip_body",
			requests: func(t *testing.T) []request {
				return []request{{contentType: "application/json", encoding: "gzip", header: testKey, body: gzipped(t, []byte(batchI01))}}
			},
			status: http.StatusOK, accepted: 2,
		},
		{
			id:       "I-17 bad_encoding",
			requests: single(request{contentType: "application/json", encoding: "br", header: testKey, body: []byte(batchI01)}),
			status:   http.StatusBadRequest, code: "unsupported_content_encoding",
			check: requireNoRows,
		},
		{
			id: "I-18 text_plain_key_in_body",
			requests: single(request{
				contentType: "text/plain;charset=UTF-8",
				body:        []byte(strings.Replace(batchI01, "{\n", `{"app_key": "`+testKey+`",`+"\n", 1)),
			}),
			status: http.StatusOK, accepted: 2,
		},
		{
			id: "I-19 key_mismatch",
			requests: single(request{
				contentType: "application/json", header: testKey,
				body: []byte(strings.Replace(batchI01, "{\n", `{"app_key": "`+otherKey+`",`+"\n", 1)),
			}),
			status: http.StatusBadRequest, code: "invalid_batch",
			check: func(t *testing.T, _ *pipelineHarness, recs []*httptest.ResponseRecorder) {
				require.Equal(t, "app_key", mustErrorDetails(t, recs[0])["field"])
			},
		},
		{
			id:       "I-20 missing_key",
			requests: single(request{contentType: "application/json", body: []byte(batchI01)}),
			status:   http.StatusUnauthorized, code: "missing_app_key",
		},
		{
			id:       "I-21 revoked_key",
			requests: single(request{contentType: "application/json", header: "pk_RevokedRevokedRevokedRevokedRe", body: []byte(batchI01)}),
			status:   http.StatusUnauthorized, code: "invalid_app_key",
			check: func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder) {
				requireNoRows(t, h, recs)
				require.Empty(t, h.sink.logged(), "ключ неизвестен — приложение не определено")
			},
		},
		{
			id:       "I-22 archived_app",
			requests: single(request{contentType: "application/json", header: archivedKey, body: []byte(batchI01)}),
			status:   http.StatusForbidden, code: "app_archived",
			check: requireNoRows,
		},
		{
			id: "I-23 too_many_events",
			requests: jsonRequest(func(t *testing.T) []byte {
				return batchBody(t, "2026-10-13T09:15:02.000Z", nil, numberedEvents(t, 0, 501, nil)...)
			}),
			status: http.StatusBadRequest, code: "invalid_batch",
			check: func(t *testing.T, _ *pipelineHarness, recs []*httptest.ResponseRecorder) {
				require.Equal(t, "events", mustErrorDetails(t, recs[0])["field"])
			},
		},
		{
			id: "I-24 body_too_large",
			requests: func(t *testing.T) []request {
				body := gzipped(t, paddedBatch(t, ingest.MaxBodyBytes+1))
				require.Less(t, len(body), 20*1024)
				return []request{{contentType: "application/json", encoding: "gzip", header: testKey, body: body}}
			},
			status: http.StatusRequestEntityTooLarge, code: "payload_too_large",
		},
		{
			id:       "I-25 invalid_json",
			requests: single(request{contentType: "application/json", header: testKey, body: []byte(`{"sent_at":`)}),
			status:   http.StatusBadRequest, code: "invalid_json",
		},
		{
			id:       "I-26 missing_sent_at",
			requests: single(request{contentType: "application/json", header: testKey, body: []byte(`{"events":[` + event + `]}`)}),
			status:   http.StatusBadRequest, code: "invalid_batch",
			check: func(t *testing.T, _ *pipelineHarness, recs []*httptest.ResponseRecorder) {
				require.Equal(t, "sent_at", mustErrorDetails(t, recs[0])["field"])
			},
		},
		{
			id:       "I-27 wrong_media_type",
			requests: single(request{contentType: "application/x-www-form-urlencoded", header: testKey, body: []byte(batchI01)}),
			status:   http.StatusUnsupportedMediaType, code: "unsupported_media_type",
		},
		{
			id: "I-28 install_rate_limit",
			requests: func(t *testing.T) []request {
				var reqs []request
				for _, part := range [][2]int{{0, 500}, {500, 500}, {1000, 1}} {
					body := batchBody(t, "2026-10-13T09:15:02.000Z", nil, numberedEvents(t, part[0], part[1], nil)...)
					reqs = append(reqs, request{contentType: "application/json", header: testKey, body: body})
				}
				return reqs
			},
			status: http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeInstallRateLimited, "events[0].install_id", true}},
			check: func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder) {
				require.Equal(t, 500, decodeResult(t, recs[0]).Accepted)
				require.Equal(t, 500, decodeResult(t, recs[1]).Accepted)
				require.Len(t, h.writer.written(), 1000)
			},
		},
		{
			id: "I-29 app_rate_limit",
			setup: func(t *testing.T, h *pipelineHarness) {
				ok, _ := h.limits.Apps.Allow(appA, 4000)
				require.True(t, ok)
			},
			requests: jsonRequest(func(t *testing.T) []byte {
				return batchBody(t, "2026-10-13T09:15:02.000Z", nil, numberedEvents(t, 0, 10, nil)...)
			}),
			status: http.StatusTooManyRequests, code: "app_rate_limited",
			check: func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder) {
				retryAfter, err := strconv.Atoi(recs[0].Header().Get("Retry-After"))
				require.NoError(t, err)
				require.GreaterOrEqual(t, retryAfter, 1)
				requireNoRows(t, h, recs)
			},
		},
		{
			id: "I-30 context_override",
			requests: jsonRequest(func(t *testing.T) []byte {
				return batchBody(t, "2026-10-13T09:15:02.000Z",
					map[string]any{"platform": "ios", "os_name": "iOS", "app_version": "3.4.0"},
					eventJSON(t, map[string]any{"context": map[string]any{"app_version": "3.5.0"}}))
			}),
			status: http.StatusOK, accepted: 1,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				r := requireOneRow(t, h)
				require.Equal(t, "3.5.0", r.AppVersion)
				require.Equal(t, "iOS", r.OSName)
				require.Equal(t, "ios", r.Platform)
			},
		},
		{
			id:       "I-31 platform_default",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, nil) }),
			status:   http.StatusOK, accepted: 1,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				require.Equal(t, "ios", requireOneRow(t, h).Platform)
			},
		},
		{
			id: "I-32 geo_from_ip",
			requests: jsonRequest(func(t *testing.T) []byte {
				rejectedWithIP := eventJSON(t, map[string]any{
					"insert_id": numberedInsertID(2), "name": "purchase",
					"context": map[string]any{"ip": londonIP},
				})
				return batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"ip": londonIP}, eventJSON(t, nil), rejectedWithIP)
			}),
			status: http.StatusOK, accepted: 1,
			rejected: []rejectionWant{{1, ingestapi.RejectionCodeUnknownEventName, "events[1].name", false}},
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				r := requireOneRow(t, h)
				require.Equal(t, "GB", r.Country)
				require.NotEmpty(t, r.City)
				require.NotContains(t, fmt.Sprintf("%+v", r), londonIP, "IP нет ни в одной колонке")
				logged := h.sink.logged()
				require.Len(t, logged, 1)
				require.NotContains(t, fmt.Sprintf("%+v", logged[0]), londonIP, "IP нет в payload_sample")
				require.NotContains(t, h.logs.String(), londonIP)
			},
		},
		{
			id:       "I-33 geo_none",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"context": map[string]any{"ip": "none"}}) }),
			status:   http.StatusOK, accepted: 1,
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				r := requireOneRow(t, h)
				require.Empty(t, r.Country)
				require.Empty(t, r.City)
			},
		},
		{
			id: "I-34 geo_invalid",
			requests: jsonRequest(func(t *testing.T) []byte {
				return oneEvent(t, map[string]any{"context": map[string]any{"ip": "999.1.1.1"}})
			}),
			status: http.StatusOK, accepted: 1,
			warnings: []warningWant{{0, ingestapi.WarningCodeGeoIpInvalid}},
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				require.Empty(t, requireOneRow(t, h).Country)
			},
		},
		{
			id:       "I-35 storage_down",
			setup:    func(_ *testing.T, h *pipelineHarness) { h.writer.fail(errStorageDown) },
			requests: jsonRequest(func(*testing.T) []byte { return []byte(batchI01) }),
			status:   http.StatusServiceUnavailable, code: "storage_unavailable",
			check: func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder) {
				require.Equal(t, "5", recs[0].Header().Get("Retry-After"))
				require.Empty(t, h.sink.logged())
			},
		},
		{
			id: "I-36 retry_same_batch",
			requests: func(*testing.T) []request {
				r := request{contentType: "application/json", header: testKey, body: []byte(batchI01)}
				return []request{r, r}
			},
			status: http.StatusOK, accepted: 2,
			check: func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder) {
				require.Equal(t, 2, decodeResult(t, recs[0]).Accepted)
				rows := h.writer.written()
				require.Len(t, rows, 4, "повтор вставляется, схлопывает ReplacingMergeTree")
				sessions := map[uuid.UUID]bool{}
				for _, r := range rows {
					sessions[r.SessionID] = true
				}
				require.Len(t, sessions, 1, "uniqExact(session_id) = 1")
			},
		},
		{
			id: "I-37 cors_preflight",
			requests: single(request{method: http.MethodOptions, headers: map[string]string{
				"Origin": "https://site.ru", "Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "content-type,x-app-key",
			}}),
			status: http.StatusNoContent,
			check: func(t *testing.T, h *pipelineHarness, recs []*httptest.ResponseRecorder) {
				hdr := recs[0].Header()
				require.Equal(t, "*", hdr.Get("Access-Control-Allow-Origin"))
				require.Contains(t, hdr.Get("Access-Control-Allow-Headers"), "X-App-Key")
				require.Zero(t, h.writer.insertCalls())
			},
		},
		{
			id:       "I-38 rejected_logged",
			requests: jsonRequest(func(t *testing.T) []byte { return oneEvent(t, map[string]any{"user_id": "ivan@example.com"}) }),
			status:   http.StatusOK, accepted: 0,
			rejected: []rejectionWant{{0, ingestapi.RejectionCodeUserIdLooksLikePii, "events[0].user_id", false}},
			check: func(t *testing.T, h *pipelineHarness, _ []*httptest.ResponseRecorder) {
				logged := h.sink.logged()
				require.Len(t, logged, 1)
				r := logged[0]
				require.Equal(t, "user_id_looks_like_pii", r.Code)
				require.Equal(t, appA, r.AppID)
				require.Equal(t, receivedAt, r.ReceivedAt)
				require.LessOrEqual(t, len(r.PayloadSample), ingest.MaxPayloadSampleBytes)
				require.Contains(t, r.PayloadSample, `"user_id":"[redacted]"`)
				require.NotContains(t, r.PayloadSample, "ivan@example.com")
			},
		},
	}
}

var errStorageDown = fmt.Errorf("clickhouse: dial tcp: connection refused")

// runVector выполняет вектор на свежем harness и сверяет последний ответ с ожиданиями.
func runVector(t *testing.T, v ingestVector) (*pipelineHarness, []*httptest.ResponseRecorder) {
	t.Helper()
	h := newPipelineHarness(t)
	if v.setup != nil {
		v.setup(t, h)
	}
	reqs := v.requests(t)
	recs := make([]*httptest.ResponseRecorder, len(reqs))
	for i, r := range reqs {
		recs[i] = h.do(t, r)
	}
	last := recs[len(recs)-1]
	switch {
	case v.status == http.StatusOK:
		res := decodeResult(t, last)
		require.Equal(t, v.accepted, res.Accepted, last.Body.String())
		require.Equal(t, receivedAt, res.ServerTime.UTC())
		require.Len(t, res.Rejected, len(v.rejected), last.Body.String())
		for i, want := range v.rejected {
			got := res.Rejected[i]
			require.Equal(t, want.index, got.Index)
			require.Equal(t, want.code, got.Code)
			require.Equal(t, want.field, got.Field)
			require.Equal(t, want.retryable, got.Retryable)
			require.NotEmpty(t, got.Message)
		}
		require.Len(t, res.Warnings, len(v.warnings), last.Body.String())
		for i, want := range v.warnings {
			require.Equal(t, want.index, res.Warnings[i].Index)
			require.Equal(t, want.code, res.Warnings[i].Code)
			require.NotEmpty(t, res.Warnings[i].Message)
		}
	case v.code != "":
		requireError(t, last, v.status, v.code)
	default:
		require.Equal(t, v.status, last.Code, last.Body.String())
	}
	if v.check != nil {
		v.check(t, h, recs)
	}
	return h, recs
}

func mustErrorDetails(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env errorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Error.Details
}
