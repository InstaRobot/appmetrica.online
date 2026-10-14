package chstore_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
)

// fakeRejectedInserter запоминает вызовы InsertRejected и дедлайны их контекстов; err — ошибка каждого вызова.
type fakeRejectedInserter struct {
	mu        sync.Mutex
	calls     [][]chstore.RejectedRow
	deadlines []time.Duration // дедлайн ctx относительно момента вызова; 0 — без дедлайна
	err       error
}

func (f *fakeRejectedInserter) InsertRejected(ctx context.Context, rows []chstore.RejectedRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, slices.Clone(rows))
	var left time.Duration
	if deadline, ok := ctx.Deadline(); ok {
		left = time.Until(deadline)
	}
	f.deadlines = append(f.deadlines, left)
	return f.err
}

func (f *fakeRejectedInserter) sizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.calls))
	for i, c := range f.calls {
		out[i] = len(c)
	}
	return out
}

func (f *fakeRejectedInserter) codes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		for _, r := range c {
			out = append(out, r.Code)
		}
	}
	return out
}

func rejectedRows(n int) []chstore.RejectedRow {
	rows := make([]chstore.RejectedRow, n)
	for i := range rows {
		rows[i] = chstore.RejectedRow{Code: "c" + strconv.Itoa(i)}
	}
	return rows
}

func TestRejectedLog_AddAndFlush(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins})

	require.Zero(t, log.Add(rejectedRows(3)...))
	require.Equal(t, 3, log.Len())
	require.NoError(t, log.Flush(t.Context()))
	require.Equal(t, []int{3}, ins.sizes(), "один INSERT на сброс")
	require.Equal(t, []string{"c0", "c1", "c2"}, ins.codes(), "порядок строк сохраняется")
	require.Zero(t, log.Len())

	require.NoError(t, log.Flush(t.Context()))
	require.Equal(t, []int{3}, ins.sizes(), "пустой буфер не пишется")
}

func TestRejectedLog_DropsWhenFull(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, Capacity: 2})

	require.Zero(t, log.Add(rejectedRows(1)...))
	require.Equal(t, 2, log.Add(rejectedRows(3)...), "в буфер помещается одна строка из трёх")
	require.Equal(t, 2, log.Len())
	require.NoError(t, log.Flush(t.Context()))
	require.Equal(t, []string{"c0", "c0"}, ins.codes())
}

func TestRejectedLog_FlushInBatches(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, BatchSize: 2})
	log.Add(rejectedRows(5)...)
	require.NoError(t, log.Flush(t.Context()))
	require.Equal(t, []int{2, 2, 1}, ins.sizes())
}

func TestRejectedLog_InsertErrorIsReported(t *testing.T) {
	boom := errors.New("clickhouse: connection refused")
	ins := &fakeRejectedInserter{err: boom}
	var reported []error
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{
		Inserter: ins, BatchSize: 2,
		OnError: func(err error) { reported = append(reported, err) },
	})
	log.Add(rejectedRows(3)...)

	err := log.Flush(t.Context())
	require.ErrorIs(t, err, boom)
	require.Len(t, reported, 2, "ошибка каждого INSERT передаётся в OnError")
	require.Zero(t, log.Len(), "best effort: строки не возвращаются в буфер")
}

// Строка, пролежавшая в буфере дольше MaxAge (ClickHouse завис), отбрасывается при сбросе, а не пишется с
// опозданием: deletion.AppPurgeDelay рассчитан на то, что отказ попадает в rejected_events не позже чем через
// MaxAge + InsertTimeout после постановки в буфер.
func TestRejectedLog_DropsRowsOlderThanMaxAge(t *testing.T) {
	ins := &fakeRejectedInserter{}
	clk := clock.NewFixed(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	var dropped []int
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{
		Inserter: ins, Clock: clk, BatchSize: 2, MaxAge: 20 * time.Second,
		OnDropped: func(n int) { dropped = append(dropped, n) },
	})

	log.Add(rejectedRows(3)...)
	clk.Advance(20 * time.Second)
	fresh := []chstore.RejectedRow{{Code: "fresh"}}
	log.Add(fresh...)
	require.NoError(t, log.Flush(t.Context()))
	require.Equal(t, []string{"c0", "c1", "c2", "fresh"}, ins.codes(), "ровно MaxAge — строка ещё пишется")

	log.Add(rejectedRows(3)...)
	clk.Advance(20*time.Second + time.Millisecond)
	log.Add(fresh...)
	require.NoError(t, log.Flush(t.Context()))
	require.Equal(t, []string{"c0", "c1", "c2", "fresh", "fresh"}, ins.codes(), "старые строки не пишутся")
	require.Equal(t, 3, sum(dropped), "отброшенные строки считаются")
	require.Zero(t, log.Len())
}

func sum(ns []int) (total int) {
	for _, n := range ns {
		total += n
	}
	return total
}

// Каждый INSERT ограничен InsertTimeout: иначе зависшая вставка держала бы остальные строки в буфере до
// таймаута чтения драйвера (5 мин).
func TestRejectedLog_InsertHasDeadline(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, BatchSize: 1, InsertTimeout: 3 * time.Second})
	log.Add(rejectedRows(2)...)
	require.NoError(t, log.Flush(t.Context()))
	require.Len(t, ins.deadlines, 2)
	for _, left := range ins.deadlines {
		require.Greater(t, left, time.Second)
		require.LessOrEqual(t, left, 3*time.Second)
	}
}

func TestRejectedLog_DefaultConfig(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins})
	require.Zero(t, log.Add(rejectedRows(chstore.DefaultRejectedLogCapacity)...))
	require.Equal(t, 1, log.Add(rejectedRows(1)...))
	require.NoError(t, log.Flush(t.Context()))
	sizes := ins.sizes()
	require.Len(t, sizes, chstore.DefaultRejectedLogCapacity/chstore.DefaultRejectedLogBatchSize)
	require.Equal(t, chstore.DefaultRejectedLogBatchSize, sizes[0])
	for _, left := range ins.deadlines {
		require.Greater(t, left, time.Duration(0), "INSERT по умолчанию ограничен DefaultRejectedLogInsertTimeout")
		require.LessOrEqual(t, left, chstore.DefaultRejectedLogInsertTimeout)
	}
}

func TestRejectedLog_RunFlushesOnInterval(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, FlushInterval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { log.Run(ctx); close(done) }()

	log.Add(rejectedRows(1)...)
	require.Eventually(t, func() bool { return len(ins.codes()) == 1 }, 2*time.Second, 5*time.Millisecond)
	cancel()
	<-done
}

func TestRejectedLog_RunFlushesWhenBatchIsFull(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, BatchSize: 3, FlushInterval: time.Hour})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { log.Run(ctx); close(done) }()

	log.Add(rejectedRows(2)...)
	time.Sleep(20 * time.Millisecond)
	require.Empty(t, ins.sizes(), "меньше BatchSize строк ждут таймера")
	log.Add(rejectedRows(1)...)
	require.Eventually(t, func() bool { return len(ins.codes()) == 3 }, 2*time.Second, 5*time.Millisecond)
	cancel()
	<-done
}

func TestRejectedLog_RunFlushesOnStop(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, FlushInterval: time.Hour})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { log.Run(ctx); close(done) }()

	log.Add(rejectedRows(2)...)
	cancel()
	<-done
	require.Equal(t, []string{"c0", "c1"}, ins.codes(), "при остановке буфер сбрасывается")
}

func TestRejectedLog_ConcurrentAddAndFlush(t *testing.T) {
	ins := &fakeRejectedInserter{}
	log := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: ins, BatchSize: 7})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				log.Add(rejectedRows(1)...)
			}
		})
		wg.Go(func() { _ = log.Flush(t.Context()) })
	}
	wg.Wait()
	require.NoError(t, log.Flush(t.Context()))
	require.Len(t, ins.codes(), 400, "ни одна строка не потеряна и не записана дважды")
}
