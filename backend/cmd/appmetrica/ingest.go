package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/ingest"
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

type ingestDeps struct {
	Keys     ingest.KeyStore
	Events   ingest.EventWriter
	Rejected chstore.RejectedInserter
	Geo      geo.Lookuper
	Ready    []ingest.ReadyCheck
}

type ingestServer struct {
	handler    http.Handler
	background []func(ctx context.Context)
}

func newIngestServer(config.Ingest, ingestDeps, prometheus.Registerer, *slog.Logger, clock.Clock) (*ingestServer, error) {
	return &ingestServer{handler: http.NotFoundHandler()}, nil
}

func metricsHandler(prometheus.Gatherer) http.Handler { return http.NotFoundHandler() }
