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

// CorrectTime переносит время события с часов устройства на часы сервера (02-tech-spec.md Story 1.2):
// event_time = received + (ts − sentAt). Событие дальше FutureTolerance в будущем прижимается к received
// (clamped); событие старше LateWindow — tooOld, граница включительно. Чистая функция без состояния.
// Разность насыщается на границах time.Duration, поэтому экстремальные даты дают clamped или tooOld.
func CorrectTime(received, sentAt, ts time.Time, cfg TimeConfig) (eventTime time.Time, clamped, tooOld bool) {
	eventTime = received.Add(ts.Sub(sentAt))
	if eventTime.After(received.Add(cfg.FutureTolerance)) {
		return received, true, false
	}
	return eventTime, false, eventTime.Before(received.Add(-cfg.LateWindow))
}

// CorrectEventTime заполняет ev.EventTime. Прижатое к received_at событие принимается с warning
// timestamp_in_future_clamped, слишком старое отклоняется event_too_old.
func CorrectEventTime(ev *Event, received, sentAt time.Time, cfg TimeConfig) (*ingestapi.IngestWarning, *ingestapi.Rejection) {
	eventTime, clamped, tooOld := CorrectTime(received, sentAt, ev.Timestamp, cfg)
	if tooOld {
		return nil, reject(ev.Index, ev.InsertID.String(), ingestapi.RejectionCodeEventTooOld, "timestamp",
			"Событие старше 7 дней после коррекции часов.")
	}
	ev.EventTime = eventTime
	if clamped {
		return &ingestapi.IngestWarning{
			Index: ev.Index, Code: ingestapi.WarningCodeTimestampInFutureClamped,
			Message: "Время события в будущем после коррекции часов; записано время приёма.",
		}, nil
	}
	return nil, nil
}
