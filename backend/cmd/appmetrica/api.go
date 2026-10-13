package main

import (
	"context"
	"flag"
	"io"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
)

// runAPI — `appmetrica api`: Read API и admin API на API_LISTEN_ADDR.
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
	h := newRouter(logger, clock.System{})
	return serveHTTP(ctx, logger, cfg.ListenAddr, h)
}
