//go:build perf

package reports

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// Перф-тест отчётов NFR-003 (T-035, 22-test-strategy.md §11.2): синтетика testutil.GeneratePerf
// (100k DAU основного приложения × 90 дней, ≈ 27 млн событий, ≈ 2 млн связок identity), каждый отчёт —
// прогрев и 20 замеров, P95 сравнивается с порогом. Запуск: make perf-reports
// (go test -tags=perf -run TestReportsPerf ./internal/reports), еженедельно — job perf-reports в ci.yml.

const perfRuns = 20

// Пороги P95 (02-tech-spec.md NFR-003, 22-test-strategy.md §11.2): отчёты на 90 днях ≤ 1,5 с, портфель и
// профиль ≤ 1 с, realtime ≤ 300 мс.
const (
	perfReportLimit   = 1500 * time.Millisecond
	perfPortfolio     = time.Second
	perfProfileLimit  = time.Second
	perfRealtimeLimit = 300 * time.Millisecond
	// split=platform основного приложения (iOS, Android, Web) — не дольше трёх рядов без split
	// (05-impact-and-regression.md §4.1 мультиплатформенности).
	perfSplitLimit = 3 * perfReportLimit
	perfSplitRatio = 3.0
)

// perfReportPath — результаты последнего прогона (рабочий файл, в git не попадает).
func perfReportPath() string {
	return filepath.Join(testutil.BackendDir(), "perf", "reports", "reports-latest.json")
}

type perfCase struct {
	name      string
	threshold time.Duration
	run       func(ctx context.Context) error
}

// perfReport — содержимое reports-latest.json.
type perfReport struct {
	GeneratedAt       time.Time             `json:"generated_at"`
	ClickHouseVersion string                `json:"clickhouse_version"`
	Dataset           perfDataset           `json:"dataset"`
	LoadSeconds       float64               `json:"load_seconds"`
	Results           []testutil.PerfResult `json:"results"`
}

type perfDataset struct {
	Seed     uint64 `json:"seed"`
	DAU      int    `json:"dau"`
	Days     int    `json:"days"`
	From     string `json:"from"`
	To       string `json:"to"`
	Events   int    `json:"events"`
	Installs int    `json:"installs"`
	Links    int    `json:"links"`
	Users    int    `json:"users"`
	AvgDAU   []int  `json:"avg_dau"`
}

func perfCases(t *testing.T, svc *Service, cfg testutil.PerfConfig) []perfCase {
	t.Helper()
	apps := make([]PortfolioApp, 0, len(testutil.PerfApps))
	for _, app := range testutil.PerfApps {
		loc, err := time.LoadLocation(app.Timezone)
		require.NoError(t, err)
		apps = append(apps, PortfolioApp{ID: app.ID, Location: loc})
	}
	main := Params{
		AppID: apps[0].ID, Location: apps[0].Location, Env: EnvProduction, Unit: identity.Persons,
		Period: Period{From: cfg.FirstDay(), To: cfg.LastDay},
	}
	installs := main
	installs.Unit = identity.Installs
	last30 := main
	last30.Period.From = cfg.LastDay.AddDate(0, 0, -29)

	ignore := func(_ any, err error) error { return err }
	return []perfCase{
		{"portfolio", perfPortfolio, func(ctx context.Context) error {
			return ignore(svc.Portfolio(ctx, PortfolioQuery{Date: cfg.LastDay, Env: EnvProduction, Unit: identity.Persons, Apps: apps}))
		}},
		{"audience_summary", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceSummary(ctx, main, true))
		}},
		{"audience_summary_installs", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceSummary(ctx, installs, true))
		}},
		{"audience_timeseries_active", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceTimeseries(ctx, main, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricActive, Compare: true}))
		}},
		{"audience_timeseries_split_platform", perfSplitLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceTimeseries(ctx, main, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricActive, SplitByPlatform: true}))
		}},
		{"audience_timeseries_avg_duration", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceTimeseries(ctx, main, TimeseriesQuery{Granularity: GranularityDay, Metric: MetricAvgDuration}))
		}},
		{"rolling_active", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.RollingActive(ctx, main))
		}},
		{"breakdown_country", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceBreakdown(ctx, main, BreakdownQuery{Dimension: DimensionCountry, Metric: MetricActive}))
		}},
		{"breakdown_app_version_new", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.AudienceBreakdown(ctx, main, BreakdownQuery{Dimension: DimensionAppVersion, Metric: MetricNew}))
		}},
		{"retention_day_30x30", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.Retention(ctx, last30, RetentionQuery{Granularity: GranularityDay, Periods: 30}))
		}},
		{"retention_week_90d", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.Retention(ctx, main, RetentionQuery{Granularity: GranularityWeek, Periods: 12}))
		}},
		{"realtime", perfRealtimeLimit, func(ctx context.Context) error {
			return ignore(svc.Realtime(ctx, main))
		}},
		{"profile_search", perfProfileLimit, func(ctx context.Context) error {
			return ignore(svc.SearchProfiles(ctx, main.AppID, testutil.PerfPowerInstall.String()))
		}},
		{"profile", perfProfileLimit, func(ctx context.Context) error {
			return ignore(svc.Profile(ctx, main.AppID, ProfileQuery{PersonKey: "u:" + testutil.PerfPowerUser, SessionsLimit: 100}))
		}},
		{"integration_health", perfReportLimit, func(ctx context.Context) error {
			return ignore(svc.IntegrationHealth(ctx, main, 24))
		}},
	}
}

// perfServerMemory — config.d перф-теста: контейнер ClickHouse выделенный, поэтому сервер может занять 90 %
// RAM вместо 50 % боевого deploy/clickhouse/config.d (в production ClickHouse делит 128 ГБ с Postgres и
// сервисами — 64 ГБ). Настройка перечитывается без рестарта (SYSTEM RELOAD CONFIG).
const perfServerMemory = `<clickhouse><max_server_memory_usage_to_ram_ratio>0.9</max_server_memory_usage_to_ram_ratio></clickhouse>`

func raiseServerMemory(t *testing.T, env *testutil.Env) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, env.ClickHouseContainer().CopyToContainer(ctx, []byte(perfServerMemory),
		"/etc/clickhouse-server/config.d/zz-perf.xml", 0o644))
	require.NoError(t, env.CH.Conn().Exec(ctx, "SYSTEM RELOAD CONFIG"))
	var ratio string
	require.NoError(t, env.CH.Conn().QueryRow(ctx,
		"SELECT value FROM system.server_settings WHERE name = 'max_server_memory_usage_to_ram_ratio'").Scan(&ratio))
	require.Equal(t, "0.9", ratio)
}

// optimizePerf сливает части таблиц после загрузки: замеры не конкурируют с фоновыми слияниями, а данные
// выглядят как в production, где части давно слиты.
func optimizePerf(t *testing.T, conn driver.Conn) {
	t.Helper()
	for _, table := range []string{"events", "install_first_seen", "user_first_seen", "identity_links"} {
		require.NoError(t, conn.Exec(t.Context(), "OPTIMIZE TABLE "+table+" FINAL"), table)
	}
}

// AC-035.1: P95 каждого отчёта на 90 днях ≤ 1,5 с, портфеля ≤ 1 с; результаты — reports-latest.json.
func TestReportsPerf_90Days100kDAU(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	cfg := testutil.DefaultPerfConfig()
	raiseServerMemory(t, env)

	started := time.Now()
	stats := env.LoadPerf(t, cfg)
	optimizePerf(t, env.CH.Conn())
	load := time.Since(started)
	t.Logf("загружено %d событий (%d установок, %d связок) за %s", stats.Events, stats.Installs, stats.Links, load.Round(time.Second))
	require.InEpsilon(t, cfg.DAU, stats.AvgDAU[0], 0.1, "синтетика: DAU основного приложения")

	var version string
	require.NoError(t, env.CH.Conn().QueryRow(ctx, "SELECT version()").Scan(&version))

	svc := NewService(roProfileConn(t, env), clock.NewFixed(cfg.Now()))
	report := perfReport{
		GeneratedAt:       time.Now().UTC().Truncate(time.Second),
		ClickHouseVersion: version,
		Dataset: perfDataset{
			Seed: cfg.Seed, DAU: cfg.DAU, Days: cfg.Days,
			From: cfg.FirstDay().Format(time.DateOnly), To: cfg.LastDay.Format(time.DateOnly),
			Events: stats.Events, Installs: stats.Installs, Links: stats.Links, Users: stats.Users, AvgDAU: stats.AvgDAU,
		},
		LoadSeconds: load.Seconds(),
	}
	for _, c := range perfCases(t, svc, cfg) {
		res := measure(ctx, c)
		if res.Error != "" {
			t.Logf("%-34s ошибка: %s", res.Name, res.Error)
		} else {
			t.Logf("%-34s P50 %7.1f мс  P95 %7.1f мс  max %7.1f мс  порог %5.0f мс", res.Name, res.P50Ms, res.P95Ms, res.MaxMs, res.ThresholdMs)
		}
		report.Results = append(report.Results, res)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(perfReportPath()), 0o755))
	require.NoError(t, os.WriteFile(perfReportPath(), append(data, '\n'), 0o644))
	t.Logf("результаты: %s", perfReportPath())

	byName := map[string]testutil.PerfResult{}
	for _, res := range report.Results {
		assert.Empty(t, res.Error, res.Name)
		assert.True(t, res.Pass, "%s: P95 %.1f мс > порога %.0f мс", res.Name, res.P95Ms, res.ThresholdMs)
		byName[res.Name] = res
	}
	none, split := byName["audience_timeseries_active"], byName["audience_timeseries_split_platform"]
	assert.LessOrEqual(t, split.P95Ms, perfSplitRatio*none.P95Ms,
		"split=platform: P95 %.1f мс больше %.0f× ряда без split (%.1f мс)", split.P95Ms, perfSplitRatio, none.P95Ms)
}

// measure — прогрев и perfRuns замеров отчёта. Ошибка (например, превышение памяти) записывается в результат,
// а не прерывает тест: отчёт reports-latest.json остаётся полным.
func measure(ctx context.Context, c perfCase) testutil.PerfResult {
	runs := make([]time.Duration, 0, perfRuns)
	for i := range perfRuns + 1 {
		start := time.Now()
		if err := c.run(ctx); err != nil {
			res := testutil.SummarizeRuns(c.name, c.threshold, nil)
			res.Error = err.Error()
			return res
		}
		if i > 0 { // первый прогон — прогрев
			runs = append(runs, time.Since(start))
		}
	}
	return testutil.SummarizeRuns(c.name, c.threshold, runs)
}

// roProfileConn — соединение с базой теста с настройками боевого профиля appmetrica_ro
// (deploy/clickhouse/users.d): отчёты меряются с теми же лимитами потоков, памяти и времени, что у api.
func roProfileConn(t *testing.T, env *testutil.Env) driver.Conn {
	t.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{env.ClickHouse.Addr},
		Auth: clickhouse.Auth{Database: env.ClickHouse.Database, Username: env.ClickHouse.User, Password: env.ClickHouse.Password},
		Settings: clickhouse.Settings{
			"max_threads":        8,
			"max_memory_usage":   8589934592,
			"max_execution_time": 30,
			"join_use_nulls":     0,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.Ping(t.Context()))
	return conn
}

// explainEventsParts — части events до и после отсечения партиций по EXPLAIN indexes = 1 (json = 1):
// по каждому чтению events — Initial Parts индекса Min-Max и Selected Parts индекса Partition.
func explainEventsParts(ctx context.Context, t *testing.T, conn driver.Conn, query string) (reads []partsRead) {
	t.Helper()
	var plan string
	require.NoError(t, conn.QueryRow(ctx, "EXPLAIN json = 1, indexes = 1 "+query).Scan(&plan))
	var root []struct {
		Plan explainNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal([]byte(plan), &root), plan)
	require.Len(t, root, 1)

	var walk func(n explainNode)
	walk = func(n explainNode) {
		if n.Type == "ReadFromMergeTree" && strings.HasSuffix(n.Description, ".events") {
			r := partsRead{initial: -1, selected: -1}
			for _, idx := range n.Indexes {
				switch idx.Type {
				case "Min-Max", "MinMax": // 26.8 — "Min-Max"
					r.initial = idx.InitialParts
				case "Partition":
					r.selected = idx.SelectedParts
				}
			}
			require.NotEqual(t, -1, r.initial, "нет индекса Min-Max: %s", plan)
			require.NotEqual(t, -1, r.selected, "нет индекса Partition: %s", plan)
			reads = append(reads, r)
		}
		for _, child := range n.Plans {
			walk(child)
		}
	}
	walk(root[0].Plan)
	require.NotEmpty(t, reads, "в плане нет чтения events: %s", plan)
	return reads
}

type partsRead struct{ initial, selected int }

type explainNode struct {
	Type        string         `json:"Node Type"`
	Description string         `json:"Description"`
	Indexes     []explainIndex `json:"Indexes"`
	Plans       []explainNode  `json:"Plans"`
}

type explainIndex struct {
	Type          string `json:"Type"`
	InitialParts  int    `json:"Initial Parts"`
	SelectedParts int    `json:"Selected Parts"`
}

// monthsBetween — партиции toYYYYMM от месяца lo до месяца hi включительно.
func monthsBetween(t *testing.T, lo, hi string) []string {
	t.Helper()
	from, err := time.Parse(time.DateOnly, lo)
	require.NoError(t, err)
	to, err := time.Parse(time.DateOnly, hi)
	require.NoError(t, err)
	var out []string
	for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(to); m = m.AddDate(0, 1, 0) {
		out = append(out, m.Format("200601"))
	}
	return out
}

// AC-035.2: запросы портфеля (sparkline за 30 дней, метрики даты, сессии) читают только части партиций месяцев
// своего диапазона {d_lo}…{d_hi}.
func TestPortfolio_ExplainPrunesPartitions(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	cfg := testutil.DefaultPerfConfig()
	cfg.DAU = 500 // 90 дней: партиции 202607 … 202610
	env.LoadPerf(t, cfg)
	conn := env.CH.Conn()
	require.NoError(t, conn.Exec(ctx, "OPTIMIZE TABLE events FINAL"))

	partParts := map[string]int{}
	rows, err := conn.Query(ctx, `SELECT partition, toUInt64(count()) FROM system.parts
WHERE database = currentDatabase() AND table = 'events' AND active GROUP BY partition`)
	require.NoError(t, err)
	for rows.Next() {
		var partition string
		var n uint64
		require.NoError(t, rows.Scan(&partition, &n))
		partParts[partition] = int(n)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []string{"202607", "202608", "202609", "202610"}, slices.Sorted(maps.Keys(partParts)))
	total := 0
	for _, n := range partParts {
		total += n
	}

	app := testutil.PerfApps[0]
	loc, err := time.LoadLocation(app.Timezone)
	require.NoError(t, err)
	day := cfg.LastDay
	base := Params{AppID: app.ID, Location: loc, Env: EnvProduction, Unit: identity.Persons}
	tests := []struct {
		name   string
		period Period
		query  string
		extra  clickhouse.Parameters
		months []string
	}{
		{
			name: "sparkline за 30 дней", period: Period{From: day.AddDate(0, 0, -(sparklineDays - 1)), To: day},
			query: qDailyActive, months: []string{"202609", "202610"},
		},
		{
			name: "метрики даты (date − 7 … date)", period: Period{From: day.AddDate(0, 0, -7), To: day},
			query: qPortfolioDate, extra: clickhouse.Parameters{"date": day.Format(time.DateOnly)}, months: []string{"202610"},
		},
		{
			name: "сессии даты", period: Period{From: day, To: day},
			query: qPortfolioSessions, months: []string{"202610"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base
			p.Period = tt.period
			params := p.Parameters()
			require.Equal(t, tt.months, monthsBetween(t, params["d_lo"], params["d_hi"]), "партиции диапазона")
			want := 0
			for _, m := range tt.months {
				want += partParts[m]
			}

			for _, r := range explainEventsParts(p.bindWith(t.Context(), tt.extra), t, conn, tt.query) {
				require.Equal(t, total, r.initial, "до отсечения — все части events")
				require.Equal(t, want, r.selected, "выбраны только части партиций %v", tt.months)
				require.Less(t, r.selected, total)
			}
		})
	}
}
