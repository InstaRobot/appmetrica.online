// Package ingest — приём событий POST /v1/events. Хендлер написан руками, из кодогена берутся только
// модели gen/ingestapi: events разбирается как []json.RawMessage, чтобы ошибка одного события
// возвращалась в rejected[], а не валила батч (07-project-structure.md, ADR 0003).
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/httpx"
)

// Processor обрабатывает батч, прошедший транспортный разбор и проверку ключа: лимиты, валидация событий,
// запись. Ошибка уровня батча возвращается как *Error (можно обёрнутой), любая другая — 500 internal_error.
type Processor interface {
	Process(ctx context.Context, b *Batch) (ingestapi.IngestResult, error)
}

// KeyLookup проверяет ключ батча (реализует *KeyCache): ошибка — *Error 401/403/503.
type KeyLookup interface {
	Lookup(key string) (KeyInfo, error)
}

// HandlerConfig — зависимости хендлера; Processor, Clock, Logger и Metrics обязательны.
type HandlerConfig struct {
	Processor Processor
	Clock     clock.Clock
	Logger    *slog.Logger
	Metrics   *Metrics
	// Keys проверяет ключ до полей батча (06-api-contracts.md §2.4.1, шаг 3); nil — ключ не проверяется
	// (unit-тесты с фейковым Processor).
	Keys KeyLookup
	// BatchErrors — журнал batch-ошибок в rejected_events; nil — не пишутся.
	BatchErrors BatchErrorLogger
	// MaxBodyBytes — лимит тела после распаковки (INGEST_MAX_BODY_BYTES); 0 — MaxBodyBytes.
	MaxBodyBytes int64
	// MaxEvents — максимум событий в батче (INGEST_MAX_EVENTS); 0 — MaxBatchEvents.
	MaxEvents int
}

// BatchErrorLogger получает каждую ошибку уровня батча после разбора Content-Type вместе с ключом
// (из заголовка или тела, если он успел разобраться) и прочитанным телом (реализует RejectedLogger).
type BatchErrorLogger interface {
	LogBatchError(appKey string, receivedAt time.Time, body []byte, e *Error)
}

type handler struct {
	cfg   HandlerConfig
	inner http.Handler
}

// NewHandler возвращает хендлер POST /v1/events. Маршрутизация по методу и пути, RequestID и CORS —
// снаружи. Порядок проверок: Content-Type → Content-Encoding → лимит тела после распаковки →
// JSON → ключ (наличие, затем Keys) → поля батча → Processor.
func NewHandler(cfg HandlerConfig) http.Handler {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = MaxBodyBytes
	}
	if cfg.MaxEvents <= 0 {
		cfg.MaxEvents = MaxBatchEvents
	}
	h := &handler{cfg: cfg}
	h.inner = httpx.Chain(http.HandlerFunc(h.serve), httpx.Gunzip(), httpx.BodyLimit(cfg.MaxBodyBytes))
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.cfg.Clock.Now()
	sw := httpx.NewStatusWriter(w)
	if supportedMediaType(r.Header.Get("Content-Type")) {
		h.inner.ServeHTTP(sw, r)
	} else {
		errUnsupportedMediaType.write(sw, r)
	}
	h.cfg.Metrics.Requests.WithLabelValues(strconv.Itoa(sw.Status())).Inc()
	h.cfg.Metrics.BatchDuration.Observe(h.cfg.Clock.Now().Sub(start).Seconds())
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	receivedAt := h.cfg.Clock.Now()
	headerKey := strings.TrimSpace(r.Header.Get("X-App-Key"))
	body, e := readBody(r, h.cfg.MaxBodyBytes)
	if e != nil {
		h.writeBatchError(w, r, headerKey, receivedAt, body, e)
		return
	}
	batch, e := decodeBatch(body, headerKey)
	if e != nil {
		h.writeBatchError(w, r, headerKey, receivedAt, body, e)
		return
	}
	batch.ReceivedAt = receivedAt
	if h.cfg.Keys != nil {
		info, err := h.cfg.Keys.Lookup(batch.AppKey)
		if err != nil {
			h.writeProcessError(w, r, batch, body, err)
			return
		}
		batch.Key = info
	}
	if e := validateBatch(batch, h.cfg.MaxEvents); e != nil {
		h.writeBatchError(w, r, batch.AppKey, receivedAt, body, e)
		return
	}
	batch.ClientIP, _ = httpx.ClientIP(r)
	h.cfg.Metrics.BatchEvents.Observe(float64(len(batch.Events)))

	res, err := h.cfg.Processor.Process(r.Context(), batch)
	if err != nil {
		h.writeProcessError(w, r, batch, body, err)
		return
	}
	if res.Rejected == nil {
		res.Rejected = []ingestapi.Rejection{}
	}
	if res.Warnings == nil {
		res.Warnings = []ingestapi.IngestWarning{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// writeBatchError отвечает ошибкой уровня батча и передаёт её в журнал rejected_events.
func (h *handler) writeBatchError(w http.ResponseWriter, r *http.Request, appKey string, receivedAt time.Time, body []byte, e *Error) {
	e.write(w, r)
	if h.cfg.BatchErrors != nil && appKey != "" {
		h.cfg.BatchErrors.LogBatchError(appKey, receivedAt, body, e)
	}
}

// writeProcessError — ошибка проверки ключа или Processor: *Error уходит клиенту как есть, прочее — 500.
func (h *handler) writeProcessError(w http.ResponseWriter, r *http.Request, b *Batch, body []byte, err error) {
	if e, ok := errors.AsType[*Error](err); ok {
		h.writeBatchError(w, r, b.AppKey, b.ReceivedAt, body, e)
		return
	}
	h.cfg.Logger.ErrorContext(r.Context(), "ingest batch failed",
		slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		slog.Any("error", err),
	)
	errInternal.write(w, r)
}
