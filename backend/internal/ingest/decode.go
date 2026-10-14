package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxBodyBytes — лимит тела запроса после распаковки gzip по умолчанию (INGEST_MAX_BODY_BYTES).
	MaxBodyBytes = 1 << 20
	// MaxBatchEvents — максимум событий в батче по умолчанию (INGEST_MAX_EVENTS).
	MaxBatchEvents = 500
)

// Batch — батч после транспортного разбора: ключ найден, sent_at разобран, events — 1..500 элементов.
// События и context остаются сырыми JSON: их поэлементно валидирует конвейер, чтобы ошибка
// в одном событии не валила весь батч.
type Batch struct {
	AppKey string
	SentAt time.Time
	// Context — сырой context батча; nil, если поля нет или оно null.
	Context    json.RawMessage
	Events     []json.RawMessage
	ReceivedAt time.Time
	Key        KeyInfo
	// ClientIP — адрес клиента из httpx.ClientIP (нулевой, если не определён). Только для гео-поиска
	// в Enricher: не сохраняется, не логируется и не попадает в ответы и метрики.
	ClientIP netip.Addr

	// raw — поля батча до validateBatch.
	raw rawBatch
}

// rawBatch — верхний уровень EventBatch; неизвестные поля игнорируются.
type rawBatch struct {
	AppKey  json.RawMessage `json:"app_key"`
	SentAt  json.RawMessage `json:"sent_at"`
	Context json.RawMessage `json:"context"`
	Events  json.RawMessage `json:"events"`
}

// supportedMediaType: application/json или text/plain (navigator.sendBeacon со строкой) с любыми параметрами.
func supportedMediaType(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && (mt == "application/json" || mt == "text/plain")
}

// readBody читает тело, уже ограниченное http.MaxBytesReader поверх распаковки gzip.
// Любая другая ошибка чтения — обрыв или битый поток gzip — считается невалидным телом.
// При ошибке возвращается прочитанное начало тела (для сэмпла в rejected_events).
func readBody(r *http.Request, limit int64) ([]byte, *Error) {
	body, err := io.ReadAll(r.Body)
	if err == nil {
		return body, nil
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return body, payloadTooLarge(limit)
	}
	return body, errInvalidJSON
}

// decodeBatch разбирает тело в порядке 06-api-contracts.md §2.4.1: JSON → ключ. Поля батча проверяет
// validateBatch уже после проверки ключа: отозванный ключ получает 401, а не 400 invalid_batch.
// headerKey — значение заголовка X-App-Key; ключ из query string не читается никогда.
func decodeBatch(body []byte, headerKey string) (*Batch, *Error) {
	var raw rawBatch
	if err := json.Unmarshal(body, &raw); err != nil {
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); !ok {
			return nil, errInvalidJSON
		}
	}
	// Корректный JSON, но не объект: массив, строка, число, null.
	if trimmed := bytes.TrimLeft(body, " \t\r\n"); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, invalidBatch("", issueType, "Тело запроса должно быть JSON-объектом EventBatch.")
	}

	key, e := appKey(headerKey, raw.AppKey)
	if e != nil {
		return nil, e
	}
	return &Batch{AppKey: key, raw: raw}, nil
}

// validateBatch проверяет sent_at и events (1..maxEvents) и заполняет SentAt, Events и Context.
func validateBatch(b *Batch, maxEvents int) *Error {
	sentAt, e := sentAt(b.raw.SentAt)
	if e != nil {
		return e
	}
	events, e := events(b.raw.Events, maxEvents)
	if e != nil {
		return e
	}
	b.SentAt, b.Events = sentAt, events
	if !isNull(b.raw.Context) {
		b.Context = b.raw.Context
	}
	return nil
}

// appKey выбирает ключ из заголовка или тела. Оба заданы и различаются — invalid_batch:
// неясно, в какое приложение писать.
func appKey(header string, raw json.RawMessage) (string, *Error) {
	header = strings.TrimSpace(header)
	var body string
	if !isNull(raw) {
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", invalidBatch("app_key", issueType, "app_key должен быть строкой.")
		}
		body = strings.TrimSpace(body)
	}
	switch {
	case header != "" && body != "" && header != body:
		return "", invalidBatch("app_key", issueMismatch, "Ключи в заголовке X-App-Key и в поле app_key различаются.")
	case header != "":
		return header, nil
	case body != "":
		return body, nil
	default:
		return "", errMissingAppKey
	}
}

func sentAt(raw json.RawMessage) (time.Time, *Error) {
	if isNull(raw) {
		return time.Time{}, invalidBatch("sent_at", issueRequired, "Поле sent_at обязательно.")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, invalidBatch("sent_at", issueType, "sent_at должен быть строкой RFC 3339 с offset.")
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, invalidBatch("sent_at", issueFormat, "sent_at должен быть в формате RFC 3339 с offset.")
	}
	return t, nil
}

func events(raw json.RawMessage, maxEvents int) ([]json.RawMessage, *Error) {
	if isNull(raw) {
		return nil, invalidBatch("events", issueRequired, "Поле events обязательно.")
	}
	var events []json.RawMessage
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, invalidBatch("events", issueType, "events должен быть массивом событий.")
	}
	rangeMsg := "events: от 1 до " + strconv.Itoa(maxEvents) + " событий."
	switch {
	case len(events) == 0:
		return nil, invalidBatch("events", issueMinItems, rangeMsg)
	case len(events) > maxEvents:
		return nil, invalidBatch("events", issueMaxItems, rangeMsg)
	}
	return events, nil
}

// isNull — поле отсутствует или равно null.
func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
