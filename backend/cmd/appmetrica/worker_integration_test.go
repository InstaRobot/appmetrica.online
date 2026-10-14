//go:build integration

package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

// `appmetrica worker` с WORKER_JOBS_ENABLED=deletion сразу после старта разбирает очередь удалений
// и пишет цикл в job_runs; user_id в логи не попадает.
func TestRunWorker_ProcessesDeletionRequests(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("POSTGRES_URL", env.PostgresURL)
	t.Setenv("CLICKHOUSE_ADDR", env.ClickHouse.Addr)
	t.Setenv("CLICKHOUSE_DATABASE", env.ClickHouse.Database)
	t.Setenv("CLICKHOUSE_USER", env.ClickHouse.User)
	t.Setenv("CLICKHOUSE_PASSWORD", env.ClickHouse.Password)
	t.Setenv("WORKER_LISTEN_ADDR", "127.0.0.1:0")
	t.Setenv("METRICS_LISTEN_ADDR", "127.0.0.1:0")
	t.Setenv("WORKER_JOBS_ENABLED", "deletion")

	q := env.PG.Queries()
	app, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "App", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC"})
	require.NoError(t, err)
	const user = "secret-user-42"
	dr, err := deletion.NewService(env.PG, clock.System{}, slog.New(slog.NewTextHandler(io.Discard, nil))).
		Create(ctx, deletion.CreateInput{AppID: app.ID, SubjectType: "user", SubjectValue: user})
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(ctx)
	var out, errb syncBuffer
	done := make(chan int, 1)
	go func() { done <- run(runCtx, []string{"worker"}, &out, &errb) }()

	require.Eventually(t, func() bool {
		got, err := q.GetDeletionRequest(ctx, sqlc.GetDeletionRequestParams{ID: dr.ID, AppID: app.ID})
		return err == nil && got.Status == sqlc.DeletionStatusDone
	}, time.Minute, 100*time.Millisecond, "stdout: %s\nstderr: %s", &out, &errb)
	require.Eventually(t, func() bool {
		runs, err := q.ListLatestJobRuns(ctx)
		return err == nil && len(runs) == 1 && runs[0].Job == sqlc.JobNameDeletion && runs[0].Status == sqlc.JobStatusOk
	}, 10*time.Second, 50*time.Millisecond)
	require.Equal(t, 2, strings.Count(out.String(), `"msg":"listening"`), "health и метрики")

	cancel()
	require.Equal(t, 0, <-done, errb.String())
	require.NotContains(t, out.String()+errb.String(), user)
	require.NotContains(t, out.String()+errb.String(), env.ClickHouse.Password)
}
