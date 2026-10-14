package merge

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// metrics — метрики job app_merge (05-impact-and-regression.md §7.3).
type metrics struct {
	// copiedEvents — merge_copied_events_total: события, скопированные шагом copy.
	copiedEvents prometheus.Counter
	// stepDuration — merge_step_duration_seconds{step}: выполненные и упавшие шаги (ожидание не считается).
	stepDuration *prometheus.HistogramVec
}

func newMetrics() *metrics {
	return &metrics{
		copiedEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "merge_copied_events_total",
			Help: "События, скопированные из источника в цель шагом copy слияния приложений.",
		}),
		stepDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "merge_step_duration_seconds",
			Help: "Длительность шага слияния приложений.",
			// 0,1 с … ≈ 7 ч: перенос ключей — доли секунды, копия 10 млн событий — минуты.
			Buckets: prometheus.ExponentialBuckets(0.1, 4, 10),
		}, []string{"step"}),
	}
}

// RegisterMetrics регистрирует merge_copied_events_total и merge_step_duration_seconds.
func (e *Executor) RegisterMetrics(reg prometheus.Registerer) error {
	for _, c := range []prometheus.Collector{e.metrics.copiedEvents, e.metrics.stepDuration} {
		if err := reg.Register(c); err != nil {
			return fmt.Errorf("merge: register metrics: %w", err)
		}
	}
	return nil
}
