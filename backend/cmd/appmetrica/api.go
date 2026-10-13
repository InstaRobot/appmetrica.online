package main

import (
	"context"
	"flag"
	"io"
	"log/slog"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
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

	h := newRouter(logger, c, api.NewServer(authSvc, logger, cfg).Register)
	return serveHTTP(ctx, logger, cfg.ListenAddr, h)
}
