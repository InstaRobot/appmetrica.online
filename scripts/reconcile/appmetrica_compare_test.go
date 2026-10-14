package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func ptr(v float64) *float64 { return &v }

func runCLI(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr, func(k string) string { return env[k] })
	return code, stdout.String(), stderr.String()
}

func findCheck(t *testing.T, r Report, date, metric string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Date == date && c.Metric == metric {
			return c
		}
	}
	t.Fatalf("нет проверки %s %s", date, metric)
	return Check{}
}

// AC-055.1: DAU 4.9 % в один день — PASS, 5.1 % в другой — FAIL, итог FAIL, exit 1.
func TestCompare_Thresholds(t *testing.T) {
	code, out, stderr := runCLI(t, nil,
		"-csv", "testdata/appmetrica-audience.csv",
		"-api", "testdata/api-timeseries.json")

	require.Equal(t, 1, code, stderr)
	require.Contains(t, out, "| 2026-11-28 | active | 1049 | 1000 | +4.9 % | PASS |")
	require.Contains(t, out, "| 2026-11-29 | active | 949 | 1000 | −5.1 % | FAIL |")
	require.Contains(t, out, "| 2026-11-28 | new | 204 | 200 | +2.0 % | PASS |")
	require.Contains(t, out, "| 2026-11-29 | new | 175 | 180 | −2.8 % | PASS |")
	require.Contains(t, out, "| 2026-11-28 | d1 | 41.0 % | 40.0 % | +1.0 п.п. | PASS |")
	require.NotContains(t, out, "| 2026-11-29 | d1 |", "D1 без значения в CSV не сравнивается")
	require.Contains(t, out, "Вердикт: FAIL")
	require.Contains(t, out, "не прошли 1 из 5")
}

// AC-055.2: D1 62.0 % против 58.5 % — 3.5 п.п. → FAIL.
func TestCompare_RetentionPoints(t *testing.T) {
	days := []Day{{Date: "2026-11-28", Active: 100, New: 50, D1: ptr(58.5)}}
	ours := Ours{
		Active: map[string]float64{"2026-11-28": 100},
		New:    map[string]float64{"2026-11-28": 50},
		D1:     map[string]float64{"2026-11-28": 62.0},
	}

	r := Compare(days, ours)

	c := findCheck(t, r, "2026-11-28", MetricD1)
	require.NotNil(t, c.Diff)
	require.InDelta(t, 3.5, *c.Diff, 1e-9)
	require.False(t, c.Pass)
	require.False(t, r.Pass())

	var buf bytes.Buffer
	require.NoError(t, WriteTable(&buf, r))
	require.Contains(t, buf.String(), "| 2026-11-28 | d1 | 62.0 % | 58.5 % | +3.5 п.п. | FAIL |")
}

func TestCompare_BoundariesPass(t *testing.T) {
	tests := []struct {
		name   string
		metric string
		ours   float64
		am     float64
		pass   bool
		diff   float64
	}{
		{"active ровно +5 %", MetricActive, 105, 100, true, 5},
		{"active ровно −5 %", MetricActive, 95, 100, true, -5},
		{"new +5.01 %", MetricNew, 10501, 10000, false, 5.01},
		{"d1 ровно 3 п.п.", MetricD1, 43, 40, true, 3},
		{"d1 −3.01 п.п.", MetricD1, 36.99, 40, false, -3.01},
		{"нули с обеих сторон", MetricActive, 0, 0, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			day := Day{Date: "2026-11-28", Active: 100, New: 100}
			ours := Ours{
				Active: map[string]float64{"2026-11-28": 100},
				New:    map[string]float64{"2026-11-28": 100},
				D1:     map[string]float64{},
			}
			switch tt.metric {
			case MetricActive:
				day.Active, ours.Active["2026-11-28"] = tt.am, tt.ours
			case MetricNew:
				day.New, ours.New["2026-11-28"] = tt.am, tt.ours
			case MetricD1:
				day.D1, ours.D1["2026-11-28"] = ptr(tt.am), tt.ours
			}

			c := findCheck(t, Compare([]Day{day}, ours), "2026-11-28", tt.metric)

			require.Equal(t, tt.pass, c.Pass)
			require.NotNil(t, c.Diff)
			require.InDelta(t, tt.diff, *c.Diff, 1e-9)
		})
	}
}

func TestCompare_MissingAndZero(t *testing.T) {
	days := []Day{
		{Date: "2026-11-28", Active: 0, New: 10, D1: ptr(40)},
		{Date: "2026-11-29", Active: 100, New: 10},
	}
	ours := Ours{
		Active: map[string]float64{"2026-11-28": 3},
		New:    map[string]float64{"2026-11-28": 10, "2026-11-29": 10},
		D1:     map[string]float64{},
	}

	r := Compare(days, ours)

	zero := findCheck(t, r, "2026-11-28", MetricActive)
	require.False(t, zero.Pass, "AppMetrica = 0 при наших 3 — расхождение не определено")
	require.Nil(t, zero.Diff)
	require.NotEmpty(t, zero.Note)

	noD1 := findCheck(t, r, "2026-11-28", MetricD1)
	require.False(t, noD1.Pass, "D1 есть в AppMetrica, но нет у нас")
	require.Nil(t, noD1.Ours)

	noActive := findCheck(t, r, "2026-11-29", MetricActive)
	require.False(t, noActive.Pass)
	require.Nil(t, noActive.Ours)
	require.Equal(t, "нет данных Read API", noActive.Note)

	require.True(t, findCheck(t, r, "2026-11-29", MetricNew).Pass)
	require.False(t, r.Pass())
	require.Equal(t, 3, r.Failed())

	var buf bytes.Buffer
	require.NoError(t, WriteTable(&buf, r))
	require.Contains(t, buf.String(), "| 2026-11-29 | active | — | 100 | — | FAIL: нет данных Read API |")
}

func TestCompare_AllPassExitZero(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "am.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("date,active,new\n2026-11-28,1000,200\n"), 0o600))
	apiPath := filepath.Join(dir, "api.json")
	require.NoError(t, os.WriteFile(apiPath, []byte(`{
		"active": {"metric":"active","granularity":"day","points":[{"ts":"2026-11-28T00:00:00+03:00","value":1010}]},
		"new": {"metric":"new","granularity":"day","points":[{"ts":"2026-11-28T00:00:00+03:00","value":199}]}
	}`), 0o600))

	code, out, stderr := runCLI(t, nil, "-csv", csvPath, "-api", apiPath)

	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "Вердикт: PASS")
	require.Contains(t, out, "проверок: 2")
}

func TestParseAppMetricaCSV_Formats(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Day
	}{
		{
			name: "английские заголовки, запятая",
			in:   "date,active,new,d1\n2026-11-28,1000,200,40.5\n",
			want: []Day{{Date: "2026-11-28", Active: 1000, New: 200, D1: ptr(40.5)}},
		},
		{
			name: "русский экспорт: BOM, точка с запятой, пробелы в числах, десятичная запятая, проценты, дд.мм.гггг",
			in: "\ufeffДата;Активные пользователи;Новые пользователи;Retention 1 день\n" +
				"29.11.2026;1\u00a0234;56;41,5 %\n28.11.2026;1 000;200;\nИтого;2 234;256;\n",
			want: []Day{
				{Date: "2026-11-28", Active: 1000, New: 200},
				{Date: "2026-11-29", Active: 1234, New: 56, D1: ptr(41.5)},
			},
		},
		{
			name: "табуляция, колонки в другом порядке, без D1",
			in:   "Новые\tДата\tАктивные\n7\t2026-11-28\t70\n",
			want: []Day{{Date: "2026-11-28", Active: 70, New: 7}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAppMetricaCSV(strings.NewReader(tt.in))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseAppMetricaCSV_Errors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"пустой файл", "", "пустой"},
		{"нет колонки новых", "date,active\n2026-11-28,1\n", "new"},
		{"нет строк", "date,active,new\n", "нет строк"},
		{"плохая дата", "date,active,new\n28/11/2026,1,2\n", "28/11/2026"},
		{"не число", "date,active,new\n2026-11-28,много,2\n", "много"},
		{"пустое active", "date,active,new\n2026-11-28,,2\n", "active"},
		{"повтор даты", "date,active,new\n2026-11-28,1,2\n2026-11-28,1,2\n", "повторяется"},
		{"отрицательное", "date,active,new\n2026-11-28,-1,2\n", "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAppMetricaCSV(strings.NewReader(tt.in))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSnapshot_Ours(t *testing.T) {
	raw := `{
		"active": {"metric":"active","granularity":"day","points":[
			{"ts":"2026-11-28T00:00:00+03:00","value":10},
			{"ts":"2026-11-29T00:00:00+03:00","value":null}]},
		"new": {"metric":"new","granularity":"day","points":[{"ts":"2026-11-28T00:00:00-05:00","value":4}]},
		"retention": {"cohort_granularity":"day","periods":1,"cohorts":[
			{"cohort_start":"2026-11-28","size":4,"values":[1,0.25]},
			{"cohort_start":"2026-11-29","size":0,"values":[null,null]},
			{"cohort_start":"2026-11-30","size":0,"values":[1]}],"average":[1,0.25]}
	}`
	var s Snapshot
	require.NoError(t, json.Unmarshal([]byte(raw), &s))

	ours, err := s.Ours()

	require.NoError(t, err)
	require.Equal(t, map[string]float64{"2026-11-28": 10}, ours.Active, "null — нет значения")
	require.Equal(t, map[string]float64{"2026-11-28": 4}, ours.New, "дата берётся в TZ приложения из offset")
	require.Equal(t, map[string]float64{"2026-11-28": 25}, ours.D1)
}

func TestSnapshot_Errors(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"нет active", `{"new":{"metric":"new","granularity":"day","points":[]}}`, "active"},
		{"не дневной ряд", `{"active":{"metric":"active","granularity":"hour","points":[]},"new":{"metric":"new","granularity":"day","points":[]}}`, "granularity"},
		{"плохой ts", `{"active":{"metric":"active","granularity":"day","points":[{"ts":"вчера","value":1}]},"new":{"metric":"new","granularity":"day","points":[]}}`, "вчера"},
		{"недельные когорты", `{"active":{"metric":"active","granularity":"day","points":[]},"new":{"metric":"new","granularity":"day","points":[]},"retention":{"cohort_granularity":"week","periods":1,"cohorts":[],"average":[]}}`, "cohort_granularity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Snapshot
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &s))
			_, err := s.Ours()
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// fakeReadAPI отвечает как Read API и записывает запросы.
func fakeReadAPI(t *testing.T, status int) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var reqs []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"Нужен ключ","request_id":"r1"}}`))
			return
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/apps/app-1/audience/timeseries":
			value := map[string]int{"active": 1049, "new": 204}[q.Get("metric")]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metric": q.Get("metric"), "granularity": "day",
				"points": []map[string]any{{"ts": "2026-11-28T00:00:00+03:00", "value": value}},
			})
		case "/v1/apps/app-1/retention":
			_, _ = w.Write([]byte(`{"cohort_granularity":"day","periods":1,"cohorts":[{"cohort_start":"2026-11-28","size":204,"values":[1,0.41]}],"average":[1,0.41]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func TestRun_FetchesReadAPI(t *testing.T) {
	srv, reqs := fakeReadAPI(t, http.StatusOK)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "am.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("date,active,new,d1\n2026-11-28,1000,200,40\n"), 0o600))
	savePath := filepath.Join(dir, "snapshot.json")

	code, out, stderr := runCLI(t, map[string]string{"AM_SK": "sk_secret"},
		"-csv", csvPath, "-api", srv.URL+"/v1/", "-app", "app-1", "-save", savePath)

	require.Equal(t, 0, code, stderr)
	require.Contains(t, out, "| 2026-11-28 | d1 | 41.0 % | 40.0 % | +1.0 п.п. | PASS |")
	require.Len(t, *reqs, 3)
	for _, r := range *reqs {
		require.Equal(t, "Bearer sk_secret", r.Header.Get("Authorization"))
		require.Empty(t, r.URL.Query().Get("key"), "ключ не передаётся в query string")
		q := r.URL.Query()
		require.Equal(t, "installs", q.Get("unit"))
		require.Equal(t, "production", q.Get("environment"))
		require.Equal(t, "2026-11-28", q.Get("from"))
		require.Equal(t, "2026-11-28", q.Get("to"))
	}
	ts := (*reqs)[0].URL.Query()
	require.Equal(t, "day", ts.Get("granularity"))
	require.Equal(t, "none", ts.Get("compare"))
	require.Equal(t, "active", ts.Get("metric"))
	require.Equal(t, "new", (*reqs)[1].URL.Query().Get("metric"))
	ret := (*reqs)[2].URL.Query()
	require.Equal(t, "day", ret.Get("cohort_granularity"))
	require.Equal(t, "1", ret.Get("periods"))
	require.NotContains(t, out+stderr, "sk_secret")

	saved, err := os.ReadFile(savePath)
	require.NoError(t, err)
	code, _, stderr = runCLI(t, nil, "-csv", csvPath, "-api", savePath)
	require.Equal(t, 0, code, "сохранённый снимок воспроизводит сверку: %s\n%s", stderr, saved)
}

func TestRun_SkipsRetentionWithoutD1(t *testing.T) {
	srv, reqs := fakeReadAPI(t, http.StatusOK)
	csvPath := filepath.Join(t.TempDir(), "am.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("date,active,new\n2026-11-28,1000,200\n"), 0o600))

	code, _, stderr := runCLI(t, map[string]string{"AM_SK": "sk_x"},
		"-csv", csvPath, "-api", srv.URL+"/v1", "-app", "app-1", "-environment", "development")

	require.Equal(t, 0, code, stderr)
	require.Len(t, *reqs, 2)
	require.Equal(t, "development", (*reqs)[0].URL.Query().Get("environment"))
}

func TestRun_Errors(t *testing.T) {
	unauthorized, _ := fakeReadAPI(t, http.StatusUnauthorized)
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		wantErr string
	}{
		{"без -csv", nil, []string{"-api", "testdata/api-timeseries.json"}, "-csv"},
		{"без -api", nil, []string{"-csv", "testdata/appmetrica-audience.csv"}, "-api"},
		{"нет файла CSV", nil, []string{"-csv", "testdata/nope.csv", "-api", "testdata/api-timeseries.json"}, "nope.csv"},
		{"нет файла снимка", nil, []string{"-csv", "testdata/appmetrica-audience.csv", "-api", "testdata/nope.json"}, "nope.json"},
		{"битый снимок", nil, []string{"-csv", "testdata/appmetrica-audience.csv", "-api", "testdata/appmetrica-audience.csv"}, "снимок"},
		{"URL без -app", map[string]string{"AM_SK": "sk_x"}, []string{"-csv", "testdata/appmetrica-audience.csv", "-api", unauthorized.URL}, "-app"},
		{"URL без AM_SK", nil, []string{"-csv", "testdata/appmetrica-audience.csv", "-api", unauthorized.URL, "-app", "app-1"}, "AM_SK"},
		{"ключ не sk_", map[string]string{"AM_SK": "pk_x"}, []string{"-csv", "testdata/appmetrica-audience.csv", "-api", unauthorized.URL, "-app", "app-1"}, "sk_"},
		{"401 от API", map[string]string{"AM_SK": "sk_x"}, []string{"-csv", "testdata/appmetrica-audience.csv", "-api", unauthorized.URL, "-app", "app-1"}, "401 unauthenticated"},
		{"неизвестный флаг", nil, []string{"-csv", "a", "-api", "b", "-zzz"}, "zzz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, stderr := runCLI(t, tt.env, tt.args...)
			require.Equal(t, 2, code, out)
			require.Contains(t, stderr, tt.wantErr)
			require.NotContains(t, stderr, "sk_x")
		})
	}
}
