package ingest

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ratelimit"
)

// Значения метки scope у ingest_rate_limited_total.
const (
	rateScopeApp     = "app"
	rateScopeInstall = "install"
)

// RateLimits — лимиты FR-005: бакет приложения списывает весь батч, бакет установки — каждое событие.
// Все поля обязательны.
type RateLimits struct {
	Apps     *ratelimit.AppLimiter
	Installs *ratelimit.InstallLimiter
	Metrics  *Metrics
}

// Register регистрирует gauge ingest_install_limiters — число лимитеров установок в памяти.
func (l RateLimits) Register(reg prometheus.Registerer) error {
	g := l.Installs.SizeGauge(prometheus.GaugeOpts{
		Name: "ingest_install_limiters",
		Help: "Лимитеры установок в памяти ingest.",
	})
	if err := reg.Register(g); err != nil {
		return fmt.Errorf("ingest: register install limiters gauge: %w", err)
	}
	return nil
}

// LimitApp — шаг конвейера после Authenticate: списывает len(events) из бакета приложения Batch.Key.AppID.
// Нехватка — весь батч 429 app_rate_limited с Retry-After в целых секундах (минимум 1), до next он не доходит.
func (l RateLimits) LimitApp(next Processor) Processor {
	return appLimiter{limits: l, next: next}
}

type appLimiter struct {
	limits RateLimits
	next   Processor
}

func (a appLimiter) Process(ctx context.Context, b *Batch) (ingestapi.IngestResult, error) {
	ok, wait := a.limits.Apps.Allow(b.Key.AppID, len(b.Events))
	if !ok {
		a.limits.Metrics.RateLimited.WithLabelValues(rateScopeApp).Inc()
		return ingestapi.IngestResult{}, &Error{
			Status:     http.StatusTooManyRequests,
			Code:       "app_rate_limited",
			Message:    "Превышен лимит " + strconv.FormatFloat(a.limits.Apps.Rate(), 'f', -1, 64) + " событий/с для приложения.",
			RetryAfter: max(1, int(math.Ceil(wait.Seconds()))),
		}
	}
	return a.next.Process(ctx, b)
}

// AllowInstall списывает одно событие из бакета установки (app_id, install_id). Нехватка — rejection
// install_rate_limited с retryable=true: клиент оставляет событие в очереди и досылает позже.
func (l RateLimits) AllowInstall(appID uuid.UUID, ev *Event) *ingestapi.Rejection {
	if ok, _ := l.Installs.Allow(ratelimit.InstallKey{AppID: appID, InstallID: ev.InstallID}, 1); ok {
		return nil
	}
	l.Metrics.RateLimited.WithLabelValues(rateScopeInstall).Inc()
	rej := reject(ev.Index, ev.InsertID.String(), ingestapi.RejectionCodeInstallRateLimited, "install_id",
		"Слишком много событий с этого устройства. Повторите позже.")
	rej.Retryable = true
	return rej
}
