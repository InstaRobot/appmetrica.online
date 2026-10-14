//go:build integration

package reports

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// Дифференциальный тест SQL отчётов (Task 32a, NFR-003): оптимизированные запросы сверяются с эталонными —
// прямой записью определений метрик 05-data-model.md §10.4 (CTE ev/act: person_key каждого события через
// links, отдельный подзапрос на метрику). Данные — testutil.GeneratePerf: анонимные дни до входа, общие
// пользователи нескольких установок, development, досыл событий, переход America/New_York на зимнее время
// 2026-11-01 внутри окна.

// Эталонные запросы — форма до Task 32a.
const (
	refAudienceSummary = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `,
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

	refRollingActive = `WITH ` + identity.LinksCTE + `, ` + evCTE + `,
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

	refBreakdownBase = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `,
evd AS (
    SELECT ` + identity.PersonKeyExpr + ` AS person_key, $DIM AS value, e.name AS name, e.session_id AS session_id
    FROM events AS e
    LEFT JOIN links AS l ON l.install_id = e.install_id
    WHERE ` + eventsFilter + `
)`

	refBreakdownActive = refBreakdownBase + `
SELECT value, uniqExact(person_key) AS count, (SELECT uniqExact(person_key) FROM evd) AS total
FROM evd GROUP BY value ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`

	refBreakdownSessions = refBreakdownBase + `
SELECT value, uniqExactIf(session_id, name = 'session_start') AS count,
       (SELECT uniqExactIf(session_id, name = 'session_start') FROM evd) AS total
FROM evd GROUP BY value ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`

	refBreakdownNew = refBreakdownBase + `,
evn AS (
    SELECT evd.person_key AS person_key, evd.value AS value
    FROM evd INNER JOIN persons_first AS pf ON pf.person_key = evd.person_key
    WHERE pf.first_seen >= {start_utc:DateTime64(3)} AND pf.first_seen < {end_utc:DateTime64(3)}
)
SELECT value, uniqExact(person_key) AS count, (SELECT uniqExact(person_key) FROM evn) AS total
FROM evn GROUP BY value ORDER BY count DESC, value ASC LIMIT {limit:UInt32}`

	refRetentionActivity = `act AS (
    SELECT DISTINCT ` + identity.PersonKeyExpr + ` AS person_key, toDate(e.event_time, {tz:String}) AS day
    FROM events AS e
    LEFT JOIN links AS l ON l.install_id = e.install_id
    WHERE ` + eventsFilter + `
)`

	refRetentionDay = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + refRetentionActivity + `,
cohort AS (
    SELECT pf.person_key AS person_key, toDate(pf.first_seen, {tz:String}) AS cohort_start
    FROM persons_first AS pf
    WHERE pf.first_seen >= {start_utc:DateTime64(3)} AND pf.first_seen < {c_end_utc:DateTime64(3)}
),
cohort_f AS (
    SELECT c.person_key AS person_key, c.cohort_start AS cohort_start
    FROM cohort AS c INNER JOIN act AS a0 ON a0.person_key = c.person_key AND a0.day = c.cohort_start
)
SELECT c.cohort_start AS cohort_start,
       toUInt16(dateDiff('day', c.cohort_start, a.day)) AS period,
       uniqExact(c.person_key) AS persons
FROM cohort_f AS c
INNER JOIN act AS a ON a.person_key = c.person_key
WHERE a.day >= c.cohort_start AND dateDiff('day', c.cohort_start, a.day) <= {periods:UInt16}
GROUP BY cohort_start, period
ORDER BY cohort_start, period`

	refRetentionWeek = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + refRetentionActivity + `,
cohort AS (
    SELECT pf.person_key AS person_key, toMonday(pf.first_seen, {tz:String}) AS cohort_start
    FROM persons_first AS pf
    WHERE pf.first_seen >= {start_utc:DateTime64(3)} AND pf.first_seen < {c_end_utc:DateTime64(3)}
),
cohort_f AS (
    SELECT DISTINCT c.person_key AS person_key, c.cohort_start AS cohort_start
    FROM cohort AS c INNER JOIN act AS a0 ON a0.person_key = c.person_key
    WHERE toMonday(a0.day) = c.cohort_start
)
SELECT c.cohort_start AS cohort_start,
       toUInt16(intDiv(dateDiff('day', c.cohort_start, toMonday(a.day)), 7)) AS period,
       uniqExact(c.person_key) AS persons
FROM cohort_f AS c
INNER JOIN act AS a ON a.person_key = c.person_key
WHERE a.day >= c.cohort_start AND intDiv(dateDiff('day', c.cohort_start, toMonday(a.day)), 7) <= {periods:UInt16}
GROUP BY cohort_start, period
ORDER BY cohort_start, period`

	refPortfolioDate = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `,
pp AS (
    SELECT person_key,
           max(toDate(event_time, {tz:String}) = {date:Date}) AS on_date,
           max(toDate(event_time, {tz:String}) = subtractDays({date:Date}, 1)) AS on_d1,
           max(toDate(event_time, {tz:String}) = subtractDays({date:Date}, 7)) AS on_d7
    FROM ev GROUP BY person_key
),
ppf AS (
    SELECT pp.person_key AS person_key, pp.on_date AS on_date, pp.on_d1 AS on_d1, pp.on_d7 AS on_d7,
           toDate(pf.first_seen, {tz:String}) AS first_day
    FROM pp LEFT JOIN persons_first AS pf ON pf.person_key = pp.person_key
)
SELECT uniqExactIf(person_key, on_date) AS active_date,
       uniqExactIf(person_key, on_d7) AS active_same_day_prev_week,
       uniqExactIf(person_key, on_date AND first_day = {date:Date}) AS new_date,
       uniqExactIf(person_key, on_d1 AND first_day = subtractDays({date:Date}, 1)) AS cohort_d1,
       uniqExactIf(person_key, on_d1 AND on_date AND first_day = subtractDays({date:Date}, 1)) AS returned_d1,
       uniqExactIf(person_key, on_d7 AND first_day = subtractDays({date:Date}, 7)) AS cohort_d7,
       uniqExactIf(person_key, on_d7 AND on_date AND first_day = subtractDays({date:Date}, 7)) AS returned_d7
FROM ppf`
)

// refTimeseriesSQL — эталонный запрос ряда метрики m по корзинам g.
func refTimeseriesSQL(t *testing.T, m Metric, g Granularity) string {
	t.Helper()
	tmpl, ok := bucketTemplates[g]
	require.True(t, ok, g)
	bucket := func(col string) string { return strings.ReplaceAll(tmpl, "$COL", col) }
	switch m {
	case MetricActive:
		return `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT ` + bucket("event_time") + ` AS ts, uniqExact(person_key) AS value FROM ev GROUP BY ts`
	case MetricNew:
		return `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `,
pb AS (SELECT DISTINCT ` + bucket("event_time") + ` AS ts, person_key FROM ev)
SELECT pb.ts AS ts, uniqExactIf(pb.person_key, ` + bucket("pf.first_seen") + ` = pb.ts) AS value
FROM pb INNER JOIN persons_first AS pf ON pf.person_key = pb.person_key
GROUP BY ts`
	case MetricSessions:
		return `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT ` + bucket("event_time") + ` AS ts, uniqExact(session_id) AS value FROM ev WHERE name = 'session_start' GROUP BY ts`
	case MetricAvgDuration:
		return `WITH ` + identity.LinksCTE + `, ` + evCTE + `,
sess AS (
    SELECT session_id, max(duration_ms) AS duration_ms, max(event_time) AS ended_at
    FROM ev WHERE name = 'session_end' GROUP BY session_id
)
SELECT ` + bucket("ended_at") + ` AS ts, toUInt64(round(avg(duration_ms))) AS value FROM sess GROUP BY ts`
	}
	t.Fatalf("метрика %s", m)
	return ""
}

func refAudienceMetrics(t *testing.T, svc *Service, p Params) AudienceMetrics {
	t.Helper()
	var (
		out         AudienceMetrics
		avg, median *uint64
	)
	require.NoError(t, svc.conn.QueryRow(p.Bind(t.Context()), refAudienceSummary).
		Scan(&out.Active, &out.New, &out.Sessions, &avg, &median))
	out.AvgDurationMs, out.MedianDurationMs = optional(avg), optional(median)
	return out
}

func refRolling(t *testing.T, svc *Service, p Params) []RollingPoint {
	t.Helper()
	from, to := civil(p.Period.From), civil(p.Period.To)
	window := p
	window.Period = Period{From: from.AddDate(0, 0, -(rollingDays - 1)), To: to}
	rows, err := svc.conn.Query(window.bindWith(t.Context(), clickhouse.Parameters{
		"from": from.Format(time.DateOnly), "to": to.Format(time.DateOnly),
	}), refRollingActive)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []RollingPoint
	for rows.Next() {
		var (
			d  time.Time
			pt RollingPoint
		)
		require.NoError(t, rows.Scan(&d, &pt.DAU, &pt.WAU, &pt.MAU))
		pt.Day = d.Format(time.DateOnly)
		out = append(out, pt)
	}
	require.NoError(t, rows.Err())
	return out
}

func refBreakdown(t *testing.T, svc *Service, p Params, d Dimension, m Metric) Breakdown {
	t.Helper()
	query := map[Metric]string{MetricActive: refBreakdownActive, MetricNew: refBreakdownNew, MetricSessions: refBreakdownSessions}[m]
	rows, err := svc.conn.Query(p.bindWith(t.Context(), clickhouse.Parameters{"limit": "1000"}),
		strings.ReplaceAll(query, "$DIM", dimensionExprs[d]))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := Breakdown{Dimension: d, Metric: m, Rows: []BreakdownRow{}}
	for rows.Next() {
		var r BreakdownRow
		require.NoError(t, rows.Scan(&r.Value, &r.Count, &out.Total))
		out.Rows = append(out.Rows, r)
	}
	require.NoError(t, rows.Err())
	for i := range out.Rows {
		out.Rows[i].Share = shareOf(out.Rows[i].Count, out.Total)
	}
	return out
}

// portfolioDate — метрики даты qPortfolioDate (или эталона) приложения p на дату day.
func portfolioDate(t *testing.T, svc *Service, p Params, day time.Time, query string) [7]uint64 {
	t.Helper()
	p.Period = Period{From: day.AddDate(0, 0, -7), To: day}
	var v [7]uint64
	require.NoError(t, svc.conn.QueryRow(p.bindWith(t.Context(), clickhouse.Parameters{"date": day.Format(time.DateOnly)}), query).
		Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6]))
	return v
}

func TestReports_MatchReferenceSQL(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	cfg := testutil.PerfConfig{Seed: 7, DAU: 400, Days: 56, LastDay: date(t, "2026-11-08")}
	env.LoadPerf(t, cfg)
	svc := NewService(env.CH.Conn(), clock.NewFixed(cfg.Now()))
	last := cfg.LastDay

	// Фильтр версий — все, кроме последней: в периоде остаются и попавшие, и отсечённые события.
	versions := map[string][]string{}
	for _, app := range testutil.PerfApps {
		var v []string
		require.NoError(t, env.CH.Conn().QueryRow(ctx,
			`SELECT arrayPopBack(arraySort(groupUniqArray(app_version))) FROM events WHERE app_id = {app:UUID}`,
			clickhouse.Named("app", app.ID.String())).Scan(&v))
		require.NotEmpty(t, v)
		versions[app.Code] = v
	}

	type variant struct {
		name    string
		env     string
		unit    identity.Unit
		filters func(app testutil.PerfApp) Filters
	}
	none := func(testutil.PerfApp) Filters { return Filters{} }
	variants := []variant{
		{"persons", EnvProduction, identity.Persons, none},
		{"installs", EnvProduction, identity.Installs, none},
		{"development", EnvDevelopment, identity.Persons, none},
		{"country", EnvProduction, identity.Persons, func(testutil.PerfApp) Filters { return Filters{Country: []string{"RU", "DE"}} }},
		{"app_version_installs", EnvProduction, identity.Installs, func(app testutil.PerfApp) Filters {
			return Filters{AppVersion: versions[app.Code]}
		}},
	}

	for _, app := range testutil.PerfApps {
		loc, err := time.LoadLocation(app.Timezone)
		require.NoError(t, err)
		for _, v := range variants {
			t.Run(app.Code+"/"+v.name, func(t *testing.T) {
				p := Params{
					AppID: app.ID, Location: loc, Env: v.env, Unit: v.unit, Filters: v.filters(app),
					Period: Period{From: last.AddDate(0, 0, -20), To: last},
				}

				// Сводка со сравнением: у предыдущего периода тоже есть события.
				got, err := svc.AudienceSummary(t.Context(), p, true)
				require.NoError(t, err)
				require.Equal(t, refAudienceMetrics(t, svc, p), got.AudienceMetrics, "summary")
				prev := p
				prev.Period = p.Period.Previous()
				require.NotNil(t, got.Compare)
				require.Equal(t, refAudienceMetrics(t, svc, prev), got.Compare.AudienceMetrics, "summary compare")
				if v.env == EnvProduction {
					require.NotZero(t, got.Active, "в периоде есть люди")
					require.NotZero(t, got.Compare.Active, "в предыдущем периоде есть люди")
				}

				for _, q := range []TimeseriesQuery{
					{Granularity: GranularityDay, Metric: MetricNew},
					{Granularity: GranularityWeek, Metric: MetricNew},
					{Granularity: GranularityDay, Metric: MetricSessions},
					{Granularity: GranularityDay, Metric: MetricAvgDuration},
				} {
					ts, err := svc.AudienceTimeseries(t.Context(), p, q)
					require.NoError(t, err)
					want, err := svc.series(t.Context(), p, q, refTimeseriesSQL(t, q.Metric, q.Granularity))
					require.NoError(t, err)
					require.Equal(t, want, ts.Points, "timeseries %s/%s", q.Metric, q.Granularity)
				}
				hours := p
				hours.Period = Period{From: last.AddDate(0, 0, -7), To: last} // 2026-11-01 — 25 часов в New York
				q := TimeseriesQuery{Granularity: GranularityHour, Metric: MetricNew}
				ts, err := svc.AudienceTimeseries(t.Context(), hours, q)
				require.NoError(t, err)
				want, err := svc.series(t.Context(), hours, q, refTimeseriesSQL(t, q.Metric, q.Granularity))
				require.NoError(t, err)
				require.Equal(t, want, ts.Points, "timeseries new/hour")

				ra, err := svc.RollingActive(t.Context(), p)
				require.NoError(t, err)
				require.Equal(t, refRolling(t, svc, p), ra.Points, "rolling")

				for _, d := range []Dimension{DimensionCountry, DimensionAppVersion, DimensionCity, DimensionOSVersion} {
					for _, m := range []Metric{MetricActive, MetricNew, MetricSessions} {
						b, err := svc.AudienceBreakdown(t.Context(), p, BreakdownQuery{Dimension: d, Metric: m, Limit: 1000})
						require.NoError(t, err)
						require.Equal(t, refBreakdown(t, svc, p, d, m), b, "breakdown %s/%s", d, m)
					}
				}

				for _, rq := range []struct {
					q      RetentionQuery
					ref    string
					step   int
					period Period
				}{
					{RetentionQuery{Granularity: GranularityDay, Periods: 14}, refRetentionDay, 1, p.Period},
					{RetentionQuery{Granularity: GranularityWeek, Periods: 6}, refRetentionWeek, 7, Period{From: last.AddDate(0, 0, -48), To: last}},
				} {
					rp := p
					rp.Period = rq.period
					got, err := svc.Retention(t.Context(), rp, rq.q)
					require.NoError(t, err)
					want, err := svc.retention(t.Context(), rp, rq.q, rq.ref, rq.step)
					require.NoError(t, err)
					require.Equal(t, want, got, "retention %s", rq.q.Granularity)
				}

				for _, day := range []time.Time{last.AddDate(0, 0, -1), last.AddDate(0, 0, -7)} {
					require.Equal(t, portfolioDate(t, svc, p, day, refPortfolioDate), portfolioDate(t, svc, p, day, qPortfolioDate),
						"portfolio %s", day.Format(time.DateOnly))
				}
			})
		}
	}
}

// Каждый отчёт сообщает наблюдателю своё имя и длительность, в том числе при ошибке.
func TestService_ObservesEveryReport(t *testing.T) {
	env := testutil.Start(t)
	cfg := testutil.PerfConfig{Seed: 7, DAU: 100, Days: 7, LastDay: date(t, "2026-10-12")}
	env.LoadPerf(t, cfg)
	clk := clock.NewFixed(cfg.Now())
	svc := NewService(env.CH.Conn(), clk)
	var got []string
	svc.ObserveQueries(func(report string, d time.Duration) {
		require.GreaterOrEqual(t, d, time.Duration(0))
		got = append(got, report)
	})

	app := testutil.PerfApps[0]
	loc, err := time.LoadLocation(app.Timezone)
	require.NoError(t, err)
	p := Params{AppID: app.ID, Location: loc, Period: Period{From: cfg.FirstDay(), To: cfg.LastDay}}
	ctx := t.Context()
	calls := []func(ctx context.Context) error{
		func(ctx context.Context) error {
			_, err := svc.Portfolio(ctx, PortfolioQuery{
				Date: cfg.LastDay, Env: EnvProduction, Unit: identity.Persons,
				Apps: []PortfolioApp{{ID: app.ID, Location: loc}},
			})
			return err
		},
		func(ctx context.Context) error { _, err := svc.AudienceSummary(ctx, p, true); return err },
		func(ctx context.Context) error {
			_, err := svc.AudienceTimeseries(ctx, p, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricActive})
			return err
		},
		func(ctx context.Context) error { _, err := svc.RollingActive(ctx, p); return err },
		func(ctx context.Context) error {
			_, err := svc.AudienceBreakdown(ctx, p, BreakdownQuery{Dimension: DimensionCountry, Metric: MetricActive})
			return err
		},
		func(ctx context.Context) error { _, err := svc.Retention(ctx, p, RetentionQuery{}); return err },
		func(ctx context.Context) error { _, err := svc.Realtime(ctx, p); return err },
		func(ctx context.Context) error {
			_, err := svc.SearchProfiles(ctx, app.ID, testutil.PerfPowerUser)
			return err
		},
		func(ctx context.Context) error {
			_, err := svc.Profile(ctx, app.ID, ProfileQuery{PersonKey: "u:" + testutil.PerfPowerUser, SessionsLimit: 10})
			return err
		},
		func(ctx context.Context) error { _, err := svc.IntegrationHealth(ctx, p, 24); return err },
	}
	for i, call := range calls {
		require.NoError(t, call(ctx), "отчёт %d", i)
	}
	require.Equal(t, []string{
		ReportPortfolio, ReportAudienceSummary, ReportAudienceTimeseries, ReportRollingActive, ReportAudienceBreakdown,
		ReportRetention, ReportRealtime, ReportProfileSearch, ReportProfile, ReportIntegrationHealth,
	}, got)

	// Ошибка ClickHouse тоже наблюдается.
	got = nil
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = svc.AudienceSummary(canceled, p, false)
	require.Error(t, err)
	require.Equal(t, []string{ReportAudienceSummary}, got)
}
