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
	// EventsAccepted — ingest_events_accepted_total{app_id}: принятые события (включая development при
	// INGEST_ACCEPT_DEVELOPMENT=false).
	EventsAccepted *prometheus.CounterVec
	// EventsRejected — ingest_events_rejected_total{code}: отклонённые события по RejectionCode.
	EventsRejected *prometheus.CounterVec
	// Warnings — ingest_warnings_total{code}: предупреждения по WarningCode.
	Warnings *prometheus.CounterVec
	// BatchDuration — ingest_batch_duration_seconds: время обработки запроса POST /v1/events.
	BatchDuration prometheus.Histogram
	// BatchEvents — ingest_batch_events: число событий в разобранном батче.
	BatchEvents prometheus.Histogram
	// InsertDuration — ingest_clickhouse_insert_duration_seconds: вставка в events с ожиданием ack.
	InsertDuration prometheus.Histogram
	// InsertErrors — ingest_clickhouse_insert_errors_total: неудачные вставки (ответ 503).
	InsertErrors prometheus.Counter
	// RejectedLogDropped — ingest_rejected_log_dropped_total: строки rejected_events, не поместившиеся в буфер.
	RejectedLogDropped prometheus.Counter
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
		EventsAccepted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_events_accepted_total",
			Help: "Принятые события по приложению.",
		}, []string{"app_id"}),
		EventsRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_events_rejected_total",
			Help: "Отклонённые события по коду причины.",
		}, []string{"code"}),
		Warnings: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_warnings_total",
			Help: "Предупреждения о принятых событиях по коду.",
		}, []string{"code"}),
		BatchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "ingest_batch_duration_seconds",
			Help:    "Время обработки запроса POST /v1/events, включая ожидание ack ClickHouse.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}),
		BatchEvents: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "ingest_batch_events",
			Help:    "Событий в батче POST /v1/events.",
			Buckets: []float64{1, 2, 5, 10, 25, 50, 100, 250, 500},
		}),
		InsertDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "ingest_clickhouse_insert_duration_seconds",
			Help:    "Вставка принятых событий в ClickHouse с ожиданием ack.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}),
		InsertErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ingest_clickhouse_insert_errors_total",
			Help: "Неудачные вставки событий в ClickHouse (ответ 503 storage_unavailable).",
		}),
		RejectedLogDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ingest_rejected_log_dropped_total",
			Help: "Строки rejected_events, отброшенные из-за переполнения буфера.",
		}),
	}
	for _, c := range []prometheus.Collector{
		m.Requests, m.KeyCacheSize, m.KeyCacheRefreshErrors, m.KeyCacheLastRefresh, m.RateLimited,
		m.EventsAccepted, m.EventsRejected, m.Warnings, m.BatchDuration, m.BatchEvents,
		m.InsertDuration, m.InsertErrors, m.RejectedLogDropped,
	} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("ingest: register metrics: %w", err)
		}
	}
	return m, nil
}
