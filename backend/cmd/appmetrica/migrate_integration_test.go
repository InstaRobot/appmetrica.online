//go:build integration

package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/testutil"
)

const (
	chMigratePassword = testutil.ClickHouseAdminPassword
	pgPassword        = testutil.PostgresPassword
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

// startStores создаёт пустые базы Postgres и ClickHouse и выставляет env для `appmetrica migrate`.
func startStores(t *testing.T) (pgURL string, ch chstore.Options) {
	t.Helper()
	env := testutil.StartEmpty(t)

	t.Setenv("POSTGRES_URL", env.PostgresURL)
	t.Setenv("CLICKHOUSE_ADDR", env.ClickHouse.Addr)
	t.Setenv("CLICKHOUSE_DATABASE", env.ClickHouse.Database)
	t.Setenv("CLICKHOUSE_MIGRATE_USER", env.ClickHouse.User)
	t.Setenv("CLICKHOUSE_MIGRATE_PASSWORD", env.ClickHouse.Password)
	// Рабочий пользователь в migrate не используется: DDL выполняет только appmetrica_admin.
	t.Setenv("CLICKHOUSE_USER", testutil.ClickHouseRWUser)
	t.Setenv("CLICKHOUSE_PASSWORD", "not-used")
	return env.PostgresURL, env.ClickHouse
}

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(t.Context(), args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestMigrateUp_BothStores(t *testing.T) {
	ctx := t.Context()
	pgURL, chOpts := startStores(t)

	code, stdout, stderr := runCLI(t, "migrate", "status")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "postgres: версия 00000")
	require.Contains(t, stdout, "clickhouse: версия 00000")
	require.Contains(t, stdout, "00003_service_tables.sql")

	code, stdout, stderr = runCLI(t, "migrate", "up")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "postgres: применено миграций: 2, версия 00002")
	require.Contains(t, stdout, "clickhouse: применено миграций: 3, версия 00003")

	code, stdout, stderr = runCLI(t, "migrate", "up")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "postgres: применено миграций: 0, версия 00002")
	require.Contains(t, stdout, "clickhouse: применено миграций: 0, версия 00003")

	code, stdout, stderr = runCLI(t, "migrate", "status")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "postgres: версия 00002")
	require.Contains(t, stdout, "clickhouse: версия 00003")
	require.NotContains(t, stdout, "ожидает")
	require.NotContains(t, stdout+stderr, pgPassword)
	require.NotContains(t, stdout+stderr, chMigratePassword)

	pg, err := pgstore.Open(ctx, pgURL)
	require.NoError(t, err)
	defer pg.Close()
	pp, err := pgstore.NewMigrationProvider(pg.Pool())
	require.NoError(t, err)
	defer func() { require.NoError(t, pp.Close()) }()
	v, err := pp.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), v)

	cp, err := chstore.NewMigrationProvider(chOpts)
	require.NoError(t, err)
	defer func() { require.NoError(t, cp.Close()) }()
	v, err = cp.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), v)
}

func TestMigrateUp_ClickHouseAuthErrorHidesPassword(t *testing.T) {
	startStores(t)
	const wrong = "wrong-migrate-password-9137"
	t.Setenv("CLICKHOUSE_MIGRATE_PASSWORD", wrong)

	code, stdout, stderr := runCLI(t, "migrate", "up")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "clickhouse")
	require.NotContains(t, stdout+stderr, wrong)
}
