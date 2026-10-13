package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/ratelimit"
)

// runIngest — `appmetrica ingest`: приём событий на INGEST_LISTEN_ADDR, метрики на METRICS_LISTEN_ADDR.
func runIngest(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if !parseFlags(flag.NewFlagSet("ingest", flag.ContinueOnError), args, stderr) {
		return 2
	}
	cfg, err := config.LoadIngest()
	if err != nil {
		say(stderr, "ingest: конфигурация: %v\n", err)
		return 1
	}
	logger := newLogger(stdout, cfg.Common, "ingest")

	pg, err := pgstore.Open(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("postgres unavailable", slog.Any("error", err))
		return 1
	}
	defer pg.Close()
	ch, err := chstore.Open(ctx, chstore.Options(cfg.ClickHouse))
	if err != nil {
		logger.Error("clickhouse unavailable", slog.Any("error", err))
		return 1
	}
	defer func() { _ = ch.Close() }()
	geoDB, err := geo.Open(cfg.GeoIPDBPath)
	if err != nil {
		logger.Error("geoip database unavailable", slog.String("path", cfg.GeoIPDBPath), slog.Any("error", err))
		return 1
	}
	defer func() { _ = geoDB.Close() }()

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	srv, err := newIngestServer(cfg, ingestDeps{
		Keys:     pg.Queries(),
		Events:   ch,
		Rejected: ch,
		Geo:      geoDB,
		Ready: []ingest.ReadyCheck{
			{Name: "postgres", Check: func(ctx context.Context) error { return pg.Pool().Ping(ctx) }},
			{Name: "clickhouse", Check: ch.Ping},
		},
	}, reg, logger, clock.System{})
	if err != nil {
		logger.Error("ingest setup failed", slog.Any("error", err))
		return 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, run := range srv.background {
		wg.Go(func() { run(ctx) })
	}
	wg.Go(func() {
		if serveHTTP(ctx, logger, cfg.MetricsListenAddr, metricsHandler(reg)) != 0 {
			cancel()
		}
	})
	code := serveHTTP(ctx, logger, cfg.ListenAddr, srv.handler)
	cancel()
	wg.Wait() // фоновые задачи сбрасывают last_used_at и rejected_events до закрытия соединений
	return code
}

// ingestDeps — внешние зависимости ingest; в production — Postgres, ClickHouse и mmdb DB-IP.
type ingestDeps struct {
	Keys     ingest.KeyStore
	Events   ingest.EventWriter
	Rejected chstore.RejectedInserter
	Geo      geo.Lookuper
	// Ready — проверки хранилищ для /ready; key_cache добавляется всегда.
	Ready []ingest.ReadyCheck
}

// ingestServer — собранный HTTP-обработчик ingest и фоновые задачи, работающие до отмены ctx.
type ingestServer struct {
	handler    http.Handler
	background []func(ctx context.Context)
}

// newIngestServer собирает ingest: маршруты POST/OPTIONS /v1/events, /health, /ready, кэш ключей, лимиты,
// конвейер и журнал отказов, и регистрирует метрики ingest_* в reg.
func newIngestServer(cfg config.Ingest, deps ingestDeps, reg prometheus.Registerer, logger *slog.Logger, clk clock.Clock) (*ingestServer, error) {
	metrics, err := ingest.NewMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("ingest metrics: %w", err)
	}
	keys := ingest.NewKeyCache(ingest.KeyCacheConfig{
		Store: deps.Keys, Clock: clk, Logger: logger, Metrics: metrics, Refresh: cfg.KeyCacheRefresh,
	})
	limits := ingest.RateLimits{
		Apps:     ratelimit.NewAppLimiter(cfg.RateAppEventsPerSec, cfg.RateAppBurst, clk),
		Installs: ratelimit.NewInstallLimiter(cfg.RateInstallPerMin, cfg.RateInstallBurst, clk),
		Metrics:  metrics,
	}
	if err := limits.Register(reg); err != nil {
		return nil, err
	}
	rejectedLog := chstore.NewRejectedLog(chstore.RejectedLogConfig{
		Inserter: deps.Rejected,
		OnError: func(err error) {
			logger.Warn("rejected_events insert failed", slog.Any("error", err))
		},
	})
	rejected := ingest.RejectedLogger{Sink: rejectedLog, Keys: keys, Metrics: metrics}
	pipeline := ingest.NewPipeline(ingest.PipelineConfig{
		Keys:     keys,
		Limits:   limits,
		Enricher: ingest.Enricher{Geo: deps.Geo, AcceptDevelopment: cfg.AcceptDevelopment},
		Time:     ingest.TimeConfig{FutureTolerance: cfg.FutureTolerance, LateWindow: cfg.LateWindow},
		Writer:   deps.Events,
		Rejected: rejected,
		Clock:    clk,
		Logger:   logger,
		Metrics:  metrics,
	})
	// CORS отвечает на preflight сам, поэтому OPTIONS до хендлера не доходит.
	events := httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: pipeline, Clock: clk, Logger: logger, Metrics: metrics, BatchErrors: rejected,
	}), httpx.CORS())
	ready := ingest.NewReadyHandler(append([]ingest.ReadyCheck{keys.ReadyCheck()}, deps.Ready...)...)
	h := newRouter(logger, clk, func(mux *http.ServeMux) {
		mux.Handle("GET /ready", ready)
		mux.Handle("POST /v1/events", events)
		mux.Handle("OPTIONS /v1/events", events)
	})
	return &ingestServer{
		handler:    h,
		background: []func(context.Context){keys.Run, limits.Installs.Run, rejectedLog.Run},
	}, nil
}

// metricsHandler — GET /metrics для Prometheus на отдельном порту METRICS_LISTEN_ADDR.
func metricsHandler(g prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(g, promhttp.HandlerOpts{})
}
