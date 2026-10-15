package reports

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"appmetrica.online/backend/internal/identity"
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
	Granularity     Granularity
	Metric          Metric
	Compare         bool // compare=previous_period; при SplitByPlatform игнорируется
	SplitByPlatform bool // split=platform
}

// Timeseries — ряд от from до to; ComparePoints — тот же ряд за предыдущий период (nil без compare); Splits — ряды
// по платформам периода (nil без split, пустой — событий нет).
type Timeseries struct {
	Metric        Metric            `json:"metric"`
	Granularity   Granularity       `json:"granularity"`
	Points        []TimeseriesPoint `json:"points"`
	ComparePoints []TimeseriesPoint `json:"compare_points,omitempty"`
	Splits        []TimeseriesSplit `json:"splits,omitzero"`
}

// TimeseriesSplit — ряд с фильтром platform=[Platform].
type TimeseriesSplit struct {
	Platform string            `json:"platform"`
	Points   []TimeseriesPoint `json:"points"`
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

// rollingDays — окно MAU: день и 29 предыдущих.
const rollingDays = 30

// bucketTemplates — начало корзины в TZ приложения (DateTime), $COL — колонка DateTime64 (05-data-model.md §10.4).
var bucketTemplates = map[Granularity]string{
	GranularityHour:  "toStartOfHour(toTimeZone($COL, {tz:String}))",
	GranularityDay:   "toDateTime(toDate($COL, {tz:String}), {tz:String})",
	GranularityWeek:  "toDateTime(toMonday($COL, {tz:String}), {tz:String})",
	GranularityMonth: "toDateTime(toStartOfMonth($COL, {tz:String}), {tz:String})",
}

// dimensionExprs — выражения измерений разбивки по events AS e. Пустое значение — "": город без названия
// не склеивается со страной, ОС без названия и версии не превращается в пробел.
var dimensionExprs = map[Dimension]string{
	DimensionPlatform:    "e.platform",
	DimensionOSName:      "e.os_name",
	DimensionOSVersion:   "trimBoth(concat(e.os_name, ' ', e.os_version))",
	DimensionAppVersion:  "e.app_version",
	DimensionDeviceModel: "e.device_model",
	DimensionDeviceType:  "e.device_type",
	DimensionCountry:     "e.country",
	DimensionCity:        "if(e.city = '', '', concat(e.city, ', ', e.country))",
	DimensionLocale:      "e.locale",
}

// Запросы аудитории (05-data-model.md §10.4, QAudience*, QTimeseries*, QRollingActive, QBreakdown*). Люди и
// сессии — только uniqExact (OQ-042). Длительность — max(duration_ms) по session_end сессии; сессии без
// session_end в неё не входят, avgOrNull/…OrNull дают NULL, если завершённых сессий нет. Люди считаются по
// peopleCTE / peopleFirstCTE (склейка после предагрегации), сессии и длительности — по events без links.
var (
	// qAudiencePeople — активные и новые текущего периода [{split_utc}, {end_utc}) и предыдущего
	// [{start_utc}, {split_utc}) за один проход: без сравнения split_utc = start_utc и предыдущий период пуст.
	qAudiencePeople = `WITH ` + identity.LinksCTE + `, ` + peopleFirstCTE(
		flag("e.event_time >= {split_utc:DateTime64(3)}", "cur"),
		flag("e.event_time < {split_utc:DateTime64(3)}", "prev"),
	) + `
SELECT uniqExactIf(person_key, cur) AS active,
       uniqExactIf(person_key, cur
           AND first_seen >= {split_utc:DateTime64(3)} AND first_seen < {end_utc:DateTime64(3)}) AS new,
       uniqExactIf(person_key, prev) AS prev_active,
       uniqExactIf(person_key, prev
           AND first_seen >= {start_utc:DateTime64(3)} AND first_seen < {split_utc:DateTime64(3)}) AS prev_new
FROM people`

	// qRollingActive — {from:Date} … {to:Date}; период Params начинается за 29 дней до from. Дни активности
	// человека d1 < d2 < … покрывают окно W (1, 7, 30 дней) на днях [dᵢ, min(dᵢ₊₁, dᵢ + W)) — отрезки не
	// пересекаются, поэтому DAU, WAU и MAU дня — префиксные суммы +1 в начале и −1 после конца отрезков.
	qRollingActive = `WITH ` + identity.LinksCTE + `, ` + peopleCTE(uniqValues("toDate(e.event_time, {tz:String})", "days")) + `,
deltas AS (
    SELECT tupleElement(ev, 1) AS day,
           sum(tupleElement(ev, 2)) AS d1, sum(tupleElement(ev, 3)) AS d7, sum(tupleElement(ev, 4)) AS d30
    FROM (
        SELECT arrayJoin(arrayFlatten(arrayMap((d, next) -> [
                   (d, 1, 1, 1), (addDays(d, 1), -1, 0, 0),
                   (least(next, addDays(d, 7)), 0, -1, 0), (least(next, addDays(d, 30)), 0, 0, -1)],
               days, arrayPushBack(arrayPopFront(days), toDate('2149-01-01'))))) AS ev
        FROM (SELECT arraySort(groupUniqArrayArray(days)) AS days FROM people GROUP BY person_key)
    )
    GROUP BY day
),
calendar AS (
    SELECT addDays(subtractDays({from:Date}, 29), number) AS day
    FROM numbers(toUInt64(dateDiff('day', subtractDays({from:Date}, 29), {to:Date}) + 1))
)
SELECT day, toUInt64(dau) AS dau, toUInt64(wau) AS wau, toUInt64(mau) AS mau FROM (
    SELECT c.day AS day, sum(dl.d1) OVER w AS dau, sum(dl.d7) OVER w AS wau, sum(dl.d30) OVER w AS mau
    FROM calendar AS c LEFT JOIN deltas AS dl ON dl.day = c.day
    WINDOW w AS (ORDER BY c.day ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
)
WHERE day BETWEEN {from:Date} AND {to:Date}
ORDER BY day`

	// qBreakdownActive, qBreakdownNew — значения измерения $DIM человека (vals) и люди по значениям.
	qBreakdownActive = `WITH ` + identity.LinksCTE + `, ` + peopleCTE(uniqValues("$DIM", "vals")) + `
SELECT value, count, total FROM (
    SELECT value, count, g, max(if(g = 1, count, 0)) OVER () AS total FROM (
        SELECT value, uniqExact(person_key) AS count, grouping(value) AS g
        FROM people ARRAY JOIN vals AS value
        GROUP BY GROUPING SETS ((value), ())
    )
)
` + breakdownOrder

	// qPeriodPlatforms — платформы событий периода с учётом фильтров (split=platform).
	qPeriodPlatforms = `SELECT DISTINCT e.platform AS platform
FROM events AS e
WHERE ` + eventsFilter + `
ORDER BY platform`

	qBreakdownNew = `WITH ` + identity.LinksCTE + `, ` + peopleFirstCTE(uniqValues("$DIM", "vals")) + `
SELECT value, count, total FROM (
    SELECT value, count, g, max(if(g = 1, count, 0)) OVER () AS total FROM (
        SELECT value, uniqExact(person_key) AS count, grouping(value) AS g
        FROM people ARRAY JOIN vals AS value
        WHERE first_seen >= {start_utc:DateTime64(3)} AND first_seen < {end_utc:DateTime64(3)}
        GROUP BY GROUPING SETS ((value), ())
    )
)
` + breakdownOrder
)

const (
	// qAudienceSessions — сессии и длительности текущего и предыдущего периода (см. qAudiencePeople): сессия
	// относится к периоду своего session_start, длительность — своего session_end. Части соединяются CROSS JOIN
	// и считаются по очереди: множества 13 млн session_id и сессий с session_end не держатся в памяти вместе.
	qAudienceSessions = `SELECT s.sessions, d.avg_duration_ms, d.median_duration_ms,
       s.prev_sessions, d.prev_avg_duration_ms, d.prev_median_duration_ms
FROM (
    SELECT uniqExactIf(e.session_id, e.event_time >= {split_utc:DateTime64(3)}) AS sessions,
           uniqExactIf(e.session_id, e.event_time < {split_utc:DateTime64(3)}) AS prev_sessions
    FROM events AS e
    WHERE ` + eventsFilter + ` AND e.name = 'session_start'
) AS s
CROSS JOIN (
    SELECT toUInt64(round(avgOrNullIf(duration_ms, ended))) AS avg_duration_ms,
           toUInt64(round(quantileExactInclusiveOrNullIf(0.5)(duration_ms, ended))) AS median_duration_ms,
           toUInt64(round(avgOrNullIf(prev_duration_ms, prev_ended))) AS prev_avg_duration_ms,
           toUInt64(round(quantileExactInclusiveOrNullIf(0.5)(prev_duration_ms, prev_ended))) AS prev_median_duration_ms
    FROM (
        SELECT e.session_id AS session_id,
               max(e.event_time >= {split_utc:DateTime64(3)}) AS ended,
               maxIf(e.duration_ms, e.event_time >= {split_utc:DateTime64(3)}) AS duration_ms,
               max(e.event_time < {split_utc:DateTime64(3)}) AS prev_ended,
               maxIf(e.duration_ms, e.event_time < {split_utc:DateTime64(3)}) AS prev_duration_ms
        FROM events AS e
        WHERE ` + eventsFilter + ` AND e.name = 'session_end'
        GROUP BY session_id
    )
) AS d`

	// qBreakdownSessions — сессии по значению $DIM их session_start; итог — отдельной частью CROSS JOIN:
	// множества session_id по значениям и общее строятся по очереди, а не одновременно (GROUPING SETS).
	qBreakdownSessions = `SELECT v.value AS value, v.count AS count, t.total AS total
FROM (
    SELECT $DIM AS value, uniqExact(e.session_id) AS count
    FROM events AS e
    WHERE ` + eventsFilter + ` AND e.name = 'session_start'
    GROUP BY value
) AS v
CROSS JOIN (
    SELECT uniqExact(e.session_id) AS total
    FROM events AS e
    WHERE ` + eventsFilter + ` AND e.name = 'session_start'
) AS t
ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`

	// breakdownOrder — строки значений (g = 0) без строки итога GROUPING SETS (g = 1), total — из неё.
	breakdownOrder = `WHERE g = 0 ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`
)

// timeseriesSQL — запрос ряда (ts DateTime, value UInt64) метрики m по корзинам g; только значения из whitelist.
func timeseriesSQL(m Metric, g Granularity) (string, error) {
	tmpl, ok := bucketTemplates[g]
	if !ok {
		return "", fmt.Errorf("%w: granularity", ErrUnsupported)
	}
	bucket := func(col string) string { return strings.ReplaceAll(tmpl, "$COL", col) }
	switch m {
	case MetricActive:
		return `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT ` + bucket("event_time") + ` AS ts, uniqExact(person_key) AS value FROM ev GROUP BY ts`, nil
	case MetricNew:
		// Новые корзины — люди, чей first_seen попадает в корзину, где они активны.
		return `WITH ` + identity.LinksCTE + `, ` + peopleFirstCTE(uniqValues(bucket("e.event_time"), "buckets")) + `
SELECT ` + bucket("first_seen") + ` AS ts, uniqExact(person_key) AS value
FROM people
WHERE first_seen >= {start_utc:DateTime64(3)} AND first_seen < {end_utc:DateTime64(3)} AND has(buckets, ts)
GROUP BY ts`, nil
	case MetricSessions:
		return `SELECT ` + bucket("e.event_time") + ` AS ts, uniqExact(e.session_id) AS value
FROM events AS e
WHERE ` + eventsFilter + ` AND e.name = 'session_start'
GROUP BY ts`, nil
	case MetricAvgDuration:
		// Сессия попадает в корзину своего session_end.
		return `SELECT ` + bucket("ended_at") + ` AS ts, toUInt64(round(avg(duration_ms))) AS value
FROM (
    SELECT e.session_id AS session_id, max(e.duration_ms) AS duration_ms, max(e.event_time) AS ended_at
    FROM events AS e
    WHERE ` + eventsFilter + ` AND e.name = 'session_end'
    GROUP BY session_id
)
GROUP BY ts`, nil
	default:
		return "", fmt.Errorf("%w: metric", ErrUnsupported)
	}
}

// breakdownSQL — запрос разбивки (value, count, total) метрики m по измерению d из whitelist.
func breakdownSQL(d Dimension, m Metric) (string, error) {
	expr, ok := dimensionExprs[d]
	if !ok {
		return "", fmt.Errorf("%w: dimension", ErrUnsupported)
	}
	var query string
	switch m {
	case MetricActive:
		query = qBreakdownActive
	case MetricNew:
		query = qBreakdownNew
	case MetricSessions:
		query = qBreakdownSessions
	default:
		return "", fmt.Errorf("%w: metric", ErrUnsupported)
	}
	return strings.ReplaceAll(query, "$DIM", expr), nil
}

// AudienceSummary — KPI аудитории за период p; compare — и за предыдущий период той же длины. Оба периода
// читаются одним проходом (граница — {split_utc}), люди и сессии считаются параллельно.
func (s *Service) AudienceSummary(ctx context.Context, p Params, compare bool) (AudienceSummary, error) {
	defer s.track(ReportAudienceSummary)()
	scan := p
	split, _ := p.Period.Bounds(p.location())
	prevPeriod := p.Period.Previous()
	if compare {
		scan.Period = Period{From: prevPeriod.From, To: p.Period.To}
	}
	ctx = scan.bindWith(ctx, clickhouse.Parameters{"split_utc": split.Format(paramDateTime)})

	var (
		cur, prev                  AudienceMetrics
		avg, median, pAvg, pMedian *uint64
	)
	err := parallel(ctx,
		func(ctx context.Context) error {
			return s.conn.QueryRow(ctx, qAudiencePeople).Scan(&cur.Active, &cur.New, &prev.Active, &prev.New)
		},
		func(ctx context.Context) error {
			return s.conn.QueryRow(ctx, qAudienceSessions).Scan(&cur.Sessions, &avg, &median, &prev.Sessions, &pAvg, &pMedian)
		},
	)
	if err != nil {
		return AudienceSummary{}, fmt.Errorf("reports: audience summary: %w", err)
	}
	cur.AvgDurationMs, cur.MedianDurationMs = optional(avg), optional(median)
	prev.AvgDurationMs, prev.MedianDurationMs = optional(pAvg), optional(pMedian)

	out := AudienceSummary{From: dateOf(p.Period.From), To: dateOf(p.Period.To), AudienceMetrics: cur}
	if !compare {
		return out, nil
	}
	out.Compare = &AudienceCompare{
		From:            dateOf(prevPeriod.From),
		To:              dateOf(prevPeriod.To),
		AudienceMetrics: prev,
		DeltaPct: AudienceDelta{
			Active:           changePct(number(cur.Active), number(prev.Active)),
			New:              changePct(number(cur.New), number(prev.New)),
			Sessions:         changePct(number(cur.Sessions), number(prev.Sessions)),
			AvgDurationMs:    changePct(cur.AvgDurationMs, prev.AvgDurationMs),
			MedianDurationMs: changePct(cur.MedianDurationMs, prev.MedianDurationMs),
		},
	}
	return out, nil
}

// AudienceTimeseries — ряд метрики по корзинам периода p, пустые корзины — 0 (avg_duration_ms — nil).
func (s *Service) AudienceTimeseries(ctx context.Context, p Params, q TimeseriesQuery) (Timeseries, error) {
	defer s.track(ReportAudienceTimeseries)()
	query, err := timeseriesSQL(q.Metric, q.Granularity)
	if err != nil {
		return Timeseries{}, err
	}
	out := Timeseries{Metric: q.Metric, Granularity: q.Granularity}
	if out.Points, err = s.series(ctx, p, q, query); err != nil {
		return Timeseries{}, err
	}
	if q.SplitByPlatform {
		// Ряд на платформу — тот же запрос с фильтром platform=[p], поэтому он совпадает с отфильтрованным рядом
		// при любой метрике.
		if out.Splits, err = s.platformSplits(ctx, p, q, query); err != nil {
			return Timeseries{}, err
		}
		return out, nil
	}
	if q.Compare {
		prev := p
		prev.Period = p.Period.Previous()
		if out.ComparePoints, err = s.series(ctx, prev, q, query); err != nil {
			return Timeseries{}, err
		}
	}
	return out, nil
}

// platformSplits — ряды query по каждой платформе событий периода p, по алфавиту платформ.
func (s *Service) platformSplits(ctx context.Context, p Params, q TimeseriesQuery, query string) ([]TimeseriesSplit, error) {
	platforms, err := s.periodPlatforms(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]TimeseriesSplit, 0, len(platforms))
	for _, platform := range platforms {
		sp := p
		sp.Filters.Platform = []string{platform}
		points, err := s.series(ctx, sp, q, query)
		if err != nil {
			return nil, err
		}
		out = append(out, TimeseriesSplit{Platform: platform, Points: points})
	}
	return out, nil
}

// periodPlatforms — различные events.platform событий периода p с учётом фильтров.
func (s *Service) periodPlatforms(ctx context.Context, p Params) ([]string, error) {
	rows, err := s.conn.Query(p.Bind(ctx), qPeriodPlatforms)
	if err != nil {
		return nil, fmt.Errorf("reports: period platforms: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var platform string
		if err := rows.Scan(&platform); err != nil {
			return nil, fmt.Errorf("reports: period platforms: %w", err)
		}
		out = append(out, platform)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reports: period platforms: %w", err)
	}
	return out, nil
}

// series — точки ряда query за период p: по корзине на каждое начало из bucketStarts.
func (s *Service) series(ctx context.Context, p Params, q TimeseriesQuery, query string) ([]TimeseriesPoint, error) {
	rows, err := s.conn.Query(p.Bind(ctx), query)
	if err != nil {
		return nil, fmt.Errorf("reports: timeseries %s: %w", q.Metric, err)
	}
	defer func() { _ = rows.Close() }()
	got := map[int64]uint64{}
	for rows.Next() {
		var (
			ts time.Time
			v  uint64
		)
		if err := rows.Scan(&ts, &v); err != nil {
			return nil, fmt.Errorf("reports: timeseries %s: %w", q.Metric, err)
		}
		got[ts.Unix()] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reports: timeseries %s: %w", q.Metric, err)
	}

	starts := bucketStarts(p.Period, p.Location, q.Granularity)
	out := make([]TimeseriesPoint, 0, len(starts))
	for _, ts := range starts {
		pt := TimeseriesPoint{TS: ts}
		if v, ok := got[ts.Unix()]; ok {
			pt.Value = number(v)
		} else if q.Metric != MetricAvgDuration {
			pt.Value = number(0)
		}
		out = append(out, pt)
	}
	return out, nil
}

// RollingActive — DAU, WAU и MAU для каждого дня периода p; окна читают события до 29 дней раньше from.
func (s *Service) RollingActive(ctx context.Context, p Params) (RollingActive, error) {
	defer s.track(ReportRollingActive)()
	from, to := civil(p.Period.From), civil(p.Period.To)
	window := p
	window.Period = Period{From: from.AddDate(0, 0, -(rollingDays - 1)), To: to}
	rows, err := s.conn.Query(window.bindWith(ctx, clickhouse.Parameters{
		"from": from.Format(time.DateOnly),
		"to":   to.Format(time.DateOnly),
	}), qRollingActive)
	if err != nil {
		return RollingActive{}, fmt.Errorf("reports: rolling active: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := RollingActive{Points: make([]RollingPoint, 0, p.Period.Days())}
	for rows.Next() {
		var (
			d  time.Time
			pt RollingPoint
		)
		if err := rows.Scan(&d, &pt.DAU, &pt.WAU, &pt.MAU); err != nil {
			return RollingActive{}, fmt.Errorf("reports: rolling active: %w", err)
		}
		pt.Day = d.Format(time.DateOnly)
		out.Points = append(out.Points, pt)
	}
	if err := rows.Err(); err != nil {
		return RollingActive{}, fmt.Errorf("reports: rolling active: %w", err)
	}
	return out, nil
}

// AudienceBreakdown — метрика по значениям измерения: count DESC, value ASC, не больше q.Limit строк.
func (s *Service) AudienceBreakdown(ctx context.Context, p Params, q BreakdownQuery) (Breakdown, error) {
	defer s.track(ReportAudienceBreakdown)()
	query, err := breakdownSQL(q.Dimension, q.Metric)
	if err != nil {
		return Breakdown{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultBreakdownLimit
	}
	rows, err := s.conn.Query(p.bindWith(ctx, clickhouse.Parameters{"limit": strconv.Itoa(limit)}), query)
	if err != nil {
		return Breakdown{}, fmt.Errorf("reports: breakdown %s: %w", q.Dimension, err)
	}
	defer func() { _ = rows.Close() }()
	out := Breakdown{Dimension: q.Dimension, Metric: q.Metric, Rows: []BreakdownRow{}}
	for rows.Next() {
		var r BreakdownRow
		if err := rows.Scan(&r.Value, &r.Count, &out.Total); err != nil {
			return Breakdown{}, fmt.Errorf("reports: breakdown %s: %w", q.Dimension, err)
		}
		out.Rows = append(out.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return Breakdown{}, fmt.Errorf("reports: breakdown %s: %w", q.Dimension, err)
	}
	for i := range out.Rows {
		out.Rows[i].Share = shareOf(out.Rows[i].Count, out.Total)
	}
	return out, nil
}

// Previous — период той же длины непосредственно перед p.
func (p Period) Previous() Period {
	from := civil(p.From)
	return Period{From: from.AddDate(0, 0, -p.Days()), To: from.AddDate(0, 0, -1)}
}

// bucketStarts — начала корзин g, покрывающих период p в TZ loc (nil — UTC): часы идут по абсолютному
// времени (сутки перехода DST — 23 или 25 корзин), недели начинаются с понедельника, месяцы — с 1-го числа.
// Первая корзина недели или месяца может начинаться раньше from. Неизвестная g — nil.
func bucketStarts(p Period, loc *time.Location, g Granularity) []time.Time {
	loc = orUTC(loc)
	start, end := p.Bounds(loc)
	y, m, d := p.From.Date()
	var (
		first time.Time
		next  func(time.Time) time.Time
	)
	switch g {
	case GranularityHour:
		first = start.In(loc)
		next = func(t time.Time) time.Time { return t.Add(time.Hour) }
	case GranularityDay:
		first = time.Date(y, m, d, 0, 0, 0, 0, loc)
		next = func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }
	case GranularityWeek:
		monday := (int(civil(p.From).Weekday()) + 6) % 7
		first = time.Date(y, m, d-monday, 0, 0, 0, 0, loc)
		next = func(t time.Time) time.Time { return t.AddDate(0, 0, 7) }
	case GranularityMonth:
		first = time.Date(y, m, 1, 0, 0, 0, 0, loc)
		next = func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }
	default:
		return nil
	}
	var out []time.Time
	for t := first; t.Before(end); t = next(t) {
		out = append(out, t)
	}
	return out
}

// changePct — round((cur − base) / base × 100, 2); nil при нулевой или отсутствующей базе.
func changePct(cur, base *float64) *float64 {
	if cur == nil || base == nil || *base == 0 {
		return nil
	}
	v := math.Round((*cur-*base) / *base * 100 * 100) / 100
	return &v
}

// shareOf — n / total, округлённая до 4 знаков; 0 при total = 0.
func shareOf(n, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)/float64(total)*1e4) / 1e4
}

func number(v uint64) *float64 {
	f := float64(v)
	return &f
}

func optional(v *uint64) *float64 {
	if v == nil {
		return nil
	}
	return number(*v)
}

func dateOf(t time.Time) string { return civil(t).Format(time.DateOnly) }
