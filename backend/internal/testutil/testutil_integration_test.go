//go:build integration

package testutil_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

func TestHarness_StartsStoresAndLoadsFixtures(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)

	chInfo, err := env.ClickHouseContainer().Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, testutil.ClickHouseImage, chInfo.Config.Image)
	require.Equal(t, "clickhouse/clickhouse-server:26.8.25.14", testutil.ClickHouseImage)
	pgInfo, err := env.PostgresContainer().Inspect(ctx)
	require.NoError(t, err)
	require.Equal(t, testutil.PostgresImage, pgInfo.Config.Image)
	require.Equal(t, "postgres:18.6-trixie", testutil.PostgresImage)

	// Миграции применены в обоих хранилищах.
	var tables uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx,
		"SELECT count() FROM system.tables WHERE database = currentDatabase() AND name IN "+
			"('events','install_first_seen','user_first_seen','identity_links','rejected_events','daily_stats')",
	).Scan(&tables))
	require.Equal(t, uint64(6), tables)
	var pgTables int
	require.NoError(t, env.PG.Pool().QueryRow(ctx,
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('apps', 'write_keys', 'settings')",
	).Scan(&pgTables))
	require.Equal(t, 3, pgTables)
	var settingsRows int
	require.NoError(t, env.PG.Pool().QueryRow(ctx, "SELECT count(*) FROM settings").Scan(&settingsRows))
	require.Equal(t, 1, settingsRows, "seed-строка settings скопирована из шаблонной базы")

	rows := env.LoadNDJSON(t, "testdata/fixtures/smoke.ndjson")
	require.Len(t, rows, 4)
	var n uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT count() FROM events").Scan(&n))
	require.Equal(t, uint64(4), n)
	var installs uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT count() FROM install_first_seen FINAL").Scan(&installs))
	require.Equal(t, uint64(2), installs, "MV в тестовой базе пишут в свои таблицы")

	// Второй вызов переиспользует контейнеры, но получает изолированные базы.
	other := testutil.Start(t)
	require.Equal(t, env.ClickHouseContainer().GetContainerID(), other.ClickHouseContainer().GetContainerID())
	require.Equal(t, env.PostgresContainer().GetContainerID(), other.PostgresContainer().GetContainerID())
	require.NotEqual(t, env.ClickHouse.Database, other.ClickHouse.Database)
	require.NotEqual(t, env.PostgresURL, other.PostgresURL)
	require.NoError(t, other.CH.Conn().QueryRow(ctx, "SELECT count() FROM events").Scan(&n))
	require.Equal(t, uint64(0), n)
}

func TestHarness_StartEmptyHasNoMigrations(t *testing.T) {
	ctx := t.Context()
	env := testutil.StartEmpty(t)

	var tables uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx,
		"SELECT count() FROM system.tables WHERE database = currentDatabase()").Scan(&tables))
	require.Equal(t, uint64(0), tables)
	var pgTables int
	require.NoError(t, env.PG.Pool().QueryRow(ctx,
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&pgTables))
	require.Equal(t, 0, pgTables)
}

func TestHarness_LoadNDJSONMissingFileFails(t *testing.T) {
	env := testutil.Start(t)
	f := runFake(func(ft testing.TB) { env.LoadNDJSON(ft, "testdata/fixtures/absent.ndjson") })
	require.True(t, f.failed)
}

func TestHarness_SharedClickHouseServer(t *testing.T) {
	srv := testutil.ClickHouseServer(t)
	require.NotEmpty(t, srv.Addr)
	require.Equal(t, srv.Addr, testutil.ClickHouseServer(t).Addr)
	require.Equal(t, "appmetrica", srv.Database)
}

func TestHarness_NewPostgresDB(t *testing.T) {
	ctx := t.Context()
	empty, migrated := testutil.NewPostgresDB(t, false), testutil.NewPostgresDB(t, true)
	require.NotEqual(t, empty, migrated)

	for dsn, want := range map[string]int{empty: 0, migrated: 1} {
		conn, err := pgx.Connect(ctx, dsn)
		require.NoError(t, err)
		var n int
		require.NoError(t, conn.QueryRow(ctx,
			"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'settings'").Scan(&n))
		require.NoError(t, conn.Close(ctx))
		require.Equal(t, want, n, dsn)
	}
}

// LoadPerf вставляет весь объём генератора: события, установки и связки identity (через MV) совпадают со
// статистикой генератора.
func TestHarness_LoadPerf(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	cfg := testutil.DefaultPerfConfig()
	cfg.DAU, cfg.Days = 300, 10

	stats := env.LoadPerf(t, cfg)
	require.Positive(t, stats.Events)

	var events, installs, links, users uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT count() FROM events").Scan(&events))
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT uniqExact(install_id) FROM events").Scan(&installs))
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT uniqExact(app_id, install_id) FROM identity_links").Scan(&links))
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT uniqExact(user_id) FROM user_first_seen").Scan(&users))
	require.Equal(t, uint64(stats.Events), events)
	require.Equal(t, uint64(stats.Installs), installs)
	require.Equal(t, uint64(stats.Links), links)
	require.Equal(t, uint64(stats.Users), users)
}
