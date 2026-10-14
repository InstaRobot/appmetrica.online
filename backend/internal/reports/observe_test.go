package reports

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
)

// Части отчёта выполняются одновременно: каждая ждёт, пока стартует другая.
func TestParallel_RunsConcurrently(t *testing.T) {
	a, b := make(chan struct{}), make(chan struct{})
	wait := func(started, other chan struct{}) func(ctx context.Context) error {
		return func(context.Context) error {
			close(started)
			select {
			case <-other:
				return nil
			case <-time.After(5 * time.Second):
				return errors.New("вторая часть не стартовала")
			}
		}
	}
	require.NoError(t, parallel(t.Context(), wait(a, b), wait(b, a)))
}

// Первая ошибка возвращается и отменяет контекст остальных частей.
func TestParallel_FirstErrorCancelsOthers(t *testing.T) {
	boom := errors.New("boom")
	err := parallel(t.Context(),
		func(context.Context) error { return boom },
		func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return errors.New("контекст не отменён")
			}
		},
	)
	require.ErrorIs(t, err, boom)
	require.NoError(t, parallel(t.Context()))
}

// Наблюдатель получает имя отчёта и время между началом и концом замера по часам сервиса.
func TestService_TrackObservesReport(t *testing.T) {
	clk := clock.NewFixed(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	svc := NewService(nil, clk)
	svc.track(ReportRetention)() // без наблюдателя — ничего не происходит

	type observed struct {
		report string
		d      time.Duration
	}
	var got []observed
	svc.ObserveQueries(func(report string, d time.Duration) { got = append(got, observed{report, d}) })
	done := svc.track(ReportRetention)
	clk.Advance(1200 * time.Millisecond)
	done()
	require.Equal(t, []observed{{ReportRetention, 1200 * time.Millisecond}}, got)

	svc.ObserveQueries(nil)
	svc.track(ReportPortfolio)()
	require.Len(t, got, 1)
}
