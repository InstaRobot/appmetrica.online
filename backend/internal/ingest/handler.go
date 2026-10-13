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

// Processor обрабатывает батч, прошедший транспортный разбор: ключ, лимиты, валидация событий, запись.
// Ошибка уровня батча возвращается как *Error (можно обёрнутой), любая другая — 500 internal_error.
type Processor interface {
	Process(ctx context.Context, b *Batch) (ingestapi.IngestResult, error)
}

// HandlerConfig — зависимости хендлера; все поля обязательны.
type HandlerConfig struct {
	Processor Processor
	Clock     clock.Clock
	Logger    *slog.Logger
	Metrics   *Metrics
	// BatchErrors — журнал batch-ошибок в rejected_events; nil — не пишутся.
	BatchErrors BatchErrorLogger
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
// снаружи. Порядок проверок: Content-Type → Content-Encoding → лимит 1 MiB после распаковки →
// JSON → ключ → поля батча → Processor.
func NewHandler(cfg HandlerConfig) http.Handler {
	h := &handler{cfg: cfg}
	h.inner = httpx.Chain(http.HandlerFunc(h.serve), httpx.Gunzip(), httpx.BodyLimit(MaxBodyBytes))
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.cfg.Clock.Now()
	sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	if supportedMediaType(r.Header.Get("Content-Type")) {
		h.inner.ServeHTTP(sw, r)
	} else {
		errUnsupportedMediaType.write(sw, r)
	}
	h.cfg.Metrics.Requests.WithLabelValues(strconv.Itoa(sw.status)).Inc()
	h.cfg.Metrics.BatchDuration.Observe(h.cfg.Clock.Now().Sub(start).Seconds())
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request) {
	receivedAt := h.cfg.Clock.Now()
	headerKey := r.Header.Get("X-App-Key")
	body, e := readBody(r)
	if e != nil {
		h.writeBatchError(w, r, strings.TrimSpace(headerKey), receivedAt, body, e)
		return
	}
	batch, e := decodeBatch(body, headerKey)
	if e != nil {
		key := strings.TrimSpace(headerKey)
		if batch != nil {
			key = batch.AppKey
		}
		h.writeBatchError(w, r, key, receivedAt, body, e)
		return
	}
	batch.ReceivedAt = receivedAt
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

// statusRecorder запоминает статус ответа для ingest_requests_total, включая ответы Gunzip.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// Unwrap даёт http.ResponseController доступ к исходному writer.
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }
