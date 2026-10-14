package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/httpx"
)

const (
	// DefaultInsertTimeout — ожидание ack ClickHouse; дольше — 503 storage_unavailable.
	DefaultInsertTimeout = 5 * time.Second

	storageRetryAfter = 5
	// keyPrefixLen — "pk_" + 8 символов: столько ключа видно в rejected_events.
	keyPrefixLen = 11
	// maxSampledNameRunes ограничивает rejected_events.event_name: имя события присылает клиент.
	maxSampledNameRunes = 64
	redactedUserID      = "[redacted]"
)

// EventWriter — вставка принятых событий с ожиданием ack (реализует *chstore.Store).
type EventWriter interface {
	InsertEvents(ctx context.Context, rows []chstore.EventRow) error
}

// RejectedSink — неблокирующий буфер rejected_events (реализует *chstore.RejectedLog).
type RejectedSink interface {
	Add(rows ...chstore.RejectedRow) (dropped int)
}

// PipelineConfig — зависимости конвейера; все поля, кроме InsertTimeout, обязательны. Ключ батча к этому
// моменту уже проверен хендлером (Batch.Key заполнен).
type PipelineConfig struct {
	Limits        RateLimits
	Enricher      Enricher
	Time          TimeConfig
	Writer        EventWriter
	Rejected      RejectedLogger
	Clock         clock.Clock
	Logger        *slog.Logger
	Metrics       *Metrics
	InsertTimeout time.Duration // 0 — DefaultInsertTimeout
}

// NewPipeline собирает Processor POST /v1/events (06-api-contracts.md §2.4.1, шаги 5–9): лимит приложения →
// для каждого события валидация, PII-guard, коррекция времени, дедуп insert_id, лимит установки, контекст
// и гео → вставка принятых в events с ожиданием ack → отказы в rejected_events.
func NewPipeline(cfg PipelineConfig) Processor {
	if cfg.InsertTimeout <= 0 {
		cfg.InsertTimeout = DefaultInsertTimeout
	}
	return cfg.Limits.LimitApp(&pipeline{cfg: cfg})
}

type pipeline struct {
	cfg PipelineConfig
}

func (p *pipeline) Process(ctx context.Context, b *Batch) (ingestapi.IngestResult, error) {
	res := ingestapi.IngestResult{
		Rejected:   []ingestapi.Rejection{},
		Warnings:   []ingestapi.IngestWarning{},
		ServerTime: b.ReceivedAt,
	}
	batchCtx, rejected := ValidateBatchContext(b)
	var rows []chstore.EventRow
	if rejected != nil {
		res.Rejected = rejected
	} else {
		rows = p.events(b, batchCtx, &res)
	}
	if err := p.write(ctx, b, rows); err != nil {
		return ingestapi.IngestResult{}, err
	}
	p.record(b, batchCtx, res)
	return res, nil
}

// events обрабатывает события по порядку и возвращает строки для вставки. Дубль insert_id получает warning
// до лимита установки и не тратит его бюджет; warning коррекции часов выдаётся только принятому событию.
func (p *pipeline) events(b *Batch, batchCtx ContextFields, res *ingestapi.IngestResult) []chstore.EventRow {
	rows := make([]chstore.EventRow, 0, len(b.Events))
	seen := make(map[uuid.UUID]struct{}, len(b.Events))
	for i, raw := range b.Events {
		ev, rej := ValidateEvent(i, raw)
		var clamped *ingestapi.IngestWarning
		if rej == nil {
			clamped, rej = CorrectEventTime(ev, b.ReceivedAt, b.SentAt, p.cfg.Time)
		}
		if rej == nil {
			if _, dup := seen[ev.InsertID]; dup {
				res.Warnings = append(res.Warnings, ingestapi.IngestWarning{
					Index: i, Code: ingestapi.WarningCodeDuplicateInsertIdInBatch,
					Message: "insert_id уже встречался в этом батче; событие пропущено.",
				})
				continue
			}
			seen[ev.InsertID] = struct{}{}
			rej = p.cfg.Limits.AllowInstall(b.Key.AppID, ev)
		}
		if rej != nil {
			res.Rejected = append(res.Rejected, *rej)
			continue
		}
		if clamped != nil {
			res.Warnings = append(res.Warnings, *clamped)
		}
		geoWarning, write := p.cfg.Enricher.Enrich(b, batchCtx, ev)
		if geoWarning != nil {
			res.Warnings = append(res.Warnings, *geoWarning)
		}
		res.Accepted++
		if write {
			rows = append(rows, NewEventRow(b, ev))
		}
	}
	return rows
}

// write вставляет строки и ждёт ack не дольше InsertTimeout. Ошибка — 503 storage_unavailable: батч не
// принят целиком, клиент повторит его, дубли погасит insert_id.
func (p *pipeline) write(ctx context.Context, b *Batch, rows []chstore.EventRow) error {
	if len(rows) == 0 {
		return nil
	}
	insertCtx, cancel := context.WithTimeout(ctx, p.cfg.InsertTimeout)
	defer cancel()
	start := p.cfg.Clock.Now()
	err := p.cfg.Writer.InsertEvents(insertCtx, rows)
	p.cfg.Metrics.InsertDuration.Observe(p.cfg.Clock.Now().Sub(start).Seconds())
	if err == nil {
		return nil
	}
	p.cfg.Metrics.InsertErrors.Inc()
	p.cfg.Logger.ErrorContext(ctx, "clickhouse insert failed",
		slog.String("request_id", httpx.RequestIDFrom(ctx)),
		slog.String("app_id", b.Key.AppID.String()),
		slog.Int("events", len(rows)),
		slog.Any("error", err),
	)
	return errStorageUnavailable
}

// record обновляет метрики принятого батча и ставит его отказы в журнал rejected_events.
func (p *pipeline) record(b *Batch, batchCtx ContextFields, res ingestapi.IngestResult) {
	m := p.cfg.Metrics
	if res.Accepted > 0 {
		m.EventsAccepted.WithLabelValues(b.Key.AppID.String()).Add(float64(res.Accepted))
	}
	for _, rej := range res.Rejected {
		m.EventsRejected.WithLabelValues(string(rej.Code)).Inc()
	}
	for _, w := range res.Warnings {
		m.Warnings.WithLabelValues(string(w.Code)).Inc()
	}
	p.cfg.Rejected.logEvents(b, batchCtx, res.Rejected)
}

var errStorageUnavailable = &Error{
	Status: http.StatusServiceUnavailable, Code: "storage_unavailable",
	Message: "Хранилище временно недоступно. Повторите весь батч.", RetryAfter: storageRetryAfter,
}

// RejectedLogger пишет отказы в rejected_events для экрана «Интеграция»: отклонённые события и
// batch-ошибки известных приложений (05-data-model.md §2.9). Все поля обязательны.
type RejectedLogger struct {
	Sink    RejectedSink
	Keys    *KeyCache
	Metrics *Metrics
}

// loggedBatchCodes — batch-коды, которые попадают в rejected_events. missing_app_key и invalid_app_key не
// привязаны к приложению, ошибки транспорта до разбора тела и 5xx — не ошибки интеграции клиента.
var loggedBatchCodes = map[string]bool{
	"invalid_json":      true,
	"invalid_batch":     true,
	"payload_too_large": true,
	"app_archived":      true,
	"app_rate_limited":  true,
}

// LogBatchError ставит в журнал batch-ошибку, если ключ принадлежит известному приложению. body — тело
// (или его прочитанное начало); в сэмпл попадает без context.ip.
func (l RejectedLogger) LogBatchError(appKey string, receivedAt time.Time, body []byte, e *Error) {
	if !loggedBatchCodes[e.Code] {
		return
	}
	info, ok := l.Keys.Peek(appKey)
	if !ok {
		return
	}
	var raw rawBatch
	var batchCtx ContextFields
	if json.Unmarshal(body, &raw) == nil {
		batchCtx, _ = parseContext(raw.Context)
	}
	rc := resolveContext(batchCtx, string(info.AppPlatform))
	field, _ := e.Details["field"].(string)
	l.add(chstore.RejectedRow{
		ReceivedAt:    receivedAt,
		AppID:         info.AppID,
		KeyPrefix:     keyPrefix(appKey),
		Code:          e.Code,
		Field:         field,
		Message:       e.Message,
		AppVersion:    rc.AppVersion,
		Platform:      rc.Platform,
		PayloadSample: PayloadSample(body),
	})
}

// logEvents — строки rejected_events для отклонённых событий принятого батча. Имя события и контекст
// берутся из сырого JSON, насколько он разбирается; невалидный context не используется.
func (l RejectedLogger) logEvents(b *Batch, batchCtx ContextFields, rejected []ingestapi.Rejection) {
	if len(rejected) == 0 {
		return
	}
	rows := make([]chstore.RejectedRow, len(rejected))
	for i, rej := range rejected {
		raw := b.Events[rej.Index]
		name, evCtx := peekEvent(raw)
		rc := resolveContext(MergeContext(batchCtx, evCtx), string(b.Key.AppPlatform))
		if rej.Code == ingestapi.RejectionCodeUserIdLooksLikePii {
			raw = redactUserID(raw)
		}
		rows[i] = chstore.RejectedRow{
			ReceivedAt:    b.ReceivedAt,
			AppID:         b.Key.AppID,
			KeyPrefix:     keyPrefix(b.AppKey),
			Code:          string(rej.Code),
			Field:         rej.Field,
			Message:       rej.Message,
			EventName:     name,
			AppVersion:    rc.AppVersion,
			Platform:      rc.Platform,
			PayloadSample: PayloadSample(raw),
		}
	}
	l.add(rows...)
}

func (l RejectedLogger) add(rows ...chstore.RejectedRow) {
	if dropped := l.Sink.Add(rows...); dropped > 0 {
		l.Metrics.RejectedLogDropped.Add(float64(dropped))
	}
}

// keyPrefix — начало ключа для диагностики; полный ключ в журнал не пишется.
func keyPrefix(key string) string {
	return key[:min(len(key), keyPrefixLen)]
}

// peekEvent — имя (не длиннее maxSampledNameRunes символов) и валидный context события; что не
// разбирается, остаётся пустым.
func peekEvent(raw json.RawMessage) (string, ContextFields) {
	var r struct {
		Name    json.RawMessage `json:"name"`
		Context json.RawMessage `json:"context"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return "", nil
	}
	name, _, _ := stringField(r.Name)
	if runes := []rune(name); len(runes) > maxSampledNameRunes {
		name = string(runes[:maxSampledNameRunes])
	}
	ctx, _ := parseContext(r.Context)
	return name, ctx
}

// redactUserID заменяет user_id в JSON события на "[redacted]": значение, похожее на email или телефон,
// не должно оказаться в rejected_events (02-tech-spec.md AC-1.5.2).
func redactUserID(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return raw
	}
	for k := range obj {
		if strings.EqualFold(k, "user_id") {
			obj[k] = json.RawMessage(`"` + redactedUserID + `"`)
		}
	}
	return marshalCompact(obj)
}
