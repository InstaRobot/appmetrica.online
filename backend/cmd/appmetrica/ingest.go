package main

import (
	"context"
	"flag"
	"io"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
)

// runIngest — `appmetrica ingest`: приём событий на INGEST_LISTEN_ADDR.
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
	h := newRouter(logger, clock.System{})
	return serveHTTP(ctx, logger, cfg.ListenAddr, h)
}
