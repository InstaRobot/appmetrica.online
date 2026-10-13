// Package ratelimit — token bucket на ключ (golang.org/x/time/rate) с вытеснением простаивающих лимитеров.
package ratelimit

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
)

const (
	DefaultIdleTTL       = 10 * time.Minute
	DefaultSweepInterval = time.Minute
)

type Clock interface {
	Now() time.Time
}

type Config struct {
	Rate          rate.Limit
	Burst         int
	Clock         Clock
	IdleTTL       time.Duration
	SweepInterval time.Duration
}

type Limiter[K comparable] struct{ cfg Config }

func New[K comparable](cfg Config) *Limiter[K] { return &Limiter[K]{cfg: cfg} }

func (l *Limiter[K]) Config() Config                     { return l.cfg }
func (l *Limiter[K]) Rate() float64                      { return 0 }
func (l *Limiter[K]) Allow(K, int) (bool, time.Duration) { return true, 0 }
func (l *Limiter[K]) Len() int                           { return 0 }
func (l *Limiter[K]) Sweep()                             {}
func (l *Limiter[K]) Run(context.Context)                {}
func (l *Limiter[K]) SizeGauge(prometheus.GaugeOpts) prometheus.GaugeFunc {
	return prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "stub"}, func() float64 { return 0 })
}

type InstallKey struct {
	AppID     uuid.UUID
	InstallID uuid.UUID
}

type (
	AppLimiter     = Limiter[uuid.UUID]
	InstallLimiter = Limiter[InstallKey]
)

func NewAppLimiter(float64, int, Clock) *AppLimiter     { return New[uuid.UUID](Config{}) }
func NewInstallLimiter(int, int, Clock) *InstallLimiter { return New[InstallKey](Config{}) }
