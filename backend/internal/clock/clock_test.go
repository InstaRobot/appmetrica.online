package clock_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
)

func TestFixed_SetAndAdvance(t *testing.T) {
	start := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	c := clock.NewFixed(start)
	require.Equal(t, start, c.Now())

	c.Advance(90 * time.Second)
	require.Equal(t, start.Add(90*time.Second), c.Now())

	later := start.Add(24 * time.Hour)
	c.Set(later)
	require.Equal(t, later, c.Now())
}

func TestFixed_ConcurrentAdvance(t *testing.T) {
	start := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	c := clock.NewFixed(start)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			c.Advance(time.Millisecond)
			_ = c.Now()
		})
	}
	wg.Wait()
	require.Equal(t, start.Add(50*time.Millisecond), c.Now())
}

func TestSystem_ReturnsUTC(t *testing.T) {
	require.Equal(t, time.UTC, clock.System{}.Now().Location())
}
