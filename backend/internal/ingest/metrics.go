package ingest

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics — метрики ingest_* (02-tech-spec.md, раздел наблюдаемости).
type Metrics struct {
	// Requests — ingest_requests_total{status}: запросы POST /v1/events по HTTP-статусу ответа.
	Requests *prometheus.CounterVec
}

// NewMetrics создаёт метрики и регистрирует их в reg; повторная регистрация возвращает ошибку.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_requests_total",
			Help: "Запросы POST /v1/events по HTTP-статусу ответа.",
		}, []string{"status"}),
	}
	if err := reg.Register(m.Requests); err != nil {
		return nil, fmt.Errorf("ingest: register metrics: %w", err)
	}
	return m, nil
}
