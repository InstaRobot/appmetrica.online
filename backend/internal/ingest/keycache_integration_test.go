//go:build integration

package ingest_test

import (
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

// requireIngestError проверяет, что Lookup вернул *ingest.Error с заданным кодом.
func requireIngestError(t *testing.T, err error, code string) {
	t.Helper()
	var e *ingest.Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, code, e.Code)
}

func TestKeyCache_PostgresRevokeArchiveAndLastUsed(t *testing.T) {
	ctx := t.Context()
	store, err := pgstore.Open(ctx, testutil.NewPostgresDB(t, true))
	require.NoError(t, err)
	t.Cleanup(store.Close)
	q := store.Queries()

	notes, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Notes iOS", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}, Timezone: "Europe/Moscow"})
	require.NoError(t, err)
	site, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Site", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformWeb}, Timezone: "UTC"})
	require.NoError(t, err)
	k1, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: notes.ID, Key: testKey, Label: "Основной"})
	require.NoError(t, err)
	k2, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: notes.ID, Key: otherKey, Label: "Android 3.x"})
	require.NoError(t, err)
	k3, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: site.ID, Key: "pk_SiteSiteSiteSiteSiteSiteSiteSite", Label: "Основной"})
	require.NoError(t, err)

	metrics, err := ingest.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	clk := clock.NewFixed(receivedAt)
	cache := ingest.NewKeyCache(ingest.KeyCacheConfig{
		Store: q, Clock: clk, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: metrics,
	})
	require.NoError(t, cache.Refresh(ctx))

	info, err := cache.Lookup(testKey)
	require.NoError(t, err)
	require.Equal(t, ingest.KeyInfo{WriteKeyID: k1.ID, AppID: notes.ID, AppStatus: sqlc.AppStatusActive, AppPlatform: sqlc.AppPlatformIos}, info)
	info, err = cache.Lookup(otherKey)
	require.NoError(t, err)
	require.Equal(t, k2.ID, info.WriteKeyID)
	info, err = cache.Lookup(k3.Key)
	require.NoError(t, err)
	require.Equal(t, sqlc.AppPlatformWeb, info.AppPlatform)

	// last_used_at уходит в Postgres одним сбросом.
	require.NoError(t, cache.FlushLastUsed(ctx))
	keys, err := q.ListWriteKeys(ctx, notes.ID)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	for _, k := range keys {
		require.NotNil(t, k.LastUsedAt, k.Label)
		require.True(t, receivedAt.Equal(*k.LastUsedAt), k.Label)
	}

	// Отзыв K1 и архив Site видны после следующего цикла обновления.
	now := receivedAt.Add(time.Second)
	_, err = q.RevokeWriteKey(ctx, sqlc.RevokeWriteKeyParams{ID: k1.ID, AppID: notes.ID, Now: &now})
	require.NoError(t, err)
	_, err = q.ArchiveApp(ctx, sqlc.ArchiveAppParams{ID: site.ID, Now: &now})
	require.NoError(t, err)

	clk.Advance(15 * time.Second)
	require.NoError(t, cache.Refresh(ctx))
	_, err = cache.Lookup(testKey)
	requireIngestError(t, err, "invalid_app_key")
	_, err = cache.Lookup(k3.Key)
	requireIngestError(t, err, "app_archived")
	_, err = cache.Lookup(otherKey)
	require.NoError(t, err)

	// Postgres закрыт: обновление падает, снимок остаётся.
	store.Close()
	require.Error(t, cache.Refresh(ctx))
	_, err = cache.Lookup(otherKey)
	require.NoError(t, err)
}
