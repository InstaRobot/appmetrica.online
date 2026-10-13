//go:build integration

package chstore_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

func TestClickHouseMigrations_AppliedAndIdempotent(t *testing.T) {
	ctx := t.Context()
	p, err := chstore.NewMigrationProvider(options(adminUser, adminPassword))
	require.NoError(t, err)
	defer func() { require.NoError(t, p.Close()) }()

	v, err := p.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), v)

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
