package ingest

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics — метрики ingest_* (02-tech-spec.md, раздел наблюдаемости).
type Metrics struct {
	// Requests — ingest_requests_total{status}: запросы POST /v1/events по HTTP-статусу ответа.
	Requests *prometheus.CounterVec
	// KeyCacheSize — ingest_key_cache_size: активных write-ключей в снимке.
	KeyCacheSize prometheus.Gauge
	// KeyCacheRefreshErrors — ingest_key_cache_refresh_errors_total: неудачные перезагрузки снимка.
	KeyCacheRefreshErrors prometheus.Counter
	// KeyCacheLastRefresh — ingest_key_cache_last_refresh_timestamp_seconds: время последней успешной загрузки.
	KeyCacheLastRefresh prometheus.Gauge
	// RateLimited — ingest_rate_limited_total{scope}: батчи с 429 (scope=app) и события с
	// install_rate_limited (scope=install).
	RateLimited *prometheus.CounterVec
}

// NewMetrics создаёт метрики и регистрирует их в reg; повторная регистрация возвращает ошибку.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_requests_total",
			Help: "Запросы POST /v1/events по HTTP-статусу ответа.",
		}, []string{"status"}),
		KeyCacheSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ingest_key_cache_size",
			Help: "Активных write-ключей в кэше ingest.",
		}),
		KeyCacheRefreshErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ingest_key_cache_refresh_errors_total",
			Help: "Неудачные перезагрузки кэша write-ключей из Postgres.",
		}),
		KeyCacheLastRefresh: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ingest_key_cache_last_refresh_timestamp_seconds",
			Help: "Unix-время последней успешной загрузки кэша write-ключей.",
		}),
		RateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_rate_limited_total",
			Help: "Отказы по лимитам: батчи с 429 app_rate_limited (scope=app), события с install_rate_limited (scope=install).",
		}, []string{"scope"}),
	}
	for _, c := range []prometheus.Collector{m.Requests, m.KeyCacheSize, m.KeyCacheRefreshErrors, m.KeyCacheLastRefresh, m.RateLimited} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("ingest: register metrics: %w", err)
		}
	}
	return m, nil
}
