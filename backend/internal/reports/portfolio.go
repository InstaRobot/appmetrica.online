package reports

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
)

// Service — отчёты по ClickHouse.
type Service struct {
	conn    driver.Conn
	clock   clock.Clock
	observe QueryObserver // nil — длительности не наблюдаются (ObserveQueries)
}

func NewService(conn driver.Conn, c clock.Clock) *Service {
	return &Service{conn: conn, clock: c}
}

const (
	sparklineDays = 30
	// openDays — сегодня и 7 предыдущих дней: в них ещё досылаются события, поэтому они всегда считаются live.
	// Job close_day закрывает день сегодня − 8 (02-tech-spec.md FR-022).
	openDays = 8
	// portfolioParallelism — сколько приложений портфеля считаются одновременно (FR-012).
	portfolioParallelism = 4
)

// PortfolioApp — приложение портфеля и его TZ (apps.timezone; nil — UTC).
type PortfolioApp struct {
	ID       uuid.UUID
	Location *time.Location
}

// PortfolioQuery — параметры GET /v1/portfolio.
type PortfolioQuery struct {
	Date          time.Time      // календарная дата; нулевая — вчера в OwnerLocation
	OwnerLocation *time.Location // settings.owner_timezone; nil — UTC
	Env           string
	Unit          identity.Unit
	Apps          []PortfolioApp
}

// Portfolio — портфель на дату; Rows — в порядке PortfolioQuery.Apps.
type Portfolio struct {
	Date        string         `json:"date"`
	Environment string         `json:"environment"`
	Unit        identity.Unit  `json:"unit"`
	Rows        []PortfolioRow `json:"rows"`
}

// PortfolioRow — строка портфеля (FR-012); дата интерпретируется в TZ приложения.
type PortfolioRow struct {
	AppID                 uuid.UUID        `json:"app_id"`
	ActiveToday           uint64           `json:"active_today"`
	ActiveDate            uint64           `json:"active_date"`
	ActiveSameDayPrevWeek uint64           `json:"active_same_day_prev_week"`
	ActiveDeltaPct        *float64         `json:"active_delta_pct"`
	NewDate               uint64           `json:"new_date"`
	SessionsDate          uint64           `json:"sessions_date"`
	RetentionD1           *float64         `json:"retention_d1"`
	RetentionD7           *float64         `json:"retention_d7"`
	Sparkline             []SparklinePoint `json:"sparkline"`
}

// SparklinePoint — активные за день Day (YYYY-MM-DD в TZ приложения).
type SparklinePoint struct {
	Day    string `json:"day"`
	Active uint64 `json:"active"`
}

// Запросы портфеля (05-data-model.md §10.4, QPortfolio*). Люди и сессии — только uniqExact (OQ-042).
const (
	// qPortfolioSessions — сессии периода по session_start (unit не влияет).
	qPortfolioSessions = `SELECT uniqExactIf(e.session_id, e.name = 'session_start') AS sessions_date
FROM events AS e
WHERE ` + eventsFilter

	// qDailyActive — активные по дням периода в TZ приложения (QTimeseriesActive, granularity=day).
	qDailyActive = `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT toDate(event_time, {tz:String}) AS day, uniqExact(person_key) AS active FROM ev GROUP BY day ORDER BY day`

	// qClosedDays — закрытые дни {closed_from:Date} … {closed_to:Date} из daily_stats (QPortfolioSparklineClosed).
	qClosedDays = `SELECT day, active
FROM daily_stats FINAL
WHERE app_id = {app_id:UUID} AND environment = {env:String} AND unit = {unit:String}
  AND day BETWEEN {closed_from:Date} AND {closed_to:Date}
ORDER BY day`
)

// qPortfolioDate — метрики даты по событиям периода date − 7 … date ({date:Date}): активные за date и date − 7,
// новые за date, когорты D1 (date − 1) и D7 (date − 7) и вернувшиеся из них в date. Когорта — люди с first_seen
// в день когорты, активные в этот день (как в QRetentionDay).
var qPortfolioDate = `WITH ` + identity.LinksCTE + `, ` + peopleFirstCTE(
	flag("toDate(e.event_time, {tz:String}) = {date:Date}", "on_date"),
	flag("toDate(e.event_time, {tz:String}) = subtractDays({date:Date}, 1)", "on_d1"),
	flag("toDate(e.event_time, {tz:String}) = subtractDays({date:Date}, 7)", "on_d7"),
) + `,
ppf AS (SELECT person_key, on_date, on_d1, on_d7, toDate(first_seen, {tz:String}) AS first_day FROM people)
SELECT uniqExactIf(person_key, on_date) AS active_date,
       uniqExactIf(person_key, on_d7) AS active_same_day_prev_week,
       uniqExactIf(person_key, on_date AND first_day = {date:Date}) AS new_date,
       uniqExactIf(person_key, on_d1 AND first_day = subtractDays({date:Date}, 1)) AS cohort_d1,
       uniqExactIf(person_key, on_d1 AND on_date AND first_day = subtractDays({date:Date}, 1)) AS returned_d1,
       uniqExactIf(person_key, on_d7 AND first_day = subtractDays({date:Date}, 7)) AS cohort_d7,
       uniqExactIf(person_key, on_d7 AND on_date AND first_day = subtractDays({date:Date}, 7)) AS returned_d7
FROM ppf`

// Portfolio считает строки портфеля на дату: до portfolioParallelism приложений одновременно, первая ошибка
// отменяет остальные.
func (s *Service) Portfolio(ctx context.Context, q PortfolioQuery) (Portfolio, error) {
	defer s.track(ReportPortfolio)()
	now := s.clock.Now()
	day := civil(q.Date)
	if q.Date.IsZero() {
		day = civil(now.In(orUTC(q.OwnerLocation))).AddDate(0, 0, -1)
	}
	out := Portfolio{
		Date:        day.Format(time.DateOnly),
		Environment: q.Env,
		Unit:        q.Unit,
		Rows:        make([]PortfolioRow, len(q.Apps)),
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	sem := make(chan struct{}, portfolioParallelism)
	for i, app := range q.Apps {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			row, err := s.portfolioRow(ctx, q, app, day, now)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("reports: portfolio app %s: %w", app.ID, err)
					cancel()
				}
				mu.Unlock()
				return
			}
			out.Rows[i] = row
		})
	}
	wg.Wait()
	if firstErr != nil {
		return Portfolio{}, firstErr
	}
	return out, nil
}

// portfolioRow — строка приложения app на дату day; «сегодня» — дата now в TZ приложения.
func (s *Service) portfolioRow(ctx context.Context, q PortfolioQuery, app PortfolioApp, day, now time.Time) (PortfolioRow, error) {
	loc := orUTC(app.Location)
	today := civil(now.In(loc))
	base := Params{AppID: app.ID, Location: loc, Env: q.Env, Unit: q.Unit}
	row := PortfolioRow{AppID: app.ID}

	p := base
	p.Period = Period{From: day.AddDate(0, 0, -7), To: day}
	var cohortD1, returnedD1, cohortD7, returnedD7 uint64
	err := s.conn.QueryRow(p.bindWith(ctx, clickhouse.Parameters{"date": day.Format(time.DateOnly)}), qPortfolioDate).
		Scan(&row.ActiveDate, &row.ActiveSameDayPrevWeek, &row.NewDate, &cohortD1, &returnedD1, &cohortD7, &returnedD7)
	if err != nil {
		return PortfolioRow{}, fmt.Errorf("date metrics: %w", err)
	}
	row.ActiveDeltaPct = deltaPct(row.ActiveDate, row.ActiveSameDayPrevWeek)
	row.RetentionD1 = ratio(returnedD1, cohortD1)
	row.RetentionD7 = ratio(returnedD7, cohortD7)

	p.Period = Period{From: day, To: day}
	if err := s.conn.QueryRow(p.Bind(ctx), qPortfolioSessions).Scan(&row.SessionsDate); err != nil {
		return PortfolioRow{}, fmt.Errorf("sessions: %w", err)
	}

	p.Period = Period{From: today, To: today}
	todayActive, err := s.dailyActive(ctx, p)
	if err != nil {
		return PortfolioRow{}, fmt.Errorf("active today: %w", err)
	}
	row.ActiveToday = todayActive[today.Format(time.DateOnly)]

	from := day.AddDate(0, 0, -(sparklineDays - 1))
	closedTo := today.AddDate(0, 0, -openDays)
	if day.Before(closedTo) {
		closedTo = day
	}
	closed, err := s.closedDays(ctx, base, from, closedTo)
	if err != nil {
		return PortfolioRow{}, fmt.Errorf("closed days: %w", err)
	}
	var live map[string]uint64
	if liveFrom, ok := firstOpenDay(from, day, closed); ok {
		p.Period = Period{From: liveFrom, To: day}
		if live, err = s.dailyActive(ctx, p); err != nil {
			return PortfolioRow{}, fmt.Errorf("sparkline: %w", err)
		}
	}
	row.Sparkline = sparkline(from, closed, live)
	return row, nil
}

// dailyActive — активные по дням периода p (YYYY-MM-DD → active).
func (s *Service) dailyActive(ctx context.Context, p Params) (map[string]uint64, error) {
	return s.dayValues(p.Bind(ctx), qDailyActive)
}

// closedDays — активные закрытых дней from … to из daily_stats; to < from — без запроса.
func (s *Service) closedDays(ctx context.Context, p Params, from, to time.Time) (map[string]uint64, error) {
	if to.Before(from) {
		return map[string]uint64{}, nil
	}
	return s.dayValues(p.bindWith(ctx, clickhouse.Parameters{
		"closed_from": from.Format(time.DateOnly),
		"closed_to":   to.Format(time.DateOnly),
	}), qClosedDays)
}

// dayValues читает строки (day Date, value UInt64).
func (s *Service) dayValues(ctx context.Context, query string) (map[string]uint64, error) {
	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]uint64{}
	for rows.Next() {
		var (
			d time.Time
			v uint64
		)
		if err := rows.Scan(&d, &v); err != nil {
			return nil, err
		}
		out[d.Format(time.DateOnly)] = v
	}
	return out, rows.Err()
}

// firstOpenDay — первый день from … to, которого нет среди закрытых; false — закрыты все.
func firstOpenDay(from, to time.Time, closed map[string]uint64) (time.Time, bool) {
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if _, ok := closed[d.Format(time.DateOnly)]; !ok {
			return d, true
		}
	}
	return time.Time{}, false
}

// sparkline — sparklineDays точек from …: закрытый день из closed, иначе из live, дня нет нигде — 0.
func sparkline(from time.Time, closed, live map[string]uint64) []SparklinePoint {
	out := make([]SparklinePoint, 0, sparklineDays)
	for i := range sparklineDays {
		d := civil(from).AddDate(0, 0, i).Format(time.DateOnly)
		v, ok := closed[d]
		if !ok {
			v = live[d]
		}
		out = append(out, SparklinePoint{Day: d, Active: v})
	}
	return out
}

// deltaPct — round((a − b) / b × 100, 1); nil при нулевой базе b.
func deltaPct(a, b uint64) *float64 {
	if b == 0 {
		return nil
	}
	v := math.Round((float64(a)-float64(b))/float64(b)*100*10) / 10
	return &v
}

// ratio — доля n / d; nil при пустой когорте d.
func ratio(n, d uint64) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}

func orUTC(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}
