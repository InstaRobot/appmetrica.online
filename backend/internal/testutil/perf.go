package testutil

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

// Синтетика объёма для перф-теста отчётов NFR-003 (T-035, 22-test-strategy.md §6.1 и §11.2): основное
// приложение со 100k DAU × 90 дней (≈ 27 млн событий, ≈ 2 млн связок identity) и два меньших для портфеля.
// Модель установки: в день рождения активна, затем каждый день доживает с вероятностью perfSurvival и, если
// дожила, активна с вероятностью perfActive. В установившемся режиме пул живых установок — N/(1−s), DAU —
// N·(1 + s·p/(1−s)) = 4,4·N, где N — новые установки в день; пул на первый день окна заполнен заранее.

// PerfSeed — seed перф-синтетики по умолчанию (детерминированный PCG из math/rand/v2).
const PerfSeed uint64 = 42

// PerfPowerUser — пользователь основного приложения, активный каждый день окна (3–4 сессии в день) на двух
// установках: профиль с сотнями сессий для замеров карточки профиля.
const PerfPowerUser = "perf-power-user"

// PerfPowerInstall — первая установка PerfPowerUser (поиск профиля по install_id).
var PerfPowerInstall = uuid.MustParse("7e4f1000-0000-4000-8000-000000000001")

// PerfApp — приложение перф-синтетики. DAU — PerfConfig.DAU / dauDivisor. Platforms — платформы событий,
// первая из них — Platform; установка получает одну платформу по долям perfPlatformShares.
type PerfApp struct {
	Code       string
	ID         uuid.UUID
	Platform   string
	Platforms  []string
	Timezone   string
	WriteKeyID uuid.UUID
	dauDivisor int
}

// PerfApps — основное приложение (DAU из конфигурации, iOS, Android и Web) и два меньших одноплатформенных
// (DAU/10 и DAU/50) для портфеля.
var PerfApps = []PerfApp{
	{
		Code: "main", ID: uuid.MustParse("7e4f0a00-0000-4000-8000-000000000001"), Platform: "ios", Platforms: []string{"ios", "android", "web"},
		Timezone: "Europe/Moscow", WriteKeyID: uuid.MustParse("7e4f0b00-0000-4000-8000-000000000001"), dauDivisor: 1,
	},
	{
		Code: "android", ID: uuid.MustParse("7e4f0a00-0000-4000-8000-000000000002"), Platform: "android", Platforms: []string{"android"},
		Timezone: "America/New_York", WriteKeyID: uuid.MustParse("7e4f0b00-0000-4000-8000-000000000002"), dauDivisor: 10,
	},
	{
		Code: "web", ID: uuid.MustParse("7e4f0a00-0000-4000-8000-000000000003"), Platform: "web", Platforms: []string{"web"},
		Timezone: "Asia/Tokyo", WriteKeyID: uuid.MustParse("7e4f0b00-0000-4000-8000-000000000003"), dauDivisor: 50,
	},
}

// PerfConfig — параметры синтетики: DAU основного приложения, число дней окна, заканчивающегося LastDay
// (локальная дата приложений, полночь UTC).
type PerfConfig struct {
	Seed    uint64
	DAU     int
	Days    int
	LastDay time.Time
}

// Границы конфигурации: DAU/50 третьего приложения ≥ 2, окно не длиннее года.
const (
	perfMinDAU  = 100
	perfMaxDAU  = 1_000_000
	perfMaxDays = 366
)

// DefaultPerfConfig — 100k DAU × 90 дней по 2026-10-12 включительно (партиции 202607 … 202610).
func DefaultPerfConfig() PerfConfig {
	return PerfConfig{Seed: PerfSeed, DAU: 100_000, Days: 90, LastDay: time.Date(2026, time.October, 12, 0, 0, 0, 0, time.UTC)}
}

// FirstDay — первый день окна.
func (c PerfConfig) FirstDay() time.Time { return c.LastDay.AddDate(0, 0, -(c.Days - 1)) }

// Now — «сейчас» замеров: 18:00 UTC последнего дня (вечер в Москве, в окне realtime есть сессии).
func (c PerfConfig) Now() time.Time { return c.LastDay.Add(18 * time.Hour) }

// Validate проверяет границы DAU, числа дней и наличие LastDay.
func (c PerfConfig) Validate() error {
	switch {
	case c.DAU < perfMinDAU || c.DAU > perfMaxDAU:
		return fmt.Errorf("testutil: perf: dau %d вне %d…%d", c.DAU, perfMinDAU, perfMaxDAU)
	case c.Days < 1 || c.Days > perfMaxDays:
		return fmt.Errorf("testutil: perf: days %d вне 1…%d", c.Days, perfMaxDays)
	case c.LastDay.IsZero():
		return errors.New("testutil: perf: не задан последний день")
	}
	return nil
}

// PerfStats — объём сгенерированных данных. Installs — установки хотя бы с одним событием, Links — из них с
// user_id (строки identity_links), Users — разные user_id, AvgDAU — среднее число активных установок в день
// по приложениям PerfApps (все environment).
type PerfStats struct {
	Days     int
	Events   int
	Installs int
	Links    int
	Users    int
	AvgDAU   []int
}

// Параметры модели (‰ и %).
const (
	perfSurvival    = 850 // ‰: установка доживает до следующего дня
	perfActive      = 600 // ‰: дожившая установка активна в день
	perfLoginNow    = 75  // %: user_id с первой сессии
	perfLoginLater  = 12  // %: user_id со второго активного дня; остальные — анонимы
	perfSharedUser  = 10  // %: вошедший пользователь уже есть на другой установке
	perfDevelopment = 1   // %: установки environment=development
	perfEnded       = 95  // %: у сессии есть session_end
	perfLate        = 1   // %: батч сессии досылается через 1–30 ч
	perfReleaseDays = 14  // новая версия приложения раз в две недели
	perfRecentUsers = 4096
)

type perfDevice struct{ osName, osVersion, model, kind string }

type perfGeo struct{ country, region, city, locale string }

// perfPlatformShares — доли платформ мультиплатформенного приложения в десятых: 70 % у первой, 20 % у второй,
// остаток у третьей.
var perfPlatformShares = []int{7, 2, 1}

var perfDevices = map[string][]perfDevice{
	"ios": {
		{"iOS", "26.1", "iPhone17,1", "phone"},
		{"iOS", "26.0", "iPhone16,2", "phone"},
		{"iOS", "18.6", "iPhone15,3", "phone"},
		{"iOS", "18.5", "iPhone14,7", "phone"},
		{"iOS", "26.1", "iPad14,1", "tablet"},
	},
	"android": {
		{"Android", "16", "Pixel 9", "phone"},
		{"Android", "15", "SM-S921B", "phone"},
		{"Android", "15", "SM-A556B", "phone"},
		{"Android", "14", "23117RA68G", "phone"},
		{"Android", "16", "SM-X710", "tablet"},
	},
	"web": {
		{"macOS", "15.6", "", "desktop"}, {"Windows", "11", "", "desktop"}, {"iOS", "26.1", "", "phone"}, {"Android", "16", "", "phone"},
	},
}

// perfGeos — страны с весами (повторы); пустая — гео не определено.
var perfGeos = []perfGeo{
	{"RU", "Moscow", "Moscow", "ru-RU"},
	{"RU", "Moscow", "Moscow", "ru-RU"},
	{"RU", "Moscow", "Moscow", "ru-RU"},
	{"RU", "Saint Petersburg", "Saint Petersburg", "ru-RU"},
	{"RU", "Saint Petersburg", "Saint Petersburg", "ru-RU"},
	{"RU", "Novosibirsk Oblast", "Novosibirsk", "ru-RU"},
	{"KZ", "Almaty", "Almaty", "ru-KZ"},
	{"BY", "Minsk", "Minsk", "ru-BY"},
	{"UZ", "Tashkent", "Tashkent", "uz-UZ"},
	{"DE", "Berlin", "Berlin", "de-DE"},
	{"US", "New York", "New York", "en-US"},
	{"US", "California", "Los Angeles", "en-US"},
	{"JP", "Tokyo", "Tokyo", "ja-JP"},
	{"", "", "", "en-US"},
}

// Когда установка получает user_id.
const (
	perfLoginNever  = iota
	perfLoginFirst  // с первой сессии
	perfLoginSecond // со второго активного дня
)

type perfInstall struct {
	id         uuid.UUID
	user       string
	login      uint8 // perfLogin*
	activeDays uint8 // 0, 1 или 2 (насыщается)
	dev        bool
	platform   uint8 // индекс в PerfApp.Platforms
	device     uint8 // индекс в perfDevices платформы приложения, по модулю — для остальных платформ
	geo        uint8
	lag        uint8 // дней до обновления на вышедшую версию
	immortal   bool  // не уходит в отток и активна каждый день
	power      bool  // 3–4 сессии в день
}

// perfAppGen — состояние генерации одного приложения: собственный поток PCG, поэтому данные приложения не
// зависят от остальных.
type perfAppGen struct {
	app      PerfApp
	loc      *time.Location
	r        *scenarioRand
	pool     []*perfInstall
	rate     float64 // новых установок в день
	born     float64
	versions []string
	builds   []string
	recent   []string // недавние user_id для общих пользователей
	active   int      // сумма активных установок по дням
	stats    *PerfStats
}

// GeneratePerf генерирует синтетику и отдаёт её батчами: по одному на приложение и локальный день, по дням
// окна, внутри дня — в порядке PerfApps. Ошибка emit останавливает генерацию.
func GeneratePerf(cfg PerfConfig, emit func([]chstore.EventRow) error) (PerfStats, error) {
	if err := cfg.Validate(); err != nil {
		return PerfStats{}, err
	}
	stats := PerfStats{Days: cfg.Days, AvgDAU: make([]int, len(PerfApps))}
	gens := make([]*perfAppGen, len(PerfApps))
	for i, app := range PerfApps {
		g, err := newPerfAppGen(cfg, app, uint64(i), &stats) //nolint:gosec // индекс приложения ≥ 0
		if err != nil {
			return PerfStats{}, err
		}
		gens[i] = g
	}
	for d := range cfg.Days {
		for _, g := range gens {
			rows := g.day(cfg, d)
			stats.Events += len(rows)
			if err := emit(rows); err != nil {
				return PerfStats{}, fmt.Errorf("testutil: perf: %w", err)
			}
		}
	}
	for i, g := range gens {
		stats.AvgDAU[i] = int(math.Round(float64(g.active) / float64(cfg.Days)))
	}
	return stats, nil
}

func newPerfAppGen(cfg PerfConfig, app PerfApp, stream uint64, stats *PerfStats) (*perfAppGen, error) {
	loc, err := time.LoadLocation(app.Timezone)
	if err != nil {
		return nil, fmt.Errorf("testutil: perf: %w", err)
	}
	dau := float64(cfg.DAU) / float64(app.dauDivisor)
	s, p := float64(perfSurvival)/1000, float64(perfActive)/1000
	g := &perfAppGen{
		app: app, loc: loc, r: newScenarioRand(cfg.Seed, 1000+stream), stats: stats,
		rate: dau / (1 + s*p/(1-s)),
	}
	for k := 0; k <= cfg.Days/perfReleaseDays+1; k++ {
		g.versions = append(g.versions, fmt.Sprintf("%d.%d.0", 4+k/10, k%10))
		g.builds = append(g.builds, fmt.Sprintf("%d", 400+k))
	}

	// Пул установок, родившихся до окна, в установившемся размере N/(1−s).
	for range int(math.Round(g.rate / (1 - s))) {
		g.pool = append(g.pool, g.newInstall())
	}
	// Установка, активная каждый день; в основном приложении — две установки PerfPowerUser.
	keeper := g.newInstall()
	keeper.immortal, keeper.dev, keeper.user = true, false, app.Code+"-keeper"
	if stream == 0 {
		keeper.id, keeper.user, keeper.power = PerfPowerInstall, PerfPowerUser, true
		ipad := g.newInstall()
		ipad.immortal, ipad.dev, ipad.user, ipad.device = true, false, PerfPowerUser, 4 // iPad14,1
		keeper.platform, ipad.platform = 0, 0
		g.pool = append(g.pool, ipad)
	}
	g.pool = append(g.pool, keeper)
	stats.Users++
	return g, nil
}

func (g *perfAppGen) newInstall() *perfInstall {
	r := g.r
	in := &perfInstall{
		id:     r.uuid(),
		dev:    r.pct(perfDevelopment),
		device: uint8(r.intn(len(perfDevices[g.app.Platform]))), //nolint:gosec // ≤ 5 устройств
		geo:    uint8(r.intn(len(perfGeos))),                    //nolint:gosec // ≤ 14 гео
		lag:    uint8(r.intn(7)),                                //nolint:gosec // 0…6
	}
	switch login := r.intn(100); {
	case login < perfLoginNow:
		in.login = perfLoginFirst
	case login < perfLoginNow+perfLoginLater:
		in.login = perfLoginSecond
	}
	in.platform = g.platformOf(in.id)
	return in
}

// platformOf выбирает платформу установки по последнему байту install_id, не расходуя поток PCG: остальные
// поля синтетики не зависят от числа платформ.
func (g *perfAppGen) platformOf(id uuid.UUID) uint8 {
	bucket := int(id[15] % 10)
	last := len(g.app.Platforms) - 1
	for i, share := range perfPlatformShares[:last] {
		if bucket < share {
			return uint8(i) //nolint:gosec // ≤ 3 платформ
		}
		bucket -= share
	}
	return uint8(last) //nolint:gosec // ≤ 3 платформ
}

// day генерирует события локального дня d приложения: сначала дожившие установки пула, затем новые.
func (g *perfAppGen) day(cfg PerfConfig, d int) []chstore.EventRow {
	r := g.r
	local := cfg.FirstDay().AddDate(0, 0, d)
	rows := make([]chstore.EventRow, 0, int(g.rate*4.4*3.2)+16)
	alive := g.pool[:0]
	for _, in := range g.pool {
		if !in.immortal && r.intn(1000) >= perfSurvival {
			continue
		}
		alive = append(alive, in)
		if in.immortal || r.intn(1000) < perfActive {
			rows = g.activate(rows, in, local, d)
		}
	}
	clear(g.pool[len(alive):])
	g.pool = alive

	before := int(g.born)
	g.born += g.rate
	for range int(g.born) - before {
		in := g.newInstall()
		g.pool = append(g.pool, in)
		rows = g.activate(rows, in, local, d)
	}
	return rows
}

// activate — сессии установки за день: вход пользователя, версия, 1–3 сессии (power — 3–4).
func (g *perfAppGen) activate(rows []chstore.EventRow, in *perfInstall, local time.Time, d int) []chstore.EventRow {
	r := g.r
	g.active++
	if in.activeDays == 0 {
		g.stats.Installs++
		if in.user != "" {
			g.stats.Links++ // установки с заранее заданным пользователем
		}
	}
	in.activeDays = min(in.activeDays+1, 2)
	if in.user == "" && (in.login == perfLoginFirst || in.login == perfLoginSecond && in.activeDays == 2) {
		g.login(in)
	}

	n := 1
	switch k := r.intn(100); {
	case in.power:
		n = 3 + r.intn(2)
	case k >= 90:
		n = 3
	case k >= 55:
		n = 2
	}
	starts := make([]time.Duration, n)
	for i := range starts {
		starts[i] = 6*time.Hour + time.Duration(r.intn(18*3600))*time.Second + r.ms(1000)
	}
	slices.Sort(starts)

	platform := g.app.Platforms[in.platform]
	devices := perfDevices[platform]
	device := devices[int(in.device)%len(devices)]
	geo := perfGeos[in.geo]
	release := max(0, d-int(in.lag)) / perfReleaseDays
	env := "production"
	if in.dev {
		env = "development"
	}
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, g.loc)
	base := chstore.EventRow{
		AppID: g.app.ID, InstallID: in.id, UserID: in.user, Environment: env, Platform: platform,
		OSName: device.osName, OSVersion: device.osVersion, AppVersion: g.versions[release], AppBuild: g.builds[release],
		DeviceModel: device.model, DeviceType: device.kind, Locale: geo.locale,
		Country: geo.country, Region: geo.region, City: geo.city, WriteKeyID: g.app.WriteKeyID,
	}
	for _, off := range starts {
		start := midnight.Add(off).UTC()
		var delay time.Duration
		if r.pct(perfLate) {
			delay = time.Hour + time.Duration(r.intn(29*3600))*time.Second
		}
		base.SessionID = r.uuid()
		rows = append(rows, scenarioEvent(r, base, "session_start", start, 0, delay))
		if r.pct(perfEnded) {
			dur := 5*time.Second + time.Duration(r.intn(30*60))*time.Second + r.ms(1000)
			rows = append(rows, scenarioEvent(r, base, "session_end", start.Add(dur), dur, delay))
		}
	}
	return rows
}

// login назначает установке user_id: новый или (perfSharedUser %) уже вошедший на другой установке.
func (g *perfAppGen) login(in *perfInstall) {
	r := g.r
	if len(g.recent) > 0 && r.pct(perfSharedUser) {
		in.user = g.recent[r.intn(len(g.recent))]
	} else {
		in.user = fmt.Sprintf("%s-%016x", g.app.Code, r.src.Uint64())
		g.stats.Users++
		if len(g.recent) < perfRecentUsers {
			g.recent = append(g.recent, in.user)
		} else {
			g.recent[r.intn(perfRecentUsers)] = in.user
		}
	}
	g.stats.Links++
}

// WritePerf пишет синтетику в формате NDJSON-фикстуры (поток по батчам, без накопления в памяти).
func WritePerf(w io.Writer, cfg PerfConfig) (PerfStats, error) {
	bw := bufio.NewWriterSize(w, 1<<20)
	stats, err := GeneratePerf(cfg, func(rows []chstore.EventRow) error { return WriteNDJSON(bw, rows) })
	if err != nil {
		return PerfStats{}, err
	}
	if err := bw.Flush(); err != nil {
		return PerfStats{}, fmt.Errorf("testutil: perf: %w", err)
	}
	return stats, nil
}

// PerfStatsTable — сводка объёма синтетики (go run ./testdata/gen -mode perf -stats).
func PerfStatsTable(cfg PerfConfig, stats PerfStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# perf: seed %d, DAU %d, %d дней %s … %s\n", cfg.Seed, cfg.DAU, cfg.Days,
		cfg.FirstDay().Format(time.DateOnly), cfg.LastDay.Format(time.DateOnly))
	fmt.Fprintf(&b, "events    %d\ninstalls  %d\nlinks     %d\nusers     %d\n", stats.Events, stats.Installs, stats.Links, stats.Users)
	b.WriteString("app       avg DAU\n")
	for i, app := range PerfApps {
		dau := 0
		if i < len(stats.AvgDAU) {
			dau = stats.AvgDAU[i]
		}
		fmt.Fprintf(&b, "%-9s %d\n", app.Code, dau)
	}
	return b.String()
}

// perfLoaders — параллельные вставки батчей в ClickHouse.
const perfLoaders = 4

// LoadPerf генерирует синтетику cfg и вставляет её в events тестовой базы обычными (не async) вставками в
// perfLoaders потоков; install_first_seen, user_first_seen и identity_links заполняют MV.
func (e *Env) LoadPerf(t testing.TB, cfg PerfConfig) PerfStats {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	batches := make(chan []chstore.EventRow, perfLoaders)
	for range perfLoaders {
		wg.Go(func() {
			for rows := range batches {
				if ctx.Err() != nil {
					continue
				}
				if err := e.insertPlain(ctx, rows); err != nil {
					once.Do(func() { firstErr = err; cancel() })
				}
			}
		})
	}
	stats, err := GeneratePerf(cfg, func(rows []chstore.EventRow) error {
		select {
		case batches <- rows:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	close(batches)
	wg.Wait()
	require.NoError(t, firstErr, "testutil: вставка perf-синтетики")
	require.NoError(t, err)
	return stats
}

func (e *Env) insertPlain(ctx context.Context, rows []chstore.EventRow) (err error) {
	batch, err := e.CH.Conn().PrepareBatch(ctx, "INSERT INTO events")
	if err != nil {
		return fmt.Errorf("prepare insert events: %w", err)
	}
	defer func() {
		if err != nil {
			_ = batch.Abort()
		}
	}()
	for i := range rows {
		if err = batch.AppendStruct(&rows[i]); err != nil {
			return fmt.Errorf("append events row %d: %w", i, err)
		}
	}
	if err = batch.Send(); err != nil {
		return fmt.Errorf("insert events: %w", err)
	}
	return nil
}

// PerfResult — замеры одного отчёта (reports-latest.json); миллисекунды дробные.
type PerfResult struct {
	Name        string  `json:"name"`
	Runs        int     `json:"runs"`
	ThresholdMs float64 `json:"threshold_ms"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	MaxMs       float64 `json:"max_ms"`
	Pass        bool    `json:"pass"`
	Error       string  `json:"error,omitempty"` // отчёт не выполнился (замеров нет)
}

// Percentile — перцентиль p (0…100) методом ближайшего ранга: P95 из 20 прогонов — 19-й по возрастанию.
// Пустой срез — 0; входной срез не меняется.
func Percentile(runs []time.Duration, p int) time.Duration {
	if len(runs) == 0 {
		return 0
	}
	sorted := slices.Clone(runs)
	slices.Sort(sorted)
	rank := int(math.Ceil(float64(p)/100*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}

// SummarizeRuns — P50, P95 и максимум прогонов; Pass — есть прогоны и P95 ≤ threshold.
func SummarizeRuns(name string, threshold time.Duration, runs []time.Duration) PerfResult {
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	p95 := Percentile(runs, 95)
	return PerfResult{
		Name: name, Runs: len(runs), ThresholdMs: ms(threshold),
		P50Ms: ms(Percentile(runs, 50)), P95Ms: ms(p95), MaxMs: ms(Percentile(runs, 100)),
		Pass: len(runs) > 0 && p95 <= threshold,
	}
}
