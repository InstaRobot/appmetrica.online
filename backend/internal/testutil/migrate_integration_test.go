//go:build integration

package testutil_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// NewPostgresAt открывает базу на заданной версии миграций, MigrateTo поднимает и откатывает её схему.
func TestHarness_MigrateTo(t *testing.T) {
	ctx := t.Context()
	pool := testutil.NewPostgresAt(t, 1)
	require.Equal(t, int64(1), pgVersion(t, pool))
	require.False(t, hasColumn(t, pool, "apps", "platforms"))

	require.NoError(t, testutil.MigrateTo(ctx, pool, 2))
	require.Equal(t, int64(2), pgVersion(t, pool))
	require.True(t, hasColumn(t, pool, "apps", "platforms"))

	require.NoError(t, testutil.MigrateTo(ctx, pool, 1))
	require.Equal(t, int64(1), pgVersion(t, pool))
	require.False(t, hasColumn(t, pool, "apps", "platforms"))
}

func pgVersion(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var v int64
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT version_id FROM goose_db_version WHERE is_applied ORDER BY id DESC LIMIT 1`).Scan(&v))
	return v
}

func hasColumn(t *testing.T, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var ok bool
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`,
		table, column).Scan(&ok))
	return ok
}
