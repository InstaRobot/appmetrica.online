package merge

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
)

// Запросы шага copy (02-data-model-diff.md §7.1). Копия партиции читает одну партицию источника по первичному
// ключу (app_id первый в ORDER BY); materialized views пересобирают install_first_seen, user_first_seen и
// identity_links цели из вставленных строк. Повтор недокопированной партиции даёт строки с тем же ключом
// сортировки — их схлопывает ReplacingMergeTree(received_at), отчёты считают людей через uniqExact.
const (
	// qSourcePartitions — месячные партиции событий источника: число строк и первый и последний день
	// в часовом поясе tz. Партиция — единица копирования шага copy.
	qSourcePartitions = `
SELECT toYYYYMM(event_date) AS partition, count() AS rows,
       toDate(min(event_time), {tz:String}) AS first_day, toDate(max(event_time), {tz:String}) AS last_day
FROM events
WHERE app_id = {source:UUID}
GROUP BY partition
ORDER BY partition`

	qCopyPartition = `
INSERT INTO events (app_id, event_date, event_time, client_time, sent_at, received_at, insert_id, name,
                    install_id, user_id, session_id, duration_ms, environment, platform, os_name, os_version,
                    app_version, app_build, device_model, device_type, locale, country, region, city, write_key_id)
SELECT {target:UUID}, event_date, event_time, client_time, sent_at, received_at, insert_id, name,
       install_id, user_id, session_id, duration_ms, environment, platform, os_name, os_version,
       app_version, app_build, device_model, device_type, locale, country, region, city, write_key_id
FROM events
WHERE app_id = {source:UUID} AND toYYYYMM(event_date) = {partition:UInt32}
SETTINGS async_insert = 0, max_memory_usage = 2000000000, max_insert_threads = 2`

	// qCopyRejected — журнал отказов источника (TTL 7 дней). rejected_events — MergeTree без схлопывания:
	// строки, уже скопированные прошлой попыткой шага, пропускаются.
	qCopyRejected = `
INSERT INTO rejected_events (received_at, app_id, key_prefix, code, field, message, event_name, app_version, platform, payload_sample)
SELECT received_at, {target:UUID}, key_prefix, code, field, message, event_name, app_version, platform, payload_sample
FROM rejected_events
WHERE app_id = {source:UUID}
  AND (received_at, key_prefix, code, field, payload_sample) NOT IN (
      SELECT received_at, key_prefix, code, field, payload_sample FROM rejected_events WHERE app_id = {target:UUID})
SETTINGS async_insert = 0`
)

// copier — ClickHouse-часть шага copy; в unit-тестах подменяется.
type copier interface {
	// partitions — партиции событий источника по возрастанию; дни — в часовом поясе tz.
	partitions(ctx context.Context, sourceID uuid.UUID, tz string) ([]Partition, error)
	copyPartition(ctx context.Context, sourceID, targetID uuid.UUID, partition uint32) error
	copyRejected(ctx context.Context, sourceID, targetID uuid.UUID) error
}

// chCopier — copier поверх соединения ClickHouse рабочего пользователя (INSERT и SELECT на базу).
type chCopier struct{ conn driver.Conn }

func (c chCopier) partitions(ctx context.Context, sourceID uuid.UUID, tz string) ([]Partition, error) {
	ctx = clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{
		"source": sourceID.String(),
		"tz":     tz,
	}))
	rows, err := c.conn.Query(ctx, qSourcePartitions)
	if err != nil {
		return nil, fmt.Errorf("merge: source partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Partition
	for rows.Next() {
		var p Partition
		if err := rows.Scan(&p.ID, &p.Rows, &p.FirstDay, &p.LastDay); err != nil {
			return nil, fmt.Errorf("merge: scan partition: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("merge: source partitions: %w", err)
	}
	return out, nil
}

func (c chCopier) copyPartition(ctx context.Context, sourceID, targetID uuid.UUID, partition uint32) error {
	ctx = clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{
		"source":    sourceID.String(),
		"target":    targetID.String(),
		"partition": fmt.Sprint(partition),
	}))
	if err := c.conn.Exec(ctx, qCopyPartition); err != nil {
		return fmt.Errorf("clickhouse: insert into events: %w", err)
	}
	return nil
}

func (c chCopier) copyRejected(ctx context.Context, sourceID, targetID uuid.UUID) error {
	ctx = clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{
		"source": sourceID.String(),
		"target": targetID.String(),
	}))
	if err := c.conn.Exec(ctx, qCopyRejected); err != nil {
		return fmt.Errorf("merge: copy rejected events: %w", err)
	}
	return nil
}
