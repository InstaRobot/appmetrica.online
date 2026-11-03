//go:build integration

package reports

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// Отчёт «События» (FR-102, T-CE-005) на сценарии и своих событиях поверх его сессий
// (testdata/fixtures/README.md, custom_events.ndjson): msk и nyc шлют свои события, tyo — нет.
var (
	eventsMsk = testutil.ScenarioApps[0]
	eventsNyc = testutil.ScenarioApps[1]
	eventsTyo = testutil.ScenarioApps[2]
)

// startEvents загружает scenario.ndjson и custom_events.ndjson и возвращает все строки фикстур.
func startEvents(t *testing.T) (*testutil.Env, []chstore.EventRow) {
	t.Helper()
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, testutil.ScenarioPath())
	rows = append(rows, env.LoadNDJSON(t, testutil.CustomEventsPath())...)
	return env, rows
}

// eventsPeriod — 30 дней 2026-10-04…2026-11-02 (предыдущий период 09-04…10-03 захватывает начало сценария).
func eventsPeriod(t testing.TB, app testutil.ScenarioApp) Params {
	t.Helper()
	return scenarioParams(t, app, "2026-10-04", "2026-11-02")
}

func eventsList(t *testing.T, svc *Service, p Params, compare bool) EventList {
	t.Helper()
	got, err := svc.EventsList(t.Context(), p, compare)
	require.NoError(t, err)
	return got
}

func eventReport(t *testing.T, svc *Service, p Params, q EventReportQuery) EventReport {
	t.Helper()
	got, err := svc.EventReport(t.Context(), p, q)
	require.NoError(t, err)
	require.Equal(t, q.Name, got.Name)
	require.Len(t, got.Timeseries, len(bucketStarts(p.Period, p.Location, q.Granularity)))
	return got
}

func eventBreakdown(t *testing.T, svc *Service, p Params, q EventBreakdownQuery) EventBreakdown {
	t.Helper()
	got, err := svc.EventBreakdown(t.Context(), p, q)
	require.NoError(t, err)
	require.Equal(t, q.Property, got.Property)
	return got
}

// eventsModel — эталон отчёта «События» по строкам фикстур (feature spec §4): событие — уникальный insert_id,
// человек — person_key события через первого владельца установки (самое раннее событие с user_id, как
// identity_links), unit=installs — установка; период, environment и фильтры — как eventsFilter.
type eventsModel struct {
	rows  []chstore.EventRow
	links map[uuid.UUID]string
}

func newEventsModel(rows []chstore.EventRow) *eventsModel {
	type link struct {
		at   time.Time
		user string
	}
	first := map[uuid.UUID]link{}
	for _, r := range rows {
		if r.UserID == "" {
			continue
		}
		if l, ok := first[r.InstallID]; !ok || r.EventTime.Before(l.at) {
			first[r.InstallID] = link{r.EventTime, r.UserID}
		}
	}
	m := &eventsModel{rows: rows, links: map[uuid.UUID]string{}}
	for id, l := range first {
		m.links[id] = l.user
	}
	return m
}

func (m *eventsModel) selected(p Params, r chstore.EventRow) bool {
	start, end := p.Period.Bounds(p.location())
	env := cmp.Or(p.Env, EnvProduction)
	in := func(values []string, v string) bool { return len(values) == 0 || slices.Contains(values, v) }
	return r.AppID == p.AppID && r.Environment == env && !r.EventTime.Before(start) && r.EventTime.Before(end) &&
		in(p.Filters.Platform, r.Platform) && in(p.Filters.AppVersion, r.AppVersion) &&
		in(p.Filters.OSName, r.OSName) && in(p.Filters.Country, r.Country)
}

func (m *eventsModel) person(p Params, r chstore.EventRow) string {
	switch {
	case p.Unit == identity.Installs:
		return identity.InstallKey(r.InstallID).String()
	case r.UserID != "":
		return identity.UserKey(r.UserID).String()
	case m.links[r.InstallID] != "":
		return identity.UserKey(m.links[r.InstallID]).String()
	default:
		return identity.InstallKey(r.InstallID).String()
	}
}

// count — события и люди строк периода p, для которых keep.
func (m *eventsModel) count(p Params, keep func(chstore.EventRow) bool) EventCount {
	events, people := map[uuid.UUID]bool{}, map[string]bool{}
	for _, r := range m.rows {
		if m.selected(p, r) && keep(r) {
			events[r.InsertID] = true
			people[m.person(p, r)] = true
		}
	}
	return EventCount{Events: uint64(len(events)), People: uint64(len(people))}
}

func named(name string) func(chstore.EventRow) bool {
	return func(r chstore.EventRow) bool { return r.Name == name }
}

func isCustom(r chstore.EventRow) bool { return r.Name != "session_start" && r.Name != "session_end" }

func previous(p Params) Params {
	p.Period = p.Period.Previous()
	return p
}

// list — имена своих событий периода p по убыванию событий, при равенстве — по имени.
func (m *eventsModel) list(p Params, compare bool) EventList {
	names := map[string]bool{}
	for _, r := range m.rows {
		if m.selected(p, r) && isCustom(r) {
			names[r.Name] = true
		}
	}
	out := EventList{Events: []EventStat{}}
	for name := range names {
		c := m.count(p, named(name))
		st := EventStat{Name: name, Events: c.Events, People: c.People}
		if compare {
			prev := m.count(previous(p), named(name))
			st.PrevEvents, st.PrevPeople = &prev.Events, &prev.People
		}
		out.Events = append(out.Events, st)
	}
	slices.SortFunc(out.Events, func(a, b EventStat) int {
		return cmp.Or(cmp.Compare(b.Events, a.Events), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// breakdown — разбивка события name по значениям свойства key: топ limit по событиям (при равенстве — по
// значению), остальные значения одной строкой (люди — по объединению), события без ключа.
func (m *eventsModel) breakdown(p Params, name, key string, limit int) EventBreakdown {
	values := map[string]bool{}
	for _, r := range m.rows {
		if v, ok := r.Properties[key]; ok && m.selected(p, r) && r.Name == name {
			values[v] = true
		}
	}
	out := EventBreakdown{Property: key, Values: []EventValueCount{}}
	for v := range values {
		c := m.count(p, func(r chstore.EventRow) bool {
			got, ok := r.Properties[key]
			return r.Name == name && ok && got == v
		})
		out.Values = append(out.Values, EventValueCount{Value: v, Events: c.Events, People: c.People})
	}
	slices.SortFunc(out.Values, func(a, b EventValueCount) int {
		return cmp.Or(cmp.Compare(b.Events, a.Events), cmp.Compare(a.Value, b.Value))
	})
	if len(out.Values) > limit {
		top := map[string]bool{}
		for _, v := range out.Values[:limit] {
			top[v.Value] = true
		}
		other := m.count(p, func(r chstore.EventRow) bool {
			v, ok := r.Properties[key]
			return r.Name == name && ok && !top[v]
		})
		out.Other = &other
		out.Values = out.Values[:limit]
	}
	out.Missing = m.count(p, func(r chstore.EventRow) bool {
		_, ok := r.Properties[key]
		return r.Name == name && !ok
	})
	return out
}

func perPerson(c EventCount) float64 {
	if c.People == 0 {
		return 0
	}
	return math.Round(float64(c.Events)/float64(c.People)*100) / 100
}

// eventsSnapshot — golden отчёта «События»: список (compare), страницы событий и разбивки.
type eventsSnapshot struct {
	List      map[string]EventList      `json:"list"`
	Report    map[string]EventReport    `json:"report"`
	Breakdown map[string]EventBreakdown `json:"breakdown"`
}

// AC-005.1: список за 30 дней — имена по убыванию событий, системных нет, повторы insert_id посчитаны один раз,
// prev-поля только при compare; совпадает с эталонной моделью и вместе со страницами событий и разбивками —
// с events.golden.json.
func TestEventsList_Golden(t *testing.T) {
	env, rows := startEvents(t)
	svc := newService(env)
	model := newEventsModel(rows)

	got := eventsSnapshot{List: map[string]EventList{}, Report: map[string]EventReport{}, Breakdown: map[string]EventBreakdown{}}
	for _, app := range []testutil.ScenarioApp{eventsMsk, eventsNyc, eventsTyo} {
		got.List[app.Code] = eventsList(t, svc, eventsPeriod(t, app), true)
	}
	for _, c := range []struct {
		app  testutil.ScenarioApp
		name string
		g    Granularity
	}{
		{eventsMsk, "purchase", GranularityDay},
		{eventsNyc, "purchase", GranularityDay},
		{eventsMsk, "note_created", GranularityWeek},
	} {
		got.Report[c.app.Code+"/"+c.name+"/"+string(c.g)] = eventReport(t, svc, eventsPeriod(t, c.app),
			EventReportQuery{Name: c.name, Granularity: c.g, Compare: true})
	}
	for _, c := range []struct {
		app            testutil.ScenarioApp
		name, property string
	}{
		{eventsMsk, "note_created", "source"},
		{eventsMsk, "purchase", "plan"},
		{eventsNyc, "purchase", "plan"},
	} {
		got.Breakdown[c.app.Code+"/"+c.name+"/"+c.property] = eventBreakdown(t, svc, eventsPeriod(t, c.app),
			EventBreakdownQuery{Name: c.name, Property: c.property})
	}
	testutil.AssertGolden(t, "events.golden.json", got)

	for _, app := range []testutil.ScenarioApp{eventsMsk, eventsNyc} {
		t.Run(app.Code, func(t *testing.T) {
			p := eventsPeriod(t, app)
			list := got.List[app.Code]
			require.Equal(t, model.list(p, true), list)
			require.False(t, list.Truncated)
			require.NotEmpty(t, list.Events)

			var events uint64
			for i, st := range list.Events {
				require.NotContains(t, []string{"session_start", "session_end"}, st.Name, "системное имя в списке")
				require.LessOrEqual(t, st.People, st.Events)
				require.NotNil(t, st.PrevEvents, "%s: prev_events при compare", st.Name)
				require.NotNil(t, st.PrevPeople, "%s: prev_people при compare", st.Name)
				if i > 0 {
					prev := list.Events[i-1]
					require.True(t, prev.Events > st.Events || prev.Events == st.Events && prev.Name < st.Name,
						"порядок: events DESC, name ASC (%s, %s)", prev.Name, st.Name)
				}
				events += st.Events
			}
			// Повторы insert_id есть в периоде, но каждое событие посчитано один раз.
			var deliveries uint64
			for _, r := range rows {
				if isCustom(r) && model.selected(p, r) {
					deliveries++
				}
			}
			require.Equal(t, model.count(p, isCustom).Events, events)
			require.Greater(t, deliveries, events, "в периоде есть повторно доставленные события")

			// Без compare prev-полей нет, текущие числа те же.
			plain := eventsList(t, svc, p, false)
			require.Len(t, plain.Events, len(list.Events))
			for i, st := range plain.Events {
				require.Nil(t, st.PrevEvents)
				require.Nil(t, st.PrevPeople)
				require.Equal(t, list.Events[i].Name, st.Name)
				require.Equal(t, list.Events[i].Events, st.Events)
				require.Equal(t, list.Events[i].People, st.People)
			}
		})
	}
	require.Equal(t, EventList{Events: []EventStat{}}, got.List[eventsTyo.Code], "tyo своих событий не шлёт")
}

// AC-005.2: страница purchase по дням — сумма ряда равна событиям сводки, plan среди ключей свойств, людей не
// больше событий; сводка и прошлый период совпадают с эталонной моделью.
func TestEventReport_Consistent(t *testing.T) {
	env, rows := startEvents(t)
	svc := newService(env)
	model := newEventsModel(rows)

	for _, app := range []testutil.ScenarioApp{eventsMsk, eventsNyc} {
		t.Run(app.Code, func(t *testing.T) {
			p := eventsPeriod(t, app)
			cur, prev := model.count(p, named("purchase")), model.count(previous(p), named("purchase"))
			got := eventReport(t, svc, p, EventReportQuery{Name: "purchase", Granularity: GranularityDay, Compare: true})

			require.Equal(t, EventSummary{
				Events: cur.Events, People: cur.People, EventsPerPerson: perPerson(cur),
				PrevEvents: &prev.Events, PrevPeople: &prev.People, PrevEventsPerPerson: new(perPerson(prev)),
			}, got.Summary)
			require.Positive(t, got.Summary.Events)
			require.LessOrEqual(t, got.Summary.People, got.Summary.Events)

			var sum uint64
			for i, pt := range got.Timeseries {
				require.True(t, bucketStarts(p.Period, p.Location, GranularityDay)[i].Equal(pt.Bucket))
				require.Equal(t, p.location().String(), pt.Bucket.Location().String())
				require.LessOrEqual(t, pt.People, pt.Events, "люди корзины %s", pt.Bucket)
				sum += pt.Events
			}
			require.Equal(t, got.Summary.Events, sum, "сумма ряда — события сводки")

			withPlan := model.count(p, func(r chstore.EventRow) bool {
				_, ok := r.Properties["plan"]
				return r.Name == "purchase" && ok
			})
			require.Equal(t, []EventPropertyKey{{Key: "plan", Events: withPlan.Events}}, got.PropertyKeys)
			require.Less(t, withPlan.Events, got.Summary.Events, "plan есть не у всех purchase")

			// Неделя и месяц — тот же итог; без compare prev-полей нет.
			for _, g := range []Granularity{GranularityWeek, GranularityMonth} {
				other := eventReport(t, svc, p, EventReportQuery{Name: "purchase", Granularity: g})
				require.Equal(t, EventSummary{Events: cur.Events, People: cur.People, EventsPerPerson: perPerson(cur)}, other.Summary)
				var total uint64
				for _, pt := range other.Timeseries {
					total += pt.Events
				}
				require.Equal(t, cur.Events, total, "сумма ряда %s", g)
			}
		})
	}
}

// AC-005.3: разбивка note_created по source с limit=20 — 20 значений и «остальные», вместе с «нет свойства»
// они дают все события note_created; люди «остальных» — по объединению значений.
func TestEventBreakdown_OtherAndMissing(t *testing.T) {
	env, rows := startEvents(t)
	svc := newService(env)
	model := newEventsModel(rows)
	p := eventsPeriod(t, eventsMsk)

	got := eventBreakdown(t, svc, p, EventBreakdownQuery{Name: "note_created", Property: "source", Limit: 20})
	require.Equal(t, model.breakdown(p, "note_created", "source", 20), got)
	require.Len(t, got.Values, 20)
	require.NotNil(t, got.Other)
	require.Positive(t, got.Other.Events)
	require.Positive(t, got.Missing.Events, "у части note_created нет source")
	sum := got.Other.Events + got.Missing.Events
	for _, v := range got.Values {
		sum += v.Events
	}
	require.Equal(t, model.count(p, named("note_created")).Events, sum)

	require.Equal(t, got, eventBreakdown(t, svc, p, EventBreakdownQuery{Name: "note_created", Property: "source"}),
		"limit по умолчанию — 20")

	all := eventBreakdown(t, svc, p, EventBreakdownQuery{Name: "note_created", Property: "source", Limit: MaxEventBreakdownLimit})
	require.Equal(t, model.breakdown(p, "note_created", "source", MaxEventBreakdownLimit), all)
	require.Nil(t, all.Other, "значений не больше limit — строки «остальные» нет")
	require.Greater(t, len(all.Values), 20)
	require.Equal(t, got.Values, all.Values[:20])

	plan := eventBreakdown(t, svc, p, EventBreakdownQuery{Name: "purchase", Property: "plan", Limit: 20})
	require.Equal(t, model.breakdown(p, "purchase", "plan", 20), plan)
	require.Nil(t, plan.Other)
	require.Positive(t, plan.Missing.Events, "plan — у 80 % purchase")

	none := eventBreakdown(t, svc, p, EventBreakdownQuery{Name: "purchase", Property: "source"})
	require.Equal(t, EventBreakdown{Property: "source", Values: []EventValueCount{}, Missing: model.count(p, named("purchase"))}, none,
		"ключа нет ни у одного события — всё в «нет свойства»")
}

// AC-005.4: platform, app_version, unit=installs и environment=development применяются, как у аудитории:
// три функции совпадают с эталонной моделью по фикстурам.
func TestEvents_FiltersAndUnit(t *testing.T) {
	env, rows := startEvents(t)
	svc := newService(env)
	model := newEventsModel(rows)

	type tc struct {
		name string
		app  testutil.ScenarioApp
		edit func(*Params)
	}
	cases := []tc{
		{"msk/platform=ios", eventsMsk, func(p *Params) { p.Filters.Platform = []string{"ios"} }},
		{"nyc/platform=ios", eventsNyc, func(p *Params) { p.Filters.Platform = []string{"ios"} }},
		{"msk/app_version=3.5.0", eventsMsk, func(p *Params) { p.Filters.AppVersion = []string{"3.5.0"} }},
		{"nyc/app_version=2.1.0", eventsNyc, func(p *Params) { p.Filters.AppVersion = []string{"2.1.0"} }},
		{"msk/unit=installs", eventsMsk, func(p *Params) { p.Unit = identity.Installs }},
		{"nyc/unit=installs", eventsNyc, func(p *Params) { p.Unit = identity.Installs }},
		{"msk/environment=development", eventsMsk, func(p *Params) { p.Env = EnvDevelopment }},
		{"nyc/environment=development", eventsNyc, func(p *Params) { p.Env = EnvDevelopment }},
		{"nyc/android+2.2.0+installs", eventsNyc, func(p *Params) {
			p.Filters.Platform, p.Filters.AppVersion, p.Unit = []string{"android"}, []string{"2.2.0"}, identity.Installs
		}},
	}
	breakdownOf := map[string][2]string{eventsMsk.Code: {"note_created", "source"}, eventsNyc.Code: {"search_used", "results"}}

	nonEmpty := 0
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := eventsPeriod(t, c.app)
			c.edit(&p)

			list := eventsList(t, svc, p, true)
			require.Equal(t, model.list(p, true), list)

			event, key := breakdownOf[c.app.Code][0], breakdownOf[c.app.Code][1]
			report := eventReport(t, svc, p, EventReportQuery{Name: event, Granularity: GranularityDay})
			cur := model.count(p, named(event))
			require.Equal(t, EventSummary{Events: cur.Events, People: cur.People, EventsPerPerson: perPerson(cur)}, report.Summary)

			require.Equal(t, model.breakdown(p, event, key, 10),
				eventBreakdown(t, svc, p, EventBreakdownQuery{Name: event, Property: key, Limit: 10}))
			if len(list.Events) > 0 {
				nonEmpty++
			}
		})
	}
	require.Greater(t, nonEmpty, 1, "фильтры оставляют события, а не только пустые ответы")

	// unit влияет на людей (за 30 дней эффекты msk случайно гасят друг друга): 10-05…10-06 две установки u_01
	// (msk 00 и msk 01) — один человек, но две установки; 10-04…10-10 общий девайс msk 00 — два человека
	// (u_01 и u_02) на одной установке.
	for _, w := range []struct {
		from, to     string
		fewerPersons bool
	}{{"2026-10-05", "2026-10-06", true}, {"2026-10-04", "2026-10-10", false}} {
		persons := scenarioParams(t, eventsMsk, w.from, w.to)
		installs := persons
		installs.Unit = identity.Installs
		byPersons, byInstalls := eventsList(t, svc, persons, false), eventsList(t, svc, installs, false)
		require.Equal(t, model.list(persons, false), byPersons)
		require.Equal(t, model.list(installs, false), byInstalls)
		found := false
		for i, st := range byPersons.Events {
			other := byInstalls.Events[i]
			require.Equal(t, st.Events, other.Events, "события от unit не зависят")
			found = found || (w.fewerPersons && st.People < other.People) || (!w.fewerPersons && st.People > other.People)
		}
		require.True(t, found, "%s…%s: unit=installs считает установки, а не людей", w.from, w.to)
	}
}

// customEvent — своё событие name установки n приложения identityApp (Europe/Moscow в period).
func customEvent(t testing.TB, n int, at, name string, props map[string]string) chstore.EventRow {
	t.Helper()
	r := session(t, n, "", at)
	r.Name, r.Properties = name, props
	return r
}

// Пределы и края: не больше 500 имён с флагом truncated, не больше 50 ключей свойств, системные имена,
// неизвестная гранулярность и limit вне 1…100, пустой период — нули, а не ошибка.
func TestEvents_LimitsAndEdges(t *testing.T) {
	const at = "2026-10-05T12:00:00+03:00"
	var rows []chstore.EventRow
	for i := range EventsListMax + 1 {
		rows = append(rows, customEvent(t, i%7, at, fmt.Sprintf("e_%03d", i), nil))
	}
	rows = append(rows, session(t, 1, "", at)) // системное имя в список не входит
	// "wide": 60 ключей по одному событию и ключ "a" у двух событий.
	for i := range EventPropertyKeysMax + 10 {
		rows = append(rows, customEvent(t, 1, at, "wide", map[string]string{fmt.Sprintf("k_%02d", i): "v"}))
	}
	rows = append(rows,
		customEvent(t, 1, at, "wide", map[string]string{"a": "1"}),
		customEvent(t, 2, at, "wide", map[string]string{"a": "2"}),
	)
	// "ratio": 4 события у трёх человек — 1.33 на человека.
	rows = append(rows,
		customEvent(t, 1, at, "ratio", nil), customEvent(t, 1, at, "ratio", nil),
		customEvent(t, 2, at, "ratio", nil), customEvent(t, 3, at, "ratio", nil),
	)
	svc := newService(startWith(t, rows))
	p := period(t, identity.Persons, "2026-10-05", "2026-10-05")
	ctx := t.Context()

	list := eventsList(t, svc, p, false)
	require.True(t, list.Truncated)
	require.Len(t, list.Events, EventsListMax)
	require.Equal(t, "wide", list.Events[0].Name, "62 события")
	require.Equal(t, EventStat{Name: "ratio", Events: 4, People: 3}, list.Events[1])
	names := make([]string, 0, len(list.Events))
	for _, st := range list.Events[2:] {
		names = append(names, st.Name)
	}
	require.True(t, slices.IsSorted(names), "при равенстве событий — по имени")
	require.Equal(t, "e_497", names[len(names)-1], "e_498…e_500 отрезаны")

	wide := eventReport(t, svc, p, EventReportQuery{Name: "wide", Granularity: GranularityHour})
	require.Len(t, wide.PropertyKeys, EventPropertyKeysMax)
	require.Equal(t, EventPropertyKey{Key: "a", Events: 2}, wide.PropertyKeys[0])
	require.Equal(t, EventPropertyKey{Key: "k_00", Events: 1}, wide.PropertyKeys[1])
	require.Equal(t, uint64(EventPropertyKeysMax+12), wide.Summary.Events)

	ratio := eventReport(t, svc, p, EventReportQuery{Name: "ratio", Granularity: GranularityDay})
	require.Equal(t, EventSummary{Events: 4, People: 3, EventsPerPerson: 1.33}, ratio.Summary)
	require.Equal(t, []EventPropertyKey{}, ratio.PropertyKeys)

	// Событий нет — нули: страница события существует, пока имя не системное.
	empty := eventReport(t, svc, p, EventReportQuery{Name: "never_sent", Granularity: GranularityDay, Compare: true})
	zero := uint64(0)
	require.Equal(t, EventSummary{PrevEvents: &zero, PrevPeople: &zero, PrevEventsPerPerson: new(0.0)}, empty.Summary)
	require.Equal(t, []EventPoint{{Bucket: bucketStarts(p.Period, p.Location, GranularityDay)[0]}}, empty.Timeseries)
	require.Equal(t, []EventPropertyKey{}, empty.PropertyKeys)
	require.Equal(t, EventBreakdown{Property: "plan", Values: []EventValueCount{}},
		eventBreakdown(t, svc, p, EventBreakdownQuery{Name: "never_sent", Property: "plan"}))

	for _, name := range []string{"session_start", "session_end"} {
		_, err := svc.EventReport(ctx, p, EventReportQuery{Name: name, Granularity: GranularityDay})
		require.ErrorIs(t, err, ErrSystemEvent, name)
		_, err = svc.EventBreakdown(ctx, p, EventBreakdownQuery{Name: name, Property: "plan"})
		require.ErrorIs(t, err, ErrSystemEvent, name)
	}
	_, err := svc.EventReport(ctx, p, EventReportQuery{Name: "ratio", Granularity: "minute"})
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = svc.EventBreakdown(ctx, p, EventBreakdownQuery{Name: "ratio", Property: "a", Limit: MaxEventBreakdownLimit + 1})
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = svc.EventBreakdown(ctx, p, EventBreakdownQuery{Name: "wide", Property: "a", Limit: MaxEventBreakdownLimit})
	require.NoError(t, err)
}

// Длительность отчёта «События» наблюдается метками events_list, event_report и event_breakdown.
func TestEvents_ObservedReports(t *testing.T) {
	svc := newService(startWith(t))
	var got []string
	svc.ObserveQueries(func(report string, _ time.Duration) { got = append(got, report) })
	p := period(t, identity.Persons, "2026-10-05", "2026-10-05")
	_, err := svc.EventsList(t.Context(), p, true)
	require.NoError(t, err)
	_, err = svc.EventReport(t.Context(), p, EventReportQuery{Name: "purchase", Granularity: GranularityDay})
	require.NoError(t, err)
	_, err = svc.EventBreakdown(t.Context(), p, EventBreakdownQuery{Name: "purchase", Property: "plan"})
	require.NoError(t, err)
	require.Equal(t, []string{ReportEventsList, ReportEventReport, ReportEventBreakdown}, got)
	require.Equal(t, []string{"events_list", "event_report", "event_breakdown"}, got, "значения метки report")
}
