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

// InsertEvents пишет строки в events одним батчем. Пустой батч — no-op.
func (s *Store) InsertEvents(ctx context.Context, rows []EventRow) error {
	return insertBatch(ctx, s, "events", rows)
}

// InsertRejected пишет строки в rejected_events одним батчем. Пустой батч — no-op.
func (s *Store) InsertRejected(ctx context.Context, rows []RejectedRow) error {
	return insertBatch(ctx, s, "rejected_events", rows)
}

func insertBatch[T any](ctx context.Context, s *Store, table string, rows []T) (err error) {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ackContext(ctx), "INSERT INTO "+table)
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
