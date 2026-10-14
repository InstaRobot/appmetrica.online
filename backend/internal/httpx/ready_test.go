package httpx_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/httpx"
)

func TestRunReadyChecks(t *testing.T) {
	ok := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("connection refused") }
	// hang ждёт отмены: таймаут проверки не даёт зависшей зависимости держать /ready.
	hang := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

	t.Run("all_ok", func(t *testing.T) {
		ready, results := httpx.RunReadyChecks(t.Context(), []httpx.ReadyCheck{{Name: "postgres", Check: ok}, {Name: "clickhouse", Check: ok}})
		require.True(t, ready)
		require.Equal(t, map[string]bool{"postgres": true, "clickhouse": true}, results)
	})

	t.Run("one_fails", func(t *testing.T) {
		ready, results := httpx.RunReadyChecks(t.Context(), []httpx.ReadyCheck{{Name: "postgres", Check: ok}, {Name: "clickhouse", Check: fail}})
		require.False(t, ready)
		require.Equal(t, map[string]bool{"postgres": true, "clickhouse": false}, results)
	})

	t.Run("no_checks", func(t *testing.T) {
		ready, results := httpx.RunReadyChecks(t.Context(), nil)
		require.True(t, ready)
		require.Empty(t, results)
	})

	t.Run("hanging_check_times_out", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		ready, results := httpx.RunReadyChecks(ctx, []httpx.ReadyCheck{{Name: "clickhouse", Check: hang}, {Name: "postgres", Check: ok}})
		require.Less(t, time.Since(start), time.Second)
		require.False(t, ready)
		require.Equal(t, map[string]bool{"clickhouse": false, "postgres": true}, results)
	})
}
