//go:build integration

package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
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
	args := []string{"--apps", "4", "--days", "3", "--daily-installs", "40", "--write-key", key}
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
	require.Len(t, apps, 4)
	names := make([]string, 0, len(apps))
	var notes sqlc.App
	for _, a := range apps {
		names = append(names, a.Name)
		if a.Name == "Seed Notes" {
			notes = a
		}
	}
	require.ElementsMatch(t, []string{"Seed iOS", "Seed Android", "Seed Site", "Seed Notes"}, names)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid, sqlc.AppPlatformWeb}, notes.Platforms)

	// События Seed Notes — с трёх платформ, часть людей с одним user_id на iOS и Web.
	var platforms []string
	require.NoError(t, env.CH.Conn().QueryRow(ctx,
		"SELECT arraySort(groupUniqArray(platform)) FROM events WHERE app_id = ?", notes.ID).Scan(&platforms))
	require.Equal(t, []string{"android", "ios", "web"}, platforms)
	var cross uint64
	require.NoError(t, env.CH.Conn().QueryRow(ctx, `
		SELECT count() FROM (
			SELECT user_id FROM events WHERE app_id = ? AND user_id != ''
			GROUP BY user_id HAVING uniqExact(platform) > 1
		)`, notes.ID).Scan(&cross))
	require.Positive(t, cross, "есть люди на iOS и Web")

	keys, err := env.PG.Queries().LoadWriteKeyCache(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, key, keys[0].Key)
	require.Equal(t, []sqlc.AppPlatform{sqlc.AppPlatformIos}, keys[0].AppPlatforms)

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
	require.Len(t, apps, 4, "приложения не дублируются")
	keys, err = env.PG.Queries().LoadWriteKeyCache(ctx)
	require.NoError(t, err)
	require.Len(t, keys, 1, "ключ не дублируется")
}
