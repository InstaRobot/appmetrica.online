package ingest

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	"appmetrica.online/backend/internal/gen/ingestapi"
)

const (
	// MaxUserIDLength — максимум символов user_id после TrimSpace.
	MaxUserIDLength = 128
	// MaxDurationMs — верхняя граница duration_ms (сутки).
	MaxDurationMs = 86_400_000
)

// Event — событие, прошедшее поэлементную валидацию.
type Event struct {
	// Index — позиция в events[], для rejected[] и warnings[].
	Index     int
	InsertID  uuid.UUID
	Name      ingestapi.EventName
	Timestamp time.Time // время по часам устройства с offset клиента (client_time)
	InstallID uuid.UUID
	SessionID uuid.UUID
	// UserID — после TrimSpace; "" — не передан.
	UserID string
	// DurationMs — 0, если не передан (у session_end обязателен).
	DurationMs int64
	// Context — заданные поля context события; слияние с батчем делает Enricher.Enrich.
	Context ContextFields
	// EventTime заполняет CorrectEventTime.
	EventTime time.Time
	// Resolved заполняет Enricher.Enrich.
	Resolved ResolvedContext
}

// rawEvent — поля Event из ingest.yaml; неизвестные поля игнорируются, как и на уровне батча (OQ-030).
type rawEvent struct {
	InsertID   json.RawMessage `json:"insert_id"`
	Name       json.RawMessage `json:"name"`
	Timestamp  json.RawMessage `json:"timestamp"`
	InstallID  json.RawMessage `json:"install_id"`
	SessionID  json.RawMessage `json:"session_id"`
	UserID     json.RawMessage `json:"user_id"`
	DurationMs json.RawMessage `json:"duration_ms"`
	Context    json.RawMessage `json:"context"`
}

// ValidateEvent проверяет events[index] и возвращает либо событие, либо первое найденное отклонение.
// Порядок проверок: объект → insert_id → name → timestamp → install_id → session_id → duration_ms → user_id
// (длина, затем PII-guard) → context. Значения полей в текст отклонения не попадают.
func ValidateEvent(index int, raw json.RawMessage) (*Event, *ingestapi.Rejection) {
	var r rawEvent
	if isNull(raw) || json.Unmarshal(raw, &r) != nil {
		return nil, reject(index, "", ingestapi.RejectionCodeInvalidField, "", "Событие должно быть JSON-объектом.")
	}
	ev := &Event{Index: index}
	var rej *ingestapi.Rejection
	if ev.InsertID, rej = uuidField(index, "", "insert_id", r.InsertID); rej != nil {
		return nil, rej
	}
	insertID := ev.InsertID.String()
	if ev.Name, rej = nameField(index, insertID, r.Name); rej != nil {
		return nil, rej
	}
	if ev.Timestamp, rej = timestampField(index, insertID, r.Timestamp); rej != nil {
		return nil, rej
	}
	if ev.InstallID, rej = uuidField(index, insertID, "install_id", r.InstallID); rej != nil {
		return nil, rej
	}
	if ev.SessionID, rej = uuidField(index, insertID, "session_id", r.SessionID); rej != nil {
		return nil, rej
	}
	if ev.DurationMs, rej = durationField(index, insertID, ev.Name, r.DurationMs); rej != nil {
		return nil, rej
	}
	if ev.UserID, rej = userIDField(index, insertID, r.UserID); rej != nil {
		return nil, rej
	}
	fields, cerr := parseContext(r.Context)
	if cerr != nil {
		return nil, rejectPath(index, insertID, ingestapi.RejectionCodeInvalidField,
			"events["+strconv.Itoa(index)+"]."+cerr.field, cerr.message)
	}
	ev.Context = fields
	return ev, nil
}

// reject собирает Rejection; field — имя поля события ("" — событие целиком), insertID "" — null.
func reject(index int, insertID string, code ingestapi.RejectionCode, field, message string) *ingestapi.Rejection {
	path := "events[" + strconv.Itoa(index) + "]"
	if field != "" {
		path += "." + field
	}
	return rejectPath(index, insertID, code, path, message)
}

// rejectPath — Rejection с полным путём field ("context.platform" для context батча).
func rejectPath(index int, insertID string, code ingestapi.RejectionCode, path, message string) *ingestapi.Rejection {
	id := nullable.NewNullNullable[string]()
	if insertID != "" {
		id = nullable.NewNullableWithValue(insertID)
	}
	return &ingestapi.Rejection{Index: index, InsertId: id, Code: code, Field: path, Message: message}
}

func missingField(index int, insertID, field string) *ingestapi.Rejection {
	return reject(index, insertID, ingestapi.RejectionCodeMissingField, field, "Поле "+field+" обязательно.")
}

// stringField разбирает необязательную строку: present=false — поля нет или null, ok=false — не строка.
func stringField(raw json.RawMessage) (s string, present, ok bool) {
	if isNull(raw) {
		return "", false, true
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", true, false
	}
	return s, true, true
}

// uuidField: только каноническая форма 8-4-4-4-12 (регистр любой), nil-UUID запрещён.
func uuidField(index int, insertID, field string, raw json.RawMessage) (uuid.UUID, *ingestapi.Rejection) {
	s, present, ok := stringField(raw)
	if !present {
		return uuid.Nil, missingField(index, insertID, field)
	}
	var id uuid.UUID
	var err error
	if ok && len(s) == 36 {
		id, err = uuid.Parse(s)
	}
	if !ok || len(s) != 36 || err != nil || id == uuid.Nil {
		return uuid.Nil, reject(index, insertID, ingestapi.RejectionCodeInvalidField, field,
			field+" должен быть UUID и не nil-UUID.")
	}
	return id, nil
}

func nameField(index int, insertID string, raw json.RawMessage) (ingestapi.EventName, *ingestapi.Rejection) {
	s, present, ok := stringField(raw)
	switch name := ingestapi.EventName(s); {
	case !present:
		return "", missingField(index, insertID, "name")
	case !ok:
		return "", reject(index, insertID, ingestapi.RejectionCodeInvalidField, "name", "name должен быть строкой.")
	case name == ingestapi.EventNameSessionStart || name == ingestapi.EventNameSessionEnd:
		return name, nil
	default:
		return "", reject(index, insertID, ingestapi.RejectionCodeUnknownEventName, "name",
			"Допустимые имена: session_start, session_end.")
	}
}

// timestampField: RFC 3339 с обязательным offset (Z или ±hh:mm).
func timestampField(index int, insertID string, raw json.RawMessage) (time.Time, *ingestapi.Rejection) {
	s, present, ok := stringField(raw)
	if !present {
		return time.Time{}, missingField(index, insertID, "timestamp")
	}
	var ts time.Time
	var err error
	if ok {
		ts, err = time.Parse(time.RFC3339Nano, s)
	}
	if !ok || err != nil {
		return time.Time{}, reject(index, insertID, ingestapi.RejectionCodeInvalidField, "timestamp",
			"timestamp должен быть в формате RFC 3339 с offset.")
	}
	return ts, nil
}

// durationField: целое 0..86 400 000, обязательно для session_end.
func durationField(index int, insertID string, name ingestapi.EventName, raw json.RawMessage) (int64, *ingestapi.Rejection) {
	if isNull(raw) {
		if name == ingestapi.EventNameSessionEnd {
			return 0, reject(index, insertID, ingestapi.RejectionCodeDurationRequired, "duration_ms",
				"Для session_end обязательно поле duration_ms (0..86400000).")
		}
		return 0, nil
	}
	var d int64
	if err := json.Unmarshal(raw, &d); err != nil || d < 0 || d > MaxDurationMs {
		return 0, reject(index, insertID, ingestapi.RejectionCodeInvalidField, "duration_ms",
			"duration_ms должен быть целым числом от 0 до 86400000.")
	}
	return d, nil
}

// userIDField: после TrimSpace пустая строка — user_id не передан; длина в символах ≤ 128; email и телефоны
// отклоняются PII-guard; U+0000 отклоняется: deletion_requests.subject_value (Postgres text) его не хранит,
// и такой user_id нельзя было бы удалить по FR-019.
func userIDField(index int, insertID string, raw json.RawMessage) (string, *ingestapi.Rejection) {
	s, _, ok := stringField(raw)
	if !ok {
		return "", reject(index, insertID, ingestapi.RejectionCodeInvalidField, "user_id", "user_id должен быть строкой.")
	}
	s = strings.TrimSpace(s)
	switch {
	case utf8.RuneCountInString(s) > MaxUserIDLength:
		return "", reject(index, insertID, ingestapi.RejectionCodeInvalidField, "user_id",
			"user_id длиннее 128 символов.")
	case strings.ContainsRune(s, 0):
		return "", reject(index, insertID, ingestapi.RejectionCodeInvalidField, "user_id",
			"user_id содержит символ U+0000.")
	case LooksLikePII(s):
		return "", reject(index, insertID, ingestapi.RejectionCodeUserIdLooksLikePii, "user_id",
			"user_id похож на email или телефон. Передавайте непрозрачный ID.")
	}
	return s, nil
}
