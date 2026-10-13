//go:build integration

package reports

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// visit — сессия установки n приложения identityApp: session_start в at и session_end через dur
// (dur = 0 — сессия без session_end).
func visit(t testing.TB, n int, user, at string, dur time.Duration) []chstore.EventRow {
	t.Helper()
	start := session(t, n, user, at)
	if dur == 0 {
		return []chstore.EventRow{start}
	}
	return []chstore.EventRow{start, sessionEnd(start, dur)}
}

// sessionEnd — session_end сессии start через dur.
func sessionEnd(start chstore.EventRow, dur time.Duration) chstore.EventRow {
	end := start
	end.InsertID = uuid.New()
	end.Name = "session_end"
	end.EventTime = start.EventTime.Add(dur)
	end.EventDate = time.Date(end.EventTime.Year(), end.EventTime.Month(), end.EventTime.Day(), 0, 0, 0, 0, time.UTC)
	end.ClientTime, end.SentAt, end.ReceivedAt = end.EventTime, end.EventTime, end.EventTime
	end.DurationMs = uint32(dur / time.Millisecond)
	return end
}

// with — копии строк rows с изменением f.
func with(rows []chstore.EventRow, f func(*chstore.EventRow)) []chstore.EventRow {
	out := make([]chstore.EventRow, len(rows))
	for i, r := range rows {
		f(&r)
		out[i] = r
	}
	return out
}

func concat(parts ...[]chstore.EventRow) []chstore.EventRow {
	var out []chstore.EventRow
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func newService(env *testutil.Env) *Service {
	return NewService(env.CH.Conn(), clock.NewFixed(portfolioNow))
}

func summary(t *testing.T, svc *Service, p Params, compare bool) AudienceSummary {
	t.Helper()
	got, err := svc.AudienceSummary(t.Context(), p, compare)
	require.NoError(t, err)
	return got
}

func timeseries(t *testing.T, svc *Service, p Params, q TimeseriesQuery) Timeseries {
	t.Helper()
	got, err := svc.AudienceTimeseries(t.Context(), p, q)
	require.NoError(t, err)
	require.Equal(t, q.Metric, got.Metric)
	require.Equal(t, q.Granularity, got.Granularity)
	if q.Compare {
		require.Len(t, got.ComparePoints, len(bucketStarts(p.Period.Previous(), p.Location, q.Granularity)))
	} else {
		require.Nil(t, got.ComparePoints)
	}
	return got
}

func rolling(t *testing.T, svc *Service, p Params) RollingActive {
	t.Helper()
	got, err := svc.RollingActive(t.Context(), p)
	require.NoError(t, err)
	require.Len(t, got.Points, p.Period.Days())
	return got
}

func breakdown(t *testing.T, svc *Service, p Params, q BreakdownQuery) Breakdown {
	t.Helper()
	got, err := svc.AudienceBreakdown(t.Context(), p, q)
	require.NoError(t, err)
	require.Equal(t, q.Dimension, got.Dimension)
	require.Equal(t, q.Metric, got.Metric)
	return got
}

// values — значения точек ряда (nil — null) в виде чисел для сравнения.
func values(points []TimeseriesPoint) []any {
	out := make([]any, 0, len(points))
	for _, p := range points {
		if p.Value == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, *p.Value)
	}
	return out
}

func stamps(points []TimeseriesPoint) []string {
	out := make([]string, 0, len(points))
	for _, p := range points {
		out = append(out, p.TS.Format(time.RFC3339))
	}
	return out
}

func rowsOf(b Breakdown) map[string]uint64 {
	out := map[string]uint64{}
	for _, r := range b.Rows {
		out[r.Value] = r.Count
	}
	return out
}

// audienceSnapshot — все отчёты аудитории приложения за период (golden и проверка идемпотентности).
type audienceSnapshot struct {
	Summary         AudienceSummary       `json:"summary"`
	SummaryInstalls AudienceSummary       `json:"summary_installs"`
	Timeseries      map[string]Timeseries `json:"timeseries"`
	Rolling         RollingActive         `json:"rolling"`
	Breakdown       map[string]Breakdown  `json:"breakdown"`
}

var allDimensions = []Dimension{
	DimensionPlatform, DimensionOSName, DimensionOSVersion, DimensionAppVersion, DimensionDeviceModel,
	DimensionDeviceType, DimensionCountry, DimensionCity, DimensionLocale,
}

func audienceReport(t *testing.T, svc *Service, p Params) audienceSnapshot {
	t.Helper()
	installs := p
	installs.Unit = identity.Installs
	out := audienceSnapshot{
		Summary:         summary(t, svc, p, true),
		SummaryInstalls: summary(t, svc, installs, false),
		Timeseries:      map[string]Timeseries{},
		Rolling:         rolling(t, svc, p),
		Breakdown:       map[string]Breakdown{},
	}
	for _, m := range []Metric{MetricActive, MetricNew, MetricSessions, MetricAvgDuration} {
		out.Timeseries["day/"+string(m)] = timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: m, Compare: true})
	}
	for _, g := range []Granularity{GranularityWeek, GranularityMonth} {
		out.Timeseries[string(g)+"/active"] = timeseries(t, svc, p, TimeseriesQuery{Granularity: g, Metric: MetricActive})
	}
	for _, d := range allDimensions {
		out.Breakdown[string(d)+"/active"] = breakdown(t, svc, p, BreakdownQuery{Dimension: d, Metric: MetricActive})
	}
	for _, d := range []Dimension{DimensionAppVersion, DimensionCountry} {
		for _, m := range []Metric{MetricNew, MetricSessions} {
			out.Breakdown[string(d)+"/"+string(m)] = breakdown(t, svc, p, BreakdownQuery{Dimension: d, Metric: m})
		}
	}
	return out
}

func scenarioParams(t testing.TB, app testutil.ScenarioApp, from, to string) Params {
	t.Helper()
	return Params{
		AppID: app.ID, Location: location(t, app.Timezone), Env: EnvProduction, Unit: identity.Persons,
		Period: Period{From: date(t, from), To: date(t, to)},
	}
}

// AC-029.1 (AC-5.2.1): отчёты аудитории сценария за 2026-10-01…2026-10-12 совпадают с golden; дневные
// active/new и DAU сверяются с эталонной моделью фикстур, итоги — между отчётами.
func TestAudience_Golden(t *testing.T) {
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, testutil.ScenarioPath())
	svc := newService(env)

	got := map[string]audienceSnapshot{}
	for _, app := range testutil.ScenarioApps {
		got[app.Code] = audienceReport(t, svc, scenarioParams(t, app, "2026-10-01", "2026-10-12"))
	}
	testutil.AssertGolden(t, "audience.golden.json", got)

	for _, app := range testutil.ScenarioApps {
		t.Run(app.Code, func(t *testing.T) {
			stats, err := testutil.ScenarioStats(rows, app)
			require.NoError(t, err)
			model := map[string]testutil.ScenarioDay{}
			for _, d := range stats {
				model[d.Day] = d
			}
			snap := got[app.Code]

			require.Equal(t, "2026-10-01", snap.Summary.From)
			require.Equal(t, "2026-10-12", snap.Summary.To)
			require.NotNil(t, snap.Summary.Compare)
			require.Equal(t, "2026-09-19", snap.Summary.Compare.From)
			require.Equal(t, "2026-09-30", snap.Summary.Compare.To)

			active, newDay, sessions := snap.Timeseries["day/active"], snap.Timeseries["day/new"], snap.Timeseries["day/sessions"]
			require.Len(t, active.Points, 12)
			var newSum, sessionSum float64
			var maxDAU uint64
			for i, p := range active.Points {
				d := p.TS.Format(time.DateOnly)
				require.Equal(t, "00:00:00", p.TS.Format(time.TimeOnly), "корзина дня — полночь в TZ приложения")
				require.Equal(t, location(t, app.Timezone).String(), p.TS.Location().String())
				require.Equal(t, f64(float64(model[d].Active)), p.Value, "DAU %s", d)
				require.Equal(t, f64(float64(model[d].New)), newDay.Points[i].Value, "новые %s", d)
				require.Equal(t, uint64(model[d].Active), snap.Rolling.Points[i].DAU, "rolling dau %s", d)
				require.Equal(t, d, snap.Rolling.Points[i].Day)
				newSum += *newDay.Points[i].Value
				sessionSum += *sessions.Points[i].Value
				maxDAU = max(maxDAU, uint64(model[d].Active))
			}
			require.Equal(t, uint64(newSum), snap.Summary.New, "новые периода — сумма новых по дням")
			require.Equal(t, uint64(sessionSum), snap.Summary.Sessions, "сессия начинается в одном дне")
			require.GreaterOrEqual(t, snap.Summary.Active, maxDAU)
			require.Positive(t, snap.Summary.Sessions)
			require.NotNil(t, snap.Summary.AvgDurationMs)
			require.NotNil(t, snap.Summary.MedianDurationMs)

			require.Equal(t, snap.Summary.Active, snap.Breakdown["platform/active"].Total)
			require.Equal(t, snap.Summary.New, snap.Breakdown["app_version/new"].Total)
			require.Equal(t, snap.Summary.Sessions, snap.Breakdown["country/sessions"].Total)
			require.GreaterOrEqual(t, snap.SummaryInstalls.Active, snap.Summary.Active)
			require.Equal(t, snap.Summary.Sessions, snap.SummaryInstalls.Sessions, "сессии не зависят от unit")

			// WAU и MAU последнего дня — активные за 7 и 30 дней по summary.
			last := snap.Rolling.Points[len(snap.Rolling.Points)-1]
			p := scenarioParams(t, app, "2026-10-06", "2026-10-12")
			require.Equal(t, summary(t, svc, p, false).Active, last.WAU)
			p.Period.From = date(t, "2026-09-13")
			require.Equal(t, summary(t, svc, p, false).Active, last.MAU)
		})
	}
}

// AC-029.2 (AC-5.2.2): длительность — только по сессиям с session_end; два session_end одной сессии — max.
func TestAudience_DurationIgnoresSessionsWithoutEnd(t *testing.T) {
	t.Run("ten_sessions_three_open", func(t *testing.T) {
		var rows []chstore.EventRow
		for i := range 10 {
			at := time.Date(2026, 10, 12, 8+i, 0, 0, 0, time.UTC).Format(time.RFC3339)
			if i >= 7 {
				rows = append(rows, visit(t, i, "", at, 0)...)
				continue
			}
			v := visit(t, i, "", at, time.Duration(i+1)*10*time.Second)
			if i == 0 {
				// Повторный session_end первой сессии с меньшей длительностью: берётся max = 10 000.
				rows = append(rows, sessionEnd(v[0], 4*time.Second))
			}
			rows = append(rows, v...)
		}
		svc := newService(startWith(t, rows))
		p := period(t, identity.Persons, "2026-10-12", "2026-10-12")

		got := summary(t, svc, p, false)
		require.Equal(t, uint64(10), got.Sessions)
		require.Equal(t, uint64(10), got.Active)
		require.Equal(t, f64(40000), got.AvgDurationMs, "(10+20+…+70) с / 7")
		require.Equal(t, f64(40000), got.MedianDurationMs)

		ts := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricAvgDuration})
		require.Equal(t, []any{40000.0}, values(ts.Points))
	})

	// Тест-вектор 02-tech-spec.md AC-5.2.2: медиана чётного числа значений — среднее двух средних.
	t.Run("median_of_two", func(t *testing.T) {
		svc := newService(startWith(t, concat(
			visit(t, 1, "", "2026-10-12T10:00:00+03:00", 10*time.Second),
			visit(t, 2, "", "2026-10-12T11:00:00+03:00", 20*time.Second),
			visit(t, 3, "", "2026-10-12T12:00:00+03:00", 0),
		)))
		got := summary(t, svc, period(t, identity.Persons, "2026-10-12", "2026-10-12"), false)
		require.Equal(t, uint64(3), got.Sessions)
		require.Equal(t, f64(15000), got.AvgDurationMs)
		require.Equal(t, f64(15000), got.MedianDurationMs)
	})

	t.Run("no_ended_sessions", func(t *testing.T) {
		svc := newService(startWith(t, visit(t, 1, "", "2026-10-12T10:00:00+03:00", 0)))
		p := period(t, identity.Persons, "2026-10-12", "2026-10-12")
		got := summary(t, svc, p, true)
		require.Equal(t, uint64(1), got.Sessions)
		require.Nil(t, got.AvgDurationMs)
		require.Nil(t, got.MedianDurationMs)
		require.NotNil(t, got.Compare)
		require.Nil(t, got.Compare.AvgDurationMs)
		require.Equal(t, AudienceDelta{}, got.Compare.DeltaPct, "база предыдущего периода — нули и null")

		ts := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricAvgDuration})
		require.Equal(t, []any{nil}, values(ts.Points))
	})
}

// AC-029.3: сутки перехода на зимнее время в America/New_York — 25 часовых точек; 01:00 EDT и 01:00 EST —
// разные корзины. Переход на летнее — 23 точки, Europe/Berlin 2026-10-25 — 25.
func TestTimeseries_DSTTransition(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-11-01T04:59:59Z"), // 00:59:59 EDT
		session(t, 2, "", "2026-11-01T05:30:41Z"), // 01:30:41 EDT
		session(t, 3, "", "2026-11-01T06:31:07Z"), // 01:31:07 EST
		session(t, 3, "", "2026-11-02T04:59:00Z"), // 23:59 EST
		session(t, 4, "", "2026-10-31T12:00:00Z"), // предыдущий день
	})
	svc := newService(env)
	nyc := location(t, "America/New_York")
	p := period(t, identity.Persons, "2026-11-01", "2026-11-01")
	p.Location = nyc

	for _, m := range []Metric{MetricActive, MetricSessions, MetricNew} {
		got := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityHour, Metric: m, Compare: true})
		require.Len(t, got.Points, 25, m)
		require.Equal(t, []string{
			"2026-11-01T00:00:00-04:00", "2026-11-01T01:00:00-04:00", "2026-11-01T01:00:00-05:00", "2026-11-01T02:00:00-05:00",
		}, stamps(got.Points[:4]), m)
		require.Equal(t, "2026-11-01T23:00:00-05:00", got.Points[24].TS.Format(time.RFC3339))
		want := make([]any, 25)
		for i := range want {
			want[i] = 0.0
		}
		want[0], want[1], want[2], want[24] = 1.0, 1.0, 1.0, 1.0
		if m == MetricNew {
			want[24] = 0.0 // установка 3 новая в 01:31 EST
		}
		require.Equal(t, want, values(got.Points), m)

		require.Len(t, got.ComparePoints, 24, "2026-10-31 — обычные сутки")
		require.Equal(t, "2026-10-31T00:00:00-04:00", got.ComparePoints[0].TS.Format(time.RFC3339))
		require.Equal(t, 1.0, *got.ComparePoints[8].Value, "08:00 EDT 31.10")
	}

	p.Period = Period{From: date(t, "2026-03-08"), To: date(t, "2026-03-08")}
	require.Len(t, timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityHour, Metric: MetricActive}).Points, 23)

	p.Location = location(t, "Europe/Berlin")
	p.Period = Period{From: date(t, "2026-10-25"), To: date(t, "2026-10-25")}
	berlin := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityHour, Metric: MetricActive})
	require.Len(t, berlin.Points, 25)
	require.Equal(t, []string{"2026-10-25T02:00:00+02:00", "2026-10-25T02:00:00+01:00"}, stamps(berlin.Points[2:4]))
}

// AC-029.5: environment=development считает только dev-события; без environment — production.
func TestAudience_EnvironmentFilter(t *testing.T) {
	rows := concat(
		visit(t, 1, "", "2026-10-12T10:00:00+03:00", 30*time.Second),
		visit(t, 2, "u-1", "2026-10-12T11:00:00+03:00", 0),
		with(visit(t, 3, "", "2026-10-12T12:00:00+03:00", 90*time.Second), func(r *chstore.EventRow) {
			r.Environment = EnvDevelopment
			r.Country = "DE"
		}),
		with(visit(t, 2, "u-1", "2026-10-12T13:00:00+03:00", 0), func(r *chstore.EventRow) { r.Environment = EnvDevelopment }),
	)
	svc := newService(startWith(t, rows))

	p := period(t, identity.Persons, "2026-10-12", "2026-10-12")
	p.Env = EnvDevelopment
	dev := summary(t, svc, p, false)
	require.Equal(t, AudienceMetrics{
		Active: 2, New: 2, Sessions: 2, AvgDurationMs: f64(90000), MedianDurationMs: f64(90000),
	}, dev.AudienceMetrics)
	require.Equal(t, []any{2.0}, values(timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricActive}).Points))
	require.Equal(t, uint64(2), rolling(t, svc, p).Points[0].DAU)
	require.Equal(t, map[string]uint64{"DE": 1, "": 1},
		rowsOf(breakdown(t, svc, p, BreakdownQuery{Dimension: DimensionCountry, Metric: MetricActive})))

	p.Env = EnvProduction
	prod := summary(t, svc, p, false)
	require.Equal(t, AudienceMetrics{
		Active: 2, New: 2, Sessions: 2, AvgDurationMs: f64(30000), MedianDurationMs: f64(30000),
	}, prod.AudienceMetrics)

	p.Env = ""
	require.Equal(t, prod, summary(t, svc, p, false), "по умолчанию — production")
}

// compare=previous_period: период той же длины сразу перед from и изменение в процентах.
func TestSummary_ComparePreviousPeriod(t *testing.T) {
	svc := newService(startWith(t, concat(
		// Предыдущий период 10-05…10-07: установки 1, 2 новые.
		visit(t, 1, "", "2026-10-05T10:00:00+03:00", 0),
		visit(t, 2, "", "2026-10-06T10:00:00+03:00", 60*time.Second),
		// Текущий 10-08…10-10: 1 и 2 вернулись, 3 новая.
		visit(t, 1, "", "2026-10-08T10:00:00+03:00", 30*time.Second),
		visit(t, 2, "", "2026-10-09T10:00:00+03:00", 90*time.Second),
		visit(t, 3, "", "2026-10-10T10:00:00+03:00", 0),
	)))
	p := period(t, identity.Persons, "2026-10-08", "2026-10-10")

	got := summary(t, svc, p, true)
	require.Equal(t, AudienceSummary{
		From: "2026-10-08", To: "2026-10-10",
		AudienceMetrics: AudienceMetrics{Active: 3, New: 1, Sessions: 3, AvgDurationMs: f64(60000), MedianDurationMs: f64(60000)},
		Compare: &AudienceCompare{
			From: "2026-10-05", To: "2026-10-07",
			AudienceMetrics: AudienceMetrics{Active: 2, New: 2, Sessions: 2, AvgDurationMs: f64(60000), MedianDurationMs: f64(60000)},
			DeltaPct: AudienceDelta{
				Active: f64(50), New: f64(-50), Sessions: f64(50), AvgDurationMs: f64(0), MedianDurationMs: f64(0),
			},
		},
	}, got)

	got.Compare = nil
	require.Equal(t, got, summary(t, svc, p, false), "compare=none — без сравнения")
}

// Тест-векторы 06-api-contracts.md §2.4.4: склейка (active_persons, active_installs), new по first_seen
// человека (new_users), дубли session_start (sessions_dedup) и пустой период.
func TestSummary_PersonsInstallsAndNew(t *testing.T) {
	start := session(t, 1, "u-1", "2026-10-10T10:00:00+03:00")
	svc := newService(startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-08T10:00:00+03:00"), // анонимный день до входа
		start, start, // повтор батча: тот же insert_id
		session(t, 2, "u-1", "2026-10-10T12:00:00+03:00"), // второе устройство u-1
		session(t, 3, "", "2026-10-10T13:00:00+03:00"),
	}))

	persons := summary(t, svc, period(t, identity.Persons, "2026-10-10", "2026-10-10"), false)
	require.Equal(t, uint64(2), persons.Active, "u-1 и установка 3")
	require.Equal(t, uint64(1), persons.New, "установка 3; u-1 впервые 10-08")
	require.Equal(t, uint64(3), persons.Sessions)

	installs := summary(t, svc, period(t, identity.Installs, "2026-10-10", "2026-10-10"), false)
	require.Equal(t, uint64(3), installs.Active)
	require.Equal(t, uint64(2), installs.New, "установки 2 и 3")
	require.Equal(t, uint64(3), installs.Sessions)

	first := summary(t, svc, period(t, identity.Persons, "2026-10-08", "2026-10-08"), false)
	require.Equal(t, uint64(1), first.Active)
	require.Equal(t, uint64(1), first.New)

	empty := summary(t, svc, period(t, identity.Persons, "2026-09-01", "2026-09-07"), true)
	require.Equal(t, AudienceMetrics{}, empty.AudienceMetrics)
	require.Equal(t, AudienceDelta{}, empty.Compare.DeltaPct)
}

// Фильтры применяются к KPI: OR внутри параметра, AND между параметрами (AC-5.2.4).
func TestSummary_Filters(t *testing.T) {
	version := func(platform, v string) func(*chstore.EventRow) {
		return func(r *chstore.EventRow) { r.Platform, r.AppVersion = platform, v }
	}
	svc := newService(startWith(t, concat(
		with(visit(t, 1, "", "2026-10-12T10:00:00+03:00", 0), version("ios", "2.4.0")),
		with(visit(t, 2, "", "2026-10-12T11:00:00+03:00", 0), version("ios", "2.5.0")),
		with(visit(t, 3, "", "2026-10-12T12:00:00+03:00", 0), version("android", "2.4.0")),
		with(visit(t, 4, "", "2026-10-12T13:00:00+03:00", 0), version("ios", "2.6.0")),
	)))
	p := period(t, identity.Persons, "2026-10-12", "2026-10-12")
	p.Filters = Filters{Platform: []string{"ios"}, AppVersion: []string{"2.4.0", "2.5.0"}}
	got := summary(t, svc, p, false)
	require.Equal(t, uint64(2), got.Active)
	require.Equal(t, uint64(2), got.Sessions)
	require.Equal(t, uint64(2), breakdown(t, svc, p, BreakdownQuery{Dimension: DimensionPlatform, Metric: MetricActive}).Total)
}

// Пустые корзины — 0 (avg_duration_ms — null), ряд полный от from до to; новые и сессии по корзинам.
func TestTimeseries_FillsGaps(t *testing.T) {
	svc := newService(startWith(t, concat(
		visit(t, 1, "", "2026-10-01T10:00:00+03:00", 20*time.Second),
		visit(t, 1, "", "2026-10-01T18:00:00+03:00", 40*time.Second),
		visit(t, 1, "", "2026-10-03T10:00:00+03:00", 0),
		visit(t, 2, "", "2026-10-03T11:00:00+03:00", 0),
	)))
	p := period(t, identity.Persons, "2026-10-01", "2026-10-04")
	series := func(m Metric) []any {
		return values(timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: m}).Points)
	}
	require.Equal(t, []any{1.0, 0.0, 2.0, 0.0}, series(MetricActive))
	require.Equal(t, []any{1.0, 0.0, 1.0, 0.0}, series(MetricNew))
	require.Equal(t, []any{2.0, 0.0, 2.0, 0.0}, series(MetricSessions))
	require.Equal(t, []any{30000.0, nil, nil, nil}, series(MetricAvgDuration))
	require.Equal(t, []string{
		"2026-10-01T00:00:00+03:00", "2026-10-02T00:00:00+03:00", "2026-10-03T00:00:00+03:00", "2026-10-04T00:00:00+03:00",
	}, stamps(timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricActive}).Points))

	// Пустой период: все корзины нулевые, сравнение тоже.
	p = period(t, identity.Persons, "2026-09-01", "2026-09-03")
	empty := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricActive, Compare: true})
	require.Equal(t, []any{0.0, 0.0, 0.0}, values(empty.Points))
	require.Equal(t, []any{0.0, 0.0, 0.0}, values(empty.ComparePoints))
}

// Недели — ISO с понедельника, месяцы — календарные, в TZ приложения; новые — по корзине first_seen.
func TestTimeseries_WeekAndMonthBuckets(t *testing.T) {
	svc := newService(startWith(t, concat(
		visit(t, 1, "", "2026-09-30T23:30:00+03:00", 0), // среда, сентябрь
		visit(t, 1, "", "2026-10-11T23:30:00+03:00", 0), // воскресенье
		visit(t, 2, "", "2026-10-12T00:30:00+03:00", 0), // понедельник (в UTC ещё 11-е)
	)))
	p := period(t, identity.Persons, "2026-09-30", "2026-10-12")

	week := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityWeek, Metric: MetricActive})
	require.Equal(t, []string{"2026-09-28T00:00:00+03:00", "2026-10-05T00:00:00+03:00", "2026-10-12T00:00:00+03:00"}, stamps(week.Points))
	require.Equal(t, []any{1.0, 1.0, 1.0}, values(week.Points))
	weekNew := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityWeek, Metric: MetricNew})
	require.Equal(t, []any{1.0, 0.0, 1.0}, values(weekNew.Points))

	month := timeseries(t, svc, p, TimeseriesQuery{Granularity: GranularityMonth, Metric: MetricActive})
	require.Equal(t, []string{"2026-09-01T00:00:00+03:00", "2026-10-01T00:00:00+03:00"}, stamps(month.Points))
	require.Equal(t, []any{1.0, 2.0}, values(month.Points))
}

// DAU — день, WAU — 7 дней, MAU — 30 дней, заканчивая днём; окна читают события до from.
func TestRollingActive_Windows(t *testing.T) {
	svc := newService(startWith(t, concat(
		visit(t, 1, "", "2026-09-08T10:00:00+03:00", 0),
		visit(t, 2, "", "2026-10-01T10:00:00+03:00", 0),
		visit(t, 3, "", "2026-10-06T10:00:00+03:00", 0),
	)))
	got := rolling(t, svc, period(t, identity.Persons, "2026-10-06", "2026-10-08"))
	require.Equal(t, []RollingPoint{
		{Day: "2026-10-06", DAU: 1, WAU: 2, MAU: 3},
		{Day: "2026-10-07", DAU: 0, WAU: 2, MAU: 3},
		{Day: "2026-10-08", DAU: 0, WAU: 1, MAU: 2},
	}, got.Points)
}

// Разбивка: пустое значение — "", доля от total без разбивки (сумма долей active может быть > 1),
// сортировка по count DESC, value ASC, limit, город с кодом страны, ОС с версией, unit=installs.
func TestBreakdown_ValuesAndShare(t *testing.T) {
	place := func(version, country, city string) func(*chstore.EventRow) {
		return func(r *chstore.EventRow) {
			r.AppVersion, r.Country, r.City = version, country, city
			if r.UserID == "u-1" {
				r.OSName, r.OSVersion = "iOS", "18.1"
			}
		}
	}
	svc := newService(startWith(t, concat(
		with(visit(t, 3, "", "2026-10-01T10:00:00+03:00", 0), place("2.4.0", "DE", "Berlin")), // до периода
		with(visit(t, 1, "u-1", "2026-10-10T10:00:00+03:00", 0), place("2.4.0", "RU", "Moscow")),
		with(visit(t, 1, "u-1", "2026-10-11T10:00:00+03:00", 0), place("2.5.0", "RU", "Moscow")),
		with(visit(t, 2, "", "2026-10-11T11:00:00+03:00", 0), place("2.5.0", "", "")),
		with(visit(t, 3, "", "2026-10-12T10:00:00+03:00", 0), place("2.5.0", "DE", "Berlin")),
		with(visit(t, 4, "u-1", "2026-10-12T11:00:00+03:00", 0), place("2.5.0", "RU", "Moscow")),
	)))
	p := period(t, identity.Persons, "2026-10-10", "2026-10-12")
	get := func(p Params, d Dimension, m Metric, limit int) Breakdown {
		return breakdown(t, svc, p, BreakdownQuery{Dimension: d, Metric: m, Limit: limit})
	}

	require.Equal(t, Breakdown{
		Dimension: DimensionAppVersion, Metric: MetricActive, Total: 3,
		Rows: []BreakdownRow{{Value: "2.5.0", Count: 3, Share: 1}, {Value: "2.4.0", Count: 1, Share: 0.3333}},
	}, get(p, DimensionAppVersion, MetricActive, 0))
	require.Equal(t, []BreakdownRow{{Value: "2.5.0", Count: 3, Share: 1}}, get(p, DimensionAppVersion, MetricActive, 1).Rows)

	require.Equal(t, []BreakdownRow{
		{Value: "", Count: 1, Share: 0.3333}, {Value: "DE", Count: 1, Share: 0.3333}, {Value: "RU", Count: 1, Share: 0.3333},
	}, get(p, DimensionCountry, MetricActive, 0).Rows)
	require.Equal(t, []string{"", "Berlin, DE", "Moscow, RU"}, valuesOf(get(p, DimensionCity, MetricActive, 0)))
	require.Equal(t, map[string]uint64{"iOS 18.1": 1, "": 2}, rowsOf(get(p, DimensionOSVersion, MetricActive, 0)))

	newBy := get(p, DimensionAppVersion, MetricNew, 0)
	require.Equal(t, uint64(2), newBy.Total, "u-1 и установка 2; установка 3 не новая")
	require.Equal(t, map[string]uint64{"2.5.0": 2, "2.4.0": 1}, rowsOf(newBy))

	sessions := get(p, DimensionCountry, MetricSessions, 0)
	require.Equal(t, uint64(5), sessions.Total)
	require.Equal(t, []BreakdownRow{
		{Value: "RU", Count: 3, Share: 0.6}, {Value: "", Count: 1, Share: 0.2}, {Value: "DE", Count: 1, Share: 0.2},
	}, sessions.Rows)

	installs := period(t, identity.Installs, "2026-10-10", "2026-10-12")
	byInstall := get(installs, DimensionAppVersion, MetricActive, 0)
	require.Equal(t, uint64(4), byInstall.Total)
	require.Equal(t, map[string]uint64{"2.5.0": 4, "2.4.0": 1}, rowsOf(byInstall))

	empty := get(period(t, identity.Persons, "2026-09-01", "2026-09-02"), DimensionCountry, MetricActive, 0)
	require.Zero(t, empty.Total)
	require.Empty(t, empty.Rows)
	require.NotNil(t, empty.Rows, "пустая разбивка — [] в JSON")
}

func valuesOf(b Breakdown) []string {
	out := make([]string, 0, len(b.Rows))
	for _, r := range b.Rows {
		out = append(out, r.Value)
	}
	return out
}

// Значения вне whitelist не доходят до SQL.
func TestAudience_Unsupported(t *testing.T) {
	svc := newService(startWith(t))
	p := period(t, identity.Persons, "2026-10-12", "2026-10-12")
	for _, q := range []TimeseriesQuery{
		{Granularity: "quarter", Metric: MetricActive},
		{Granularity: GranularityDay, Metric: "retention"},
	} {
		_, err := svc.AudienceTimeseries(t.Context(), p, q)
		require.ErrorIs(t, err, ErrUnsupported)
	}
	for _, q := range []BreakdownQuery{
		{Dimension: "ip", Metric: MetricActive},
		{Dimension: DimensionCountry, Metric: MetricAvgDuration},
		{Dimension: "country; DROP TABLE events", Metric: MetricActive},
	} {
		_, err := svc.AudienceBreakdown(t.Context(), p, q)
		require.ErrorIs(t, err, ErrUnsupported, q.Dimension)
		require.False(t, strings.Contains(err.Error(), "DROP"), "значение не цитируется")
	}
}
