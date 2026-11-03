package chstore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// ackContext включает асинхронную вставку с ожиданием подтверждения: сервер отвечает только после
// сброса буфера в таблицу, поэтому nil означает, что строки уже видны SELECT (ответ 200 ingest после ack).
// clickhouse.WithAsync действует только на Exec, для PrepareBatch те же настройки передаются явно.
func ackContext(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"async_insert":          1,
		"wait_for_async_insert": 1,
	}))
}

// eventColumns — явный список колонок INSERT в порядке полей EventRow. Без списка AppendStruct требует поле
// на каждую колонку таблицы, и образ предыдущего релиза перестаёт писать после ADD COLUMN: откат
// потребовал бы migrate down (проверка — rollback_integration_test.go).
const eventColumns = ` (app_id, event_date, event_time, client_time, sent_at, received_at, insert_id, name, install_id,
 user_id, session_id, duration_ms, environment, platform, os_name, os_version, app_version, app_build,
 device_model, device_type, locale, country, region, city, write_key_id, properties)`

// InsertEvents пишет строки в events одним батчем. Пустой батч — no-op.
func (s *Store) InsertEvents(ctx context.Context, rows []EventRow) error {
	return insertBatch(ctx, s, "events", eventColumns, rows)
}

// InsertRejected пишет строки в rejected_events одним батчем. Пустой батч — no-op.
func (s *Store) InsertRejected(ctx context.Context, rows []RejectedRow) error {
	return insertBatch(ctx, s, "rejected_events", "", rows)
}

// insertBatch пишет rows в table; columns — необязательный список колонок в скобках.
func insertBatch[T any](ctx context.Context, s *Store, table, columns string, rows []T) (err error) {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ackContext(ctx), "INSERT INTO "+table+columns)
	if err != nil {
		return fmt.Errorf("chstore: prepare insert %s: %w", table, err)
	}
	defer func() {
		if err != nil {
			_ = batch.Abort()
		}
	}()
	for i := range rows {
		if err = batch.AppendStruct(&rows[i]); err != nil {
			return fmt.Errorf("chstore: append %s row %d: %w", table, i, err)
		}
	}
	if err = batch.Send(); err != nil {
		return fmt.Errorf("chstore: insert %s: %w", table, err)
	}
	return nil
}
