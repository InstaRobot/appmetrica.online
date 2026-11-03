package testutil

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
)

// Фикстура своих событий отчёта «События» (FR-102, T-CE-005): события каталога внутри сессий установок
// сценария того же seed. scenario.ndjson не меняется (feature spec §3, Р-9): свои события лежат отдельным
// файлом, тесты загружают оба. Случайность — отдельные потоки PCG, поэтому поток сценария не сдвигается.

// CustomEventsPath — backend/testdata/fixtures/custom_events.ndjson.
func CustomEventsPath() string { return filepath.Join(FixturesDir(), "custom_events.ndjson") }

// customEventsStream — старшие биты номера потока PCG своих событий: потоки сценария — ai<<16|ii.
const customEventsStream uint64 = 0xce << 32

// customProperty — свойство события каталога. values nil — целое 0…40 строкой; weights nil — равные веса;
// каждое missingEvery-е событие приложения с этим именем — без свойства (0 — свойство есть всегда).
type customProperty struct {
	key          string
	values       []string
	weights      []int
	missingEvery int
}

// customEventSpec — событие каталога: вес выбора и свойство.
type customEventSpec struct {
	name   string
	weight int
	prop   customProperty
}

// noteSources — 25 значений source у note_created: при limit=20 разбивка получает строку «остальные».
var noteSources = []string{
	"keyboard", "voice", "share", "widget", "shortcut", "camera", "clipboard", "template", "import", "email",
	"web_clipper", "watch", "siri", "scan", "drag_drop", "dictation", "files", "messages", "safari", "spotlight",
	"notification", "handoff", "quick_note", "lock_screen", "api",
}

// noteSourceWeights — убывающие веса 30…6: у каждого из 25 значений есть события, топ однозначен.
func noteSourceWeights() []int {
	out := make([]int, len(noteSources))
	for i := range out {
		out[i] = 30 - i
	}
	return out
}

var (
	searchResults = customProperty{key: "results"}
	paywallPlaces = customProperty{key: "place", values: []string{"onboarding", "limit", "settings"}}
	purchasePlans = customProperty{key: "plan", values: []string{"monthly", "yearly", "lifetime"}, weights: []int{5, 3, 1}, missingEvery: 5}
)

// customEventCatalog — события приложений фикстуры (имена демо-каталога FR-102): у «Заметок» все пять, у «Трекера» —
// три. Приложение tyo своих событий не шлёт.
var customEventCatalog = map[string][]customEventSpec{
	"msk": {
		{name: "note_created", weight: 6, prop: customProperty{key: "source", values: noteSources, weights: noteSourceWeights(), missingEvery: 10}},
		{name: "search_used", weight: 3, prop: searchResults},
		{name: "note_shared", weight: 2, prop: customProperty{key: "target", values: []string{"messages", "mail", "telegram", "copy_link"}}},
		{name: "paywall_shown", weight: 2, prop: paywallPlaces},
		{name: "purchase", weight: 1, prop: purchasePlans},
	},
	"nyc": {
		{name: "search_used", weight: 4, prop: searchResults},
		{name: "paywall_shown", weight: 2, prop: paywallPlaces},
		{name: "purchase", weight: 1, prop: purchasePlans},
	},
}

// customSession — сессия сценария: первая доставка session_start, время session_end (zero — нет) и число
// доставок батча.
type customSession struct {
	start  chstore.EventRow
	end    time.Time
	copies int
}

// scenarioSessions — сессии сценария rows по приложениям в порядке (install_id, начало).
func scenarioSessions(rows []chstore.EventRow) map[uuid.UUID][]*customSession {
	byID := map[uuid.UUID]*customSession{}
	var order []*customSession
	for _, r := range rows {
		s, ok := byID[r.SessionID]
		if !ok {
			s = &customSession{}
			byID[r.SessionID] = s
			order = append(order, s)
		}
		switch r.Name {
		case "session_start":
			if s.copies == 0 {
				s.start = r
			}
			s.copies++
		case "session_end":
			if s.end.IsZero() {
				s.end = r.EventTime
			}
		}
	}
	slices.SortStableFunc(order, func(a, b *customSession) int {
		return cmp.Or(
			bytes.Compare(a.start.InstallID[:], b.start.InstallID[:]),
			a.start.EventTime.Compare(b.start.EventTime),
		)
	})
	out := map[uuid.UUID][]*customSession{}
	for _, s := range order {
		out[s.start.AppID] = append(out[s.start.AppID], s)
	}
	return out
}

// GenerateCustomEvents строит свои события для seed поверх сценария GenerateScenario(seed): в каждой сессии
// приложений каталога 0–4 события (в повторённом батче — не меньше двух), время — внутри сессии, контекст и доставка — как у её session_start
// (задержка досыла, повтор батча); ещё ≈ 4 % событий клиент отправил повторно с тем же insert_id.
func GenerateCustomEvents(seed uint64) []chstore.EventRow {
	sessions := scenarioSessions(GenerateScenario(seed))
	var rows []chstore.EventRow
	for ai, app := range ScenarioApps {
		catalog, ok := customEventCatalog[app.Code]
		if !ok {
			continue
		}
		weights := make([]int, len(catalog))
		for i, spec := range catalog {
			weights[i] = spec.weight
		}
		r := newScenarioRand(seed, customEventsStream|uint64(ai)) //nolint:gosec // индекс мал
		seen := map[string]int{}
		for _, s := range sessions[app.ID] {
			n := r.intn(5)
			if s.copies > 1 {
				n = max(n, 2) // повторённый батч несёт и свои события: они тоже дошли несколько раз
			}
			for range n {
				spec := catalog[r.weighted(weights)]
				seen[spec.name]++
				e := customEvent(r, s, spec, seen[spec.name])
				rows = append(rows, e)
				copies := s.copies
				if r.pct(4) {
					copies++
				}
				for k := 1; k < copies; k++ {
					retry := e
					retry.ReceivedAt = e.ReceivedAt.Add(time.Duration(k)*4*time.Second + r.ms(2000))
					rows = append(rows, retry)
				}
			}
		}
	}
	slices.SortStableFunc(rows, func(a, b chstore.EventRow) int {
		return cmp.Or(
			a.ReceivedAt.Compare(b.ReceivedAt),
			bytes.Compare(a.AppID[:], b.AppID[:]),
			bytes.Compare(a.InstallID[:], b.InstallID[:]),
			a.EventTime.Compare(b.EventTime),
			strings.Compare(a.Name, b.Name),
		)
	})
	return rows
}

// customEvent — n-е событие spec приложения в сессии s: момент внутри сессии (у сессии без session_end —
// в первую минуту), отправка и приём с теми же задержками, что у session_start.
func customEvent(r *scenarioRand, s *customSession, spec customEventSpec, n int) chstore.EventRow {
	start := s.start
	window := time.Minute
	if !s.end.IsZero() {
		window = s.end.Sub(start.EventTime)
	}
	at := start.EventTime.Add(time.Millisecond + r.ms(int(window.Milliseconds())-1))
	e := start
	e.Name = spec.name
	e.InsertID = r.uuid()
	e.EventTime, e.ClientTime = at, at
	e.EventDate = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	e.SentAt = at.Add(start.SentAt.Sub(start.EventTime))
	e.ReceivedAt = at.Add(start.ReceivedAt.Sub(start.EventTime))
	e.DurationMs = 0
	e.Properties = nil
	if p := spec.prop; p.missingEvery == 0 || n%p.missingEvery != 0 {
		e.Properties = map[string]string{p.key: r.propertyValue(p)}
	}
	return e
}

func (r *scenarioRand) propertyValue(p customProperty) string {
	switch {
	case p.values == nil:
		return strconv.Itoa(r.intn(41))
	case p.weights == nil:
		return p.values[r.intn(len(p.values))]
	default:
		return p.values[r.weighted(p.weights)]
	}
}

// weighted — индекс по весам weights (сумма > 0).
func (r *scenarioRand) weighted(weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	n := r.intn(total)
	for i, w := range weights {
		if n < w {
			return i
		}
		n -= w
	}
	return len(weights) - 1
}

// WriteCustomEvents пишет свои события для seed в формате NDJSON-фикстуры.
func WriteCustomEvents(w io.Writer, seed uint64) error {
	header := fmt.Sprintf("# Свои события отчёта «События» (FR-102, T-CE-005) в сессиях scenario.ndjson: описание — README.md.\n"+
		"# Сгенерировано: cd backend && go run ./testdata/gen -mode custom-events -seed %d. Руками не редактировать.\n", seed)
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("testutil: свои события: %w", err)
	}
	return WriteNDJSON(w, GenerateCustomEvents(seed))
}

// CustomEventsStatsTable — markdown-таблица README фикстуры своих событий: по строке на событие каталога
// приложения — строки (с повторами), события (уникальные insert_id), события со свойством и число его значений.
func CustomEventsStatsTable(rows []chstore.EventRow) string {
	type key struct {
		app  uuid.UUID
		name string
	}
	type stat struct {
		rows             int
		events, withProp map[uuid.UUID]bool
		values           map[string]bool
	}
	stats, propKeys := map[key]*stat{}, map[key]string{}
	for _, app := range ScenarioApps {
		for _, spec := range customEventCatalog[app.Code] {
			k := key{app.ID, spec.name}
			stats[k] = &stat{events: map[uuid.UUID]bool{}, withProp: map[uuid.UUID]bool{}, values: map[string]bool{}}
			propKeys[k] = spec.prop.key
		}
	}
	for _, r := range rows {
		k := key{r.AppID, r.Name}
		s, ok := stats[k]
		if !ok {
			continue // не из каталога: TestCustomEvents_Sujet
		}
		s.rows++
		s.events[r.InsertID] = true
		if v, ok := r.Properties[propKeys[k]]; ok {
			s.withProp[r.InsertID] = true
			s.values[v] = true
		}
	}

	var b strings.Builder
	b.WriteString("| Приложение | Событие | Строк | Событий | Свойство | Событий со свойством | Значений |\n")
	b.WriteString("|---|---|---:|---:|---|---:|---:|\n")
	for _, app := range ScenarioApps {
		for _, spec := range customEventCatalog[app.Code] {
			s := stats[key{app.ID, spec.name}]
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %s | %d | %d |\n",
				app.Code, spec.name, s.rows, len(s.events), spec.prop.key, len(s.withProp), len(s.values))
		}
	}
	return b.String()
}
