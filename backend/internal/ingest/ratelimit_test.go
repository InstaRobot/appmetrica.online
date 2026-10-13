package ingest_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/ratelimit"
)

func newRateLimits(t *testing.T, c *clock.Fixed, installBurst int) (ingest.RateLimits, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics, err := ingest.NewMetrics(reg)
	require.NoError(t, err)
	return ingest.RateLimits{
		Apps:     ratelimit.NewAppLimiter(2000, 4000, c),
		Installs: ratelimit.NewInstallLimiter(100, installBurst, c),
		Metrics:  metrics,
	}, reg
}

func TestRateLimits_AllowInstall(t *testing.T) {
	c := clock.NewFixed(receivedAt)
	limits, reg := newRateLimits(t, c, 2)
	install := uuid.MustParse("6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21")
	ev := func(index int) *ingest.Event {
		return &ingest.Event{
			Index:     index,
			InsertID:  uuid.MustParse("0199d6c1-0000-7000-8000-000000000007"),
			InstallID: install,
		}
	}

	require.Nil(t, limits.AllowInstall(appA, ev(0)))
	require.Nil(t, limits.AllowInstall(appA, ev(1)))
	rej := limits.AllowInstall(appA, ev(7))
	require.Equal(t, &ingestapi.Rejection{
		Index:     7,
		InsertId:  nullable.NewNullableWithValue("0199d6c1-0000-7000-8000-000000000007"),
		Code:      ingestapi.RejectionCodeInstallRateLimited,
		Field:     "events[7].install_id",
		Message:   "Слишком много событий с этого устройства. Повторите позже.",
		Retryable: true,
	}, rej, "пример RejectedInstallRateLimited из ingest.yaml")

	require.Nil(t, limits.AllowInstall(appB, ev(8)), "тот же install_id в другом приложении — свой бакет")
	require.NotNil(t, limits.AllowInstall(appA, ev(9)))
	require.Equal(t, map[string]float64{"install": 2}, labelCounts(t, reg, "ingest_rate_limited_total", "scope"))

	c.Advance(600 * time.Millisecond)
	require.Nil(t, limits.AllowInstall(appA, ev(10)), "100 событий/мин — одно событие за 0,6 с")
	require.NotNil(t, limits.AllowInstall(appA, ev(11)))
}

func TestRateLimits_InstallLimitersGauge(t *testing.T) {
	c := clock.NewFixed(receivedAt)
	limits, reg := newRateLimits(t, c, 1000)
	require.NoError(t, limits.Register(reg))
	require.InDelta(t, 0, metricValue(t, reg, "ingest_install_limiters"), 0)

	for i := range 3 {
		require.Nil(t, limits.AllowInstall(appA, &ingest.Event{InstallID: uuid.New(), Index: i}))
	}
	require.InDelta(t, 3, metricValue(t, reg, "ingest_install_limiters"), 0)

	c.Advance(ratelimit.DefaultIdleTTL)
	limits.Installs.Sweep()
	require.InDelta(t, 0, metricValue(t, reg, "ingest_install_limiters"), 0)
	require.Error(t, limits.Register(reg), "повторная регистрация — ошибка, а не паника")
}
