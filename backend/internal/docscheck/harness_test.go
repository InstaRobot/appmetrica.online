package docscheck_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/ratelimit"
	"appmetrica.online/backend/internal/testutil"
)

// ingestHandler собирает POST /v1/events как `appmetrica ingest`: CORS → RequestID → хендлер → конвейер.
// Часы системные: сниппеты берут время из `date` и `time.Now`.
func ingestHandler(t *testing.T, keys ingest.KeyStore, events ingest.EventWriter, rejected ingest.RejectedSink) http.Handler {
	t.Helper()
	metrics, err := ingest.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	clk := clock.System{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := ingest.NewKeyCache(ingest.KeyCacheConfig{Store: keys, Clock: clk, Logger: logger, Metrics: metrics})
	require.NoError(t, cache.Refresh(t.Context()))
	geoDB, err := geo.Open(testutil.GeoDBPath())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, geoDB.Close()) })

	rl := ingest.RejectedLogger{Sink: rejected, Keys: cache, Metrics: metrics}
	p := ingest.NewPipeline(ingest.PipelineConfig{
		Keys: cache,
		Limits: ingest.RateLimits{
			Apps:     ratelimit.NewAppLimiter(2000, 4000, clk),
			Installs: ratelimit.NewInstallLimiter(100, 1000, clk),
			Metrics:  metrics,
		},
		Enricher: ingest.Enricher{Geo: geoDB, AcceptDevelopment: true},
		Time:     ingest.DefaultTimeConfig,
		Writer:   events,
		Rejected: rl,
		Clock:    clk,
		Logger:   logger,
		Metrics:  metrics,
	})
	return httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: p, Clock: clk, Logger: logger, Metrics: metrics, BatchErrors: rl,
	}), httpx.CORS(), httpx.RequestID(clk))
}
