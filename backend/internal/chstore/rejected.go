package chstore

import (
	"context"
	"time"
)

const (
	DefaultRejectedLogCapacity  = 10_000
	DefaultRejectedLogBatchSize = 1000
	DefaultRejectedLogFlush     = 2 * time.Second
)

type RejectedInserter interface {
	InsertRejected(ctx context.Context, rows []RejectedRow) error
}

type RejectedLogConfig struct {
	Inserter      RejectedInserter
	Capacity      int
	BatchSize     int
	FlushInterval time.Duration
	OnError       func(err error)
}

type RejectedLog struct{}

func NewRejectedLog(RejectedLogConfig) *RejectedLog { return &RejectedLog{} }
func (l *RejectedLog) Add(...RejectedRow) int       { return 0 }
func (l *RejectedLog) Len() int                     { return 0 }
func (l *RejectedLog) Flush(context.Context) error  { return nil }
func (l *RejectedLog) Run(ctx context.Context)      { <-ctx.Done() }
