package chstore

import (
	"time"

	"github.com/google/uuid"
)

// EventRow — одна строка appmetrica.events. Порядок полей = порядок колонок DDL 00001_init.sql.
type EventRow struct {
	AppID       uuid.UUID `ch:"app_id"`
	EventDate   time.Time `ch:"event_date"`  // UTC-дата event_time
	EventTime   time.Time `ch:"event_time"`  // скорректированное время (FR-002)
	ClientTime  time.Time `ch:"client_time"` // timestamp клиента как есть
	SentAt      time.Time `ch:"sent_at"`
	ReceivedAt  time.Time `ch:"received_at"`
	InsertID    uuid.UUID `ch:"insert_id"`
	Name        string    `ch:"name"` // "session_start" | "session_end"
	InstallID   uuid.UUID `ch:"install_id"`
	UserID      string    `ch:"user_id"` // "" если отсутствует
	SessionID   uuid.UUID `ch:"session_id"`
	DurationMs  uint32    `ch:"duration_ms"`
	Environment string    `ch:"environment"`
	Platform    string    `ch:"platform"`
	OSName      string    `ch:"os_name"`
	OSVersion   string    `ch:"os_version"`
	AppVersion  string    `ch:"app_version"`
	AppBuild    string    `ch:"app_build"`
	DeviceModel string    `ch:"device_model"`
	DeviceType  string    `ch:"device_type"`
	Locale      string    `ch:"locale"`
	Country     string    `ch:"country"`
	Region      string    `ch:"region"`
	City        string    `ch:"city"`
	WriteKeyID  uuid.UUID `ch:"write_key_id"`
}

// RejectedRow — одна строка appmetrica.rejected_events.
type RejectedRow struct {
	ReceivedAt    time.Time `ch:"received_at"`
	AppID         uuid.UUID `ch:"app_id"` // uuid.Nil, если ключ не распознан
	KeyPrefix     string    `ch:"key_prefix"`
	Code          string    `ch:"code"`
	Field         string    `ch:"field"`
	Message       string    `ch:"message"`
	EventName     string    `ch:"event_name"`
	AppVersion    string    `ch:"app_version"`
	Platform      string    `ch:"platform"`
	PayloadSample string    `ch:"payload_sample"` // ≤ 4096 байт, без context.ip
}
