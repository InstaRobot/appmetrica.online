package chstore

import (
	"context"
	"errors"
	"time"

	"appmetrica.online/backend/internal/clock"
)

// Значения RejectedLogConfig по умолчанию (02-tech-spec.md Story 1.1, шаг 7).
const (
	DefaultRejectedLogCapacity  = 10_000
	DefaultRejectedLogBatchSize = 1000
	DefaultRejectedLogFlush     = 2 * time.Second
	// DefaultRejectedLogMaxAge — предел ожидания строки в буфере: при зависшем ClickHouse строки старше
	// отбрасываются, а не пишутся с опозданием. Вместе с DefaultRejectedLogInsertTimeout ограничивает, насколько
	// позже момента отказа строка может появиться в rejected_events (на это опирается deletion.AppPurgeDelay).
	DefaultRejectedLogMaxAge = 20 * time.Second
	// DefaultRejectedLogInsertTimeout — предел одного INSERT; без него драйвер ждёт ответ до 5 мин, и строки
	// за зависшей вставкой лежали бы в буфере неограниченно.
	DefaultRejectedLogInsertTimeout = 5 * time.Second

	rejectedStopTimeout = 10 * time.Second
)

// RejectedInserter — запись в rejected_events (реализует *Store).
type RejectedInserter interface {
	InsertRejected(ctx context.Context, rows []RejectedRow) error
}

// RejectedLogConfig — параметры журнала отказов; нулевые значения заменяются значениями по умолчанию.
type RejectedLogConfig struct {
	Inserter      RejectedInserter
	Clock         clock.Clock // nil — clock.System
	Capacity      int
	BatchSize     int
	FlushInterval time.Duration
	// MaxAge — строки, пролежавшие в буфере дольше, при сбросе отбрасываются (OnDropped), а не пишутся.
	MaxAge time.Duration
	// InsertTimeout — предел одного INSERT.
	InsertTimeout time.Duration
	// OnError получает ошибку каждого неудачного INSERT; nil — ошибки только возвращаются из Flush.
	OnError func(err error)
	// OnDropped получает число строк, отброшенных при сбросе по MaxAge; nil — без учёта. Строки, не
	// поместившиеся в буфер, считает вызывающий по результату Add.
	OnDropped func(n int)
}

// RejectedLog — асинхронная запись rejected_events в режиме best effort: Add не блокирует запрос ingest,
// строки сверх Capacity отбрасываются, Run сбрасывает буфер раз в FlushInterval или по BatchSize строк.
// Строки неудачного INSERT не повторяются, строки старше MaxAge не пишутся: журнал диагностический и
// живёт 7 дней, а запись с опозданием нарушила бы расчёт deletion.AppPurgeDelay.
type RejectedLog struct {
	cfg  RejectedLogConfig
	rows chan queuedRow
	full chan struct{}
}

// queuedRow — строка буфера и момент постановки в него.
type queuedRow struct {
	row   RejectedRow
	added time.Time
}

func NewRejectedLog(cfg RejectedLogConfig) *RejectedLog {
	if cfg.Clock == nil {
		cfg.Clock = clock.System{}
	}
	if cfg.Capacity <= 0 {
		cfg.Capacity = DefaultRejectedLogCapacity
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultRejectedLogBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultRejectedLogFlush
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultRejectedLogMaxAge
	}
	if cfg.InsertTimeout <= 0 {
		cfg.InsertTimeout = DefaultRejectedLogInsertTimeout
	}
	return &RejectedLog{cfg: cfg, rows: make(chan queuedRow, cfg.Capacity), full: make(chan struct{}, 1)}
}

// Add ставит строки в буфер без блокировки и возвращает число отброшенных из-за переполнения.
func (l *RejectedLog) Add(rows ...RejectedRow) (dropped int) {
	now := l.cfg.Clock.Now()
	for _, r := range rows {
		select {
		case l.rows <- queuedRow{row: r, added: now}:
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

// Flush записывает строки, накопленные к моменту вызова, INSERT-ами по BatchSize строк и не дольше
// InsertTimeout каждый. Строки старше MaxAge отбрасываются. Возвращает объединённую ошибку неудачных
// INSERT; их строки потеряны.
func (l *RejectedLog) Flush(ctx context.Context) error {
	var errs []error
	for n := len(l.rows); n > 0; {
		batch := make([]RejectedRow, 0, min(n, l.cfg.BatchSize))
		dropped := 0
		staleBefore := l.cfg.Clock.Now().Add(-l.cfg.MaxAge)
	fill:
		for len(batch)+dropped < cap(batch) {
			select {
			case q := <-l.rows:
				if q.added.Before(staleBefore) {
					dropped++
					continue
				}
				batch = append(batch, q.row)
			default:
				break fill // буфер опустошил параллельный Flush
			}
		}
		if dropped > 0 && l.cfg.OnDropped != nil {
			l.cfg.OnDropped(dropped)
		}
		if len(batch)+dropped == 0 {
			break
		}
		n -= len(batch) + dropped
		if len(batch) == 0 {
			continue
		}
		if err := l.insert(ctx, batch); err != nil {
			if l.cfg.OnError != nil {
				l.cfg.OnError(err)
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (l *RejectedLog) insert(ctx context.Context, batch []RejectedRow) error {
	ctx, cancel := context.WithTimeout(ctx, l.cfg.InsertTimeout)
	defer cancel()
	return l.cfg.Inserter.InsertRejected(ctx, batch)
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
