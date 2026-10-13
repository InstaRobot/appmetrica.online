package ingest

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
)

type EventWriter interface {
	InsertEvents(ctx context.Context, rows []chstore.EventRow) error
}

type RejectedSink interface {
	Add(rows ...chstore.RejectedRow) (dropped int)
}

type RejectedLogger struct {
	Sink    RejectedSink
	Keys    *KeyCache
	Metrics *Metrics
}

func (l RejectedLogger) LogBatchError(string, time.Time, []byte, *Error) {}

type PipelineConfig struct {
	Keys          *KeyCache
	Limits        RateLimits
	Enricher      Enricher
	Time          TimeConfig
	Writer        EventWriter
	Rejected      RejectedLogger
	Clock         clock.Clock
	Logger        *slog.Logger
	Metrics       *Metrics
	InsertTimeout time.Duration
}

func NewPipeline(cfg PipelineConfig) Processor { return stubPipeline{} }

type stubPipeline struct{}

func (stubPipeline) Process(context.Context, *Batch) (ingestapi.IngestResult, error) {
	return ingestapi.IngestResult{}, errors.New("not implemented")
}

func (c *KeyCache) Peek(string) (KeyInfo, bool) { return KeyInfo{}, false }
