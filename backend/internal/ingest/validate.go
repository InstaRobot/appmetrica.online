package ingest

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/gen/ingestapi"
)

// MaxUserIDLength — максимум символов user_id после TrimSpace.
const MaxUserIDLength = 128

// Event — событие, прошедшее поэлементную валидацию.
type Event struct {
	Index      int
	InsertID   uuid.UUID
	Name       ingestapi.EventName
	Timestamp  time.Time
	InstallID  uuid.UUID
	SessionID  uuid.UUID
	UserID     string
	DurationMs int64
	Context    json.RawMessage
	EventTime  time.Time
}

// ValidateEvent — заглушка T-010.
func ValidateEvent(index int, raw json.RawMessage) (*Event, *ingestapi.Rejection) {
	_, _ = index, raw
	return &Event{}, nil
}
