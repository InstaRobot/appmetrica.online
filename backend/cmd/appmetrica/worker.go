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

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/telegram"
	"appmetrica.online/backend/internal/worker"
)

// runWorker — `appmetrica worker`: фоновые задачи из WORKER_JOBS_ENABLED; на WORKER_LISTEN_ADDR отдаёт /health,
// на METRICS_LISTEN_ADDR — метрики.
func runWorker(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if !parseFlags(flag.NewFlagSet("worker", flag.ContinueOnError), args, stderr) {
		return 2
	}
	cfg, err := config.LoadWorker()
	if err != nil {
		say(stderr, "worker: конфигурация: %v\n", err)
		return 1
	}
	logger := newLogger(stdout, cfg.Common, "worker")
	c := clock.System{}

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

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	var tg worker.TelegramSender
	if cfg.TelegramBotToken != "" {
		client := telegram.New(cfg.TelegramBotToken)
		if err := client.RegisterMetrics(reg); err != nil {
			logger.Error("metrics init failed", slog.Any("error", err))
			return 1
		}
		tg = client
	}

	executor := deletion.NewExecutor(pg, deletion.NewClickHousePurger(ch.Conn()), c, logger,
		deletion.WithOnFailed(worker.DeletionFailedNotifier(pg.Queries(), tg, logger)))
	sched := worker.NewScheduler(pg.Queries(), c, logger, cfg.JobsEnabled,
		worker.DeletionJob(executor),
		worker.CloseDayJob(pg.Queries(), reports.NewService(ch.Conn(), c), pg, c),
	)
	if err := sched.RegisterMetrics(reg); err != nil {
		logger.Error("metrics init failed", slog.Any("error", err))
		return 1
	}
	logger.Info("jobs enabled", slog.String("jobs", strings.Join(sched.Jobs(), ",")))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() { sched.Run(ctx) })
	wg.Go(func() {
		if serveHTTP(ctx, logger, cfg.MetricsListenAddr, metricsHandler(reg)) != 0 {
			cancel()
		}
	})
	code := serveHTTP(ctx, logger, cfg.ListenAddr, newRouter(logger, c))
	cancel()
	wg.Wait()
	return code
}
