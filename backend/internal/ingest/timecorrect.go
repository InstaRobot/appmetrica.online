package ingest

import (
	"time"

	"appmetrica.online/backend/internal/gen/ingestapi"
)

// TimeConfig — пороги коррекции часов (INGEST_FUTURE_TOLERANCE, INGEST_LATE_WINDOW).
type TimeConfig struct {
	FutureTolerance time.Duration
	LateWindow      time.Duration
}

// DefaultTimeConfig — значения по умолчанию из .env.example: 10 минут и 168 часов.
var DefaultTimeConfig = TimeConfig{FutureTolerance: 10 * time.Minute, LateWindow: 168 * time.Hour}

// CorrectTime — заглушка T-010.
func CorrectTime(received, sentAt, ts time.Time, cfg TimeConfig) (eventTime time.Time, clamped, tooOld bool) {
	_, _, _, _ = received, sentAt, ts, cfg
	return time.Time{}, false, false
}

// CorrectEventTime — заглушка T-010.
func CorrectEventTime(ev *Event, received, sentAt time.Time, cfg TimeConfig) (*ingestapi.IngestWarning, *ingestapi.Rejection) {
	_, _, _, _ = ev, received, sentAt, cfg
	return nil, nil
}
