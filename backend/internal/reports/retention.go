package reports

import (
	"context"
	"errors"
	"time"
)

// Пределы и умолчания periods getRetention (api.yaml): day 1..90 (30), week 1..52 (12).
const (
	DefaultDayPeriods  = 30
	MaxDayPeriods      = 90
	DefaultWeekPeriods = 12
	MaxWeekPeriods     = 52
)

// RetentionQuery — параметры getRetention: когорты по дням или ISO-неделям (GranularityDay | GranularityWeek),
// Periods ≤ 0 — умолчание для Granularity.
type RetentionQuery struct {
	Granularity Granularity
	Periods     int
}

// Retention — матрица retention (FR-014).
type Retention struct {
	CohortGranularity Granularity       `json:"cohort_granularity"`
	Periods           int               `json:"periods"`
	Cohorts           []RetentionCohort `json:"cohorts"`
	Average           []*float64        `json:"average"`
}

// RetentionCohort — когорта, начинающаяся в CohortStart (YYYY-MM-DD в TZ приложения). Values — D0…Dperiods
// (W0…Wperiods), nil — период ещё не завершился или когорта пуста.
type RetentionCohort struct {
	CohortStart string     `json:"cohort_start"`
	Size        uint64     `json:"size"`
	Values      []*float64 `json:"values"`
}

// cohortPeriod — ключ числа вернувшихся: начало когорты (полночь UTC) и номер периода.
type cohortPeriod struct {
	cohort int64
	period int
}

// Retention — классический retention когорт периода p.
func (s *Service) Retention(_ context.Context, _ Params, _ RetentionQuery) (Retention, error) {
	return Retention{}, errors.New("reports: retention не реализован")
}

// cohortStarts — начала когорт g периода p.
func cohortStarts(_ Period, _ Granularity) []time.Time { return nil }

// retentionMatrix — матрица по числам вернувшихся.
func retentionMatrix(_ Granularity, _ int, _ []time.Time, _ map[cohortPeriod]uint64, _ time.Time) Retention {
	return Retention{}
}

// withDefaults — granularity day и periods по умолчанию.
func (q RetentionQuery) withDefaults() RetentionQuery { return q }
