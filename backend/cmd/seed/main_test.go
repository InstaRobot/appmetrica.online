package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
)

var testToday = time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)

func testConfig(t *testing.T) genConfig {
	t.Helper()
	loc, err := time.LoadLocation(seedTimezone)
	require.NoError(t, err)
	return genConfig{
		Days:          35,
		DailyInstalls: 400,
		Seed:          1,
		Today:         testToday,
		Location:      loc,
	}
}

var testApp = seedApp{
	ID:         uuid.MustParse("0199e0a0-0000-7000-8000-000000000001"),
	Platforms:  []string{"ios"},
	WriteKeyID: uuid.MustParse("0199e0a0-0000-7000-8000-0000000000aa"),
}

func TestGenerateApp_Deterministic(t *testing.T) {
	cfg := testConfig(t)
	cfg.Days, cfg.DailyInstalls = 5, 30

	first := generateApp(cfg, 0, testApp)
	require.NotEmpty(t, first)
	require.Equal(t, first, generateApp(cfg, 0, testApp), "один seed — одинаковые строки (повторный make seed идемпотентен)")

	cfg.Seed = 2
	require.NotEqual(t, first, generateApp(cfg, 0, testApp), "другой seed — другие данные")
	cfg.Seed = 1
	require.NotEqual(t, first, generateApp(cfg, 1, testApp), "у разных приложений разные установки")
}

// installStats — первый день и дни активности установки в TZ приложения.
type installStats struct {
	first  time.Time
	active map[time.Time]bool
	user   string
}

func TestGenerateApp_Shape(t *testing.T) {
	cfg := testConfig(t)
	rows := generateApp(cfg, 0, testApp)

	installs := map[uuid.UUID]*installStats{}
	sessions := map[uuid.UUID][]chstore.EventRow{}
	insertIDs := map[uuid.UUID]bool{}
	firstDay := testToday.AddDate(0, 0, -cfg.Days)
	for _, r := range rows {
		require.Equal(t, testApp.ID, r.AppID)
		require.Equal(t, testApp.WriteKeyID, r.WriteKeyID)
		require.Equal(t, "production", r.Environment)
		require.Equal(t, "ios", r.Platform)
		require.NotEmpty(t, r.Country)
		require.True(t, r.EventTime.Before(testToday), "сегодняшних событий нет: %s", r.EventTime)
		require.False(t, r.EventTime.Before(firstDay.Add(-3*time.Hour)), "событие раньше окна: %s", r.EventTime)
		require.Equal(t, r.EventTime.UTC().Truncate(24*time.Hour), r.EventDate, "event_date — UTC-дата event_time")
		require.False(t, r.ReceivedAt.Before(r.EventTime))
		require.False(t, insertIDs[r.InsertID], "insert_id уникален")
		insertIDs[r.InsertID] = true

		day := localDay(r.EventTime, cfg.Location)
		st := installs[r.InstallID]
		if st == nil {
			st = &installStats{first: day, active: map[time.Time]bool{}, user: r.UserID}
			installs[r.InstallID] = st
		}
		if day.Before(st.first) {
			st.first = day
		}
		st.active[day] = true
		require.Equal(t, st.user, r.UserID, "user_id установки постоянен")
		sessions[r.SessionID] = append(sessions[r.SessionID], r)
	}

	t.Run("daily new installs", func(t *testing.T) {
		perDay := map[time.Time]int{}
		for _, st := range installs {
			perDay[st.first]++
		}
		require.Len(t, perDay, cfg.Days)
		for day, n := range perDay {
			require.Equal(t, cfg.DailyInstalls, n, day.Format(time.DateOnly))
		}
	})

	t.Run("sessions are start/end pairs within one local day", func(t *testing.T) {
		for id, evs := range sessions {
			require.Len(t, evs, 2, id.String())
			start, end := evs[0], evs[1]
			require.Equal(t, "session_start", start.Name)
			require.Equal(t, "session_end", end.Name)
			require.Equal(t, start.InstallID, end.InstallID)
			require.Positive(t, end.DurationMs)
			require.Equal(t, start.EventTime.Add(time.Duration(end.DurationMs)*time.Millisecond), end.EventTime)
			require.Equal(t, localDay(start.EventTime, cfg.Location), localDay(end.EventTime, cfg.Location))
		}
	})

	t.Run("retention curve D1 about 35 percent, D7 about 15 percent", func(t *testing.T) {
		d1 := retention(installs, 1, testToday, cfg.Location)
		d7 := retention(installs, 7, testToday, cfg.Location)
		require.InDelta(t, 0.35, d1, 0.03, "D1")
		require.InDelta(t, 0.15, d7, 0.03, "D7")
	})

	t.Run("user ids", func(t *testing.T) {
		withUser := 0
		devices := map[string]int{}
		for _, st := range installs {
			if st.user != "" {
				withUser++
				devices[st.user]++
			}
		}
		share := float64(withUser) / float64(len(installs))
		require.InDelta(t, 0.20, share, 0.02, "20 %% установок с user_id")
		second := 0
		for _, n := range devices {
			if n > 1 {
				second += n - 1
			}
		}
		require.InDelta(t, 0.05, float64(second)/float64(len(installs)), 0.015, "5 %% установок — второе устройство")
	})
}

// «Seed Notes» — одно приложение на iOS, Android и Web: у каждой установки своя платформа, около 5 % людей
// с одним user_id на iOS и Web (пересечение для долей портфеля и итога «люди сквозь платформы»).
func TestGenerateApp_MultiPlatform(t *testing.T) {
	cfg := testConfig(t)
	notes := seedApp{
		ID:        uuid.MustParse("0199e0a0-0000-7000-8000-000000000004"),
		Platforms: []string{"ios", "android", "web"},
	}
	rows := generateApp(cfg, 3, notes)
	require.Equal(t, rows, generateApp(cfg, 3, notes), "генератор детерминирован")

	osByPlatform := map[string]map[string]bool{}
	for p, models := range devices {
		osByPlatform[p] = map[string]bool{}
		for _, d := range models {
			osByPlatform[p][d.osName] = true
		}
	}
	installPlatform := map[uuid.UUID]string{}
	userPlatforms := map[string]map[string]bool{}
	installsPerPlatform := map[string]int{}
	anonymous := 0
	for _, r := range rows {
		require.Equal(t, notes.ID, r.AppID)
		require.Equal(t, uuid.Nil, r.WriteKeyID, "у Seed Notes нет ключа")
		require.Contains(t, notes.Platforms, r.Platform)
		require.True(t, osByPlatform[r.Platform][r.OSName], "ОС %q у платформы %q", r.OSName, r.Platform)
		if p, ok := installPlatform[r.InstallID]; ok {
			require.Equal(t, p, r.Platform, "платформа установки постоянна")
			continue
		}
		installPlatform[r.InstallID] = r.Platform
		installsPerPlatform[r.Platform]++
		if r.UserID == "" {
			anonymous++
			continue
		}
		if userPlatforms[r.UserID] == nil {
			userPlatforms[r.UserID] = map[string]bool{}
		}
		userPlatforms[r.UserID][r.Platform] = true
	}

	for _, p := range notes.Platforms {
		require.Greater(t, installsPerPlatform[p], len(installPlatform)/10, "платформа %s заметна", p)
	}
	cross := 0
	for user, ps := range userPlatforms {
		if len(ps) > 1 {
			require.Equal(t, map[string]bool{"ios": true, "web": true}, ps, "сквозной человек %s — iOS и Web", user)
			cross++
		}
	}
	persons := anonymous + len(userPlatforms)
	require.InDelta(t, 0.05, float64(cross)/float64(persons), 0.012, "около 5 %% людей на iOS и Web")
}

func localDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// retention — классический day-N: доля установок когорты, активных ровно на N-й день.
func retention(installs map[uuid.UUID]*installStats, n int, today time.Time, loc *time.Location) float64 {
	lastClosed := localDay(today, loc).AddDate(0, 0, -1) // данные — по вчерашний день в TZ приложения
	cohort, retained := 0, 0
	for _, st := range installs {
		day := st.first.AddDate(0, 0, n)
		if day.After(lastClosed) {
			continue
		}
		cohort++
		if st.active[day] {
			retained++
		}
	}
	return float64(retained) / float64(cohort)
}

func TestParseOptions(t *testing.T) {
	const key = "pk_devseedkey0000000000000000000000"
	cases := []struct {
		name string
		args []string
		ok   bool
		want options
	}{
		{"defaults", []string{"--write-key", key}, true, options{Apps: 4, Days: 35, DailyInstalls: 400, WriteKey: key, Seed: 1}},
		{
			"all flags",
			[]string{"--apps", "1", "--days", "3", "--daily-installs", "10", "--seed", "7", "--write-key", key},
			true,
			options{Apps: 1, Days: 3, DailyInstalls: 10, WriteKey: key, Seed: 7},
		},
		{"missing key", nil, false, options{}},
		{"bad key format", []string{"--write-key", "pk_short"}, false, options{}},
		{"secret key instead of write key", []string{"--write-key", "sk_devseedkey0000000000000000000000"}, false, options{}},
		{"too many apps", []string{"--apps", "5", "--write-key", key}, false, options{}},
		{"zero apps", []string{"--apps", "0", "--write-key", key}, false, options{}},
		{"zero days", []string{"--days", "0", "--write-key", key}, false, options{}},
		{"zero installs", []string{"--daily-installs", "0", "--write-key", key}, false, options{}},
		{"unknown flag", []string{"--write-key", key, "--nope"}, false, options{}},
		{"positional args", []string{"--write-key", key, "extra"}, false, options{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, ok := parseOptions(tc.args, &stderr)
			require.Equal(t, tc.ok, ok, stderr.String())
			if tc.ok {
				require.Equal(t, tc.want, got)
			} else {
				require.NotEmpty(t, stderr.String(), "ошибка объяснена")
			}
		})
	}
}

func TestRun_ExitCodes(t *testing.T) {
	t.Setenv("POSTGRES_URL", "")
	clk := clock.NewFixed(testToday)
	var stdout, stderr bytes.Buffer
	require.Equal(t, 2, run(t.Context(), []string{"--apps", "9"}, &stdout, &stderr, clk))
	stderr.Reset()
	require.Equal(t, 1, run(t.Context(), []string{"--write-key", "pk_devseedkey0000000000000000000000"}, &stdout, &stderr, clk),
		"без POSTGRES_URL — ошибка выполнения")
	require.Contains(t, stderr.String(), "POSTGRES_URL")
}
