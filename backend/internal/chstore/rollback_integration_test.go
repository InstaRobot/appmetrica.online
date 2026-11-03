//go:build integration

package chstore_test

import (
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// legacyEventRow — строка events до миграции 00004: образ предыдущего релиза не знает про properties.
type legacyEventRow struct {
	AppID       uuid.UUID `ch:"app_id"`
	EventDate   time.Time `ch:"event_date"`
	EventTime   time.Time `ch:"event_time"`
	ClientTime  time.Time `ch:"client_time"`
	SentAt      time.Time `ch:"sent_at"`
	ReceivedAt  time.Time `ch:"received_at"`
	InsertID    uuid.UUID `ch:"insert_id"`
	Name        string    `ch:"name"`
	InstallID   uuid.UUID `ch:"install_id"`
	UserID      string    `ch:"user_id"`
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

// Откат релиза FR-102 (спека §9 п. 4): пишет ли образ предыдущей версии — `INSERT INTO events` без списка
// колонок и AppendStruct структуры без поля properties — в таблицу, где колонка properties уже есть.
// Ответ: не пишет (AppendStruct отказывает). Результат записан в спеку; тест держит его проверяемым
// при обновлении clickhouse-go.
func TestInsertEvents_LegacyInsertIntoTableWithExtraColumn(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	require.NoError(t, env.CH.Conn().Exec(ctx,
		"ALTER TABLE events ADD COLUMN IF NOT EXISTS properties Map(LowCardinality(String), String)"))

	at := base.UTC()
	row := legacyEventRow{
		AppID: uuid.New(), EventDate: time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
		EventTime: at, ClientTime: at, SentAt: at, ReceivedAt: at, InsertID: uuid.New(), Name: "session_start",
		InstallID: uuid.New(), SessionID: uuid.New(), Environment: "production", Platform: "ios",
	}
	batchCtx := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"async_insert": 1, "wait_for_async_insert": 1,
	}))
	batch, err := env.CH.Conn().PrepareBatch(batchCtx, "INSERT INTO events")
	require.NoError(t, err)
	// AppendStruct требует поле на каждую колонку таблицы: старый образ перестаёт писать, откат —
	// сначала `appmetrica migrate` ClickHouse down до 00003.
	err = batch.AppendStruct(&row)
	require.ErrorContains(t, err, `missing destination name "properties"`)
	require.NoError(t, batch.Abort())
}
