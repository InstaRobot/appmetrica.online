//go:build integration

package chstore_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/testutil"
)

func TestClickHouseMigrations_AppliedAndIdempotent(t *testing.T) {
	ctx := t.Context()
	p, err := chstore.NewMigrationProvider(options(t, adminUser, adminPassword))
	require.NoError(t, err)
	defer func() { require.NoError(t, p.Close()) }()

	v, err := p.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(4), v)

	res, err := p.Up(ctx)
	require.NoError(t, err)
	require.Empty(t, res, "повторный up — no-op")

	type table struct {
		Name       string `ch:"name"`
		Engine     string `ch:"engine"`
		EngineFull string `ch:"engine_full"`
	}
	var tables []table
	require.NoError(t, adminConn(t).Select(ctx, &tables, `
		SELECT name, engine, engine_full FROM system.tables
		WHERE database = 'appmetrica' ORDER BY name`))
	got := map[string]table{}
	for _, tb := range tables {
		got[tb.Name] = tb
	}

	wantEngines := map[string]string{
		"events":                "ReplacingMergeTree",
		"install_first_seen":    "AggregatingMergeTree",
		"user_first_seen":       "AggregatingMergeTree",
		"identity_links":        "AggregatingMergeTree",
		"rejected_events":       "MergeTree",
		"daily_stats":           "ReplacingMergeTree",
		"install_first_seen_mv": "MaterializedView",
		"user_first_seen_mv":    "MaterializedView",
		"identity_links_mv":     "MaterializedView",
		"goose_db_version":      "MergeTree", // таблица версий попадает в бэкап clickhouse-backup
	}
	require.Len(t, got, len(wantEngines))
	for name, engine := range wantEngines {
		require.Equal(t, engine, got[name].Engine, name)
	}

	require.Contains(t, got["events"].EngineFull, "TTL event_date + toIntervalMonth(25)")
	require.Contains(t, got["rejected_events"].EngineFull, "TTL toDate(received_at) + toIntervalDay(7)")
	require.NotContains(t, got["daily_stats"].EngineFull, "TTL", "daily_stats хранится бессрочно")
	for _, agg := range []string{"install_first_seen", "user_first_seen", "identity_links"} {
		require.Contains(t, got[agg].EngineFull, "toIntervalMonth(25)", agg)
	}
}

// AC-002.1: 00004 добавляет последней колонкой properties; у старых строк {}; down убирает колонку,
// строки остаются.
func TestMigration00004EventProperties(t *testing.T) {
	ctx := t.Context()
	env := testutil.StartEmpty(t)
	p, err := chstore.NewMigrationProvider(env.ClickHouse)
	require.NoError(t, err)
	defer func() { require.NoError(t, p.Close()) }()
	conn := env.CH.Conn()

	_, err = p.UpTo(ctx, 3)
	require.NoError(t, err)
	at := base.UTC()
	appID := uuid.New()
	require.NoError(t, conn.Exec(ctx, `
		INSERT INTO events (app_id, event_date, event_time, client_time, sent_at, received_at, insert_id, name,
		                    install_id, session_id, environment, platform)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'session_start', ?, ?, 'production', 'ios')`,
		appID, at, at, at, at, at, uuid.New(), uuid.New(), uuid.New()))

	columns := func() []string {
		var names []string
		rows, err := conn.Query(ctx, `SELECT name FROM system.columns
			WHERE database = currentDatabase() AND table = 'events' ORDER BY position`)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var n string
			require.NoError(t, rows.Scan(&n))
			names = append(names, n)
		}
		require.NoError(t, rows.Err())
		return names
	}
	require.NotContains(t, columns(), "properties")

	_, err = p.UpTo(ctx, 4)
	require.NoError(t, err)
	cols := columns()
	require.Equal(t, "properties", cols[len(cols)-1], "колонка последняя")
	var typ string
	require.NoError(t, conn.QueryRow(ctx, `SELECT type FROM system.columns
		WHERE database = currentDatabase() AND table = 'events' AND name = 'properties'`).Scan(&typ))
	require.Equal(t, "Map(LowCardinality(String), String)", typ)
	var props map[string]string
	require.NoError(t, conn.QueryRow(ctx, "SELECT properties FROM events WHERE app_id = ?", appID).Scan(&props))
	require.Empty(t, props, "у старых строк {}")

	_, err = p.DownTo(ctx, 3)
	require.NoError(t, err)
	require.NotContains(t, columns(), "properties")
	require.Equal(t, uint64(1), countEvents(t, conn, appID), "строки на месте после down")
}
