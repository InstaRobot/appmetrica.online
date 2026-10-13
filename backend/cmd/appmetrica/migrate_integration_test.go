//go:build integration

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/pgstore"
)

const (
	chMigratePassword = "test_admin_password"
	pgPassword        = "test_pg_password"
)

// startStores поднимает пустые Postgres и ClickHouse и выставляет env для `appmetrica migrate`.
func startStores(t *testing.T) (pgURL string, ch chstore.Options) {
	t.Helper()
	ctx := t.Context()

	pg, err := postgres.Run(ctx, "postgres:18.6-trixie",
		postgres.WithDatabase("appmetrica"),
		postgres.WithUsername("appmetrica"),
		postgres.WithPassword(pgPassword),
		postgres.BasicWaitStrategies(),
	)
	if pg != nil {
		t.Cleanup(func() { _ = pg.Terminate(t.Context()) })
	}
	require.NoError(t, err)
	pgURL, err = pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	chc, err := tcclickhouse.Run(ctx, "clickhouse/clickhouse-server:26.8.25.14",
		tcclickhouse.WithDatabase("appmetrica"),
		tcclickhouse.WithUsername("appmetrica_admin"),
		tcclickhouse.WithPassword(chMigratePassword),
	)
	if chc != nil {
		t.Cleanup(func() { _ = chc.Terminate(t.Context()) })
	}
	require.NoError(t, err)
	addr, err := chc.ConnectionHost(ctx)
	require.NoError(t, err)

	t.Setenv("POSTGRES_URL", pgURL)
	t.Setenv("CLICKHOUSE_ADDR", addr)
	t.Setenv("CLICKHOUSE_DATABASE", "appmetrica")
	t.Setenv("CLICKHOUSE_MIGRATE_USER", "appmetrica_admin")
	t.Setenv("CLICKHOUSE_MIGRATE_PASSWORD", chMigratePassword)
	// Рабочий пользователь в migrate не используется: DDL выполняет только appmetrica_admin.
	t.Setenv("CLICKHOUSE_USER", "appmetrica_rw")
	t.Setenv("CLICKHOUSE_PASSWORD", "not-used")
	return pgURL, chstore.Options{Addr: addr, Database: "appmetrica", User: "appmetrica_admin", Password: chMigratePassword}
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
	require.Contains(t, stdout, "postgres: применено миграций: 1, версия 00001")
	require.Contains(t, stdout, "clickhouse: применено миграций: 3, версия 00003")

	code, stdout, stderr = runCLI(t, "migrate", "up")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "postgres: применено миграций: 0, версия 00001")
	require.Contains(t, stdout, "clickhouse: применено миграций: 0, версия 00003")

	code, stdout, stderr = runCLI(t, "migrate", "status")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, stdout, "postgres: версия 00001")
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
	require.Equal(t, int64(1), v)

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
