package config_test

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/config"
)

// knownVars — все переменные, которые читает пакет config. Тесты очищают их,
// чтобы окружение разработчика (например, .env из Makefile) не влияло на результат.
var knownVars = []string{
	"APP_ENV", "LOG_LEVEL", "METRICS_LISTEN_ADDR", "POSTGRES_URL",
	"CLICKHOUSE_ADDR", "CLICKHOUSE_DATABASE", "CLICKHOUSE_USER", "CLICKHOUSE_PASSWORD",
	"CLICKHOUSE_MIGRATE_USER", "CLICKHOUSE_MIGRATE_PASSWORD",
	"INGEST_LISTEN_ADDR", "GEOIP_DB_PATH", "INGEST_MAX_BODY_BYTES", "INGEST_MAX_EVENTS",
	"INGEST_LATE_WINDOW", "INGEST_FUTURE_TOLERANCE", "RATE_APP_EVENTS_PER_SEC", "RATE_APP_BURST",
	"RATE_INSTALL_PER_MIN", "RATE_INSTALL_BURST", "INGEST_ACCEPT_DEVELOPMENT", "KEY_CACHE_REFRESH",
	"API_LISTEN_ADDR", "PUBLIC_BASE_URL", "WEBAUTHN_RP_ID", "WEBAUTHN_RP_ORIGINS", "WEBAUTHN_RP_NAME",
	"SESSION_TTL", "SESSION_IDLE_TTL", "KEY_HASH_PEPPER", "MIGRATE_ON_START", "API_PUBLIC_BEARER_ENABLED",
	"WORKER_LISTEN_ADDR", "WORKER_JOBS_ENABLED", "TELEGRAM_BOT_TOKEN", "HEALTHCHECKS_PING_URL",
	"BACKUP_API_URL", "DISK_FREE_ALERT_PCT", "INGEST_HEALTH_URL", "API_HEALTH_URL", "INGEST_METRICS_URL",
}

const devPepper = "ZGV2LXBlcHBlci1kby1ub3QtdXNlLWluLXByb2R1Y3Rpb24="

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range knownVars {
		t.Setenv(k, "")
	}
}

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

// loadEnvExample читает корневой .env.example (формат KEY=value, # — комментарии).
func loadEnvExample(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", ".env.example"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	vars := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		require.True(t, ok, "строка без '=': %q", line)
		vars[k] = v
	}
	require.NoError(t, sc.Err())
	return vars
}

func TestLoad_DefaultsAndRequired(t *testing.T) {
	t.Run("api without POSTGRES_URL", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("KEY_HASH_PEPPER", devPepper)

		_, err := config.LoadAPI()
		require.ErrorContains(t, err, "POSTGRES_URL is required")
	})

	t.Run("every process requires POSTGRES_URL", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("KEY_HASH_PEPPER", devPepper)

		_, err := config.LoadIngest()
		require.ErrorContains(t, err, "POSTGRES_URL is required")
		_, err = config.LoadWorker()
		require.ErrorContains(t, err, "POSTGRES_URL is required")
		_, err = config.LoadMigrate()
		require.ErrorContains(t, err, "POSTGRES_URL is required")
	})

	t.Run("env.example values", func(t *testing.T) {
		clearEnv(t)
		setEnv(t, loadEnvExample(t))

		ing, err := config.LoadIngest()
		require.NoError(t, err)
		require.Equal(t, 500, ing.MaxEvents)
		require.Equal(t, 168*time.Hour, ing.LateWindow)
		require.Equal(t, 10*time.Minute, ing.FutureTolerance)
		require.Equal(t, 15*time.Second, ing.KeyCacheRefresh)
		require.Equal(t, int64(1<<20), ing.MaxBodyBytes)
		require.Equal(t, ":8081", ing.ListenAddr)
		require.InDelta(t, 2000.0, ing.RateAppEventsPerSec, 1e-9)
		require.Equal(t, 4000, ing.RateAppBurst)
		require.Equal(t, 100, ing.RateInstallPerMin)
		require.Equal(t, 1000, ing.RateInstallBurst)
		require.True(t, ing.AcceptDevelopment)
		require.Equal(t, ".geo/dbip-city-lite.mmdb", ing.GeoIPDBPath)
		require.Equal(t, config.EnvDevelopment, ing.AppEnv)
		require.Equal(t, "localhost:9000", ing.ClickHouse.Addr)
		require.Equal(t, "appmetrica", ing.ClickHouse.Database)
		require.Equal(t, "appmetrica_rw", ing.ClickHouse.User)

		api, err := config.LoadAPI()
		require.NoError(t, err)
		require.Equal(t, 720*time.Hour, api.SessionTTL)
		require.Equal(t, 168*time.Hour, api.SessionIdleTTL)
		require.Equal(t, ":8080", api.ListenAddr)
		require.Equal(t, "http://localhost:3000", api.PublicBaseURL)
		require.Equal(t, "localhost", api.WebAuthnRPID)
		require.Equal(t, []string{"http://localhost:3000"}, api.WebAuthnRPOrigins)
		require.Equal(t, []byte("dev-pepper-do-not-use-in-production"), api.KeyHashPepper)
		require.False(t, api.MigrateOnStart)
		require.True(t, api.PublicBearerEnabled)
		require.Equal(t, "appmetrica_admin", api.ClickHouseMigrate.User)
		require.Empty(t, api.TelegramBotToken)
		require.Empty(t, api.BackupAPIURL)

		w, err := config.LoadWorker()
		require.NoError(t, err)
		require.Equal(t, ":8082", w.ListenAddr)
		require.Equal(t, []string{"system_check", "digest", "close_day", "deletion", "session_cleanup", "app_merge"}, w.JobsEnabled)
		require.Equal(t, 20, w.DiskFreeAlertPct)
		require.Empty(t, w.TelegramBotToken)
		require.Equal(t, "http://localhost:3000", w.PublicBaseURL)

		m, err := config.LoadMigrate()
		require.NoError(t, err)
		require.Equal(t, "appmetrica_admin", m.ClickHouse.User)
		require.Equal(t, "dev_ch_admin_password", m.ClickHouse.Password)
	})

	t.Run("defaults when optional vars are unset", func(t *testing.T) {
		clearEnv(t)
		setEnv(t, map[string]string{
			"POSTGRES_URL":    "postgres://u:p@localhost:5432/db",
			"KEY_HASH_PEPPER": devPepper,
		})

		ing, err := config.LoadIngest()
		require.NoError(t, err)
		require.Equal(t, 500, ing.MaxEvents)
		require.Equal(t, 168*time.Hour, ing.LateWindow)
		require.Equal(t, 10*time.Minute, ing.FutureTolerance)
		require.Equal(t, 15*time.Second, ing.KeyCacheRefresh)
		require.Equal(t, int64(1048576), ing.MaxBodyBytes)
		require.Equal(t, config.EnvProduction, ing.AppEnv)
		require.Equal(t, slog.LevelInfo, ing.LogLevel)
		require.Equal(t, ":9090", ing.MetricsListenAddr)

		api, err := config.LoadAPI()
		require.NoError(t, err)
		require.Equal(t, 720*time.Hour, api.SessionTTL)
		require.Equal(t, 168*time.Hour, api.SessionIdleTTL)
		require.True(t, api.PublicBearerEnabled)

		w, err := config.LoadWorker()
		require.NoError(t, err)
		require.Equal(t, []string{"system_check", "digest", "close_day", "deletion", "session_cleanup", "app_merge"}, w.JobsEnabled)
	})
}

// APP_ENV=e2e — стек Playwright: api открывает служебные маршруты /internal/e2e/*.
func TestLoad_AppEnvE2E(t *testing.T) {
	clearEnv(t)
	setEnv(t, map[string]string{
		"APP_ENV":         "e2e",
		"POSTGRES_URL":    "postgres://u:p@localhost:5432/db",
		"KEY_HASH_PEPPER": devPepper,
	})

	api, err := config.LoadAPI()
	require.NoError(t, err)
	require.Equal(t, config.Env("e2e"), api.AppEnv)
}

// AC-008.4: job app_merge включён по умолчанию и разрешён в WORKER_JOBS_ENABLED; неизвестное имя — ошибка.
func TestJobsIncludeAppMerge(t *testing.T) {
	require.Contains(t, config.Jobs, "app_merge")

	clearEnv(t)
	setEnv(t, map[string]string{
		"POSTGRES_URL":    "postgres://u:p@localhost:5432/db",
		"KEY_HASH_PEPPER": devPepper,
	})
	w, err := config.LoadWorker()
	require.NoError(t, err)
	require.Contains(t, w.JobsEnabled, "app_merge")

	setEnv(t, map[string]string{"WORKER_JOBS_ENABLED": "app_merge,deletion"})
	w, err = config.LoadWorker()
	require.NoError(t, err)
	require.Equal(t, []string{"app_merge", "deletion"}, w.JobsEnabled)

	setEnv(t, map[string]string{"WORKER_JOBS_ENABLED": "app_merge,app_merges"})
	require.ErrorContains(t, loadWorker(), `WORKER_JOBS_ENABLED: unknown job "app_merges"`)
}

// api читает те же TELEGRAM_BOT_TOKEN и BACKUP_API_URL, что и worker: тестовое сообщение и статус бэкапа.
func TestLoad_APITelegramAndBackup(t *testing.T) {
	clearEnv(t)
	setEnv(t, map[string]string{
		"POSTGRES_URL":       "postgres://u:p@localhost:5432/db",
		"KEY_HASH_PEPPER":    devPepper,
		"TELEGRAM_BOT_TOKEN": "123:abc",
		"BACKUP_API_URL":     "http://clickhouse-backup:7171",
	})

	api, err := config.LoadAPI()
	require.NoError(t, err)
	require.Equal(t, "123:abc", api.TelegramBotToken)
	require.Equal(t, "http://clickhouse-backup:7171", api.BackupAPIURL)

	w, err := config.LoadWorker()
	require.NoError(t, err)
	require.Equal(t, api.TelegramBotToken, w.TelegramBotToken)
	require.Equal(t, api.BackupAPIURL, w.BackupAPIURL)
}

// worker читает тот же PUBLIC_BASE_URL, что и api: ссылка на портфель в дайджесте Telegram.
func TestLoad_WorkerPublicBaseURL(t *testing.T) {
	clearEnv(t)
	setEnv(t, map[string]string{
		"POSTGRES_URL":    "postgres://u:p@localhost:5432/db",
		"KEY_HASH_PEPPER": devPepper,
		"PUBLIC_BASE_URL": "https://appmetrica.online/",
	})

	api, err := config.LoadAPI()
	require.NoError(t, err)
	w, err := config.LoadWorker()
	require.NoError(t, err)
	require.Equal(t, "https://appmetrica.online", w.PublicBaseURL)
	require.Equal(t, api.PublicBaseURL, w.PublicBaseURL)
}

func TestLoad_InvalidValues(t *testing.T) {
	base := map[string]string{
		"POSTGRES_URL":    "postgres://u:p@localhost:5432/db",
		"KEY_HASH_PEPPER": devPepper,
	}
	tests := []struct {
		name    string
		env     map[string]string
		load    func() error
		wantErr string
	}{
		{"bad duration", map[string]string{"INGEST_LATE_WINDOW": "week"}, loadIngest, "INGEST_LATE_WINDOW: invalid duration"},
		{"negative duration", map[string]string{"KEY_CACHE_REFRESH": "-1s"}, loadIngest, "KEY_CACHE_REFRESH: must be positive"},
		{"bad int", map[string]string{"INGEST_MAX_EVENTS": "many"}, loadIngest, "INGEST_MAX_EVENTS: invalid integer"},
		{"zero int", map[string]string{"INGEST_MAX_EVENTS": "0"}, loadIngest, "INGEST_MAX_EVENTS: must be positive"},
		{"bad bool", map[string]string{"INGEST_ACCEPT_DEVELOPMENT": "yes"}, loadIngest, "INGEST_ACCEPT_DEVELOPMENT: invalid boolean"},
		{"bad app env", map[string]string{"APP_ENV": "staging"}, loadIngest, "APP_ENV: must be production, development or e2e"},
		{"bad log level", map[string]string{"LOG_LEVEL": "verbose"}, loadIngest, "LOG_LEVEL: must be debug, info, warn or error"},
		{"missing pepper", map[string]string{"KEY_HASH_PEPPER": ""}, loadAPI, "KEY_HASH_PEPPER is required"},
		{"pepper not base64", map[string]string{"KEY_HASH_PEPPER": "not base64!"}, loadAPI, "KEY_HASH_PEPPER: invalid base64"},
		{"short pepper", map[string]string{"KEY_HASH_PEPPER": "c2hvcnQ="}, loadAPI, "KEY_HASH_PEPPER: must decode to at least 32 bytes"},
		{"bad origin", map[string]string{"WEBAUTHN_RP_ORIGINS": "localhost:3000"}, loadAPI, "WEBAUTHN_RP_ORIGINS: invalid origin"},
		{"unknown job", map[string]string{"WORKER_JOBS_ENABLED": "digest,mine_bitcoin"}, loadWorker, `WORKER_JOBS_ENABLED: unknown job "mine_bitcoin"`},
		{"pct out of range", map[string]string{"DISK_FREE_ALERT_PCT": "101"}, loadWorker, "DISK_FREE_ALERT_PCT: must be between 1 and 99"},
		{"bad worker base url", map[string]string{"PUBLIC_BASE_URL": "https://appmetrica.online/app"}, loadWorker, "PUBLIC_BASE_URL: invalid origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			setEnv(t, base)
			setEnv(t, tt.env)
			require.ErrorContains(t, tt.load(), tt.wantErr)
		})
	}
}

func TestLoad_ReportsAllErrors(t *testing.T) {
	clearEnv(t)
	t.Setenv("INGEST_MAX_EVENTS", "x")

	err := loadAPI()
	require.ErrorContains(t, err, "POSTGRES_URL is required")
	require.ErrorContains(t, err, "KEY_HASH_PEPPER is required")
}

func TestLoad_SecretsNotInErrors(t *testing.T) {
	clearEnv(t)
	t.Setenv("POSTGRES_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("KEY_HASH_PEPPER", "c2VjcmV0LXNob3J0")

	err := loadAPI()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "c2VjcmV0LXNob3J0")
	require.NotContains(t, err.Error(), "secret-short")
}

func loadIngest() error { _, err := config.LoadIngest(); return err }
func loadAPI() error    { _, err := config.LoadAPI(); return err }
func loadWorker() error { _, err := config.LoadWorker(); return err }
