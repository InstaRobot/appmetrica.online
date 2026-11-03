package reports

import (
	"bytes"
	"context"
	"errors"
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

// Фикстура своих событий отчёта «События» (testutil.GenerateCustomEvents, testdata/fixtures/README.md). Тесты
// генератора лежат рядом с её потребителем, отчётом «События»: генератор сценария проверяется в testutil.

const customEventsHint = "перегенерируйте: cd backend && go run ./testdata/gen -mode custom-events -seed 42, " +
	"таблицу README — go run ./testdata/gen -mode custom-events -seed 42 -stats"

// buildFixtureGen собирает генератор testdata/gen во временный каталог.
func buildFixtureGen(t *testing.T) string {
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

// runFixtureGen запускает собранный генератор и возвращает stdout, stderr и код выхода.
func runFixtureGen(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
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

// fixtureRows читает NDJSON-фикстуру path.
func fixtureRows(t *testing.T, path string) []chstore.EventRow {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err, customEventsHint)
	defer func() { _ = f.Close() }()
	rows, err := testutil.ParseNDJSON(f, filepath.Base(path))
	require.NoError(t, err)
	return rows
}

// Фикстура своих событий детерминирована и совпадает с закоммиченной; другой seed даёт другой файл.
func TestCustomEvents_Deterministic(t *testing.T) {
	bin := buildFixtureGen(t)
	dir := t.TempDir()
	a, b, other := filepath.Join(dir, "a.ndjson"), filepath.Join(dir, "b.ndjson"), filepath.Join(dir, "c.ndjson")

	for _, out := range []string{a, b} {
		_, stderr, code := runFixtureGen(t, bin, "-mode", "custom-events", "-seed", "42", "-out", out)
		require.Zero(t, code, stderr)
	}
	first, err := os.ReadFile(a)
	require.NoError(t, err)
	second, err := os.ReadFile(b)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.True(t, bytes.Equal(first, second), "два запуска с seed 42 дали разные файлы")

	committed, err := os.ReadFile(testutil.CustomEventsPath())
	require.NoError(t, err, customEventsHint)
	require.True(t, bytes.Equal(committed, first), "custom_events.ndjson устарел: %s", customEventsHint)

	var inProcess bytes.Buffer
	require.NoError(t, testutil.WriteCustomEvents(&inProcess, testutil.ScenarioSeed))
	require.True(t, bytes.Equal(committed, inProcess.Bytes()), "WriteCustomEvents расходится с CLI")

	_, stderr, code := runFixtureGen(t, bin, "-mode", "custom-events", "-seed", "7", "-out", other)
	require.Zero(t, code, stderr)
	third, err := os.ReadFile(other)
	require.NoError(t, err)
	require.False(t, bytes.Equal(first, third), "другой seed должен давать другой файл")
}

func TestCustomEvents_GeneratorUsage(t *testing.T) {
	bin := buildFixtureGen(t)
	for _, args := range [][]string{
		{"-mode", "custom-events", "-dau", "500"},
		{"-mode", "custom-events", "-days", "3"},
		{"-mode", "custom-events", "extra"},
	} {
		_, stderr, code := runFixtureGen(t, bin, args...)
		require.Equal(t, 2, code, "%v: %s", args, stderr)
		require.Contains(t, stderr, "usage", args)
	}

	out := filepath.Join(t.TempDir(), "s.ndjson")
	stdout, stderr, code := runFixtureGen(t, bin, "-mode", "custom-events", "-seed", "42", "-stats", "-out", out)
	require.Zero(t, code, stderr)
	require.Equal(t, testutil.CustomEventsStatsTable(testutil.GenerateCustomEvents(42)), stdout)
	require.NoFileExists(t, out, "-stats не пишет файл")
}

func TestCustomEvents_WriteParseRoundTrip(t *testing.T) {
	generated := testutil.GenerateCustomEvents(testutil.ScenarioSeed)
	require.NotEmpty(t, generated)
	var buf bytes.Buffer
	require.NoError(t, testutil.WriteCustomEvents(&buf, testutil.ScenarioSeed))
	require.True(t, strings.HasPrefix(buf.String(), "#"), "фикстура начинается с комментария о генераторе")
	parsed, err := testutil.ParseNDJSON(&buf, "custom_events.ndjson")
	require.NoError(t, err)
	require.Equal(t, generated, parsed, "все поля, включая properties и insert_id, переживают NDJSON")
}

// Каждое своё событие лежит внутри сессии установки сценария того же seed: те же приложение, установка,
// user_id, environment и контекст устройства, время — между session_start и session_end (у сессии без
// session_end — в первую минуту после старта).
func TestCustomEvents_InsideScenarioSessions(t *testing.T) {
	type scenarioSession struct {
		start chstore.EventRow
		end   time.Time // zero — сессия без session_end
	}
	sessions := map[uuid.UUID]*scenarioSession{}
	for _, r := range fixtureRows(t, testutil.ScenarioPath()) {
		s, ok := sessions[r.SessionID]
		if !ok {
			s = &scenarioSession{}
			sessions[r.SessionID] = s
		}
		switch r.Name {
		case "session_start":
			s.start = r
		case "session_end":
			s.end = r.EventTime
		}
	}

	for _, r := range fixtureRows(t, testutil.CustomEventsPath()) {
		s, ok := sessions[r.SessionID]
		require.True(t, ok, "событие %s вне сессий сценария", r.InsertID)
		want := s.start
		want.EventDate, want.EventTime, want.ClientTime, want.SentAt, want.ReceivedAt = r.EventDate, r.EventTime, r.ClientTime, r.SentAt, r.ReceivedAt
		want.InsertID, want.Name, want.Properties = r.InsertID, r.Name, r.Properties
		require.Equal(t, want, r, "контекст события %s совпадает с его session_start", r.InsertID)

		require.True(t, r.EventTime.After(s.start.EventTime), "событие %s раньше session_start", r.InsertID)
		if s.end.IsZero() {
			require.LessOrEqual(t, r.EventTime.Sub(s.start.EventTime), time.Minute, "событие %s открытой сессии", r.InsertID)
		} else {
			require.True(t, r.EventTime.Before(s.end), "событие %s позже session_end", r.InsertID)
		}
		require.Equal(t, r.EventTime, r.ClientTime)
		require.Equal(t, time.Date(r.EventTime.Year(), r.EventTime.Month(), r.EventTime.Day(), 0, 0, 0, 0, time.UTC), r.EventDate)
		require.False(t, r.SentAt.Before(r.EventTime), "sent_at раньше event_time")
		require.False(t, r.ReceivedAt.Before(r.SentAt), "received_at раньше sent_at")
		require.Zero(t, r.DurationMs)
	}
}

// Сюжет фикстуры (T-CE-005): два приложения, имена каталога FR-102 без системных, повторы insert_id,
// разные платформы и версии, development, plan у 80 % purchase, 25 значений source у note_created.
func TestCustomEvents_Sujet(t *testing.T) {
	rows := fixtureRows(t, testutil.CustomEventsPath())
	catalog := map[string]bool{
		"note_created": true, "note_shared": true, "search_used": true, "paywall_shown": true, "purchase": true,
	}
	apps, platforms, envs := map[uuid.UUID]bool{}, map[string]bool{}, map[string]bool{}
	versions := map[uuid.UUID]map[string]bool{}
	byInsert := map[uuid.UUID][]chstore.EventRow{}
	for _, r := range rows {
		require.True(t, catalog[r.Name], "имя %q не из каталога", r.Name)
		apps[r.AppID] = true
		platforms[r.Platform] = true
		envs[r.Environment] = true
		if versions[r.AppID] == nil {
			versions[r.AppID] = map[string]bool{}
		}
		versions[r.AppID][r.AppVersion] = true
		byInsert[r.InsertID] = append(byInsert[r.InsertID], r)
	}
	require.Len(t, apps, 2)
	require.Len(t, platforms, 2)
	require.Equal(t, map[string]bool{"production": true, "development": true}, envs)
	for app, vs := range versions {
		require.GreaterOrEqual(t, len(vs), 2, "у приложения %s несколько версий", app)
	}

	// Повтор батча: строки с тем же insert_id отличаются только received_at.
	dups, triples := 0, 0
	unique := make([]chstore.EventRow, 0, len(byInsert))
	for _, g := range byInsert {
		unique = append(unique, g[0])
		if len(g) == 1 {
			continue
		}
		dups++
		if len(g) == 3 {
			triples++
		}
		for _, r := range g[1:] {
			a, b := g[0], r
			a.ReceivedAt, b.ReceivedAt = time.Time{}, time.Time{}
			require.Equal(t, a, b)
		}
	}
	require.Positive(t, dups, "в фикстуре есть повторно доставленные события")
	require.Positive(t, triples, "свои события повторённого трижды батча msk 05")

	// plan — у 80 % purchase (по уникальным событиям); у note_created 25 значений source, часть — без source.
	var purchases, withPlan, notes, withSource int
	sources := map[string]bool{}
	for _, r := range unique {
		switch r.Name {
		case "purchase":
			purchases++
			if _, ok := r.Properties["plan"]; ok {
				withPlan++
			}
		case "note_created":
			notes++
			if v, ok := r.Properties["source"]; ok {
				withSource++
				sources[v] = true
			}
		}
	}
	require.Positive(t, purchases)
	require.InDelta(t, 0.8, float64(withPlan)/float64(purchases), 0.02, "plan у %d из %d purchase", withPlan, purchases)
	require.Len(t, sources, 25)
	require.Positive(t, notes-withSource, "есть note_created без source")
}

func TestCustomEvents_READMETableUpToDate(t *testing.T) {
	table := testutil.CustomEventsStatsTable(fixtureRows(t, testutil.CustomEventsPath()))
	require.True(t, strings.HasPrefix(table, "| Приложение |"), table)
	readme, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), "README.md"))
	require.NoError(t, err)
	require.Contains(t, string(readme), table, "таблица своих событий в README устарела: %s", customEventsHint)
}
