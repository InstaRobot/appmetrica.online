package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
)

// runAPI — `appmetrica api`: Read API и admin API на API_LISTEN_ADDR, метрики на METRICS_LISTEN_ADDR.
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

	pg, err := pgstore.Open(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("postgres unavailable", slog.Any("error", err))
		return 1
	}
	defer pg.Close()
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

	h := newRouter(logger, c, api.NewServer(authSvc, apps.NewService(pg, c, logger), logger, cfg).Register)
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
