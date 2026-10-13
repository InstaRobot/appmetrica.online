// Package ingest — приём событий POST /v1/events.
package ingest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
)

type Processor interface {
	Process(ctx context.Context, b *Batch) (ingestapi.IngestResult, error)
}

type Metrics struct {
	Requests *prometheus.CounterVec
}

func NewMetrics(prometheus.Registerer) (*Metrics, error) { return &Metrics{}, nil }

type HandlerConfig struct {
	Processor Processor
	Clock     clock.Clock
	Logger    *slog.Logger
	Metrics   *Metrics
}

// NewHandler — заглушка до реализации T-009.
func NewHandler(HandlerConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotImplemented) })
}
