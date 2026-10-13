package testutil_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// fakeT записывает провалы вместо остановки настоящего теста; FailNow завершает горутину, как testing.T.
type fakeT struct {
	testing.TB
	failed bool
	msgs   []string
}

func (f *fakeT) Helper() {}

func (f *fakeT) Name() string { return "fake" }

func (f *fakeT) Errorf(format string, args ...any) {
	f.failed = true
	f.msgs = append(f.msgs, fmt.Sprintf(format, args...))
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.Errorf(format, args...)
	f.FailNow()
}

func (f *fakeT) FailNow() {
	f.failed = true
	runtime.Goexit()
}

func runFake(fn func(t testing.TB)) *fakeT {
	f := &fakeT{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(f)
	}()
	<-done
	return f
}

type report struct {
	Active int      `json:"active"`
	Days   []string `json:"days"`
}

func TestGolden_UpdateAndDiff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fixtures")
	path := filepath.Join(dir, "x.golden.json")
	got := report{Active: 12, Days: []string{"2026-10-12"}}
	const want = "{\n  \"active\": 12,\n  \"days\": [\n    \"2026-10-12\"\n  ]\n}\n"

	t.Run("update writes file", func(t *testing.T) {
		golden := testutil.Golden{Dir: dir, Update: true}
		golden.Assert(t, "x.golden.json", got)

		b, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want, string(b)) //nolint:testifylint // проверяется точный формат файла, а не JSON-эквивалентность
	})

	t.Run("update overwrites stale file", func(t *testing.T) {
		require.NoError(t, os.WriteFile(path, []byte("{\"active\": 1}\n"), 0o600))
		golden := testutil.Golden{Dir: dir, Update: true}
		golden.Assert(t, "x.golden.json", got)

		b, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want, string(b)) //nolint:testifylint // проверяется точный формат файла, а не JSON-эквивалентность
	})

	t.Run("match passes", func(t *testing.T) {
		golden := testutil.Golden{Dir: dir}
		f := runFake(func(ft testing.TB) { golden.Assert(ft, "x.golden.json", got) })
		require.False(t, f.failed, f.msgs)
	})

	t.Run("raw JSON is canonicalized", func(t *testing.T) {
		golden := testutil.Golden{Dir: dir}
		raw := []byte(`{"days":["2026-10-12"],"active":12}`)
		f := runFake(func(ft testing.TB) { golden.Assert(ft, "x.golden.json", raw) })
		require.False(t, f.failed, f.msgs)
	})

	t.Run("mismatch fails with unified diff", func(t *testing.T) {
		golden := testutil.Golden{Dir: dir}
		f := runFake(func(ft testing.TB) {
			golden.Assert(ft, "x.golden.json", report{Active: 13, Days: []string{"2026-10-12"}})
		})
		require.True(t, f.failed)
		msg := strings.Join(f.msgs, "\n")
		require.Contains(t, msg, "--- Expected")
		require.Contains(t, msg, "+++ Actual")
		require.Contains(t, msg, "@@ ")
		require.Contains(t, msg, "-  \"active\": 12,")
		require.Contains(t, msg, "+  \"active\": 13,")
		require.Contains(t, msg, "x.golden.json")
		require.Contains(t, msg, "-update")

		b, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want, string(b), "без -update эталон не меняется") //nolint:testifylint // точный формат файла
	})

	t.Run("missing golden fails with hint", func(t *testing.T) {
		golden := testutil.Golden{Dir: dir}
		f := runFake(func(ft testing.TB) { golden.Assert(ft, "absent.golden.json", got) })
		require.True(t, f.failed)
		msg := strings.Join(f.msgs, "\n")
		require.Contains(t, msg, "absent.golden.json")
		require.Contains(t, msg, "-update")
		require.NoFileExists(t, filepath.Join(dir, "absent.golden.json"))
	})

	t.Run("invalid raw JSON fails", func(t *testing.T) {
		golden := testutil.Golden{Dir: dir}
		f := runFake(func(ft testing.TB) { golden.Assert(ft, "x.golden.json", []byte(`{"active":`)) })
		require.True(t, f.failed)
	})

	t.Run("default dir is backend/testdata/fixtures", func(t *testing.T) {
		require.FileExists(t, filepath.Join(testutil.FixturesDir(), "smoke.ndjson"))
	})
}

func TestParseNDJSON(t *testing.T) {
	appID := uuid.MustParse("0b6f1c1e-5d1a-4a53-9a47-2f0f6f7f4a01")
	installID := uuid.MustParse("6a1d6a0e-9c55-4c1b-8a3e-9b1e2f6c7d01")
	sessionID := uuid.MustParse("c3a4d9f2-1b2c-4d5e-8f90-a1b2c3d4e501")
	line := fmt.Sprintf(`{"app_id":%q,"event_time":"2026-10-12T21:30:00.250Z","install_id":%q,`+
		`"session_id":%q,"name":"session_end","duration_ms":60000,"user_id":"u1","app_version":"3.4.0","country":"RU"}`,
		appID, installID, sessionID)

	t.Run("comments, blank lines and defaults", func(t *testing.T) {
		src := "# day | person | event\n\n" + line + "\n"
		rows, err := testutil.ParseNDJSON(strings.NewReader(src), "x.ndjson")
		require.NoError(t, err)
		require.Len(t, rows, 1)

		r := rows[0]
		at := time.Date(2026, 10, 12, 21, 30, 0, 250_000_000, time.UTC)
		require.Equal(t, appID, r.AppID)
		require.Equal(t, installID, r.InstallID)
		require.Equal(t, sessionID, r.SessionID)
		require.Equal(t, "session_end", r.Name)
		require.Equal(t, uint32(60000), r.DurationMs)
		require.Equal(t, "u1", r.UserID)
		require.Equal(t, "3.4.0", r.AppVersion)
		require.Equal(t, "RU", r.Country)
		require.True(t, at.Equal(r.EventTime))
		require.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), r.EventDate)
		require.True(t, at.Equal(r.ClientTime))
		require.True(t, at.Equal(r.SentAt))
		require.True(t, at.Equal(r.ReceivedAt))
		require.Equal(t, "production", r.Environment)
		require.NotEqual(t, uuid.Nil, r.InsertID)
	})

	t.Run("insert_id is deterministic per source line", func(t *testing.T) {
		src := line + "\n" + line + "\n"
		a, err := testutil.ParseNDJSON(strings.NewReader(src), "x.ndjson")
		require.NoError(t, err)
		b, err := testutil.ParseNDJSON(strings.NewReader(src), "x.ndjson")
		require.NoError(t, err)
		require.Equal(t, a[0].InsertID, b[0].InsertID)
		require.NotEqual(t, a[0].InsertID, a[1].InsertID)
	})

	t.Run("event_time in other offset is stored as UTC date", func(t *testing.T) {
		src := strings.Replace(line, "2026-10-12T21:30:00.250Z", "2026-10-13T00:30:00.250+03:00", 1)
		rows, err := testutil.ParseNDJSON(strings.NewReader(src), "x.ndjson")
		require.NoError(t, err)
		require.Equal(t, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), rows[0].EventDate)
	})

	t.Run("explicit received_at and environment", func(t *testing.T) {
		src := strings.Replace(line, `"country":"RU"`,
			`"country":"RU","received_at":"2026-10-12T21:31:00Z","environment":"development"`, 1)
		rows, err := testutil.ParseNDJSON(strings.NewReader(src), "x.ndjson")
		require.NoError(t, err)
		require.True(t, time.Date(2026, 10, 12, 21, 31, 0, 0, time.UTC).Equal(rows[0].ReceivedAt))
		require.Equal(t, "development", rows[0].Environment)
	})

	errCases := []struct {
		name, src, want string
	}{
		{"invalid json", "# c\n{\"app_id\":", "x.ndjson:2"},
		{"unknown field", strings.Replace(line, `"country"`, `"contry"`, 1), "contry"},
		{"missing app_id", strings.Replace(line, `"app_id"`, `"_"`, 1), "x.ndjson:1"},
		{"missing name", strings.Replace(line, `"name":"session_end",`, "", 1), "name"},
		{"missing event_time", strings.Replace(line, `"event_time"`, `"sent_at"`, 1), "event_time"},
		{"nil install_id", strings.Replace(line, installID.String(), uuid.Nil.String(), 1), "install_id"},
		{"missing session_id", strings.Replace(line, `"session_id"`, `"x_session_id"`, 1), "x_session_id"},
		{"empty fixture", "# only comment\n", "нет событий"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testutil.ParseNDJSON(strings.NewReader(tc.src), "x.ndjson")
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
