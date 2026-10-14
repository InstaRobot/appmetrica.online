package reports

import (
	"context"
	"sync"
	"time"
)

// Отчёты — значения метки report метрики api_clickhouse_query_duration_seconds.
const (
	ReportPortfolio          = "portfolio"
	ReportAudienceSummary    = "audience_summary"
	ReportAudienceTimeseries = "audience_timeseries"
	ReportRollingActive      = "rolling_active"
	ReportAudienceBreakdown  = "audience_breakdown"
	ReportRetention          = "retention"
	ReportRealtime           = "realtime"
	ReportProfileSearch      = "profile_search"
	ReportProfile            = "profile"
	ReportIntegrationHealth  = "integration_health"
)

// QueryObserver получает время, которое ClickHouse отвечал на отчёт report: от первого запроса вызова до
// последнего, включая неудачные вызовы.
type QueryObserver func(report string, d time.Duration)

// ObserveQueries подключает наблюдателя длительности отчётов; nil отключает его. Вызывается до первого отчёта.
func (s *Service) ObserveQueries(o QueryObserver) {
	s.observe = o
}

// track начинает замер отчёта report: defer s.track(report)().
func (s *Service) track(report string) func() {
	if s.observe == nil {
		return func() {}
	}
	start := s.clock.Now()
	return func() { s.observe(report, s.clock.Now().Sub(start)) }
}

// parallel выполняет fns одновременно; первая ошибка отменяет контекст остальных и возвращается.
func parallel(ctx context.Context, fns ...func(ctx context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for _, fn := range fns {
		wg.Go(func() {
			if err := fn(ctx); err != nil {
				once.Do(func() { firstErr = err; cancel() })
			}
		})
	}
	wg.Wait()
	return firstErr
}
