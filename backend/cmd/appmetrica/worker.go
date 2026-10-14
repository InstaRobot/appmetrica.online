package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"strings"

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

	stats := reports.NewService(ch.Conn(), c)
	executor := deletion.NewExecutor(pg, deletion.NewClickHousePurger(ch.Conn()), c, logger,
		worker.DeletionFailedNotifier(pg.Queries(), tg, logger))
	checkCfg := worker.SystemCheckConfig{
		Store: pg.Queries(), ClickHouse: ch, Telegram: tg, Lock: pg, Clock: c, Logger: logger,
		HealthchecksPingURL: cfg.HealthchecksPingURL,
		DiskFreeAlertPct:    cfg.DiskFreeAlertPct,
		IngestHealthURL:     cfg.IngestHealthURL,
		APIHealthURL:        cfg.APIHealthURL,
		IngestMetricsURL:    cfg.IngestMetricsURL,
	}
	if cfg.BackupAPIURL != "" {
		checkCfg.Backups = chstore.NewBackupClient(cfg.BackupAPIURL, nil)
	}
	check := worker.NewSystemCheck(checkCfg)
	sched := worker.NewScheduler(pg.Queries(), c, logger, cfg.JobsEnabled,
		check.Job(),
		worker.DeletionJob(executor),
		worker.CloseDayJob(pg.Queries(), stats, pg, c, worker.DefaultCloseMaxDays),
		worker.DigestJob(pg.Queries(), stats, tg, c, logger, cfg.PublicBaseURL),
		worker.SessionCleanupJob(pg.Queries(), c),
	)
	if err := errors.Join(sched.RegisterMetrics(reg), check.RegisterMetrics(reg)); err != nil {
		logger.Error("metrics init failed", slog.Any("error", err))
		return 1
	}
	logger.Info("jobs enabled", slog.String("jobs", strings.Join(sched.Jobs(), ",")))

	return serveProcess(ctx, logger, cfg.ListenAddr, newRouter(logger, c), cfg.MetricsListenAddr, metricsHandler(reg), sched.Run)
}
