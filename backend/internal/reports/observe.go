package reports

import (
	"context"
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

// QueryObserver получает время, которое ClickHouse отвечал на отчёт report.
type QueryObserver func(report string, d time.Duration)

// ObserveQueries — заглушка: наблюдатель пока не подключается.
func (s *Service) ObserveQueries(QueryObserver) {}

// track — заглушка замера отчёта.
func (s *Service) track(string) func() { return func() {} }

// parallel — заглушка: выполняет fns по очереди до первой ошибки.
func parallel(ctx context.Context, fns ...func(ctx context.Context) error) error {
	for _, fn := range fns {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}
