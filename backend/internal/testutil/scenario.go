package testutil

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // TZ приложений сценария не зависят от tzdata системы

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
)

// ScenarioSeed — seed, с которым сгенерирован закоммиченный testdata/fixtures/scenario.ndjson.
const ScenarioSeed uint64 = 42

// ScenarioApp — приложение сценария golden-тестов отчётов.
type ScenarioApp struct {
	Code       string // короткий код для таблиц README: msk, nyc, tyo
	ID         uuid.UUID
	Name       string
	Platform   string
	Timezone   string
	WriteKeyID uuid.UUID
}

// ScenarioApps — три приложения сценария (testdata/fixtures/README.md).
var ScenarioApps = []ScenarioApp{
	{
		Code: "msk", ID: uuid.MustParse("5ce0a000-0000-4000-8000-000000000001"), Name: "Заметки iOS",
		Platform: "ios", Timezone: "Europe/Moscow", WriteKeyID: uuid.MustParse("5ce0b000-0000-4000-8000-000000000001"),
	},
	{
		Code: "nyc", ID: uuid.MustParse("5ce0a000-0000-4000-8000-000000000002"), Name: "Трекер Android",
		Platform: "android", Timezone: "America/New_York", WriteKeyID: uuid.MustParse("5ce0b000-0000-4000-8000-000000000002"),
	},
	{
		Code: "tyo", ID: uuid.MustParse("5ce0a000-0000-4000-8000-000000000003"), Name: "Сайт",
		Platform: "web", Timezone: "Asia/Tokyo", WriteKeyID: uuid.MustParse("5ce0b000-0000-4000-8000-000000000003"),
	},
}

// ScenarioFirstDay и ScenarioLastDay — границы сценария: локальные даты приложений (полночь UTC), включительно.
var (
	ScenarioFirstDay = time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	ScenarioLastDay  = time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
)

// ScenarioPath — backend/testdata/fixtures/scenario.ndjson.
func ScenarioPath() string { return filepath.Join(FixturesDir(), "scenario.ndjson") }

// ScenarioInstallID — install_id установки n (с 0) приложения app: 5ce1000A-0000-4000-8000-0000000000NN,
// где A — номер приложения из последней цифры app_id, NN — n.
func ScenarioInstallID(app ScenarioApp, n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("5ce1000%d-0000-4000-8000-0000000000%02d", app.ID[15], n))
}

const day = 24 * time.Hour

// scenarioDay — индекс локального дня сценария для даты 2026 года.
func scenarioDay(m time.Month, d int) int {
	return int(time.Date(2026, m, d, 0, 0, 0, 0, time.UTC).Sub(ScenarioFirstDay) / day)
}

func scenarioDays() int { return int(ScenarioLastDay.Sub(ScenarioFirstDay)/day) + 1 }

// scenarioRand — детерминированный источник: только PCG.Uint64, чей алгоритм зафиксирован в math/rand/v2,
// поэтому файл не меняется между версиями Go.
type scenarioRand struct{ src *rand.PCG }

func newScenarioRand(seed, stream uint64) *scenarioRand {
	return &scenarioRand{src: rand.NewPCG(seed, stream)}
}

func (r *scenarioRand) intn(n int) int { return int(r.src.Uint64() % uint64(n)) } //nolint:gosec // n > 0, малые значения

func (r *scenarioRand) pct(p int) bool { return r.intn(100) < p }

func (r *scenarioRand) ms(n int) time.Duration { return time.Duration(r.intn(n)) * time.Millisecond }

func (r *scenarioRand) uuid() uuid.UUID {
	var id uuid.UUID
	binary.BigEndian.PutUint64(id[:8], r.src.Uint64())
	binary.BigEndian.PutUint64(id[8:], r.src.Uint64())
	id[6] = id[6]&0x0f | 0x40 // версия 4
	id[8] = id[8]&0x3f | 0x80 // вариант RFC 4122
	return id
}

// Описание устройств и версий приложения.
type scenarioDevice struct{ osName, osVersion, model, kind string }

type scenarioGeo struct{ country, region, city string }

type scenarioProfile struct {
	devices  []scenarioDevice
	geos     []scenarioGeo
	locale   string
	versions [2][2]string // {версия, сборка}: старая и вышедшая в день release
	release  int
}

var scenarioProfiles = map[string]scenarioProfile{
	"msk": {
		devices: []scenarioDevice{
			{"iOS", "26.1", "iPhone17,1", "phone"}, {"iOS", "18.6", "iPhone16,2", "phone"}, {"iOS", "26.0", "iPad14,1", "tablet"},
		},
		geos: []scenarioGeo{
			{"RU", "Moscow", "Moscow"}, {"RU", "Saint Petersburg", "Saint Petersburg"}, {"KZ", "Almaty", "Almaty"}, {"", "", ""},
		},
		locale: "ru-RU", versions: [2][2]string{{"3.4.0", "340"}, {"3.5.0", "350"}}, release: scenarioDay(time.October, 15),
	},
	"nyc": {
		devices: []scenarioDevice{
			{"Android", "16", "Pixel 9", "phone"}, {"Android", "15", "SM-S921B", "phone"}, {"Android", "16", "SM-X710", "tablet"},
		},
		geos: []scenarioGeo{
			{"US", "New York", "New York"}, {"US", "California", "Los Angeles"}, {"CA", "Ontario", "Toronto"},
		},
		locale: "en-US", versions: [2][2]string{{"2.1.0", "2100"}, {"2.2.0", "2200"}}, release: scenarioDay(time.October, 20),
	},
	"tyo": {
		devices: []scenarioDevice{
			{"macOS", "15.6", "", "desktop"}, {"Windows", "11", "", "desktop"}, {"iOS", "26.1", "", "phone"}, {"Android", "16", "", "phone"},
		},
		geos: []scenarioGeo{
			{"JP", "Tokyo", "Tokyo"}, {"JP", "Osaka", "Osaka"}, {"KR", "Seoul", "Seoul"},
		},
		locale: "ja-JP", versions: [2][2]string{{"2026.10.1", ""}, {"2026.10.2", ""}}, release: scenarioDay(time.October, 10),
	},
}

// scenarioSession — одна сессия: старт, длительность, наличие session_end и особенности доставки.
type scenarioSession struct {
	start  time.Time
	dur    time.Duration
	ended  bool
	delay  time.Duration // досыл: батч отправлен позже на delay
	copies int           // сколько раз батч дошёл до сервера (повторы клиента)
}

// scenarioPlan — сценарий установки. Регулярные установки получают first и ret из seed.
type scenarioPlan struct {
	env    string
	first  int                  // день первой сессии
	ret    int                  // вероятность (%) активности в каждый следующий день
	forced []int                // дни, когда установка активна всегда
	user   func(day int) string // user_id в день; nil — аноним
	extra  []scenarioSession    // дополнительные сессии с фиксированным временем
	late   int                  // день, сессии которого досланы через 6 дней (0 — нет)
	dup    int                  // день, батч первой сессии которого отправлен трижды (0 — нет)
	random bool                 // first и ret выбираются из seed
}

func userAlways(u string) func(int) string { return func(int) string { return u } }

func userFrom(from int, u string) func(int) string {
	return func(d int) string {
		if d < from {
			return ""
		}
		return u
	}
}

func daysRange(from, to int) []int {
	var ds []int
	for d := from; d <= to; d++ {
		ds = append(ds, d)
	}
	return ds
}

// localAt — момент локального времени дня сценария в loc.
func localAt(loc *time.Location, d, h, m, s int) time.Time {
	date := ScenarioFirstDay.AddDate(0, 0, d)
	return time.Date(date.Year(), date.Month(), date.Day(), h, m, s, 0, loc).UTC()
}

func scenarioPlans(code string, loc *time.Location) []scenarioPlan {
	sep, oct, nov := time.September, time.October, time.November
	regular := func(env string) scenarioPlan { return scenarioPlan{env: env, random: true} }
	const prod, dev = "production", "development"
	switch code {
	case "msk":
		plans := []scenarioPlan{
			{ // общий девайс: аноним → A → аноним → B → аноним → B
				env: prod, first: scenarioDay(oct, 1), ret: 50, forced: daysRange(scenarioDay(oct, 1), scenarioDay(oct, 12)),
				user: func(d int) string {
					switch {
					case d < scenarioDay(oct, 4), d == scenarioDay(oct, 7), d == scenarioDay(oct, 11):
						return ""
					case d <= scenarioDay(oct, 6):
						return "u_01"
					default:
						return "u_02"
					}
				},
			},
			{env: prod, first: scenarioDay(oct, 5), ret: 70, user: userAlways("u_01")}, // второе устройство u_01
			{ // три анонимных дня, затем вход u_03
				env: prod, first: scenarioDay(oct, 2), ret: 60, forced: daysRange(scenarioDay(oct, 2), scenarioDay(oct, 6)),
				user: userFrom(scenarioDay(oct, 5), "u_03"),
			},
			{ // досыл: сессии 2026-10-06 пришли через 6 дней
				env: prod, first: scenarioDay(sep, 30), ret: 60, forced: []int{scenarioDay(oct, 6)},
				user: userAlways("u_04"), late: scenarioDay(oct, 6),
			},
			{ // сессия через полночь 10-10 23:50 → 10-11 00:25
				env: prod, first: scenarioDay(oct, 8), ret: 50, user: userAlways("u_05"),
				extra: []scenarioSession{{start: localAt(loc, scenarioDay(oct, 10), 23, 50, 12), dur: 35*time.Minute + 28*time.Second}},
			},
			{ // повтор батча: первая сессия 2026-10-12 дошла трижды
				env: prod, first: scenarioDay(oct, 3), ret: 60, forced: []int{scenarioDay(oct, 12)}, dup: scenarioDay(oct, 12),
			},
		}
		for range 10 {
			plans = append(plans, regular(prod))
		}
		return append(plans,
			scenarioPlan{env: dev, first: scenarioDay(oct, 1), ret: 70, forced: []int{scenarioDay(oct, 13)}, user: userAlways("u_01")},
			scenarioPlan{env: dev, first: scenarioDay(oct, 6), ret: 40, forced: []int{scenarioDay(oct, 13)}},
		)
	case "nyc":
		plans := []scenarioPlan{
			{env: prod, first: 0, ret: 80, user: userAlways("u_06")},
			{env: prod, first: scenarioDay(oct, 2), ret: 50, user: userAlways("u_07")},
			{env: prod, first: scenarioDay(oct, 9), ret: 50, user: userAlways("u_08")},
			{
				env: prod, first: scenarioDay(oct, 14), ret: 50, forced: daysRange(scenarioDay(oct, 14), scenarioDay(oct, 17)),
				user: userFrom(scenarioDay(oct, 16), "u_09"),
			},
			{ // 2026-11-01 01:30 EDT — до перевода часов
				env: prod, first: scenarioDay(oct, 20), ret: 40,
				extra: []scenarioSession{{start: time.Date(2026, nov, 1, 5, 30, 41, 0, time.UTC), dur: 9*time.Minute + 3*time.Second}},
			},
			{ // 2026-11-01 01:31 EST — тот же локальный час после перевода
				env: prod, first: scenarioDay(oct, 26), ret: 40,
				extra: []scenarioSession{{start: time.Date(2026, nov, 1, 6, 31, 7, 0, time.UTC), dur: 12*time.Minute + 15*time.Second}},
			},
			{ // через полночь 10-31 23:40 → 11-01 00:20
				env: prod, first: scenarioDay(oct, 25), ret: 45,
				extra: []scenarioSession{{start: localAt(loc, scenarioDay(oct, 31), 23, 40, 5), dur: 40*time.Minute + 9*time.Second}},
			},
		}
		for range 4 {
			plans = append(plans, regular(prod))
		}
		return append(plans, scenarioPlan{env: dev, first: scenarioDay(oct, 5), ret: 50})
	case "tyo":
		plans := []scenarioPlan{
			{env: prod, first: scenarioDay(sep, 29), ret: 70, user: userAlways("u_10")},
			{env: prod, first: scenarioDay(oct, 5), ret: 50, user: userAlways("u_11")},
			{env: prod, first: scenarioDay(oct, 11), ret: 60, user: userAlways("u_12")},
			{ // через полночь 10-07 23:55 → 10-08 00:10
				env: prod, first: scenarioDay(oct, 4), ret: 50,
				extra: []scenarioSession{{start: localAt(loc, scenarioDay(oct, 7), 23, 55, 30), dur: 14*time.Minute + 50*time.Second}},
			},
		}
		for range 5 {
			plans = append(plans, regular(prod))
		}
		return append(plans, scenarioPlan{env: dev, first: scenarioDay(oct, 2), ret: 30})
	default:
		return nil
	}
}

// Окна регулярных сессий (локальные часы): утро, день, вечер — сессии одного дня не пересекаются.
var scenarioSlots = [3]int{8, 13, 18}

const scenarioSlotMinutes = 4 * 60

// GenerateScenario строит события сценария для seed. Структура (кто, когда и какие особые случаи)
// задана планами установок, seed выбирает активность регулярных дней, время, длительности и ID.
func GenerateScenario(seed uint64) []chstore.EventRow {
	var rows []chstore.EventRow
	for ai, app := range ScenarioApps {
		loc, err := time.LoadLocation(app.Timezone)
		if err != nil {
			panic(fmt.Sprintf("testutil: TZ %s: %v", app.Timezone, err))
		}
		profile := scenarioProfiles[app.Code]
		for ii, plan := range scenarioPlans(app.Code, loc) {
			r := newScenarioRand(seed, uint64(ai)<<16|uint64(ii)) //nolint:gosec // индексы малы
			rows = append(rows, generateInstall(r, app, profile, ScenarioInstallID(app, ii), plan, loc)...)
		}
	}
	slices.SortStableFunc(rows, func(a, b chstore.EventRow) int {
		return cmp.Or(
			a.ReceivedAt.Compare(b.ReceivedAt),
			bytes.Compare(a.AppID[:], b.AppID[:]),
			bytes.Compare(a.InstallID[:], b.InstallID[:]),
			a.EventTime.Compare(b.EventTime),
			strings.Compare(b.Name, a.Name), // session_start раньше session_end
		)
	})
	return rows
}

func generateInstall(
	r *scenarioRand, app ScenarioApp, profile scenarioProfile, installID uuid.UUID, plan scenarioPlan, loc *time.Location,
) []chstore.EventRow {
	if plan.random {
		plan.first = r.intn(29)
		plan.ret = [...]int{80, 55, 30, 10}[r.intn(4)]
	}
	device := profile.devices[r.intn(len(profile.devices))]
	geo := profile.geos[r.intn(len(profile.geos))]
	upgradeLag := r.intn(5)

	var sessions []scenarioSession
	for d := plan.first; d < scenarioDays(); d++ {
		if d != plan.first && !slices.Contains(plan.forced, d) && !r.pct(plan.ret) {
			continue
		}
		daySessions := regularSessions(r, loc, d)
		for i := range daySessions {
			if plan.late != 0 && d == plan.late {
				daySessions[i].delay = 6*day + 2*time.Second
			}
		}
		if plan.dup != 0 && d == plan.dup {
			daySessions[0].copies = 3
		}
		sessions = append(sessions, daySessions...)
	}
	for _, s := range plan.extra {
		s.ended, s.copies = true, 1
		sessions = append(sessions, s)
	}

	var rows []chstore.EventRow
	for _, s := range sessions {
		local := s.start.In(loc)
		d := int(time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC).Sub(ScenarioFirstDay) / day)
		user := ""
		if plan.user != nil {
			user = plan.user(d)
		}
		version := profile.versions[0]
		if d >= profile.release+upgradeLag {
			version = profile.versions[1]
		}
		base := chstore.EventRow{
			AppID:       app.ID,
			InstallID:   installID,
			UserID:      user,
			SessionID:   r.uuid(),
			Environment: plan.env,
			Platform:    app.Platform,
			OSName:      device.osName,
			OSVersion:   device.osVersion,
			AppVersion:  version[0],
			AppBuild:    version[1],
			DeviceModel: device.model,
			DeviceType:  device.kind,
			Locale:      profile.locale,
			Country:     geo.country,
			Region:      geo.region,
			City:        geo.city,
			WriteKeyID:  app.WriteKeyID,
		}
		events := []chstore.EventRow{scenarioEvent(r, base, "session_start", s.start, 0, s.delay)}
		if s.ended {
			events = append(events, scenarioEvent(r, base, "session_end", s.start.Add(s.dur), s.dur, s.delay))
		}
		for _, e := range events {
			rows = append(rows, e)
			for k := 1; k < s.copies; k++ {
				retry := e
				retry.ReceivedAt = e.ReceivedAt.Add(time.Duration(k)*4*time.Second + r.ms(2000))
				rows = append(rows, retry)
			}
		}
	}
	return rows
}

// regularSessions — 1–3 сессии дня d в разных окнах scenarioSlots.
func regularSessions(r *scenarioRand, loc *time.Location, d int) []scenarioSession {
	n := 1
	if r.pct(35) {
		n++
		if r.pct(25) {
			n++
		}
	}
	slots := []int{0, 1, 2}
	for i := len(slots) - 1; i > 0; i-- {
		j := r.intn(i + 1)
		slots[i], slots[j] = slots[j], slots[i]
	}
	slots = slots[:n]
	slices.Sort(slots)

	out := make([]scenarioSession, 0, n)
	for _, slot := range slots {
		minute := r.intn(scenarioSlotMinutes - 40)
		start := localAt(loc, d, scenarioSlots[slot]+minute/60, minute%60, r.intn(60)).Add(r.ms(1000))
		dur := 20*time.Second + time.Duration(r.intn(25*60))*time.Second + r.ms(1000)
		out = append(out, scenarioSession{start: start, dur: dur, ended: r.pct(88), copies: 1})
	}
	return out
}

// scenarioEvent — событие сессии: клиент отправляет батч через 0.3–1 с (плюс задержка досыла),
// сервер принимает через 80–380 мс; часы устройства верные, поэтому client_time = event_time.
func scenarioEvent(r *scenarioRand, base chstore.EventRow, name string, at time.Time, dur, delay time.Duration) chstore.EventRow {
	e := base
	e.Name = name
	e.InsertID = r.uuid()
	e.EventTime = at
	e.EventDate = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	e.ClientTime = at
	e.SentAt = at.Add(delay + 300*time.Millisecond + r.ms(700))
	e.ReceivedAt = e.SentAt.Add(80*time.Millisecond + r.ms(300))
	e.DurationMs = uint32(dur.Milliseconds()) //nolint:gosec // длительность сессии < 1 ч
	return e
}

// WriteScenario пишет сценарий для seed в формате NDJSON-фикстуры.
func WriteScenario(w io.Writer, seed uint64) error {
	header := fmt.Sprintf("# Сценарий golden-тестов отчётов (T-026): описание и ожидаемые числа — README.md.\n"+
		"# Сгенерировано: cd backend && go run ./testdata/gen -seed %d. Руками не редактировать.\n", seed)
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("testutil: сценарий: %w", err)
	}
	return WriteNDJSON(w, GenerateScenario(seed))
}

// ScenarioDay — ожидаемые метрики приложения за локальный день (production, unit=persons).
type ScenarioDay struct {
	Day        string // YYYY-MM-DD в TZ приложения
	Active     int
	New        int
	D1Returned int // сколько из New активны на следующий день
}

// ScenarioStats считает DAU, новых и D1 по дням окна сценария для app — эталонная модель определений
// 05-data-model.md §2.8 и §10.4: person_key через первого владельца установки (identity_links без учёта
// environment), first_seen человека — минимум по его установкам и явным событиям в production.
func ScenarioStats(rows []chstore.EventRow, app ScenarioApp) ([]ScenarioDay, error) {
	loc, err := time.LoadLocation(app.Timezone)
	if err != nil {
		return nil, fmt.Errorf("testutil: TZ %s: %w", app.Timezone, err)
	}
	type link struct {
		at   time.Time
		user string
	}
	links := map[uuid.UUID]link{}
	for _, r := range rows {
		if r.AppID != app.ID || r.UserID == "" {
			continue
		}
		if l, ok := links[r.InstallID]; !ok || r.EventTime.Before(l.at) {
			links[r.InstallID] = link{r.EventTime, r.UserID}
		}
	}

	localDay := func(t time.Time) string { return t.In(loc).Format(time.DateOnly) }
	first := map[string]time.Time{}
	seen := func(key string, t time.Time) {
		if f, ok := first[key]; !ok || t.Before(f) {
			first[key] = t
		}
	}
	active := map[string]map[string]bool{}
	for _, r := range rows {
		if r.AppID != app.ID || r.Environment != "production" {
			continue
		}
		installKey := "i:" + r.InstallID.String()
		if l, ok := links[r.InstallID]; ok {
			installKey = "u:" + l.user
		}
		key := installKey
		if r.UserID != "" {
			key = "u:" + r.UserID
			seen(key, r.EventTime)
		}
		seen(installKey, r.EventTime)
		d := localDay(r.EventTime)
		if active[d] == nil {
			active[d] = map[string]bool{}
		}
		active[d][key] = true
	}

	out := make([]ScenarioDay, 0, scenarioDays())
	for i := range scenarioDays() {
		d := ScenarioFirstDay.AddDate(0, 0, i).Format(time.DateOnly)
		next := ScenarioFirstDay.AddDate(0, 0, i+1).Format(time.DateOnly)
		st := ScenarioDay{Day: d, Active: len(active[d])}
		for key := range active[d] {
			if localDay(first[key]) == d {
				st.New++
				if active[next][key] {
					st.D1Returned++
				}
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// ScenarioStatsTable — markdown-таблица ожидаемых чисел для README фикстур: по строке на день,
// для каждого приложения DAU, новые и D1 как «вернулись/новые».
func ScenarioStatsTable(rows []chstore.EventRow) (string, error) {
	var b strings.Builder
	b.WriteString("| День |")
	align := "|---|"
	stats := make([][]ScenarioDay, len(ScenarioApps))
	for i, app := range ScenarioApps {
		s, err := ScenarioStats(rows, app)
		if err != nil {
			return "", err
		}
		stats[i] = s
		fmt.Fprintf(&b, " %s DAU | %s новые | %s D1 |", app.Code, app.Code, app.Code)
		align += "---:|---:|---:|"
	}
	b.WriteString("\n" + align + "\n")
	for i := range scenarioDays() {
		fmt.Fprintf(&b, "| %s |", stats[0][i].Day)
		for _, s := range stats {
			d1 := "—"
			if s[i].New > 0 {
				d1 = fmt.Sprintf("%d/%d", s[i].D1Returned, s[i].New)
			}
			fmt.Fprintf(&b, " %d | %d | %s |", s[i].Active, s[i].New, d1)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
