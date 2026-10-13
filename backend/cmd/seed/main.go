// Команда seed заливает в локальные Postgres и ClickHouse демо-данные (`make seed`, 12-dev-runbook.md §4.3):
// приложения «Seed iOS», «Seed Android», «Seed Site» (TZ Europe/Moscow), write-ключ --write-key для «Seed iOS»
// и события за --days дней по вчерашний день: --daily-installs новых установок в день, удержание D1≈35 %,
// D7≈15 %, 20 % установок с user_id, 5 % — второе устройство того же user_id.
//
// События пишутся в ClickHouse напрямую, минуя ingest и окно приёма 7 дней. Генератор детерминирован
// (seed RNG + дата запуска): повторный запуск в тот же день вставляет те же строки, и ReplacingMergeTree
// схлопывает их по insert_id; приложения и ключ ищутся по имени и значению и не дублируются.
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	seedTimezone = "Europe/Moscow"
	insertChunk  = 20_000

	retentionD1 = 0.35
	retentionD7 = 0.15
	userShare   = 0.20 // доля установок с user_id
	secondShare = 0.05 // из них — второе устройство уже известного user_id
)

// retentionDecay — показатель степенной кривой p(n) = D1·n^(−b), проходящей через D1 и D7.
var retentionDecay = math.Log(retentionD1/retentionD7) / math.Log(7)

var writeKeyRe = regexp.MustCompile(`^pk_[A-Za-z0-9]{32}$`)

type appSpec struct {
	Name     string
	Platform sqlc.AppPlatform
}

var appSpecs = []appSpec{
	{"Seed iOS", sqlc.AppPlatformIos},
	{"Seed Android", sqlc.AppPlatformAndroid},
	{"Seed Site", sqlc.AppPlatformWeb},
}

type options struct {
	Apps          int
	Days          int
	DailyInstalls int
	WriteKey      string
	Seed          uint64
}

// seedApp — приложение в Postgres, для которого генерируются события.
type seedApp struct {
	ID         uuid.UUID
	Platform   string
	WriteKeyID uuid.UUID // uuid.Nil у приложений без ключа
}

type genConfig struct {
	Days          int
	DailyInstalls int
	Seed          uint64
	Today         time.Time // момент запуска; данные — по вчерашний день в Location
	Location      *time.Location
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, clock.System{})
	stop()
	os.Exit(code)
}

// run возвращает код выхода: 0 — успех, 1 — ошибка выполнения, 2 — ошибка использования.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, clk clock.Clock) int {
	opts, ok := parseOptions(args, stderr)
	if !ok {
		return 2
	}
	if err := seed(ctx, opts, stdout, clk); err != nil {
		say(stderr, "seed: %v\n", err)
		return 1
	}
	return 0
}

func parseOptions(args []string, stderr io.Writer) (options, bool) {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.IntVar(&o.Apps, "apps", len(appSpecs), "число приложений (1..3)")
	fs.IntVar(&o.Days, "days", 35, "дней истории по вчерашний день")
	fs.IntVar(&o.DailyInstalls, "daily-installs", 400, "новых установок в день на приложение")
	fs.StringVar(&o.WriteKey, "write-key", "", "write-ключ pk_ для «Seed iOS» (DEV_WRITE_KEY)")
	fs.Uint64Var(&o.Seed, "seed", 1, "seed генератора")
	if err := fs.Parse(args); err != nil {
		return options{}, false
	}
	var problems []string
	if fs.NArg() > 0 {
		problems = append(problems, fmt.Sprintf("лишние аргументы: %s", strings.Join(fs.Args(), " ")))
	}
	if o.Apps < 1 || o.Apps > len(appSpecs) {
		problems = append(problems, fmt.Sprintf("--apps: от 1 до %d", len(appSpecs)))
	}
	if o.Days < 1 {
		problems = append(problems, "--days: не меньше 1")
	}
	if o.DailyInstalls < 1 {
		problems = append(problems, "--daily-installs: не меньше 1")
	}
	if !writeKeyRe.MatchString(o.WriteKey) {
		problems = append(problems, "--write-key: нужен ключ вида pk_ + 32 символа [A-Za-z0-9]")
	}
	if len(problems) > 0 {
		say(stderr, "seed: %s\n", strings.Join(problems, "; "))
		return options{}, false
	}
	return o, true
}

func seed(ctx context.Context, opts options, stdout io.Writer, clk clock.Clock) error {
	pgURL := os.Getenv("POSTGRES_URL")
	if pgURL == "" {
		return errors.New("не задан POSTGRES_URL")
	}
	loc, err := time.LoadLocation(seedTimezone)
	if err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	pg, err := pgstore.Open(ctx, pgURL)
	if err != nil {
		return err
	}
	defer pg.Close()
	ch, err := chstore.Open(ctx, chstore.Options{
		Addr:     envOr("CLICKHOUSE_ADDR", "localhost:9000"),
		Database: envOr("CLICKHOUSE_DATABASE", "appmetrica"),
		User:     envOr("CLICKHOUSE_USER", "appmetrica_rw"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()

	apps, err := ensureApps(ctx, pg.Queries(), opts)
	if err != nil {
		return err
	}
	cfg := genConfig{Days: opts.Days, DailyInstalls: opts.DailyInstalls, Seed: opts.Seed, Today: clk.Now(), Location: loc}
	for i, app := range apps {
		rows := generateApp(cfg, i, app)
		for start := 0; start < len(rows); start += insertChunk {
			if err := ch.InsertEvents(ctx, rows[start:min(start+insertChunk, len(rows))]); err != nil {
				return fmt.Errorf("%s: %w", appSpecs[i].Name, err)
			}
		}
		say(stdout, "%-13s %s  %d событий\n", appSpecs[i].Name, app.ID, len(rows))
	}
	return nil
}

// ensureApps находит или создаёт приложения appSpecs[:opts.Apps] и write-ключ первого из них.
func ensureApps(ctx context.Context, q *sqlc.Queries, opts options) ([]seedApp, error) {
	existing, err := q.ListApps(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	byName := make(map[string]sqlc.App, len(existing))
	for _, a := range existing {
		byName[strings.ToLower(a.Name)] = a
	}
	out := make([]seedApp, 0, opts.Apps)
	for _, spec := range appSpecs[:opts.Apps] {
		a, ok := byName[strings.ToLower(spec.Name)]
		if !ok {
			a, err = q.CreateApp(ctx, sqlc.CreateAppParams{Name: spec.Name, Platform: spec.Platform, Timezone: seedTimezone})
			if err != nil {
				return nil, fmt.Errorf("create app %q: %w", spec.Name, err)
			}
		}
		out = append(out, seedApp{ID: a.ID, Platform: string(a.Platform)})
	}
	keyID, err := ensureWriteKey(ctx, q, out[0].ID, opts.WriteKey)
	if err != nil {
		return nil, err
	}
	out[0].WriteKeyID = keyID
	return out, nil
}

func ensureWriteKey(ctx context.Context, q *sqlc.Queries, appID uuid.UUID, key string) (uuid.UUID, error) {
	keys, err := q.ListWriteKeys(ctx, appID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("list write keys: %w", err)
	}
	for _, k := range keys {
		if k.Key == key {
			if k.RevokedAt != nil {
				return uuid.Nil, errors.New("write-ключ --write-key отозван")
			}
			return k.ID, nil
		}
	}
	k, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: appID, Key: key, Label: "seed"})
	if err != nil {
		// Уникальность write_keys.key: ключ уже привязан к другому приложению.
		return uuid.Nil, fmt.Errorf("create write key: %w", err)
	}
	return k.ID, nil
}

// device — постоянный контекст установки.
type device struct {
	osName, osVersion, model, deviceType string
	appVersion                           string
	country, region, city                string
}

var (
	appVersions = []string{"1.2.0", "1.3.0", "1.4.0"}
	devices     = map[string][]device{
		"ios": {
			{osName: "iOS", osVersion: "26.1", model: "iPhone17,1", deviceType: "phone"},
			{osName: "iOS", osVersion: "26.0", model: "iPhone16,2", deviceType: "phone"},
			{osName: "iOS", osVersion: "18.6", model: "iPad14,3", deviceType: "tablet"},
		},
		"android": {
			{osName: "Android", osVersion: "16", model: "Pixel 9", deviceType: "phone"},
			{osName: "Android", osVersion: "15", model: "SM-S921B", deviceType: "phone"},
			{osName: "Android", osVersion: "14", model: "2311DRK48G", deviceType: "phone"},
		},
		"web": {
			{osName: "macOS", osVersion: "26.1", deviceType: "desktop"},
			{osName: "Windows", osVersion: "11", deviceType: "desktop"},
			{osName: "iOS", osVersion: "26.1", deviceType: "phone"},
		},
	}
	// geos — страны с накопленными весами (RU 70 %, KZ 10 %, BY 8 %, DE 7 %, US 5 %).
	geos = []struct {
		cum                   float64
		country, region, city string
	}{
		{0.70, "RU", "Moscow", "Moscow"},
		{0.80, "KZ", "Almaty", "Almaty"},
		{0.88, "BY", "Minsk City", "Minsk"},
		{0.95, "DE", "Land Berlin", "Berlin"},
		{1.00, "US", "California", "San Francisco"},
	}
)

// generateApp строит события одного приложения: idx различает поток RNG приложений.
func generateApp(cfg genConfig, idx int, app seedApp) []chstore.EventRow {
	rng := rand.New(rand.NewPCG(cfg.Seed, uint64(idx)+1)) //nolint:gosec // демо-данные, не криптография
	y, m, d := cfg.Today.In(cfg.Location).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, cfg.Location)
	models := devices[app.Platform]
	if len(models) == 0 {
		models = devices["web"]
	}

	var (
		rows  []chstore.EventRow
		users []string
	)
	for back := cfg.Days; back >= 1; back-- {
		first := today.AddDate(0, 0, -back)
		for range cfg.DailyInstalls {
			dev := models[rng.IntN(len(models))]
			dev.appVersion = appVersions[rng.IntN(len(appVersions))]
			g := rng.Float64()
			for _, geo := range geos {
				if g < geo.cum {
					dev.country, dev.region, dev.city = geo.country, geo.region, geo.city
					break
				}
			}
			var user string
			switch r := rng.Float64(); {
			case r < secondShare && len(users) > 0:
				user = users[rng.IntN(len(users))]
			case r < userShare:
				user = fmt.Sprintf("seed-%05d", len(users)+1)
				users = append(users, user)
			}
			in := install{id: newUUID(rng), user: user, dev: dev}

			rows = in.sessions(rows, rng, app, first, 1+rng.IntN(3))
			for n := 1; n < back; n++ {
				if rng.Float64() < retentionD1*math.Pow(float64(n), -retentionDecay) {
					rows = in.sessions(rows, rng, app, first.AddDate(0, 0, n), 1+rng.IntN(2))
				}
			}
		}
	}
	return rows
}

type install struct {
	id   uuid.UUID
	user string
	dev  device
}

// sessions добавляет count пар session_start/session_end в день day (полночь в TZ приложения).
// Сессии начинаются с 08:00 до 22:00 и короче 20 минут — не переходят через полночь.
func (in install) sessions(rows []chstore.EventRow, rng *rand.Rand, app seedApp, day time.Time, count int) []chstore.EventRow {
	for range count {
		start := day.Add(8*time.Hour + time.Duration(rng.Int64N(int64(14*time.Hour/time.Millisecond)))*time.Millisecond).UTC()
		durationMs := 30_000 + rng.Uint32N(19*60_000+30_000)
		end := start.Add(time.Duration(durationMs) * time.Millisecond)
		session := newUUID(rng)
		rows = append(rows,
			in.row(app, "session_start", start, end, session, 0, newUUID(rng)),
			in.row(app, "session_end", end, end, session, durationMs, newUUID(rng)))
	}
	return rows
}

func (in install) row(app seedApp, name string, at, sent time.Time, session uuid.UUID, durationMs uint32, insertID uuid.UUID) chstore.EventRow {
	return chstore.EventRow{
		AppID:       app.ID,
		EventDate:   time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
		EventTime:   at,
		ClientTime:  at,
		SentAt:      sent,
		ReceivedAt:  sent.Add(time.Second),
		InsertID:    insertID,
		Name:        name,
		InstallID:   in.id,
		UserID:      in.user,
		SessionID:   session,
		DurationMs:  durationMs,
		Environment: "production",
		Platform:    app.Platform,
		OSName:      in.dev.osName,
		OSVersion:   in.dev.osVersion,
		AppVersion:  in.dev.appVersion,
		DeviceModel: in.dev.model,
		DeviceType:  in.dev.deviceType,
		Locale:      "ru-RU",
		Country:     in.dev.country,
		Region:      in.dev.region,
		City:        in.dev.city,
		WriteKeyID:  app.WriteKeyID,
	}
}

// newUUID — UUIDv4 из детерминированного RNG.
func newUUID(rng *rand.Rand) uuid.UUID {
	var u uuid.UUID
	binary.LittleEndian.PutUint64(u[:8], rng.Uint64())
	binary.LittleEndian.PutUint64(u[8:], rng.Uint64())
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return u
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
