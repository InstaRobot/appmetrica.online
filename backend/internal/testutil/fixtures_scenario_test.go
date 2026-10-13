package testutil_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/testutil"
)

const regenerateHint = "перегенерируйте: cd backend && go run ./testdata/gen -seed 42, таблицу README — go run ./testdata/gen -seed 42 -stats"

// buildGen собирает генератор testdata/gen во временный каталог.
func buildGen(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	require.NoError(t, err, "для теста генератора нужен go в PATH")
	bin := filepath.Join(t.TempDir(), "gen")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "build", "-o", bin, "./testdata/gen")
	cmd.Dir = testutil.BackendDir()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build ./testdata/gen: %s", out)
	return bin
}

// runGen запускает собранный генератор и возвращает stdout, stderr и код выхода.
func runGen(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Dir = testutil.BackendDir()
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		require.NoError(t, err)
	}
	return outBuf.String(), errBuf.String(), code
}

func loadScenario(t *testing.T) []chstore.EventRow {
	t.Helper()
	f, err := os.Open(testutil.ScenarioPath())
	require.NoError(t, err, regenerateHint)
	defer func() { _ = f.Close() }()
	rows, err := testutil.ParseNDJSON(f, "scenario.ndjson")
	require.NoError(t, err)
	return rows
}

func TestFixtures_Deterministic(t *testing.T) {
	bin := buildGen(t)
	dir := t.TempDir()
	a, b, other := filepath.Join(dir, "a.ndjson"), filepath.Join(dir, "b.ndjson"), filepath.Join(dir, "c.ndjson")

	for _, out := range []string{a, b} {
		_, stderr, code := runGen(t, bin, "-seed", "42", "-out", out)
		require.Zero(t, code, stderr)
	}
	first, err := os.ReadFile(a)
	require.NoError(t, err)
	second, err := os.ReadFile(b)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.True(t, bytes.Equal(first, second), "два запуска с seed 42 дали разные файлы")

	committed, err := os.ReadFile(testutil.ScenarioPath())
	require.NoError(t, err, regenerateHint)
	require.True(t, bytes.Equal(committed, first), "scenario.ndjson устарел: %s", regenerateHint)

	var inProcess bytes.Buffer
	require.NoError(t, testutil.WriteScenario(&inProcess, testutil.ScenarioSeed))
	require.True(t, bytes.Equal(committed, inProcess.Bytes()), "WriteScenario расходится с CLI")

	_, stderr, code := runGen(t, bin, "-seed", "7", "-out", other)
	require.Zero(t, code, stderr)
	third, err := os.ReadFile(other)
	require.NoError(t, err)
	require.False(t, bytes.Equal(first, third), "другой seed должен давать другой файл")
}

func TestFixtures_GeneratorUsage(t *testing.T) {
	bin := buildGen(t)
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "seed не число", args: []string{"-seed", "x"}, wantCode: 2, wantErr: "usage"},
		{name: "отрицательный seed", args: []string{"-seed", "-1"}, wantCode: 2, wantErr: "usage"},
		{name: "лишний аргумент", args: []string{"-seed", "42", "extra"}, wantCode: 2, wantErr: "usage"},
		{name: "неизвестная подкоманда", args: []string{"perf"}, wantCode: 2, wantErr: "usage"},
		{name: "неизвестный флаг", args: []string{"-days", "3"}, wantCode: 2, wantErr: "usage"},
		{name: "каталог вывода — файл", args: []string{"-out", filepath.Join(blocker, "sub", "s.ndjson")}, wantCode: 1, wantErr: "gen:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runGen(t, bin, tt.args...)
			require.Equal(t, tt.wantCode, code, stderr)
			require.Contains(t, stderr, tt.wantErr)
		})
	}

	t.Run("stats печатает таблицу README и не пишет файл", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "s.ndjson")
		stdout, stderr, code := runGen(t, bin, "-seed", "42", "-stats", "-out", out)
		require.Zero(t, code, stderr)
		want, err := testutil.ScenarioStatsTable(testutil.GenerateScenario(42))
		require.NoError(t, err)
		require.Equal(t, want, stdout)
		require.NoFileExists(t, out)
	})
}

func TestScenario_WriteParseRoundTrip(t *testing.T) {
	generated := testutil.GenerateScenario(testutil.ScenarioSeed)
	require.NotEmpty(t, generated)
	var buf bytes.Buffer
	require.NoError(t, testutil.WriteScenario(&buf, testutil.ScenarioSeed))
	require.True(t, strings.HasPrefix(buf.String(), "#"), "фикстура начинается с комментария о генераторе")
	parsed, err := testutil.ParseNDJSON(&buf, "scenario.ndjson")
	require.NoError(t, err)
	require.Equal(t, generated, parsed, "все поля, включая insert_id, переживают NDJSON")
}

// TestScenario_Invariants проверяет состав сценария без ClickHouse (то же, что AC-026.1, и особые случаи).
func TestScenario_Invariants(t *testing.T) {
	require.Len(t, testutil.ScenarioApps, 3)
	rows := loadScenario(t)

	appByID := map[uuid.UUID]testutil.ScenarioApp{}
	tzs := map[string]bool{}
	for _, a := range testutil.ScenarioApps {
		appByID[a.ID] = a
		tzs[a.Timezone] = true
	}
	require.Equal(t, map[string]bool{"Europe/Moscow": true, "America/New_York": true, "Asia/Tokyo": true}, tzs)

	type sessKey struct{ app, install, session uuid.UUID }
	type dupKey struct {
		app     uuid.UUID
		date    time.Time
		install uuid.UUID
		insert  uuid.UUID
	}
	apps := map[uuid.UUID]bool{}
	installs := map[uuid.UUID]uuid.UUID{}
	installUsers := map[uuid.UUID]map[string]bool{}
	users := map[string]bool{}
	sessionOwner := map[uuid.UUID]uuid.UUID{}
	starts := map[sessKey]chstore.EventRow{}
	ends := map[sessKey][]chstore.EventRow{}
	dups := map[dupKey][]chstore.EventRow{}
	devApps := map[uuid.UUID]bool{}
	late := 0
	for _, r := range rows {
		app, ok := appByID[r.AppID]
		require.True(t, ok, "неизвестный app_id %s", r.AppID)
		apps[r.AppID] = true
		if owner, seen := installs[r.InstallID]; seen {
			require.Equal(t, owner, r.AppID, "установка %s в двух приложениях", r.InstallID)
		}
		installs[r.InstallID] = r.AppID
		if s, seen := sessionOwner[r.SessionID]; seen {
			require.Equal(t, s, r.InstallID, "сессия %s в двух установках", r.SessionID)
		}
		sessionOwner[r.SessionID] = r.InstallID
		if r.UserID != "" {
			users[r.UserID] = true
			if installUsers[r.InstallID] == nil {
				installUsers[r.InstallID] = map[string]bool{}
			}
			installUsers[r.InstallID][r.UserID] = true
		}
		if r.ReceivedAt.Sub(r.EventTime) > 5*24*time.Hour {
			late++
		}
		if r.Environment == "development" {
			devApps[r.AppID] = true
		} else {
			require.Equal(t, "production", r.Environment)
		}
		require.Equal(t, app.Platform, r.Platform)
		require.False(t, r.ReceivedAt.Before(r.SentAt), "received_at раньше sent_at")
		require.False(t, r.SentAt.Before(r.EventTime), "sent_at раньше event_time")

		loc, err := time.LoadLocation(app.Timezone)
		require.NoError(t, err)
		local := r.EventTime.In(loc)
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		require.False(t, day.Before(testutil.ScenarioFirstDay) || day.After(testutil.ScenarioLastDay),
			"событие %s вне окна сценария", local)

		k := sessKey{r.AppID, r.InstallID, r.SessionID}
		switch r.Name {
		case "session_start":
			require.Zero(t, r.DurationMs)
			if prev, seen := starts[k]; seen {
				require.Equal(t, prev.InsertID, r.InsertID, "второй session_start сессии — только повтор батча")
			}
			starts[k] = r
		case "session_end":
			require.Positive(t, r.DurationMs)
			ends[k] = append(ends[k], r)
		default:
			t.Fatalf("неизвестное событие %q", r.Name)
		}
		dk := dupKey{r.AppID, r.EventDate, r.InstallID, r.InsertID}
		dups[dk] = append(dups[dk], r)
	}

	// AC-026.1 на стороне Go.
	require.Len(t, apps, 3)
	require.Len(t, installs, 40)
	require.Len(t, users, 12)
	multi := 0
	for _, us := range installUsers {
		if len(us) > 1 {
			multi++
			require.Len(t, us, 2)
		}
	}
	require.Equal(t, 1, multi, "ровно одна установка с двумя user_id (общий девайс)")
	require.Positive(t, late, "досыл: received_at − event_time > 5 дней")

	// Дубли батчей: повтор с тем же insert_id отличается только received_at.
	dupGroups := 0
	for _, g := range dups {
		if len(g) == 1 {
			continue
		}
		dupGroups++
		require.LessOrEqual(t, len(g), 3)
		for _, r := range g[1:] {
			a, b := g[0], r
			a.ReceivedAt, b.ReceivedAt = time.Time{}, time.Time{}
			require.Equal(t, a, b)
		}
	}
	require.Positive(t, dupGroups, "в сценарии есть повторно отправленный батч")

	// Development-трафик в каждом приложении.
	require.Len(t, devApps, 3)

	// Сессии: у каждого session_end есть session_start, длительность согласована со временем;
	// есть сессии без session_end и сессии через полночь в каждом приложении.
	midnight := map[uuid.UUID]bool{}
	for k, es := range ends {
		start, ok := starts[k]
		require.True(t, ok, "session_end без session_start: %v", k)
		for _, e := range es {
			require.Equal(t, time.Duration(e.DurationMs)*time.Millisecond, e.EventTime.Sub(start.EventTime))
			loc, err := time.LoadLocation(appByID[k.app].Timezone)
			require.NoError(t, err)
			if start.EventTime.In(loc).Day() != e.EventTime.In(loc).Day() {
				midnight[k.app] = true
			}
		}
	}
	require.Less(t, len(ends), len(starts), "есть сессии без session_end")
	require.Len(t, midnight, 3, "сессия через полночь в TZ каждого приложения")

	// DST America/New_York 2026-11-01: события в повторяющемся часе 01:00–02:00 по обе стороны перехода.
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	zones := map[string]bool{}
	for _, r := range rows {
		if appByID[r.AppID].Timezone != "America/New_York" || r.Environment != "production" {
			continue
		}
		local := r.EventTime.In(ny)
		if local.Format(time.DateOnly) == "2026-11-01" && local.Hour() == 1 {
			name, _ := local.Zone()
			zones[name] = true
		}
	}
	require.Equal(t, map[string]bool{"EDT": true, "EST": true}, zones)
}

func TestScenario_READMETableUpToDate(t *testing.T) {
	table, err := testutil.ScenarioStatsTable(loadScenario(t))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(table, "| День |"), table)
	readme, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), "README.md"))
	require.NoError(t, err)
	require.Contains(t, string(readme), table, "таблица ожидаемых чисел в README устарела: %s", regenerateHint)
}

// statsRow — короткая запись события для проверки определений метрик.
func statsRow(app testutil.ScenarioApp, install, user, env, at, name string) chstore.EventRow {
	ts, err := time.Parse(time.RFC3339, at)
	if err != nil {
		panic(err)
	}
	ts = ts.UTC()
	installID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("install:"+install))
	return chstore.EventRow{
		AppID:       app.ID,
		EventDate:   time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC),
		EventTime:   ts,
		ClientTime:  ts,
		SentAt:      ts,
		ReceivedAt:  ts,
		InsertID:    uuid.NewSHA1(uuid.NameSpaceOID, []byte(install+user+env+at+name)),
		Name:        name,
		InstallID:   installID,
		UserID:      user,
		SessionID:   uuid.NewSHA1(uuid.NameSpaceOID, []byte("session:"+install+at)),
		Environment: env,
		Platform:    app.Platform,
	}
}

func TestScenarioStats_Definitions(t *testing.T) {
	require.Len(t, testutil.ScenarioApps, 3)
	msk := testutil.ScenarioApps[0]
	require.Equal(t, "Europe/Moscow", msk.Timezone)
	const prod, dev, start = "production", "development", "session_start"

	type want struct{ active, newcomers, d1 int }
	tests := []struct {
		name string
		rows []chstore.EventRow
		want map[string]want
	}{
		{
			name: "анонимные дни приписываются первому владельцу",
			rows: []chstore.EventRow{
				statsRow(msk, "a", "", prod, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "a", "", prod, "2026-10-02T09:00:00Z", start),
				statsRow(msk, "a", "U", prod, "2026-10-03T09:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {1, 1, 1}, "2026-10-02": {1, 0, 0}, "2026-10-03": {1, 0, 0}},
		},
		{
			name: "общий девайс: анонимные сессии первого владельца, второй — новый человек",
			rows: []chstore.EventRow{
				statsRow(msk, "a", "A", prod, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "a", "", prod, "2026-10-02T09:00:00Z", start),
				statsRow(msk, "a", "B", prod, "2026-10-02T12:00:00Z", start),
				statsRow(msk, "a", "", prod, "2026-10-03T09:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {1, 1, 1}, "2026-10-02": {2, 1, 0}, "2026-10-03": {1, 0, 0}},
		},
		{
			name: "пользователь на двух устройствах — один человек",
			rows: []chstore.EventRow{
				statsRow(msk, "a", "U", prod, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "b", "U", prod, "2026-10-01T10:00:00Z", start),
				statsRow(msk, "b", "U", prod, "2026-10-02T10:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {1, 1, 1}, "2026-10-02": {1, 0, 0}},
		},
		{
			name: "день считается в TZ приложения",
			rows: []chstore.EventRow{
				statsRow(msk, "a", "", prod, "2026-09-30T21:30:00Z", start),
			},
			want: map[string]want{"2026-09-30": {0, 0, 0}, "2026-10-01": {1, 1, 0}},
		},
		{
			name: "development и дубли не влияют",
			rows: []chstore.EventRow{
				statsRow(msk, "a", "", prod, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "a", "", prod, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "d", "", dev, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "d", "", dev, "2026-10-02T09:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {1, 1, 0}, "2026-10-02": {0, 0, 0}},
		},
		{
			name: "первое появление в development не делает человека старым в production",
			rows: []chstore.EventRow{
				statsRow(msk, "d", "U", dev, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "a", "U", prod, "2026-10-03T09:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {0, 0, 0}, "2026-10-03": {1, 1, 0}},
		},
		{
			name: "D1 — доля новых, вернувшихся на следующий день",
			rows: []chstore.EventRow{
				statsRow(msk, "a", "", prod, "2026-10-01T09:00:00Z", start),
				statsRow(msk, "b", "", prod, "2026-10-01T10:00:00Z", start),
				statsRow(msk, "a", "", prod, "2026-10-02T09:00:00Z", start),
				statsRow(msk, "b", "", prod, "2026-10-03T09:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {2, 2, 1}, "2026-10-02": {1, 0, 0}, "2026-10-03": {1, 0, 0}},
		},
		{
			name: "события другого приложения не учитываются",
			rows: []chstore.EventRow{
				statsRow(testutil.ScenarioApps[1], "x", "", prod, "2026-10-01T15:00:00Z", start),
			},
			want: map[string]want{"2026-10-01": {0, 0, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := testutil.ScenarioStats(tt.rows, msk)
			require.NoError(t, err)
			wantDays := int(testutil.ScenarioLastDay.Sub(testutil.ScenarioFirstDay).Hours()/24) + 1
			require.Len(t, days, wantDays, "по строке на каждый день окна")
			byDay := map[string]testutil.ScenarioDay{}
			for _, d := range days {
				byDay[d.Day] = d
			}
			for day, w := range tt.want {
				got, ok := byDay[day]
				require.True(t, ok, day)
				gotW := want{got.Active, got.New, got.D1Returned}
				require.Equal(t, w, gotW, day)
			}
		})
	}
}

func TestScenarioStats_Errors(t *testing.T) {
	require.NotEmpty(t, testutil.ScenarioApps)
	bad := testutil.ScenarioApps[0]
	bad.Timezone = "Mars/Olympus"
	_, err := testutil.ScenarioStats(nil, bad)
	require.Error(t, err)

	empty, err := testutil.ScenarioStats(nil, testutil.ScenarioApps[0])
	require.NoError(t, err)
	for _, d := range empty {
		require.Equal(t, testutil.ScenarioDay{Day: d.Day}, d)
	}

	table, err := testutil.ScenarioStatsTable(nil)
	require.NoError(t, err)
	require.Contains(t, table, fmt.Sprintf("| %s |", testutil.ScenarioFirstDay.Format(time.DateOnly)))
	require.NotContains(t, table, "/", "без новых D1 не показывается дробью")
}
