package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/clock"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
)

type metrics struct {
	clock    clock.Clock
	duration *prometheus.HistogramVec
}

// RegisterMetrics подключает api_request_duration_seconds{operation,status}: время операций api.yaml от разбора
// параметров до записи ответа. Отказы аутентификации и CSRF до операции не попадают — их считает auth.
// Вызывается до Register.
func (s *Server) RegisterMetrics(reg prometheus.Registerer, c clock.Clock) error {
	m := &metrics{
		clock: c,
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "api_request_duration_seconds",
			Help:    "Длительность операций Read API и admin API по operationId и HTTP-статусу.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 1.5, 2.5, 5, 10},
		}, []string{"operation", "status"}),
	}
	if err := reg.Register(m.duration); err != nil {
		return fmt.Errorf("api: register metrics: %w", err)
	}
	s.metrics = m
	return nil
}

// observedOp — operationId запроса: его заполняет strict-middleware withOperation, читает observe.
type observedOp struct{ id string }

type operationKey struct{}

// withOperation — strict-сервер передаёт имя Go-метода (GetPortfolio); operationId в api.yaml — то же имя
// со строчной первой буквой (getPortfolio).
func withOperation(f gen.StrictHandlerFunc, method string) gen.StrictHandlerFunc {
	operationID := strings.ToLower(method[:1]) + method[1:]
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		if op, ok := ctx.Value(operationKey{}).(*observedOp); ok {
			op.id = operationID
		}
		return f(ctx, w, r, request)
	}
}

// observe — middleware обёртки операций: меряет запрос и пишет статус ответа.
func (m *metrics) observe(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := m.clock.Now()
		op := &observedOp{}
		sw := httpx.NewStatusWriter(w)
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), operationKey{}, op)))
		if op.id != "" {
			m.duration.WithLabelValues(op.id, strconv.Itoa(sw.Status())).Observe(m.clock.Now().Sub(start).Seconds())
		}
	})
}
