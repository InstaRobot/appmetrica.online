//go:build integration

package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

func TestSeed_CreatesAppsKeyAndEventsIdempotently(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	t.Setenv("POSTGRES_URL", env.PostgresURL)
	t.Setenv("CLICKHOUSE_ADDR", env.ClickHouse.Addr)
	t.Setenv("CLICKHOUSE_DATABASE", env.ClickHouse.Database)
	t.Setenv("CLICKHOUSE_USER", env.ClickHouse.User)
	t.Setenv("CLICKHOUSE_PASSWORD", env.ClickHouse.Password)

	const key = "pk_devseedkey0000000000000000000000"
	args := []string{"--apps", "3", "--days", "3", "--daily-installs", "10", "--write-key", key}
	clk := clock.NewFixed(testToday.Add(9 * time.Hour))

	countEvents := func() (rows, sessions uint64) {
		t.Helper()
		require.NoError(t, env.CH.Conn().QueryRow(ctx,
			"SELECT count(), uniqExact(session_id) FROM events FINAL").Scan(&rows, &sessions))
		return rows, sessions
	}

	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(ctx, args, &stdout, &stderr, clk), stderr.String())
	require.Contains(t, stdout.String(), "Seed iOS")
	rows, sessions := countEvents()
	require.Positive(t, rows)
	require.Equal(t, rows, 2*sessions, "у каждой сессии start и end")

	apps, err := env.PG.Queries().ListApps(ctx, true)
	require.NoError(t, err)
	require.Len(t, apps, 3)
	names := []string{apps[0].Name, apps[1].Name, apps[2].Name}
	require.ElementsMatch(t, []string{"Seed iOS", "Seed Android", "Seed Site"}, names)

	keys, err := env.PG.Queries().LoadWriteKeyCache(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, key, keys[0].Key)
	require.EqualValues(t, "ios", keys[0].AppPlatform)

	var keyed uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx,
		"SELECT count() FROM events WHERE write_key_id = ?", keys[0].WriteKeyID).Scan(&keyed))
	require.Positive(t, keyed, "события Seed iOS помечены ключом")

	stdout.Reset()
	require.Equal(t, 0, run(ctx, args, &stdout, &stderr, clk), "повторный запуск: %s", stderr.String())
	again, againSessions := countEvents()
	require.Equal(t, rows, again, "повторный запуск не дублирует события")
	require.Equal(t, sessions, againSessions)
	apps, err = env.PG.Queries().ListApps(ctx, true)
	require.NoError(t, err)
	require.Len(t, apps, 3, "приложения не дублируются")
	keys, err = env.PG.Queries().LoadWriteKeyCache(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1, "ключ не дублируется")
}
