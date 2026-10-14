package reports

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"appmetrica.online/backend/internal/identity"
)

// Пределы и умолчания periods getRetention (api.yaml): day 1..90 (30), week 1..52 (12).
const (
	DefaultDayPeriods  = 30
	MaxDayPeriods      = 90
	DefaultWeekPeriods = 12
	MaxWeekPeriods     = 52
)

// RetentionQuery — параметры getRetention: когорты по дням или ISO-неделям (GranularityDay | GranularityWeek,
// "" — day), Periods ≤ 0 — умолчание для Granularity.
type RetentionQuery struct {
	Granularity Granularity
	Periods     int
}

// Retention — матрица retention (FR-014). Average[N] — Σ вернувшихся / Σ size по когортам с завершённым
// периодом N, nil — таких когорт нет или все пусты.
type Retention struct {
	CohortGranularity Granularity       `json:"cohort_granularity"`
	Periods           int               `json:"periods"`
	Cohorts           []RetentionCohort `json:"cohorts"`
	Average           []*float64        `json:"average"`
}

// RetentionCohort — когорта, начинающаяся в CohortStart (YYYY-MM-DD в TZ приложения; для недель — понедельник).
// Values — D0…Dperiods (W0…Wperiods): доля когорты с событием в период N; nil — период ещё не завершился к
// сегодняшнему дню в TZ приложения или когорта пуста.
type RetentionCohort struct {
	CohortStart string     `json:"cohort_start"`
	Size        uint64     `json:"size"`
	Values      []*float64 `json:"values"`
}

// cohortPeriod — ключ числа вернувшихся: начало когорты (полночь UTC, Unix) и номер периода.
type cohortPeriod struct {
	cohort int64
	period int
}

// Запрос retention (05-data-model.md §10.4, QRetentionDay / QRetentionWeek). Когорта — люди с first_seen в
// [{start_utc}, {c_end_utc}), активные в день (неделю) когорты; активность — дни (понедельники недель) $UNIT
// событий людей с {start_utc} до {end_utc} = min(конец последнего периода, сейчас), по массиву на человека
// (peopleFirstCTE). Период N — intDiv(дней от начала когорты, $STEP). Фильтры eventsFilter действуют и на
// событие когорты, и на возвраты. Люди — только uniqExact (OQ-042).
const qRetention = `WITH ` + identity.LinksCTE + `, $PEOPLE,
cohort AS (
    SELECT person_key, $UNIT(first_seen, {tz:String}) AS cohort_start,
           arrayDistinct(arrayMap(u -> intDiv(dateDiff('day', cohort_start, u), $STEP),
               arrayFilter(u -> u >= cohort_start, units))) AS periods
    FROM people
    WHERE first_seen >= {start_utc:DateTime64(3)} AND first_seen < {c_end_utc:DateTime64(3)}
)
SELECT cohort_start, toUInt16(period) AS period, uniqExact(person_key) AS persons
FROM cohort ARRAY JOIN periods AS period
WHERE has(periods, 0) AND period <= {periods:UInt16}
GROUP BY cohort_start, period
ORDER BY cohort_start, period`

var (
	qRetentionDay  = retentionSQL("toDate", "1")
	qRetentionWeek = retentionSQL("toMonday", "7")
)

// retentionSQL — qRetention с функцией начала периода unit (toDate | toMonday) и его длиной step в днях.
func retentionSQL(unit, step string) string {
	people := peopleFirstCTE(uniqValues(unit+"(e.event_time, {tz:String})", "units"))
	return strings.NewReplacer("$PEOPLE", people, "$UNIT", unit, "$STEP", step).Replace(qRetention)
}

// withDefaults — granularity day и periods по умолчанию для granularity.
func (q RetentionQuery) withDefaults() RetentionQuery {
	if q.Granularity == "" {
		q.Granularity = GranularityDay
	}
	if q.Periods <= 0 {
		q.Periods = DefaultDayPeriods
		if q.Granularity == GranularityWeek {
			q.Periods = DefaultWeekPeriods
		}
	}
	return q
}

// retentionSpec — SQL, длина периода в днях и предел periods для granularity из whitelist.
func retentionSpec(g Granularity) (query string, step, maxPeriods int, err error) {
	switch g {
	case GranularityDay:
		return qRetentionDay, 1, MaxDayPeriods, nil
	case GranularityWeek:
		return qRetentionWeek, 7, MaxWeekPeriods, nil
	default:
		return "", 0, 0, fmt.Errorf("%w: cohort_granularity", ErrUnsupported)
	}
}

// Retention — классический retention когорт периода p (FR-014): для дней — каждый день from…to, для недель —
// ISO-недели, содержащие from и to, целиком. Активность читается до конца последнего периода, но не позже
// текущего момента; незавершённые к сегодняшнему дню периоды — nil.
func (s *Service) Retention(ctx context.Context, p Params, q RetentionQuery) (Retention, error) {
	defer s.track(ReportRetention)()
	q = q.withDefaults()
	query, step, maxPeriods, err := retentionSpec(q.Granularity)
	if err != nil {
		return Retention{}, err
	}
	if q.Periods > maxPeriods {
		return Retention{}, fmt.Errorf("%w: periods", ErrUnsupported)
	}
	return s.retention(ctx, p, q, query, step)
}

// retention — матрица по запросу query (qRetentionDay | qRetentionWeek) с длиной периода step дней.
func (s *Service) retention(ctx context.Context, p Params, q RetentionQuery, query string, step int) (Retention, error) {
	loc := p.location()
	starts := cohortStarts(p.Period, q.Granularity)
	if len(starts) == 0 {
		return retentionMatrix(q.Granularity, q.Periods, nil, nil, time.Time{}), nil
	}
	first, last := starts[0], starts[len(starts)-1]
	cohortEnd := last.AddDate(0, 0, step-1)
	_, cEnd := Period{From: first, To: cohortEnd}.Bounds(loc)

	act := p
	act.Period = Period{From: first, To: cohortEnd.AddDate(0, 0, step*q.Periods)}
	_, actEnd := act.Period.Bounds(loc)
	now := s.clock.Now().UTC()
	if now.Before(actEnd) {
		actEnd = now
	}

	rows, err := s.conn.Query(act.bindWith(ctx, clickhouse.Parameters{
		"c_end_utc": cEnd.Format(paramDateTime),
		"end_utc":   actEnd.Format(paramDateTime),
		"d_hi":      actEnd.Format(time.DateOnly),
		"periods":   strconv.Itoa(q.Periods),
	}), query)
	if err != nil {
		return Retention{}, fmt.Errorf("reports: retention: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := map[cohortPeriod]uint64{}
	for rows.Next() {
		var (
			cohort  time.Time
			period  uint16
			persons uint64
		)
		if err := rows.Scan(&cohort, &period, &persons); err != nil {
			return Retention{}, fmt.Errorf("reports: retention: %w", err)
		}
		counts[cohortPeriod{cohort: civil(cohort).Unix(), period: int(period)}] = persons
	}
	if err := rows.Err(); err != nil {
		return Retention{}, fmt.Errorf("reports: retention: %w", err)
	}
	return retentionMatrix(q.Granularity, q.Periods, starts, counts, civil(now.In(loc))), nil
}

// cohortStarts — начала когорт g периода p (полночь UTC): для day — каждый день from…to, для week —
// понедельники ISO-недель от недели from до недели to. Неизвестная g — nil.
func cohortStarts(p Period, g Granularity) []time.Time {
	from, to := civil(p.From), civil(p.To)
	step := 1
	switch g {
	case GranularityDay:
	case GranularityWeek:
		from, to, step = monday(from), monday(to), 7
	default:
		return nil
	}
	var out []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, step) {
		out = append(out, d)
	}
	return out
}

// monday — понедельник ISO-недели даты d.
func monday(d time.Time) time.Time {
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// retentionMatrix — матрица по числам вернувшихся counts (период 0 — размер когорты). Период N когорты
// завершён, если его последний день раньше today (полночь UTC даты сегодня в TZ приложения).
func retentionMatrix(g Granularity, periods int, starts []time.Time, counts map[cohortPeriod]uint64, today time.Time) Retention {
	step := 1
	if g == GranularityWeek {
		step = 7
	}
	out := Retention{
		CohortGranularity: g,
		Periods:           periods,
		Cohorts:           make([]RetentionCohort, 0, len(starts)),
		Average:           make([]*float64, periods+1),
	}
	returnedSum := make([]uint64, periods+1)
	sizeSum := make([]uint64, periods+1)
	for _, start := range starts {
		key := start.Unix()
		c := RetentionCohort{
			CohortStart: start.Format(time.DateOnly),
			Size:        counts[cohortPeriod{cohort: key}],
			Values:      make([]*float64, periods+1),
		}
		for n := range periods + 1 {
			if c.Size == 0 || start.AddDate(0, 0, step*(n+1)).After(today) {
				continue
			}
			persons := counts[cohortPeriod{cohort: key, period: n}]
			c.Values[n] = ratio(persons, c.Size)
			returnedSum[n] += persons
			sizeSum[n] += c.Size
		}
		out.Cohorts = append(out.Cohorts, c)
	}
	for n := range periods + 1 {
		out.Average[n] = ratio(returnedSum[n], sizeSum[n])
	}
	return out
}
