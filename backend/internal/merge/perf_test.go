//go:build perf

package merge_test

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

// Перф-тест слияния AC-009.3: источник с 10 млн событий за год (13 месячных партиций, 1 млн установок),
// шаги copy и purge_source укладываются в таймаут запуска job app_merge (30 минут), пиковая память запроса
// ≤ 2 GB по system.query_log. Объём — MERGE_PERF_EVENTS (по умолчанию 10 000 000). Запуск:
// go test -count=1 -tags=perf -timeout 60m -v -run TestMergePerf ./internal/merge

const (
	perfMergeEvents   = 10_000_000
	perfMergeLimit    = 30 * time.Minute
	perfMergeMemLimit = 2_000_000_000
)

// qPerfSourceEvents — n событий источника равномерно с 2025-10-14 по 2026-10-13 (UTC).
const qPerfSourceEvents = `
INSERT INTO events (app_id, event_date, event_time, client_time, sent_at, received_at, insert_id, name,
                    install_id, user_id, session_id, duration_ms, environment, platform, os_name, os_version,
                    app_version, app_build, device_model, device_type, locale, country, region, city, write_key_id)
SELECT {source:UUID}, toDate(ts), ts, ts, ts, ts, generateUUIDv4(number), 'session_start',
       toUUID(concat('00000000-0000-4000-8000-', leftPad(lower(hex(number % 1000000)), 12, '0'))),
       concat('u', toString(number % 300000)), generateUUIDv4(number + 1), 60000, 'production', 'android',
       'Android', '15', '3.4.0', '340', 'Pixel 9', 'phone', 'ru-RU', 'RU', '', '', {key:UUID}
FROM (
    SELECT number, toDateTime64('2025-10-14 00:00:00', 3, 'UTC') + toIntervalMillisecond(intDiv(number * 31536000000, {n:UInt64})) AS ts
    FROM numbers({n:UInt64})
)
SETTINGS max_partitions_per_insert_block = 100`

func TestMergePerf(t *testing.T) {
	n := uint64(perfMergeEvents)
	if v := os.Getenv("MERGE_PERF_EVENTS"); v != "" {
		var err error
		n, err = strconv.ParseUint(v, 10, 64)
		require.NoError(t, err)
	}
	te := testutil.Start(t)
	ctx := t.Context()
	q := te.PG.Queries()
	target, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Perf iOS", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}, Timezone: "Europe/Moscow"})
	require.NoError(t, err)
	source, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Perf Android", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformAndroid}, Timezone: "UTC"})
	require.NoError(t, err)

	loadStarted := time.Now()
	gen := clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{
		"source": source.ID.String(), "key": uuid.New().String(), "n": strconv.FormatUint(n, 10),
	}))
	require.NoError(t, te.CH.Conn().Exec(gen, qPerfSourceEvents))
	t.Logf("source: %d events loaded in %s", n, time.Since(loadStarted).Round(time.Second))

	m, err := q.CreateAppMerge(ctx, sqlc.CreateAppMergeParams{
		TargetAppID: target.ID, SourceAppID: &source.ID, SourceName: source.Name, SourcePlatforms: source.Platforms,
		SourceTimezone: source.Timezone, TotalEvents: int64(n), //nolint:gosec // объём теста ≪ 2^63
	})
	require.NoError(t, err)
	_, err = te.PG.Pool().Exec(ctx,
		"UPDATE app_merges SET status = 'running', step = 'copy', started_at = now(), keys_moved_at = now() WHERE id = $1", m.ID)
	require.NoError(t, err)

	exec := merge.NewExecutor(te.PG, te.CH.Conn(), clock.System{}, slog.New(slog.DiscardHandler), nil)
	started := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, perfMergeLimit)
	defer cancel()
	res, err := exec.RunOnce(runCtx, uuid.New())
	elapsed := time.Since(started)
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	t.Logf("copy + purge_source + reclose: %s", elapsed.Round(time.Second))
	require.LessOrEqual(t, elapsed, perfMergeLimit)

	var targetRows, sourceRows uint64
	require.NoError(t, te.CH.Conn().QueryRow(clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{
		"target": target.ID.String(), "source": source.ID.String(),
	})), "SELECT countIf(app_id = {target:UUID}), countIf(app_id = {source:UUID}) FROM events").Scan(&targetRows, &sourceRows))
	require.Equal(t, n, targetRows)
	require.Zero(t, sourceRows)

	// Память запросов слияния (копирование партиций и журнала отказов, удаление источника), без генерации данных.
	require.NoError(t, te.CH.Conn().Exec(ctx, "SYSTEM FLUSH LOGS"))
	var peak uint64
	var queries uint64
	require.NoError(t, te.CH.Conn().QueryRow(clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{
		"since": started.UTC().Add(-time.Second).Format("2006-01-02 15:04:05"),
	})), `SELECT max(memory_usage), count() FROM system.query_log
WHERE type = 'QueryFinish' AND current_database = currentDatabase() AND event_time >= {since:DateTime}
  AND query_kind IN ('Insert', 'Delete', 'Alter', 'Select') AND query NOT LIKE '%system.query_log%'`).Scan(&peak, &queries))
	t.Logf("peak memory_usage: %.1f MB over %d queries", float64(peak)/1e6, queries)
	require.Positive(t, queries)
	require.LessOrEqual(t, peak, uint64(perfMergeMemLimit))
}
