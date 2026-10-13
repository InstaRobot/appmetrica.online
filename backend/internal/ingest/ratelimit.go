package ingest

import (
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ratelimit"
)

type RateLimits struct {
	Apps     *ratelimit.AppLimiter
	Installs *ratelimit.InstallLimiter
	Metrics  *Metrics
}

func (l RateLimits) LimitApp(next Processor) Processor { return next }

func (l RateLimits) AllowInstall(uuid.UUID, *Event) *ingestapi.Rejection { return nil }

func (l RateLimits) Register(prometheus.Registerer) error { return nil }
