package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/reports"
)

// runAPI — `appmetrica api`: Read API и admin API на API_LISTEN_ADDR, метрики на METRICS_LISTEN_ADDR.
// При MIGRATE_ON_START=true перед стартом применяет миграции Postgres и ClickHouse (пользователь для DDL).
func runAPI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if !parseFlags(flag.NewFlagSet("api", flag.ContinueOnError), args, stderr) {
		return 2
	}
	cfg, err := config.LoadAPI()
	if err != nil {
		say(stderr, "api: конфигурация: %v\n", err)
		return 1
	}
	logger := newLogger(stdout, cfg.Common, "api")
	c := clock.System{}

	if cfg.MigrateOnStart {
		var out strings.Builder
		if err := migrateUp(ctx, cfg.PostgresURL, cfg.ClickHouseMigrate, &out); err != nil {
			logger.Error("migrations failed", slog.Any("error", err))
			return 1
		}
		logger.Info("migrations applied", slog.String("result", strings.TrimSpace(out.String())))
	}

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
	authSvc, err := auth.NewService(pg, c, logger, cfg)
	if err != nil {
		logger.Error("auth init failed", slog.Any("error", err))
		return 1
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	if err := authSvc.RegisterMetrics(reg); err != nil {
		logger.Error("metrics init failed", slog.Any("error", err))
		return 1
	}

	srv := api.NewServer(authSvc, apps.NewService(pg, c, logger), logger, cfg).
		WithReports(reports.NewService(ch.Conn(), c), pg.Queries()).
		WithReadyChecks(
			httpx.ReadyCheck{Name: "postgres", Check: func(ctx context.Context) error { return pg.Pool().Ping(ctx) }},
			httpx.ReadyCheck{Name: "clickhouse", Check: ch.Ping},
		)
	if err := srv.RegisterMetrics(reg, c); err != nil {
		logger.Error("metrics init failed", slog.Any("error", err))
		return 1
	}
	h := newRouter(logger, c, srv.Register)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() {
		if serveHTTP(ctx, logger, cfg.MetricsListenAddr, metricsHandler(reg)) != 0 {
			cancel()
		}
	})
	code := serveHTTP(ctx, logger, cfg.ListenAddr, h)
	cancel()
	wg.Wait()
	return code
}
