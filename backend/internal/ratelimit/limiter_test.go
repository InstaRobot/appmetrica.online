package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/ratelimit"
)

var start = time.Date(2026, 10, 13, 9, 15, 2, 0, time.UTC)

// fakeClock — детерминированные часы; internal/clock пакету ratelimit недоступен (07-project-structure.md).
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var appA = uuid.MustParse("0199d690-0000-7000-8000-00000000000a")

func installKey(i int) ratelimit.InstallKey {
	id := uuid.UUID{}
	id[12], id[13], id[14], id[15] = byte(i>>24), byte(i>>16), byte(i>>8), byte(i)
	return ratelimit.InstallKey{AppID: appA, InstallID: id}
}

// defaultInstalls — RATE_INSTALL_PER_MIN=100, RATE_INSTALL_BURST=1000.
func defaultInstalls(c *fakeClock) *ratelimit.InstallLimiter {
	return ratelimit.NewInstallLimiter(100, 1000, c)
}

func allowed(l *ratelimit.InstallLimiter, key ratelimit.InstallKey, n int) bool {
	ok, _ := l.Allow(key, n)
	return ok
}

func TestInstallLimiter_BurstAndRefill(t *testing.T) {
	t.Run("500+500+1 (I-28)", func(t *testing.T) {
		c := newClock()
		l := defaultInstalls(c)
		x := installKey(1)

		require.True(t, allowed(l, x, 500))
		require.True(t, allowed(l, x, 500))
		ok, retryAfter := l.Allow(x, 1)
		require.False(t, ok, "1001-е событие сверх burst 1000")
		require.InDelta(t, 600*time.Millisecond, retryAfter, float64(time.Microsecond), "1 событие пополняется за 60/100 с")

		c.Advance(60 * time.Second)
		require.True(t, allowed(l, x, 100), "за минуту пополнено 100 событий")
		require.False(t, allowed(l, x, 1))
	})

	t.Run("third batch of 500 is rejected event by event (AC-1.5.3)", func(t *testing.T) {
		c := newClock()
		l := defaultInstalls(c)
		x := installKey(1)
		for range 1000 {
			require.True(t, allowed(l, x, 1))
		}
		for range 500 {
			require.False(t, allowed(l, x, 1))
		}
		c.Advance(60 * time.Second)
		for range 100 {
			require.True(t, allowed(l, x, 1))
		}
		require.False(t, allowed(l, x, 1))
	})

	t.Run("denied request spends no tokens", func(t *testing.T) {
		c := newClock()
		l := defaultInstalls(c)
		x := installKey(1)
		require.True(t, allowed(l, x, 900))
		require.False(t, allowed(l, x, 101))
		require.True(t, allowed(l, x, 100), "отказ не списал 101 токен")
	})

	t.Run("buckets are independent per install and per app", func(t *testing.T) {
		c := newClock()
		l := defaultInstalls(c)
		x := installKey(1)
		require.True(t, allowed(l, x, 1000))
		require.False(t, allowed(l, x, 1))

		require.True(t, allowed(l, installKey(2), 1000), "другая установка того же приложения")
		otherApp := ratelimit.InstallKey{AppID: uuid.MustParse("0199d690-0000-7000-8000-00000000000b"), InstallID: x.InstallID}
		require.True(t, allowed(l, otherApp, 1000), "тот же install_id в другом приложении")
	})

	t.Run("refill never exceeds burst", func(t *testing.T) {
		c := newClock()
		l := defaultInstalls(c)
		x := installKey(1)
		require.True(t, allowed(l, x, 1))
		c.Advance(24 * time.Hour)
		require.False(t, allowed(l, x, 1001))
		require.True(t, allowed(l, x, 1000))
	})

	t.Run("request larger than burst is never allowed", func(t *testing.T) {
		c := newClock()
		l := defaultInstalls(c)
		ok, retryAfter := l.Allow(installKey(1), 1001)
		require.False(t, ok)
		require.Positive(t, retryAfter)
	})
}

func TestInstallLimiter_SweepIdle(t *testing.T) {
	c := newClock()
	l := defaultInstalls(c)
	reg := prometheus.NewRegistry()
	gauge := l.SizeGauge(prometheus.GaugeOpts{Name: "ingest_install_limiters", Help: "Лимитеры установок в памяти."})
	require.NoError(t, reg.Register(gauge))

	for i := range 10_000 {
		require.True(t, allowed(l, installKey(i), 1))
	}
	require.Equal(t, 10_000, l.Len())
	require.InDelta(t, 10_000, gaugeValue(t, reg, "ingest_install_limiters"), 0)

	c.Advance(20 * time.Minute)
	l.Sweep()
	require.Zero(t, l.Len(), "map очищена")
	require.Zero(t, gaugeValue(t, reg, "ingest_install_limiters"))
}

func gaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() == name {
			require.Len(t, mf.GetMetric(), 1)
			return mf.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("метрика %s не зарегистрирована", name)
	return 0
}

func TestInstallLimiter_SweepKeepsRecent(t *testing.T) {
	c := newClock()
	l := defaultInstalls(c)
	idle, active := installKey(1), installKey(2)
	require.True(t, allowed(l, idle, 1000))
	require.True(t, allowed(l, active, 1000))

	c.Advance(9 * time.Minute)
	require.False(t, allowed(l, active, 1000), "обращение обновляет время простоя, даже при отказе")
	require.True(t, allowed(l, active, 500), "за 9 минут пополнено 900, осталось 400")
	c.Advance(time.Minute)
	l.Sweep()
	require.Equal(t, 1, l.Len(), "простой ровно 10 минут — вытеснение, 1 минута — нет")

	require.False(t, allowed(l, active, 501), "оставшийся лимитер сохранил расход: 400 + 100 за минуту")
	require.True(t, allowed(l, active, 500))
	require.True(t, allowed(l, idle, 1000), "вытесненная установка начинает с полного burst")
}

func TestInstallLimiter_Run(t *testing.T) {
	c := newClock()
	l := ratelimit.New[ratelimit.InstallKey](ratelimit.Config{
		Rate: 100.0 / 60, Burst: 1000, Clock: c, IdleTTL: time.Minute, SweepInterval: 5 * time.Millisecond,
	})
	require.True(t, allowed(l, installKey(1), 1))
	c.Advance(2 * time.Minute)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		l.Run(ctx)
		close(done)
	}()
	require.Eventually(t, func() bool { return l.Len() == 0 }, 2*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
	}
}

func TestInstallLimiter_Concurrent(t *testing.T) {
	c := newClock()
	l := defaultInstalls(c)
	x := installKey(1)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		pass int
	)
	for g := range 16 {
		wg.Go(func() {
			for i := range 200 {
				if allowed(l, x, 1) {
					mu.Lock()
					pass++
					mu.Unlock()
				}
				_ = allowed(l, installKey(g*1000+i), 1)
				if i%50 == 0 {
					l.Sweep()
				}
			}
		})
	}
	wg.Wait()
	require.Equal(t, 1000, pass, "16×200 конкурентных событий одной установки: пропущен ровно burst")
}

func TestAppLimiter_RetryAfter(t *testing.T) {
	c := newClock()
	l := ratelimit.NewAppLimiter(2000, 4000, c)
	require.InDelta(t, 2000, l.Rate(), 0)

	ok, _ := l.Allow(appA, 4000)
	require.True(t, ok)
	ok, retryAfter := l.Allow(appA, 10)
	require.False(t, ok, "burst 4000 исчерпан (I-29)")
	require.InDelta(t, 5*time.Millisecond, retryAfter, float64(time.Microsecond), "10 событий при 2000/с")

	c.Advance(5 * time.Millisecond)
	ok, _ = l.Allow(appA, 10)
	require.True(t, ok)

	ok, retryAfter = l.Allow(appA, 4000)
	require.False(t, ok)
	require.InDelta(t, 2*time.Second, retryAfter, float64(time.Microsecond))
}

func TestNew_DefaultsAndValidation(t *testing.T) {
	l := ratelimit.New[string](ratelimit.Config{Rate: 1, Burst: 1, Clock: newClock()})
	cfg := l.Config()
	require.Equal(t, ratelimit.DefaultIdleTTL, cfg.IdleTTL)
	require.Equal(t, ratelimit.DefaultSweepInterval, cfg.SweepInterval)

	for name, cfg := range map[string]ratelimit.Config{
		"zero rate":  {Rate: 0, Burst: 1, Clock: newClock()},
		"zero burst": {Rate: 1, Burst: 0, Clock: newClock()},
		"nil clock":  {Rate: 1, Burst: 1},
	} {
		t.Run(name, func(t *testing.T) {
			require.Panics(t, func() { ratelimit.New[string](cfg) })
		})
	}
}

func BenchmarkInstallLimiter_Allow(b *testing.B) {
	l := defaultInstalls(newClock())
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = l.Allow(installKey(i%4096), 1)
			i++
		}
	})
}
