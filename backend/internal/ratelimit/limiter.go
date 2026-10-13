// Package ratelimit — token bucket на ключ (golang.org/x/time/rate) в шардированной map с вытеснением
// простаивающих лимитеров (02-tech-spec.md FR-005). Лимиты локальны для процесса; по IP не лимитируем (CGNAT).
// Пакет не зависит от internal/*: время приходит через интерфейс Clock (его реализует clock.Clock).
package ratelimit

import (
	"context"
	"fmt"
	"hash/maphash"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
)

const (
	// DefaultIdleTTL — простой, после которого лимитер ключа вытесняется. За 10 минут бакет установки
	// (100 событий/мин, burst 1000) пополняется полностью, так что вытеснение не ослабляет лимит.
	DefaultIdleTTL = 10 * time.Minute
	// DefaultSweepInterval — период Sweep в Run.
	DefaultSweepInterval = time.Minute

	shardCount = 64
)

// Clock — источник текущего времени.
type Clock interface {
	Now() time.Time
}

// Config — параметры бакета одного ключа. Rate и Burst положительны, Clock обязателен; нулевые IdleTTL и
// SweepInterval заменяются значениями по умолчанию.
type Config struct {
	// Rate — пополнение, токенов в секунду.
	Rate  rate.Limit
	Burst int
	Clock Clock
	// IdleTTL — простой ключа (с последнего Allow), после которого Sweep удаляет его лимитер.
	IdleTTL time.Duration
	// SweepInterval — период вызова Sweep в Run.
	SweepInterval time.Duration
}

// Limiter — независимый token bucket на каждый ключ K. Безопасен для конкурентного использования.
type Limiter[K comparable] struct {
	cfg    Config
	seed   maphash.Seed
	shards [shardCount]shard[K]
}

type shard[K comparable] struct {
	mu sync.Mutex
	m  map[K]*entry
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time // под mu шарда
}

// New создаёт лимитер; некорректный Config — ошибка программиста (значения проверяет config.Load*), поэтому паника.
func New[K comparable](cfg Config) *Limiter[K] {
	if cfg.Rate <= 0 || cfg.Burst <= 0 || cfg.Clock == nil {
		panic(fmt.Sprintf("ratelimit: invalid config: rate=%v burst=%d clock=%v", cfg.Rate, cfg.Burst, cfg.Clock))
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = DefaultIdleTTL
	}
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = DefaultSweepInterval
	}
	l := &Limiter[K]{cfg: cfg, seed: maphash.MakeSeed()}
	for i := range l.shards {
		l.shards[i].m = make(map[K]*entry)
	}
	return l
}

// Config возвращает параметры лимитера с подставленными значениями по умолчанию.
func (l *Limiter[K]) Config() Config { return l.cfg }

// Rate — пополнение в токенах в секунду.
func (l *Limiter[K]) Rate() float64 { return float64(l.cfg.Rate) }

// Allow списывает n токенов из бакета key. Если токенов не хватает, ничего не списывает и возвращает
// время до пополнения недостающих (всегда > 0). При n > Burst запрос не пройдёт никогда.
// Любой вызов, в том числе отказ, продлевает жизнь лимитера ключа.
func (l *Limiter[K]) Allow(key K, n int) (ok bool, retryAfter time.Duration) {
	now := l.cfg.Clock.Now()
	s := l.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.m[key]
	if e == nil {
		e = &entry{lim: rate.NewLimiter(l.cfg.Rate, l.cfg.Burst)}
		s.m[key] = e
	}
	e.lastSeen = now
	if e.lim.AllowN(now, n) {
		return true, 0
	}
	deficit := float64(n) - e.lim.TokensAt(now)
	return false, time.Duration(math.Ceil(deficit / float64(l.cfg.Rate) * float64(time.Second)))
}

// Len — число лимитеров в памяти.
func (l *Limiter[K]) Len() int {
	total := 0
	for i := range l.shards {
		s := &l.shards[i]
		s.mu.Lock()
		total += len(s.m)
		s.mu.Unlock()
	}
	return total
}

// Sweep удаляет лимитеры ключей, простаивающих не меньше IdleTTL.
func (l *Limiter[K]) Sweep() {
	cutoff := l.cfg.Clock.Now().Add(-l.cfg.IdleTTL)
	for i := range l.shards {
		s := &l.shards[i]
		s.mu.Lock()
		for k, e := range s.m {
			if !e.lastSeen.After(cutoff) {
				delete(s.m, k)
			}
		}
		s.mu.Unlock()
	}
}

// Run вызывает Sweep каждые SweepInterval до отмены ctx.
func (l *Limiter[K]) Run(ctx context.Context) {
	t := time.NewTicker(l.cfg.SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Sweep()
		}
	}
}

// SizeGauge — gauge с числом лимитеров в памяти (Len), вычисляется при сборе метрик.
func (l *Limiter[K]) SizeGauge(opts prometheus.GaugeOpts) prometheus.GaugeFunc {
	return prometheus.NewGaugeFunc(opts, func() float64 { return float64(l.Len()) })
}

func (l *Limiter[K]) shard(key K) *shard[K] {
	return &l.shards[maphash.Comparable(l.seed, key)%shardCount]
}

// InstallKey — ключ лимита установки: install_id в рамках приложения.
type InstallKey struct {
	AppID     uuid.UUID
	InstallID uuid.UUID
}

type (
	// AppLimiter — лимит приложения по app_id (RATE_APP_EVENTS_PER_SEC, RATE_APP_BURST).
	AppLimiter = Limiter[uuid.UUID]
	// InstallLimiter — лимит установки (RATE_INSTALL_PER_MIN, RATE_INSTALL_BURST).
	InstallLimiter = Limiter[InstallKey]
)

// NewAppLimiter — eventsPerSec событий в секунду, burst событий.
func NewAppLimiter(eventsPerSec float64, burst int, c Clock) *AppLimiter {
	return New[uuid.UUID](Config{Rate: rate.Limit(eventsPerSec), Burst: burst, Clock: c})
}

// NewInstallLimiter — perMin событий в минуту, burst событий.
func NewInstallLimiter(perMin, burst int, c Clock) *InstallLimiter {
	return New[InstallKey](Config{Rate: rate.Limit(float64(perMin) / 60), Burst: burst, Clock: c})
}
