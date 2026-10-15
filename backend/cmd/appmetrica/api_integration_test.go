//go:build integration

package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/pgstore"
)

// syncBuffer — вывод процесса, который читает тест, пока процесс пишет в него из своих горутин.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startAPI запускает `appmetrica api` на свободных портах и ждёт, пока оба листенера (API и метрики) поднимутся.
func startAPI(t *testing.T) (stdout *syncBuffer, stop func() (code int, stderr string)) {
	t.Helper()
	t.Setenv("API_LISTEN_ADDR", "127.0.0.1:0")
	t.Setenv("METRICS_LISTEN_ADDR", "127.0.0.1:0")
	t.Setenv("KEY_HASH_PEPPER", "ZGV2LXBlcHBlci1kby1ub3QtdXNlLWluLXByb2R1Y3Rpb24=")
	t.Setenv("APP_ENV", "production")

	ctx, cancel := context.WithCancel(t.Context())
	var out, errb syncBuffer
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"api"}, &out, &errb) }()

	require.Eventually(t, func() bool {
		select {
		case code := <-done:
			done <- code
			return true
		default:
			return strings.Count(out.String(), `"msg":"listening"`) == 2
		}
	}, time.Minute, 50*time.Millisecond)
	return &out, func() (int, string) {
		cancel()
		return <-done, errb.String() + out.String()
	}
}

func TestRunAPI_MigrateOnStart(t *testing.T) {
	ctx := t.Context()
	pgURL, chOpts := startStores(t)
	t.Setenv("CLICKHOUSE_USER", chOpts.User)
	t.Setenv("CLICKHOUSE_PASSWORD", chOpts.Password)
	t.Setenv("MIGRATE_ON_START", "true")

	out, stop := startAPI(t)
	code, logs := stop()
	require.Equal(t, 0, code, logs)
	require.Contains(t, out.String(), `"msg":"migrations applied"`)
	require.NotContains(t, logs, chMigratePassword)
	require.NotContains(t, logs, pgPassword)

	pg, err := pgstore.Open(ctx, pgURL)
	require.NoError(t, err)
	defer pg.Close()
	pp, err := pgstore.NewMigrationProvider(pg.Pool())
	require.NoError(t, err)
	defer func() { require.NoError(t, pp.Close()) }()
	v, err := pp.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), v)

	cp, err := chstore.NewMigrationProvider(chOpts)
	require.NoError(t, err)
	defer func() { require.NoError(t, cp.Close()) }()
	v, err = cp.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), v)

	// Повторный старт на актуальной схеме — no-op.
	out, stop = startAPI(t)
	code, logs = stop()
	require.Equal(t, 0, code, logs)
	require.Contains(t, out.String(), `"msg":"migrations applied"`)
}

// Без MIGRATE_ON_START схема не трогается: миграции — отдельный шаг `appmetrica migrate up`.
func TestRunAPI_NoMigrateByDefault(t *testing.T) {
	ctx := t.Context()
	pgURL, chOpts := startStores(t)
	t.Setenv("CLICKHOUSE_USER", chOpts.User)
	t.Setenv("CLICKHOUSE_PASSWORD", chOpts.Password)

	out, stop := startAPI(t)
	code, logs := stop()
	require.Equal(t, 0, code, logs)
	require.NotContains(t, out.String(), "migrations applied")

	pg, err := pgstore.Open(ctx, pgURL)
	require.NoError(t, err)
	defer pg.Close()
	pp, err := pgstore.NewMigrationProvider(pg.Pool())
	require.NoError(t, err)
	defer func() { require.NoError(t, pp.Close()) }()
	v, err := pp.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Zero(t, v)
}
