package reports

import (
	"context"
	"errors"
	"time"
)

// Granularity — корзина временного ряда в TZ приложения: неделя — ISO (с понедельника), месяц — календарный.
type Granularity string

const (
	GranularityHour  Granularity = "hour"
	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
)

// Metric — метрика временного ряда или разбивки.
type Metric string

const (
	MetricActive      Metric = "active"
	MetricNew         Metric = "new"
	MetricSessions    Metric = "sessions"
	MetricAvgDuration Metric = "avg_duration_ms" // только временной ряд
)

// Dimension — измерение разбивки аудитории.
type Dimension string

const (
	DimensionPlatform    Dimension = "platform"
	DimensionOSName      Dimension = "os_name"
	DimensionOSVersion   Dimension = "os_version"
	DimensionAppVersion  Dimension = "app_version"
	DimensionDeviceModel Dimension = "device_model"
	DimensionDeviceType  Dimension = "device_type"
	DimensionCountry     Dimension = "country"
	DimensionCity        Dimension = "city"
	DimensionLocale      Dimension = "locale"
)

// DefaultBreakdownLimit — строк разбивки по умолчанию.
const DefaultBreakdownLimit = 50

// ErrUnsupported — granularity, metric или dimension вне whitelist отчёта.
var ErrUnsupported = errors.New("reports: неподдерживаемые granularity, metric или dimension")

var errNotImplemented = errors.New("reports: not implemented")

// AudienceMetrics — KPI аудитории за период. Длительности — по session_end (max на сессию), nil — нет ни одной
// завершённой сессии.
type AudienceMetrics struct {
	Active           uint64   `json:"active"`
	New              uint64   `json:"new"`
	Sessions         uint64   `json:"sessions"`
	AvgDurationMs    *float64 `json:"avg_duration_ms"`
	MedianDurationMs *float64 `json:"median_duration_ms"`
}

// AudienceSummary — getAudienceSummary: from, to — даты периода в TZ приложения.
type AudienceSummary struct {
	From string `json:"from"`
	To   string `json:"to"`
	AudienceMetrics
	Compare *AudienceCompare `json:"compare,omitempty"`
}

// AudienceCompare — предыдущий период той же длины и изменение к нему в процентах.
type AudienceCompare struct {
	From string `json:"from"`
	To   string `json:"to"`
	AudienceMetrics
	DeltaPct AudienceDelta `json:"delta_pct"`
}

// AudienceDelta — (текущее − предыдущее) / предыдущее × 100; nil — база 0 или нет значения.
type AudienceDelta struct {
	Active           *float64 `json:"active"`
	New              *float64 `json:"new"`
	Sessions         *float64 `json:"sessions"`
	AvgDurationMs    *float64 `json:"avg_duration_ms"`
	MedianDurationMs *float64 `json:"median_duration_ms"`
}

// TimeseriesQuery — параметры getAudienceTimeseries.
type TimeseriesQuery struct {
	Granularity Granularity
	Metric      Metric
	Compare     bool // compare=previous_period
}

// Timeseries — ряд от from до to; ComparePoints — тот же ряд за предыдущий период (nil без compare).
type Timeseries struct {
	Metric        Metric            `json:"metric"`
	Granularity   Granularity       `json:"granularity"`
	Points        []TimeseriesPoint `json:"points"`
	ComparePoints []TimeseriesPoint `json:"compare_points,omitempty"`
}

// TimeseriesPoint — корзина, начинающаяся в TS (в TZ приложения). Value nil — нет значения (avg_duration_ms).
type TimeseriesPoint struct {
	TS    time.Time `json:"ts"`
	Value *float64  `json:"value"`
}

// RollingActive — getRollingActive.
type RollingActive struct {
	Points []RollingPoint `json:"points"`
}

// RollingPoint — активные за день Day, за 7 и за 30 дней, заканчивая им.
type RollingPoint struct {
	Day string `json:"day"`
	DAU uint64 `json:"dau"`
	WAU uint64 `json:"wau"`
	MAU uint64 `json:"mau"`
}

// BreakdownQuery — параметры getAudienceBreakdown; Limit ≤ 0 — DefaultBreakdownLimit.
type BreakdownQuery struct {
	Dimension Dimension
	Metric    Metric
	Limit     int
}

// Breakdown — разбивка метрики по измерению; Total — метрика без разбивки.
type Breakdown struct {
	Dimension Dimension      `json:"dimension"`
	Metric    Metric         `json:"metric"`
	Total     uint64         `json:"total"`
	Rows      []BreakdownRow `json:"rows"`
}

// BreakdownRow — значение измерения ("" — не определено), метрика и доля от Total.
type BreakdownRow struct {
	Value string  `json:"value"`
	Count uint64  `json:"count"`
	Share float64 `json:"share"`
}

func (s *Service) AudienceSummary(_ context.Context, _ Params, _ bool) (AudienceSummary, error) {
	return AudienceSummary{}, errNotImplemented
}

func (s *Service) AudienceTimeseries(_ context.Context, _ Params, _ TimeseriesQuery) (Timeseries, error) {
	return Timeseries{}, errNotImplemented
}

func (s *Service) RollingActive(_ context.Context, _ Params) (RollingActive, error) {
	return RollingActive{}, errNotImplemented
}

func (s *Service) AudienceBreakdown(_ context.Context, _ Params, _ BreakdownQuery) (Breakdown, error) {
	return Breakdown{}, errNotImplemented
}

// Previous — период той же длины непосредственно перед p.
func (p Period) Previous() Period {
	return p
}

// bucketStarts — начала корзин g, покрывающих период p в TZ loc.
func bucketStarts(_ Period, _ *time.Location, _ Granularity) []time.Time {
	return nil
}

// changePct — round((cur − base) / base × 100, 2); nil при нулевой или отсутствующей базе.
func changePct(_, _ *float64) *float64 {
	return nil
}

// shareOf — n / total, округлённая до 4 знаков; 0 при total = 0.
func shareOf(_, _ uint64) float64 {
	return 0
}
