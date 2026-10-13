// Package config загружает конфигурацию процессов из переменных окружения (полный список — .env.example).
// На каждый процесс своя структура и свой Load*. Пустое значение переменной равносильно её отсутствию.
// Тексты ошибок никогда не содержат значений переменных: среди них есть пароли и ключи.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Env string

const (
	EnvProduction  Env = "production"
	EnvDevelopment Env = "development"
	// EnvE2E — стек Playwright: api открывает служебные маршруты /internal/e2e/*.
	EnvE2E Env = "e2e"
)

// Jobs — задачи worker, допустимые в WORKER_JOBS_ENABLED.
var Jobs = []string{"system_check", "digest", "close_day", "deletion", "session_cleanup"}

type ClickHouse struct {
	Addr     string
	Database string
	User     string
	Password string
}

// Common — настройки, общие для всех Go-процессов.
type Common struct {
	AppEnv            Env
	LogLevel          slog.Level
	MetricsListenAddr string
	PostgresURL       string
	ClickHouse        ClickHouse
}

type Ingest struct {
	Common
	ListenAddr          string
	GeoIPDBPath         string
	MaxBodyBytes        int64
	MaxEvents           int
	LateWindow          time.Duration
	FutureTolerance     time.Duration
	RateAppEventsPerSec float64
	RateAppBurst        int
	RateInstallPerMin   int
	RateInstallBurst    int
	AcceptDevelopment   bool
	KeyCacheRefresh     time.Duration
}

type API struct {
	Common
	ListenAddr          string
	PublicBaseURL       string
	WebAuthnRPID        string
	WebAuthnRPOrigins   []string
	WebAuthnRPName      string
	SessionTTL          time.Duration
	SessionIdleTTL      time.Duration
	KeyHashPepper       []byte
	MigrateOnStart      bool
	PublicBearerEnabled bool
	// ClickHouseMigrate — пользователь для DDL при MIGRATE_ON_START=true.
	ClickHouseMigrate ClickHouse
}

type Worker struct {
	Common
	ListenAddr          string
	JobsEnabled         []string
	TelegramBotToken    string
	HealthchecksPingURL string
	BackupAPIURL        string
	DiskFreeAlertPct    int
	IngestHealthURL     string
	APIHealthURL        string
	IngestMetricsURL    string
}

// Migrate — конфигурация `appmetrica migrate`: ClickHouse подключается пользователем для DDL.
type Migrate struct {
	Common
}

func LoadIngest() (Ingest, error) {
	r := &reader{lookup: os.LookupEnv}
	c := Ingest{
		Common:              r.common("appmetrica_rw"),
		ListenAddr:          r.str("INGEST_LISTEN_ADDR", ":8081"),
		GeoIPDBPath:         r.str("GEOIP_DB_PATH", "/app/geo/dbip-city-lite.mmdb"),
		MaxBodyBytes:        int64(r.positiveInt("INGEST_MAX_BODY_BYTES", 1<<20)),
		MaxEvents:           r.positiveInt("INGEST_MAX_EVENTS", 500),
		LateWindow:          r.duration("INGEST_LATE_WINDOW", 168*time.Hour),
		FutureTolerance:     r.duration("INGEST_FUTURE_TOLERANCE", 10*time.Minute),
		RateAppEventsPerSec: r.positiveFloat("RATE_APP_EVENTS_PER_SEC", 2000),
		RateAppBurst:        r.positiveInt("RATE_APP_BURST", 4000),
		RateInstallPerMin:   r.positiveInt("RATE_INSTALL_PER_MIN", 100),
		RateInstallBurst:    r.positiveInt("RATE_INSTALL_BURST", 1000),
		AcceptDevelopment:   r.bool("INGEST_ACCEPT_DEVELOPMENT", true),
		KeyCacheRefresh:     r.duration("KEY_CACHE_REFRESH", 15*time.Second),
	}
	return c, r.err()
}

func LoadAPI() (API, error) {
	r := &reader{lookup: os.LookupEnv}
	c := API{
		Common:              r.common("appmetrica_ro"),
		ListenAddr:          r.str("API_LISTEN_ADDR", ":8080"),
		PublicBaseURL:       r.origin("PUBLIC_BASE_URL", "http://localhost:3000"),
		WebAuthnRPID:        r.str("WEBAUTHN_RP_ID", "localhost"),
		WebAuthnRPName:      r.str("WEBAUTHN_RP_NAME", "appmetrica.online"),
		SessionTTL:          r.duration("SESSION_TTL", 720*time.Hour),
		SessionIdleTTL:      r.duration("SESSION_IDLE_TTL", 168*time.Hour),
		KeyHashPepper:       r.pepper("KEY_HASH_PEPPER"),
		MigrateOnStart:      r.bool("MIGRATE_ON_START", false),
		PublicBearerEnabled: r.bool("API_PUBLIC_BEARER_ENABLED", true),
		ClickHouseMigrate:   r.clickHouseMigrate(),
	}
	for _, o := range r.list("WEBAUTHN_RP_ORIGINS", "http://localhost:3000") {
		c.WebAuthnRPOrigins = append(c.WebAuthnRPOrigins, r.checkOrigin("WEBAUTHN_RP_ORIGINS", o))
	}
	return c, r.err()
}

func LoadWorker() (Worker, error) {
	r := &reader{lookup: os.LookupEnv}
	c := Worker{
		Common:              r.common("appmetrica_rw"),
		ListenAddr:          r.str("WORKER_LISTEN_ADDR", ":8082"),
		JobsEnabled:         r.list("WORKER_JOBS_ENABLED", strings.Join(Jobs, ",")),
		TelegramBotToken:    r.str("TELEGRAM_BOT_TOKEN", ""),
		HealthchecksPingURL: r.str("HEALTHCHECKS_PING_URL", ""),
		BackupAPIURL:        r.str("BACKUP_API_URL", ""),
		DiskFreeAlertPct:    r.positiveInt("DISK_FREE_ALERT_PCT", 20),
		IngestHealthURL:     r.str("INGEST_HEALTH_URL", "http://localhost:8081/health"),
		APIHealthURL:        r.str("API_HEALTH_URL", "http://localhost:8080/health"),
		IngestMetricsURL:    r.str("INGEST_METRICS_URL", "http://localhost:9091/metrics"),
	}
	for _, j := range c.JobsEnabled {
		if !slices.Contains(Jobs, j) {
			r.fail(fmt.Errorf("WORKER_JOBS_ENABLED: unknown job %q", j))
		}
	}
	if c.DiskFreeAlertPct >= 100 {
		r.fail(errors.New("DISK_FREE_ALERT_PCT: must be between 1 and 99"))
	}
	return c, r.err()
}

func LoadMigrate() (Migrate, error) {
	r := &reader{lookup: os.LookupEnv}
	c := Migrate{Common: r.common("appmetrica_rw")}
	c.ClickHouse = r.clickHouseMigrate()
	return c, r.err()
}

// reader читает переменные и копит ошибки, чтобы за один запуск показать все проблемы конфигурации.
type reader struct {
	lookup func(string) (string, bool)
	errs   []error
}

func (r *reader) fail(err error) { r.errs = append(r.errs, err) }

func (r *reader) err() error { return errors.Join(r.errs...) }

func (r *reader) value(key string) (string, bool) {
	v, ok := r.lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

func (r *reader) str(key, def string) string {
	if v, ok := r.value(key); ok {
		return v
	}
	return def
}

func (r *reader) required(key string) string {
	v, ok := r.value(key)
	if !ok {
		r.fail(fmt.Errorf("%s is required", key))
	}
	return v
}

func (r *reader) positiveInt(key string, def int) int {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	switch {
	case err != nil:
		r.fail(fmt.Errorf("%s: invalid integer", key))
	case n <= 0:
		r.fail(fmt.Errorf("%s: must be positive", key))
	}
	return n
}

func (r *reader) positiveFloat(key string, def float64) float64 {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	switch {
	case err != nil:
		r.fail(fmt.Errorf("%s: invalid number", key))
	case f <= 0:
		r.fail(fmt.Errorf("%s: must be positive", key))
	}
	return f
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	switch {
	case err != nil:
		r.fail(fmt.Errorf("%s: invalid duration", key))
	case d <= 0:
		r.fail(fmt.Errorf("%s: must be positive", key))
	}
	return d
}

func (r *reader) bool(key string, def bool) bool {
	v, ok := r.value(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.fail(fmt.Errorf("%s: invalid boolean", key))
	}
	return b
}

// list разбирает значение через запятую, пустые элементы отбрасываются.
func (r *reader) list(key, def string) []string {
	var out []string
	for item := range strings.SplitSeq(r.str(key, def), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (r *reader) origin(key, def string) string {
	return r.checkOrigin(key, r.str(key, def))
}

// checkOrigin требует абсолютный http(s)-адрес без пути, query и fragment.
func (r *reader) checkOrigin(key, v string) string {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		r.fail(fmt.Errorf("%s: invalid origin, expected scheme://host[:port]", key))
		return v
	}
	return strings.TrimSuffix(v, "/")
}

func (r *reader) pepper(key string) []byte {
	v := r.required(key)
	if v == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(v)
	switch {
	case err != nil:
		r.fail(fmt.Errorf("%s: invalid base64", key))
	case len(b) < 32:
		r.fail(fmt.Errorf("%s: must decode to at least 32 bytes", key))
	}
	return b
}

func (r *reader) common(defaultCHUser string) Common {
	c := Common{
		MetricsListenAddr: r.str("METRICS_LISTEN_ADDR", ":9090"),
		PostgresURL:       r.required("POSTGRES_URL"),
		ClickHouse: ClickHouse{
			Addr:     r.str("CLICKHOUSE_ADDR", "localhost:9000"),
			Database: r.str("CLICKHOUSE_DATABASE", "appmetrica"),
			User:     r.str("CLICKHOUSE_USER", defaultCHUser),
			Password: r.str("CLICKHOUSE_PASSWORD", ""),
		},
	}
	switch env := Env(r.str("APP_ENV", string(EnvProduction))); env {
	case EnvProduction, EnvDevelopment, EnvE2E:
		c.AppEnv = env
	default:
		r.fail(errors.New("APP_ENV: must be production, development or e2e"))
	}
	switch lvl := r.str("LOG_LEVEL", "info"); lvl {
	case "debug", "info", "warn", "error":
		_ = c.LogLevel.UnmarshalText([]byte(lvl))
	default:
		r.fail(errors.New("LOG_LEVEL: must be debug, info, warn or error"))
	}
	return c
}

func (r *reader) clickHouseMigrate() ClickHouse {
	return ClickHouse{
		Addr:     r.str("CLICKHOUSE_ADDR", "localhost:9000"),
		Database: r.str("CLICKHOUSE_DATABASE", "appmetrica"),
		User:     r.str("CLICKHOUSE_MIGRATE_USER", "appmetrica_admin"),
		Password: r.str("CLICKHOUSE_MIGRATE_PASSWORD", ""),
	}
}
