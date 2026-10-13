package main

import (
	"context"
	"flag"
	"io"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
)

// runWorker — `appmetrica worker`: фоновые задачи; на WORKER_LISTEN_ADDR отдаёт /health.
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
	h := newRouter(logger, clock.System{})
	return serveHTTP(ctx, logger, cfg.ListenAddr, h)
}
