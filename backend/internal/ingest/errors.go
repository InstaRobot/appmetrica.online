package ingest

import (
	"net/http"
	"strconv"
	"strings"

	"appmetrica.online/backend/internal/httpx"
)

// Error — ошибка уровня батча: весь запрос отклоняется конвертом Error из ingest.yaml.
// Её возвращают и транспорт, и Processor (invalid_app_key, app_archived, app_rate_limited,
// storage_unavailable); прочие ошибки Processor превращаются в 500 internal_error.
type Error struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
	// RetryAfter > 0 — заголовок Retry-After в секундах (429, 503).
	RetryAfter int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func (e *Error) write(w http.ResponseWriter, r *http.Request) {
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfter))
	}
	httpx.WriteError(w, r, e.Status, e.Code, e.Message, e.Details)
}

// Значения details.issue для invalid_batch.
const (
	issueRequired = "required"
	issueType     = "type"
	issueFormat   = "format"
	issueMinItems = "min_items"
	issueMaxItems = "max_items"
	issueMismatch = "mismatch"
)

func invalidBatch(field, issue, message string) *Error {
	details := map[string]any{"issue": issue}
	if field != "" {
		details["field"] = field
	}
	return &Error{Status: http.StatusBadRequest, Code: "invalid_batch", Message: message, Details: details}
}

var (
	errUnsupportedMediaType = &Error{
		Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type",
		Message: "Content-Type должен быть application/json или text/plain.",
	}
	errInvalidJSON = &Error{
		Status: http.StatusBadRequest, Code: "invalid_json",
		Message: "Тело запроса не является корректным JSON.",
	}
	errMissingAppKey = &Error{
		Status: http.StatusUnauthorized, Code: "missing_app_key",
		Message: "Передайте ключ в заголовке X-App-Key или в поле app_key.",
	}
	errInternal = &Error{
		Status: http.StatusInternalServerError, Code: "internal_error",
		Message: "Внутренняя ошибка. Повторите батч позже.",
	}
)

// payloadTooLarge — 413 для тела, превысившего limit байт после распаковки.
func payloadTooLarge(limit int64) *Error {
	return &Error{
		Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large",
		Message: "Тело после распаковки больше " + groupDigits(limit) + " байт. Разбейте батч.",
	}
}

// groupDigits — число с разделением разрядов пробелом: 1048576 → «1 048 576».
func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
