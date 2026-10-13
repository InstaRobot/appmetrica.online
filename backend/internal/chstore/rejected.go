package chstore

import (
	"context"
	"errors"
	"time"
)

// Значения RejectedLogConfig по умолчанию (02-tech-spec.md Story 1.1, шаг 7).
const (
	DefaultRejectedLogCapacity  = 10_000
	DefaultRejectedLogBatchSize = 1000
	DefaultRejectedLogFlush     = 2 * time.Second

	rejectedStopTimeout = 10 * time.Second
)

// RejectedInserter — запись в rejected_events (реализует *Store).
type RejectedInserter interface {
	InsertRejected(ctx context.Context, rows []RejectedRow) error
}

// RejectedLogConfig — параметры журнала отказов; нулевые значения заменяются значениями по умолчанию.
type RejectedLogConfig struct {
	Inserter      RejectedInserter
	Capacity      int
	BatchSize     int
	FlushInterval time.Duration
	// OnError получает ошибку каждого неудачного INSERT; nil — ошибки только возвращаются из Flush.
	OnError func(err error)
}

// RejectedLog — асинхронная запись rejected_events в режиме best effort: Add не блокирует запрос ingest,
// строки сверх Capacity отбрасываются, Run сбрасывает буфер раз в FlushInterval или по BatchSize строк.
// Строки неудачного INSERT не повторяются: журнал диагностический и живёт 7 дней.
type RejectedLog struct {
	cfg  RejectedLogConfig
	rows chan RejectedRow
	full chan struct{}
}

func NewRejectedLog(cfg RejectedLogConfig) *RejectedLog {
	if cfg.Capacity <= 0 {
		cfg.Capacity = DefaultRejectedLogCapacity
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultRejectedLogBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultRejectedLogFlush
	}
	return &RejectedLog{cfg: cfg, rows: make(chan RejectedRow, cfg.Capacity), full: make(chan struct{}, 1)}
}

// Add ставит строки в буфер без блокировки и возвращает число отброшенных из-за переполнения.
func (l *RejectedLog) Add(rows ...RejectedRow) (dropped int) {
	for _, r := range rows {
		select {
		case l.rows <- r:
		default:
			dropped++
		}
	}
	if len(l.rows) >= l.cfg.BatchSize {
		select {
		case l.full <- struct{}{}:
		default:
		}
	}
	return dropped
}

// Len — строк в буфере.
func (l *RejectedLog) Len() int { return len(l.rows) }

// Flush записывает строки, накопленные к моменту вызова, INSERT-ами по BatchSize строк. Возвращает
// объединённую ошибку неудачных INSERT; их строки потеряны.
func (l *RejectedLog) Flush(ctx context.Context) error {
	var errs []error
	for n := len(l.rows); n > 0; {
		batch := make([]RejectedRow, 0, min(n, l.cfg.BatchSize))
	fill:
		for len(batch) < cap(batch) {
			select {
			case r := <-l.rows:
				batch = append(batch, r)
			default:
				break fill // буфер опустошил параллельный Flush
			}
		}
		if len(batch) == 0 {
			break
		}
		n -= len(batch)
		if err := l.cfg.Inserter.InsertRejected(ctx, batch); err != nil {
			if l.cfg.OnError != nil {
				l.cfg.OnError(err)
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Run сбрасывает буфер по таймеру и по заполнению BatchSize до отмены ctx, затем выполняет последний сброс.
func (l *RejectedLog) Run(ctx context.Context) {
	ticker := time.NewTicker(l.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rejectedStopTimeout)
			_ = l.Flush(stopCtx)
			cancel()
			return
		case <-ticker.C:
			_ = l.Flush(ctx)
		case <-l.full:
			_ = l.Flush(ctx)
		}
	}
}
