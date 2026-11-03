package reports

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/identity"
)

// Пределы отчёта «События» (FR-102, feature spec §4).
const (
	EventsListMax              = 500 // имён в списке; остальные отрезаются с флагом truncated
	EventPropertyKeysMax       = 50  // ключей свойств на странице события
	DefaultEventBreakdownLimit = 20  // значений разбивки по умолчанию
	MaxEventBreakdownLimit     = 100
)

// ErrSystemEvent — session_start и session_end в отчёт «События» не входят (Р-5): их показывают аудитория и сессии.
var ErrSystemEvent = errors.New("reports: системное событие не входит в отчёт «События»")

// systemEventNames — системные имена ingest (SystemEventName в contracts/openapi/ingest.yaml).
var systemEventNames = []string{"session_start", "session_end"}

func isSystemEvent(name string) bool { return slices.Contains(systemEventNames, name) }

// Условия по events AS e, дописываемые к eventsFilter: свои события списка и одно имя страницы события.
var (
	customEventsWhere = "\n      AND e.name NOT IN ('" + strings.Join(systemEventNames, "', '") + "')"
	eventNameWhere    = "\n      AND e.name = {name:String}"
)

// Запросы отчёта «События» (feature spec §4; QProfileEvents — в профиле). Отличия от аудитории:
//   - событие считается uniqExact(insert_id): до слияния частей ReplacingMergeTree повтор батча виден несколькими
//     строками с тем же insert_id, а count() посчитал бы каждую (прецедент integration_health.go, OQ-042; правило
//     линтера — eventMetrics в sql_lint_test.go);
//   - люди — peopleWhereCTE (склейка после предагрегации, как у аудитории): строки людей несут значения своих
//     событий (имя, корзину или значение свойства), ARRAY JOIN раскладывает их по значениям, uniqExact(person_key)
//     считает людей значения; unit=installs считает установки;
//   - compare читает оба периода одним проходом, граница — {split_utc} (как qAudiencePeople).
var (
	// qEventsList — имена своих событий с событиями текущего периода, events DESC, name ASC; {limit} — на одно
	// больше EventsListMax, чтобы узнать об обрезке. Люди — по парам (имя, текущий период) строк people.
	qEventsList = `WITH ` + identity.LinksCTE + `, ` + peopleWhereCTE(customEventsWhere,
		uniqValues("(e.name, e.event_time >= {split_utc:DateTime64(3)})", "names")) + `,
by_people AS (
    SELECT tupleElement(n, 1) AS name,
           uniqExactIf(person_key, tupleElement(n, 2)) AS people,
           uniqExactIf(person_key, NOT tupleElement(n, 2)) AS prev_people
    FROM people ARRAY JOIN names AS n
    GROUP BY name
),
by_events AS (
    SELECT e.name AS name,
           uniqExactIf(e.insert_id, e.event_time >= {split_utc:DateTime64(3)}) AS events,
           uniqExactIf(e.insert_id, e.event_time < {split_utc:DateTime64(3)}) AS prev_events
    FROM events AS e
    WHERE ` + eventsFilter + customEventsWhere + `
    GROUP BY name
)
SELECT v.name AS name, v.events AS events, p.people AS people, v.prev_events AS prev_events, p.prev_people AS prev_people
FROM by_events AS v
INNER JOIN by_people AS p ON p.name = v.name
WHERE v.events > 0
ORDER BY events DESC, name ASC
LIMIT {limit:UInt32}`

	// qEventSummary — события и люди имени {name} текущего и предыдущего периода; части соединяются CROSS JOIN.
	qEventSummary = `WITH ` + identity.LinksCTE + `, ` + peopleWhereCTE(eventNameWhere,
		flag("e.event_time >= {split_utc:DateTime64(3)}", "cur"),
		flag("e.event_time < {split_utc:DateTime64(3)}", "prev"),
	) + `
SELECT v.events, p.people, v.prev_events, p.prev_people
FROM (
    SELECT uniqExactIf(e.insert_id, e.event_time >= {split_utc:DateTime64(3)}) AS events,
           uniqExactIf(e.insert_id, e.event_time < {split_utc:DateTime64(3)}) AS prev_events
    FROM events AS e
    WHERE ` + eventsFilter + eventNameWhere + `
) AS v
CROSS JOIN (
    SELECT uniqExactIf(person_key, cur) AS people, uniqExactIf(person_key, prev) AS prev_people
    FROM people
) AS p`

	// qEventPropertyKeys — ключи свойств событий {name} периода с числом событий, где ключ есть: events DESC,
	// key ASC, не больше EventPropertyKeysMax.
	qEventPropertyKeys = `SELECT k AS property_key, uniqExact(e.insert_id) AS events
FROM events AS e
ARRAY JOIN mapKeys(e.properties) AS k
WHERE ` + eventsFilter + eventNameWhere + `
GROUP BY property_key
ORDER BY events DESC, property_key ASC
LIMIT ` + strconv.Itoa(EventPropertyKeysMax)

	// qEventBreakdown — события {name} по значению свойства {property}: строки kind = value (топ {limit} по
	// событиям, при равенстве — по значению), other (остальные значения одной строкой) и missing (ключа нет,
	// mapContains = 0). Пара (есть ключ, значение) отличает пустое значение от отсутствия ключа. События строки
	// other — сумма событий её значений: у события одно значение ключа, множества не пересекаются. Люди other —
	// uniqExactMerge состояний её значений, то есть объединение, а не сумма.
	qEventBreakdown = `WITH ` + identity.LinksCTE + `, ` + peopleWhereCTE(eventNameWhere,
		uniqValues("(mapContains(e.properties, {property:String}), e.properties[{property:String}])", "vals")) + `,
by_people AS (
    SELECT tupleElement(v, 1) AS has, tupleElement(v, 2) AS value, uniqExactState(person_key) AS people_state
    FROM people ARRAY JOIN vals AS v
    GROUP BY has, value
),
by_events AS (
    SELECT mapContains(e.properties, {property:String}) AS has, e.properties[{property:String}] AS value,
           uniqExact(e.insert_id) AS events
    FROM events AS e
    WHERE ` + eventsFilter + eventNameWhere + `
    GROUP BY has, value
),
ranked AS (
    SELECT v.has AS has, v.value AS value, v.events AS value_events, p.people_state AS people_state,
           row_number() OVER (PARTITION BY v.has ORDER BY v.events DESC, v.value ASC) AS pos
    FROM by_events AS v
    INNER JOIN by_people AS p ON p.has = v.has AND p.value = v.value
)
SELECT multiIf(NOT has, 'missing', pos <= {limit:UInt32}, 'value', 'other') AS kind,
       if(kind = 'value', value, '') AS label,
       sum(value_events) AS events, uniqExactMerge(people_state) AS people
FROM ranked
GROUP BY kind, label
ORDER BY events DESC, label ASC`
)

// eventTimeseriesSQL — события и люди имени {name} по корзинам g (ts DateTime, events, people); только корзины
// с событиями, остальные дополняет eventSeries.
func eventTimeseriesSQL(g Granularity) (string, error) {
	tmpl, ok := bucketTemplates[g]
	if !ok {
		return "", fmt.Errorf("%w: granularity", ErrUnsupported)
	}
	bucket := strings.ReplaceAll(tmpl, "$COL", "e.event_time")
	return `WITH ` + identity.LinksCTE + `, ` + peopleWhereCTE(eventNameWhere, uniqValues(bucket, "buckets")) + `
SELECT v.ts AS ts, v.events AS events, p.people AS people
FROM (
    SELECT ` + bucket + ` AS ts, uniqExact(e.insert_id) AS events
    FROM events AS e
    WHERE ` + eventsFilter + eventNameWhere + `
    GROUP BY ts
) AS v
INNER JOIN (
    SELECT ts, uniqExact(person_key) AS people
    FROM people ARRAY JOIN buckets AS ts
    GROUP BY ts
) AS p ON p.ts = v.ts`, nil
}

// EventList — listEvents: свои события периода по именам, events DESC, name ASC; Truncated — имён было больше
// EventsListMax.
type EventList struct {
	Events    []EventStat `json:"events"`
	Truncated bool        `json:"truncated"`
}

// EventStat — события и люди имени за период; Prev* — за предыдущий период той же длины (только compare).
type EventStat struct {
	Name       string  `json:"name"`
	Events     uint64  `json:"events"`
	People     uint64  `json:"people"`
	PrevEvents *uint64 `json:"prev_events,omitempty"`
	PrevPeople *uint64 `json:"prev_people,omitempty"`
}

// EventReportQuery — параметры getEventReport.
type EventReportQuery struct {
	Name        string
	Granularity Granularity
	Compare     bool // compare=previous_period: prev-поля сводки
}

// EventReport — getEventReport: сводка, ряд по корзинам периода (пустые — нули) и ключи свойств периода.
type EventReport struct {
	Name         string             `json:"name"`
	Summary      EventSummary       `json:"summary"`
	Timeseries   []EventPoint       `json:"timeseries"`
	PropertyKeys []EventPropertyKey `json:"property_keys"`
}

// EventSummary — события, люди и событий на человека (2 знака, 0 без людей); Prev* — за предыдущий период той же
// длины (только compare).
type EventSummary struct {
	Events              uint64   `json:"events"`
	People              uint64   `json:"people"`
	EventsPerPerson     float64  `json:"events_per_person"`
	PrevEvents          *uint64  `json:"prev_events,omitempty"`
	PrevPeople          *uint64  `json:"prev_people,omitempty"`
	PrevEventsPerPerson *float64 `json:"prev_events_per_person,omitempty"`
}

// EventPoint — корзина, начинающаяся в Bucket (в TZ приложения).
type EventPoint struct {
	Bucket time.Time `json:"bucket"`
	Events uint64    `json:"events"`
	People uint64    `json:"people"`
}

// EventPropertyKey — ключ свойства и число событий, где он есть.
type EventPropertyKey struct {
	Key    string `json:"key"`
	Events uint64 `json:"events"`
}

// EventBreakdownQuery — параметры getEventBreakdown; Limit ≤ 0 — DefaultEventBreakdownLimit, больше
// MaxEventBreakdownLimit — ErrUnsupported.
type EventBreakdownQuery struct {
	Name     string
	Property string
	Limit    int
}

// EventBreakdown — события по значениям свойства Property: Values — топ Limit по событиям, Other — остальные
// значения (nil, если значений не больше Limit), Missing — события без ключа.
type EventBreakdown struct {
	Property string            `json:"property"`
	Values   []EventValueCount `json:"values"`
	Other    *EventCount       `json:"other,omitempty"`
	Missing  EventCount        `json:"missing"`
}

// EventValueCount — значение свойства, его события и люди.
type EventValueCount struct {
	Value  string `json:"value"`
	Events uint64 `json:"events"`
	People uint64 `json:"people"`
}

// EventCount — события и люди строки разбивки.
type EventCount struct {
	Events uint64 `json:"events"`
	People uint64 `json:"people"`
}

// EventsList — свои события периода p по именам; compare — и за предыдущий период той же длины. Имена без событий
// в текущем периоде в список не попадают.
func (s *Service) EventsList(ctx context.Context, p Params, compare bool) (EventList, error) {
	defer s.track(ReportEventsList)()
	scan, extra := compareScan(p, compare)
	extra["limit"] = strconv.Itoa(EventsListMax + 1)
	rows, err := s.conn.Query(scan.bindWith(ctx, extra), qEventsList)
	if err != nil {
		return EventList{}, fmt.Errorf("reports: events list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := EventList{Events: []EventStat{}}
	for rows.Next() {
		var (
			st                     EventStat
			prevEvents, prevPeople uint64
		)
		if err := rows.Scan(&st.Name, &st.Events, &st.People, &prevEvents, &prevPeople); err != nil {
			return EventList{}, fmt.Errorf("reports: events list: %w", err)
		}
		if compare {
			st.PrevEvents, st.PrevPeople = &prevEvents, &prevPeople
		}
		out.Events = append(out.Events, st)
	}
	if err := rows.Err(); err != nil {
		return EventList{}, fmt.Errorf("reports: events list: %w", err)
	}
	if len(out.Events) > EventsListMax {
		out.Events, out.Truncated = out.Events[:EventsListMax], true
	}
	return out, nil
}

// EventReport — страница события q.Name за период p: сводка (compare — и предыдущий период), ряд по корзинам
// q.Granularity и ключи свойств; части считаются параллельно. Системное имя — ErrSystemEvent, событий нет — нули.
func (s *Service) EventReport(ctx context.Context, p Params, q EventReportQuery) (EventReport, error) {
	defer s.track(ReportEventReport)()
	if isSystemEvent(q.Name) {
		return EventReport{}, ErrSystemEvent
	}
	seriesSQL, err := eventTimeseriesSQL(q.Granularity)
	if err != nil {
		return EventReport{}, err
	}
	out := EventReport{Name: q.Name}
	err = parallel(ctx,
		func(ctx context.Context) (err error) {
			out.Summary, err = s.eventSummary(ctx, p, q.Name, q.Compare)
			return err
		},
		func(ctx context.Context) (err error) {
			out.Timeseries, err = s.eventSeries(ctx, p, q, seriesSQL)
			return err
		},
		func(ctx context.Context) (err error) {
			out.PropertyKeys, err = s.eventPropertyKeys(ctx, p, q.Name)
			return err
		},
	)
	if err != nil {
		return EventReport{}, err
	}
	return out, nil
}

func (s *Service) eventSummary(ctx context.Context, p Params, name string, compare bool) (EventSummary, error) {
	scan, extra := compareScan(p, compare)
	extra["name"] = chstore.StringParam(name)
	var (
		out                    EventSummary
		prevEvents, prevPeople uint64
	)
	err := s.conn.QueryRow(scan.bindWith(ctx, extra), qEventSummary).Scan(&out.Events, &out.People, &prevEvents, &prevPeople)
	if err != nil {
		return EventSummary{}, fmt.Errorf("reports: event summary: %w", err)
	}
	out.EventsPerPerson = eventsPerPerson(out.Events, out.People)
	if compare {
		prevPerPerson := eventsPerPerson(prevEvents, prevPeople)
		out.PrevEvents, out.PrevPeople, out.PrevEventsPerPerson = &prevEvents, &prevPeople, &prevPerPerson
	}
	return out, nil
}

// eventSeries — точки ряда query за период p: по корзине на каждое начало из bucketStarts, пустые — нули.
func (s *Service) eventSeries(ctx context.Context, p Params, q EventReportQuery, query string) ([]EventPoint, error) {
	rows, err := s.conn.Query(p.bindWith(ctx, clickhouse.Parameters{"name": chstore.StringParam(q.Name)}), query)
	if err != nil {
		return nil, fmt.Errorf("reports: event timeseries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	got := map[int64]EventPoint{}
	for rows.Next() {
		var (
			ts time.Time
			pt EventPoint
		)
		if err := rows.Scan(&ts, &pt.Events, &pt.People); err != nil {
			return nil, fmt.Errorf("reports: event timeseries: %w", err)
		}
		got[ts.Unix()] = pt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reports: event timeseries: %w", err)
	}
	starts := bucketStarts(p.Period, p.Location, q.Granularity)
	out := make([]EventPoint, 0, len(starts))
	for _, ts := range starts {
		pt := got[ts.Unix()]
		pt.Bucket = ts
		out = append(out, pt)
	}
	return out, nil
}

func (s *Service) eventPropertyKeys(ctx context.Context, p Params, name string) ([]EventPropertyKey, error) {
	rows, err := s.conn.Query(p.bindWith(ctx, clickhouse.Parameters{"name": chstore.StringParam(name)}), qEventPropertyKeys)
	if err != nil {
		return nil, fmt.Errorf("reports: event property keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []EventPropertyKey{}
	for rows.Next() {
		var k EventPropertyKey
		if err := rows.Scan(&k.Key, &k.Events); err != nil {
			return nil, fmt.Errorf("reports: event property keys: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reports: event property keys: %w", err)
	}
	return out, nil
}

// EventBreakdown — события q.Name за период p по значениям свойства q.Property: топ q.Limit, остальные и без
// ключа. Системное имя — ErrSystemEvent.
func (s *Service) EventBreakdown(ctx context.Context, p Params, q EventBreakdownQuery) (EventBreakdown, error) {
	defer s.track(ReportEventBreakdown)()
	if isSystemEvent(q.Name) {
		return EventBreakdown{}, ErrSystemEvent
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultEventBreakdownLimit
	}
	if limit > MaxEventBreakdownLimit {
		return EventBreakdown{}, fmt.Errorf("%w: limit", ErrUnsupported)
	}
	rows, err := s.conn.Query(p.bindWith(ctx, clickhouse.Parameters{
		"name":     chstore.StringParam(q.Name),
		"property": chstore.StringParam(q.Property),
		"limit":    strconv.Itoa(limit),
	}), qEventBreakdown)
	if err != nil {
		return EventBreakdown{}, fmt.Errorf("reports: event breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := EventBreakdown{Property: q.Property, Values: []EventValueCount{}}
	for rows.Next() {
		var (
			kind, label string
			c           EventCount
		)
		if err := rows.Scan(&kind, &label, &c.Events, &c.People); err != nil {
			return EventBreakdown{}, fmt.Errorf("reports: event breakdown: %w", err)
		}
		switch kind {
		case "value":
			out.Values = append(out.Values, EventValueCount{Value: label, Events: c.Events, People: c.People})
		case "other":
			out.Other = &c
		default: // missing
			out.Missing = c
		}
	}
	if err := rows.Err(); err != nil {
		return EventBreakdown{}, fmt.Errorf("reports: event breakdown: %w", err)
	}
	return out, nil
}

// compareScan — период прохода и {split_utc} для p: граница — начало p, при compare проход начинается с
// предыдущего периода той же длины, без него предыдущий период пуст.
func compareScan(p Params, compare bool) (Params, clickhouse.Parameters) {
	split, _ := p.Period.Bounds(p.location())
	scan := p
	if compare {
		scan.Period = Period{From: p.Period.Previous().From, To: p.Period.To}
	}
	return scan, clickhouse.Parameters{"split_utc": split.Format(paramDateTime)}
}

// eventsPerPerson — events / people, округлённое до 2 знаков; 0, если людей нет.
func eventsPerPerson(events, people uint64) float64 {
	if people == 0 {
		return 0
	}
	return math.Round(float64(events)/float64(people)*100) / 100
}
