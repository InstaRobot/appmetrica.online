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
// session_end в неё не входят, avgOrNull/…OrNull дают NULL, если завершённых сессий нет.
const (
	qAudienceSummary = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `,
sess AS (
    SELECT session_id, max(duration_ms) AS duration_ms FROM ev WHERE name = 'session_end' GROUP BY session_id
)
SELECT
    (SELECT uniqExact(person_key) FROM ev) AS active,
    (SELECT uniqExact(a.person_key) FROM ev AS a
        INNER JOIN persons_first AS pf ON pf.person_key = a.person_key
        WHERE pf.first_seen >= {start_utc:DateTime64(3)} AND pf.first_seen < {end_utc:DateTime64(3)}) AS new,
    (SELECT uniqExactIf(session_id, name = 'session_start') FROM ev) AS sessions,
    (SELECT toUInt64(round(avgOrNull(duration_ms))) FROM sess) AS avg_duration_ms,
    (SELECT toUInt64(round(quantileExactInclusiveOrNull(0.5)(duration_ms))) FROM sess) AS median_duration_ms`

	// qRollingActive — {from:Date} … {to:Date}; период Params начинается за 29 дней до from.
	qRollingActive = `WITH ` + identity.LinksCTE + `, ` + evCTE + `,
daily AS (SELECT toDate(event_time, {tz:String}) AS day, uniqExactState(person_key) AS st FROM ev GROUP BY day),
calendar AS (
    SELECT addDays(subtractDays({from:Date}, 29), number) AS day
    FROM numbers(toUInt64(dateDiff('day', subtractDays({from:Date}, 29), {to:Date}) + 1))
),
daily_full AS (SELECT c.day AS day, d.st AS st FROM calendar AS c LEFT JOIN daily AS d ON d.day = c.day)
SELECT day, dau, wau, mau FROM (
    SELECT day,
           finalizeAggregation(st) AS dau,
           uniqExactMerge(st) OVER (ORDER BY toUInt32(day) RANGE BETWEEN 6 PRECEDING AND CURRENT ROW) AS wau,
           uniqExactMerge(st) OVER (ORDER BY toUInt32(day) RANGE BETWEEN 29 PRECEDING AND CURRENT ROW) AS mau
    FROM daily_full
)
WHERE day BETWEEN {from:Date} AND {to:Date}
ORDER BY day`

	// qBreakdownBase — события периода с person_key и значением измерения $DIM.
	qBreakdownBase = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `,
evd AS (
    SELECT ` + identity.PersonKeyExpr + ` AS person_key, $DIM AS value, e.name AS name, e.session_id AS session_id
    FROM events AS e
    LEFT JOIN links AS l ON l.install_id = e.install_id
    WHERE ` + eventsFilter + `
)`

	qBreakdownActive = qBreakdownBase + `
SELECT value, uniqExact(person_key) AS count, (SELECT uniqExact(person_key) FROM evd) AS total
FROM evd GROUP BY value ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`

	qBreakdownSessions = qBreakdownBase + `
SELECT value, uniqExactIf(session_id, name = 'session_start') AS count,
       (SELECT uniqExactIf(session_id, name = 'session_start') FROM evd) AS total
FROM evd GROUP BY value ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`

	qBreakdownNew = qBreakdownBase + `,
evn AS (
    SELECT evd.person_key AS person_key, evd.value AS value
    FROM evd INNER JOIN persons_first AS pf ON pf.person_key = evd.person_key
    WHERE pf.first_seen >= {start_utc:DateTime64(3)} AND pf.first_seen < {end_utc:DateTime64(3)}
)
SELECT value, uniqExact(person_key) AS count, (SELECT uniqExact(person_key) FROM evn) AS total
FROM evn GROUP BY value ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`
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
		// Новые корзины — активные в ней люди, чей first_seen попадает в ту же корзину.
		return `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `,
pb AS (SELECT DISTINCT ` + bucket("event_time") + ` AS ts, person_key FROM ev)
SELECT pb.ts AS ts, uniqExactIf(pb.person_key, ` + bucket("pf.first_seen") + ` = pb.ts) AS value
FROM pb INNER JOIN persons_first AS pf ON pf.person_key = pb.person_key
GROUP BY ts`, nil
	case MetricSessions:
		return `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT ` + bucket("event_time") + ` AS ts, uniqExact(session_id) AS value FROM ev WHERE name = 'session_start' GROUP BY ts`, nil
	case MetricAvgDuration:
		// Сессия попадает в корзину своего session_end.
		return `WITH ` + identity.LinksCTE + `, ` + evCTE + `,
sess AS (
    SELECT session_id, max(duration_ms) AS duration_ms, max(event_time) AS ended_at
    FROM ev WHERE name = 'session_end' GROUP BY session_id
)
SELECT ` + bucket("ended_at") + ` AS ts, toUInt64(round(avg(duration_ms))) AS value FROM sess GROUP BY ts`, nil
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

// AudienceSummary — KPI аудитории за период p; compare — и за предыдущий период той же длины.
func (s *Service) AudienceSummary(ctx context.Context, p Params, compare bool) (AudienceSummary, error) {
	cur, err := s.audienceMetrics(ctx, p)
	if err != nil {
		return AudienceSummary{}, err
	}
	out := AudienceSummary{From: dateOf(p.Period.From), To: dateOf(p.Period.To), AudienceMetrics: cur}
	if !compare {
		return out, nil
	}
	prev := p
	prev.Period = p.Period.Previous()
	base, err := s.audienceMetrics(ctx, prev)
	if err != nil {
		return AudienceSummary{}, err
	}
	out.Compare = &AudienceCompare{
		From:            dateOf(prev.Period.From),
		To:              dateOf(prev.Period.To),
		AudienceMetrics: base,
		DeltaPct: AudienceDelta{
			Active:           changePct(number(cur.Active), number(base.Active)),
			New:              changePct(number(cur.New), number(base.New)),
			Sessions:         changePct(number(cur.Sessions), number(base.Sessions)),
			AvgDurationMs:    changePct(cur.AvgDurationMs, base.AvgDurationMs),
			MedianDurationMs: changePct(cur.MedianDurationMs, base.MedianDurationMs),
		},
	}
	return out, nil
}

func (s *Service) audienceMetrics(ctx context.Context, p Params) (AudienceMetrics, error) {
	var (
		out         AudienceMetrics
		avg, median *uint64
	)
	if err := s.conn.QueryRow(p.Bind(ctx), qAudienceSummary).Scan(&out.Active, &out.New, &out.Sessions, &avg, &median); err != nil {
		return AudienceMetrics{}, fmt.Errorf("reports: audience summary: %w", err)
	}
	out.AvgDurationMs = optional(avg)
	out.MedianDurationMs = optional(median)
	return out, nil
}

// AudienceTimeseries — ряд метрики по корзинам периода p, пустые корзины — 0 (avg_duration_ms — nil).
func (s *Service) AudienceTimeseries(ctx context.Context, p Params, q TimeseriesQuery) (Timeseries, error) {
	query, err := timeseriesSQL(q.Metric, q.Granularity)
	if err != nil {
		return Timeseries{}, err
	}
	out := Timeseries{Metric: q.Metric, Granularity: q.Granularity}
	if out.Points, err = s.series(ctx, p, q, query); err != nil {
		return Timeseries{}, err
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
