package testutil_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/testutil"
)

// smallPerf — уменьшенная конфигурация генератора для юнит-тестов (те же правила, что у 100k DAU × 90 дней).
func smallPerf(dau, days int) testutil.PerfConfig {
	cfg := testutil.DefaultPerfConfig()
	cfg.DAU, cfg.Days = dau, days
	return cfg
}

func generatePerf(t *testing.T, cfg testutil.PerfConfig) ([]chstore.EventRow, testutil.PerfStats) {
	t.Helper()
	var rows []chstore.EventRow
	stats, err := testutil.GeneratePerf(cfg, func(batch []chstore.EventRow) error {
		require.NotEmpty(t, batch, "пустые батчи не отдаются")
		rows = append(rows, batch...)
		return nil
	})
	require.NoError(t, err)
	return rows, stats
}

func TestPerf_DefaultConfig(t *testing.T) {
	cfg := testutil.DefaultPerfConfig()
	require.Equal(t, testutil.PerfSeed, cfg.Seed)
	require.Equal(t, 100_000, cfg.DAU)
	require.Equal(t, 90, cfg.Days)
	require.Equal(t, time.Date(2026, time.October, 12, 0, 0, 0, 0, time.UTC), cfg.LastDay)
	require.Equal(t, time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC), cfg.FirstDay(), "90 дней включительно")
	require.Equal(t, time.Date(2026, time.October, 12, 18, 0, 0, 0, time.UTC), cfg.Now())
	require.NoError(t, cfg.Validate())
	require.Len(t, testutil.PerfApps, 3)
	require.Equal(t, []string{"ios", "android", "web"}, testutil.PerfApps[0].Platforms, "основное приложение мультиплатформенное")
	for _, app := range testutil.PerfApps {
		require.Equal(t, app.Platform, app.Platforms[0], app.Code)
	}
}

func TestPerf_ConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testutil.PerfConfig)
	}{
		{name: "DAU 0", edit: func(c *testutil.PerfConfig) { c.DAU = 0 }},
		{name: "DAU меньше минимума", edit: func(c *testutil.PerfConfig) { c.DAU = 99 }},
		{name: "DAU больше 1 млн", edit: func(c *testutil.PerfConfig) { c.DAU = 1_000_001 }},
		{name: "0 дней", edit: func(c *testutil.PerfConfig) { c.Days = 0 }},
		{name: "больше 366 дней", edit: func(c *testutil.PerfConfig) { c.Days = 367 }},
		{name: "нет последнего дня", edit: func(c *testutil.PerfConfig) { c.LastDay = time.Time{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testutil.DefaultPerfConfig()
			tt.edit(&cfg)
			require.Error(t, cfg.Validate())
			_, err := testutil.GeneratePerf(cfg, func([]chstore.EventRow) error {
				t.Fatal("при неверной конфигурации события не генерируются")
				return nil
			})
			require.Error(t, err)
		})
	}
}

func TestPerf_Deterministic(t *testing.T) {
	cfg := smallPerf(300, 14)
	first, firstStats := generatePerf(t, cfg)
	second, secondStats := generatePerf(t, cfg)
	require.Equal(t, firstStats, secondStats)
	require.Equal(t, first, second, "один seed — одинаковые события")

	cfg.Seed = 7
	other, _ := generatePerf(t, cfg)
	require.NotEqual(t, first, other, "другой seed — другие события")

	var a, b bytes.Buffer
	statsA, err := testutil.WritePerf(&a, smallPerf(300, 3))
	require.NoError(t, err)
	statsB, err := testutil.WritePerf(&b, smallPerf(300, 3))
	require.NoError(t, err)
	require.Equal(t, statsA, statsB)
	require.True(t, bytes.Equal(a.Bytes(), b.Bytes()), "NDJSON детерминирован")
}

func TestPerf_WriteParseRoundTrip(t *testing.T) {
	cfg := smallPerf(200, 3)
	var buf bytes.Buffer
	stats, err := testutil.WritePerf(&buf, cfg)
	require.NoError(t, err)
	parsed, err := testutil.ParseNDJSON(&buf, "perf.ndjson")
	require.NoError(t, err)
	rows, _ := generatePerf(t, cfg)
	require.Len(t, parsed, stats.Events)
	require.Len(t, parsed, len(rows))
	for i := range rows {
		require.Equal(t, rows[i].InsertID, parsed[i].InsertID)
		require.True(t, rows[i].EventTime.Equal(parsed[i].EventTime))
	}
}

// Объём: средний DAU установок в установившемся режиме близок к заданному, ~3 события на активную установку
// в день, большинство установок с user_id (связки identity).
func TestPerf_VolumeMatchesConfig(t *testing.T) {
	cfg := smallPerf(3000, 30)
	rows, stats := generatePerf(t, cfg)

	require.Equal(t, len(rows), stats.Events)
	require.Equal(t, cfg.Days, stats.Days)
	require.Len(t, stats.AvgDAU, len(testutil.PerfApps))
	require.InEpsilon(t, 3000, stats.AvgDAU[0], 0.1, "основное приложение: %v", stats.AvgDAU)
	require.InEpsilon(t, 300, stats.AvgDAU[1], 0.15, "второе приложение — DAU/10")
	require.InEpsilon(t, 60, stats.AvgDAU[2], 0.3, "третье приложение — DAU/50")

	activeDays := 0
	for _, dau := range stats.AvgDAU {
		activeDays += dau * cfg.Days
	}
	perActive := float64(stats.Events) / float64(activeDays)
	require.InDelta(t, 3.0, perActive, 0.4, "событий на активную установку в день")

	require.Greater(t, stats.Installs, stats.AvgDAU[0], "установки обновляются: отток и новые")
	require.InDelta(t, 0.85, float64(stats.Links)/float64(stats.Installs), 0.07, "доля установок с user_id")
	require.Less(t, stats.Users, stats.Links, "часть пользователей — на нескольких установках")
}

func TestPerf_Invariants(t *testing.T) {
	cfg := smallPerf(400, 20)
	rows, stats := generatePerf(t, cfg)

	apps := map[uuid.UUID]testutil.PerfApp{}
	for _, app := range testutil.PerfApps {
		apps[app.ID] = app
	}
	type sessionKey struct{ install, session uuid.UUID }
	starts := map[sessionKey]chstore.EventRow{}
	insertIDs := map[uuid.UUID]bool{}
	installUser := map[uuid.UUID]string{}
	installApp := map[uuid.UUID]uuid.UUID{}
	installPlatform := map[uuid.UUID]string{}
	mainPlatforms := map[string]int{}
	users := map[string]bool{}
	powerDays := map[string]int{}

	for _, r := range rows {
		app, ok := apps[r.AppID]
		require.True(t, ok, "событие неизвестного приложения %s", r.AppID)
		loc, err := time.LoadLocation(app.Timezone)
		require.NoError(t, err)

		require.False(t, insertIDs[r.InsertID], "insert_id повторяется")
		insertIDs[r.InsertID] = true
		require.Contains(t, []string{"session_start", "session_end"}, r.Name)
		require.Contains(t, []string{"production", "development"}, r.Environment)
		require.Contains(t, app.Platforms, r.Platform)
		if prev, ok := installPlatform[r.InstallID]; ok {
			require.Equal(t, prev, r.Platform, "платформа установки не меняется")
		}
		installPlatform[r.InstallID] = r.Platform
		if r.AppID == testutil.PerfApps[0].ID && r.Name == "session_start" {
			mainPlatforms[r.Platform]++
		}
		require.Equal(t, app.WriteKeyID, r.WriteKeyID)
		require.NotEmpty(t, r.AppVersion)
		require.NotEmpty(t, r.OSName)

		require.Equal(t, time.UTC, r.EventTime.Location())
		y, m, d := r.EventTime.Date()
		require.Equal(t, time.Date(y, m, d, 0, 0, 0, 0, time.UTC), r.EventDate, "event_date — UTC-дата event_time")
		require.True(t, r.ClientTime.Equal(r.EventTime))
		require.False(t, r.SentAt.Before(r.EventTime))
		require.True(t, r.ReceivedAt.After(r.SentAt))

		local := r.EventTime.In(loc)
		localDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		if r.Name == "session_start" {
			require.False(t, localDay.Before(cfg.FirstDay()), "старт сессии до окна: %s", local)
			require.False(t, localDay.After(cfg.LastDay), "старт сессии после окна: %s", local)
			require.Zero(t, r.DurationMs)
			starts[sessionKey{r.InstallID, r.SessionID}] = r
		}

		if prev, ok := installApp[r.InstallID]; ok {
			require.Equal(t, prev, r.AppID, "установка в одном приложении")
		}
		installApp[r.InstallID] = r.AppID
		if r.UserID != "" {
			users[r.UserID] = true
			if prev, ok := installUser[r.InstallID]; ok {
				require.Equal(t, prev, r.UserID, "user_id установки не меняется")
			}
			installUser[r.InstallID] = r.UserID
		}
		if r.InstallID == testutil.PerfPowerInstall && r.Name == "session_start" {
			require.Equal(t, testutil.PerfPowerUser, r.UserID)
			powerDays[localDay.Format(time.DateOnly)]++
		}
	}
	for _, r := range rows {
		if r.Name != "session_end" {
			continue
		}
		start, ok := starts[sessionKey{r.InstallID, r.SessionID}]
		require.True(t, ok, "session_end без session_start")
		require.Equal(t, r.EventTime.Sub(start.EventTime).Milliseconds(), int64(r.DurationMs))
		require.Positive(t, r.DurationMs)
		require.Equal(t, start.UserID, r.UserID)
		require.Equal(t, start.AppVersion, r.AppVersion)
	}

	require.Equal(t, len(installApp), stats.Installs)
	require.Equal(t, len(installUser), stats.Links)
	require.Equal(t, len(users), stats.Users)
	require.Len(t, powerDays, cfg.Days, "power user активен каждый день")
	for d, n := range powerDays {
		require.GreaterOrEqual(t, n, 3, "power user: ≥ 3 сессии в день %s", d)
	}
	require.Equal(t, testutil.PerfApps[0].ID, installApp[testutil.PerfPowerInstall])
	powerInstalls := 0
	for id, u := range installUser {
		if u == testutil.PerfPowerUser {
			powerInstalls++
			require.Equal(t, testutil.PerfApps[0].ID, installApp[id])
		}
	}
	require.Equal(t, 2, powerInstalls, "power user на двух установках основного приложения")
	require.Equal(t, "ios", installPlatform[testutil.PerfPowerInstall])

	// Сессии основного приложения: iOS ≈ 70 %, Android ≈ 20 %, Web ≈ 10 %.
	total := mainPlatforms["ios"] + mainPlatforms["android"] + mainPlatforms["web"]
	require.InDelta(t, 0.7, float64(mainPlatforms["ios"])/float64(total), 0.1, "доля iOS")
	require.InDelta(t, 0.2, float64(mainPlatforms["android"])/float64(total), 0.1, "доля Android")
	require.InDelta(t, 0.1, float64(mainPlatforms["web"])/float64(total), 0.07, "доля Web")
}

func TestPerf_EmitErrorStops(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	_, err := testutil.GeneratePerf(smallPerf(200, 5), func([]chstore.EventRow) error {
		calls++
		return boom
	})
	require.ErrorIs(t, err, boom)
	require.Equal(t, 1, calls, "после ошибки генерация прекращается")
}

// Батчи идут по дням: каждый батч — события одного локального дня одного приложения, дни не убывают.
func TestPerf_BatchesByDay(t *testing.T) {
	cfg := smallPerf(200, 5)
	var last time.Time
	batches := 0
	_, err := testutil.GeneratePerf(cfg, func(batch []chstore.EventRow) error {
		batches++
		min := slices.MinFunc(batch, func(a, b chstore.EventRow) int { return a.EventDate.Compare(b.EventDate) })
		require.False(t, min.EventDate.Before(last.AddDate(0, 0, -1)), "UTC-даты батчей не откатываются назад")
		if min.EventDate.After(last) {
			last = min.EventDate
		}
		for _, r := range batch {
			require.Equal(t, batch[0].AppID, r.AppID, "батч одного приложения")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, cfg.Days*len(testutil.PerfApps), batches)
}

func TestPerf_StatsTable(t *testing.T) {
	_, stats := generatePerf(t, smallPerf(200, 3))
	table := testutil.PerfStatsTable(smallPerf(200, 3), stats)
	for _, want := range []string{"seed 42", "2026-10-10 … 2026-10-12", "events", "installs", "links", "users", "avg DAU"} {
		require.Contains(t, table, want)
	}
	for _, app := range testutil.PerfApps {
		require.Contains(t, table, app.Code)
	}
}

func TestPercentile(t *testing.T) {
	ms := func(vs ...int) []time.Duration {
		out := make([]time.Duration, len(vs))
		for i, v := range vs {
			out[i] = time.Duration(v) * time.Millisecond
		}
		return out
	}
	twenty := ms(20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1)
	orig := slices.Clone(twenty)

	tests := []struct {
		name string
		runs []time.Duration
		p    int
		want time.Duration
	}{
		{name: "пусто", runs: nil, p: 95, want: 0},
		{name: "один прогон", runs: ms(7), p: 95, want: 7 * time.Millisecond},
		{name: "P95 из 20 — 19-й по возрастанию", runs: twenty, p: 95, want: 19 * time.Millisecond},
		{name: "P50 из 20 — 10-й", runs: twenty, p: 50, want: 10 * time.Millisecond},
		{name: "P100 — максимум", runs: twenty, p: 100, want: 20 * time.Millisecond},
		{name: "P0 — минимум", runs: twenty, p: 0, want: time.Millisecond},
		{name: "P95 из 3", runs: ms(5, 1, 3), p: 95, want: 5 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, testutil.Percentile(tt.runs, tt.p))
		})
	}
	require.Equal(t, orig, twenty, "входной срез не сортируется на месте")
}

func TestSummarizeRuns(t *testing.T) {
	runs := make([]time.Duration, 20)
	for i := range runs {
		runs[i] = time.Duration(i+1) * 50 * time.Millisecond // 50 мс … 1 с
	}
	got := testutil.SummarizeRuns("portfolio", time.Second, runs)
	require.Equal(t, testutil.PerfResult{
		Name: "portfolio", Runs: 20, ThresholdMs: 1000, P50Ms: 500, P95Ms: 950, MaxMs: 1000, Pass: true,
	}, got)

	got = testutil.SummarizeRuns("portfolio", 950*time.Millisecond, runs)
	require.True(t, got.Pass, "P95 ровно на пороге — проходит")
	got = testutil.SummarizeRuns("portfolio", 949*time.Millisecond, runs)
	require.False(t, got.Pass)

	got = testutil.SummarizeRuns("empty", time.Second, nil)
	require.False(t, got.Pass, "без прогонов — не проходит")
	require.Zero(t, got.Runs)

	got = testutil.SummarizeRuns("sub-ms", time.Second, []time.Duration{1500 * time.Microsecond})
	require.InDelta(t, 1.5, got.P95Ms, 1e-9, "миллисекунды дробные")
}
