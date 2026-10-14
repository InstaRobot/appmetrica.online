// Command reconcile сверяет отчёты Read API appmetrica.online с экспортом AppMetrica (NFR-010, T-055).
//
//	AM_SK=sk_… go run . -csv appmetrica.csv -api https://api.appmetrica.online/v1 -app <app_id> -save snapshot.json
//	go run . -csv appmetrica.csv -api snapshot.json
//
// Порядок 14-дневной сверки — docs/reconciliation/README.md.
// Exit: 0 — все проверки прошли, 1 — есть расхождения сверх порога, 2 — ошибка запуска или входных данных.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	MetricActive = "active"
	MetricNew    = "new"
	MetricD1     = "d1"

	// Пороги NFR-010: DAU и новые — относительное расхождение, D1 — абсолютное в процентных пунктах.
	maxCountDiffPct = 5.0
	maxD1DiffPoints = 3.0
	// Допуск на погрешность float, чтобы ровно 5 % и ровно 3 п.п. проходили.
	epsilon = 1e-9

	tokenEnv       = "AM_SK"
	requestTimeout = 30 * time.Second
	maxBodyBytes   = 10 << 20
	dateLayout     = time.DateOnly
)

// Day — строка экспорта AppMetrica за один день. D1 — retention первого дня когорты этого дня в процентах.
type Day struct {
	Date   string
	Active float64
	New    float64
	D1     *float64
}

// Ours — значения Read API по датам (YYYY-MM-DD в TZ приложения); D1 в процентах.
type Ours struct {
	Active map[string]float64
	New    map[string]float64
	D1     map[string]float64
}

// Check — сравнение одной метрики за один день. Diff — % для active/new, п.п. для d1; nil — не определено.
type Check struct {
	Date       string
	Metric     string
	Ours       *float64
	AppMetrica *float64
	Diff       *float64
	Pass       bool
	Note       string
}

type Report struct {
	Checks []Check
}

func (r Report) Failed() int {
	n := 0
	for _, c := range r.Checks {
		if !c.Pass {
			n++
		}
	}
	return n
}

func (r Report) Pass() bool { return len(r.Checks) > 0 && r.Failed() == 0 }

func (r Report) days() int {
	seen := map[string]struct{}{}
	for _, c := range r.Checks {
		seen[c.Date] = struct{}{}
	}
	return len(seen)
}

// Compare сравнивает каждый день экспорта AppMetrica с нашими данными. AppMetrica — база для расхождения:
// diff = (наши − AppMetrica) / AppMetrica. D1 сравнивается только в днях, где он есть в экспорте.
func Compare(days []Day, ours Ours) Report {
	var r Report
	for _, d := range days {
		r.Checks = append(r.Checks,
			countCheck(d.Date, MetricActive, d.Active, ours.Active),
			countCheck(d.Date, MetricNew, d.New, ours.New))
		if d.D1 != nil {
			r.Checks = append(r.Checks, d1Check(d.Date, *d.D1, ours.D1))
		}
	}
	return r
}

func countCheck(date, metric string, am float64, ours map[string]float64) Check {
	c := Check{Date: date, Metric: metric, AppMetrica: &am}
	o, ok := ours[date]
	if !ok {
		c.Note = "нет данных Read API"
		return c
	}
	c.Ours = &o
	if am == 0 {
		if o == 0 {
			c.Diff, c.Pass = new(float64), true
		} else {
			c.Note = "AppMetrica = 0"
		}
		return c
	}
	diff := (o - am) * 100 / am
	c.Diff, c.Pass = &diff, math.Abs(diff) <= maxCountDiffPct+epsilon
	return c
}

func d1Check(date string, am float64, ours map[string]float64) Check {
	c := Check{Date: date, Metric: MetricD1, AppMetrica: &am}
	o, ok := ours[date]
	if !ok {
		c.Note = "нет данных Read API"
		return c
	}
	diff := o - am
	c.Ours, c.Diff, c.Pass = &o, &diff, math.Abs(diff) <= maxD1DiffPoints+epsilon
	return c
}

// WriteTable печатает отчёт markdown-таблицей: её можно сохранить в docs/reconciliation/<date>.md.
func WriteTable(w io.Writer, r Report) error {
	var b strings.Builder
	b.WriteString("| Дата | Метрика | Наши | AppMetrica | Расхождение | Итог |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, c := range r.Checks {
		status := "PASS"
		if !c.Pass {
			status = "FAIL"
			if c.Note != "" {
				status += ": " + c.Note
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", c.Date, c.Metric,
			formatValue(c.Metric, c.Ours), formatValue(c.Metric, c.AppMetrica), formatDiff(c.Metric, c.Diff), status)
	}
	fmt.Fprintf(&b, "\nПороги: active и new ≤ %.0f %%, d1 ≤ %.0f п.п. (unit=installs).\n", maxCountDiffPct, maxD1DiffPoints)
	if r.Pass() {
		fmt.Fprintf(&b, "Вердикт: PASS — проверок: %d, дней: %d\n", len(r.Checks), r.days())
	} else {
		fmt.Fprintf(&b, "Вердикт: FAIL — не прошли %d из %d проверок, дней: %d\n", r.Failed(), len(r.Checks), r.days())
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func formatValue(metric string, v *float64) string {
	switch {
	case v == nil:
		return "—"
	case metric == MetricD1:
		return fmt.Sprintf("%.1f %%", *v)
	default:
		return fmt.Sprintf("%.0f", *v)
	}
}

func formatDiff(metric string, v *float64) string {
	if v == nil {
		return "—"
	}
	unit := "%"
	if metric == MetricD1 {
		unit = "п.п."
	}
	return strings.Replace(fmt.Sprintf("%+.1f %s", *v, unit), "-", "−", 1)
}

// Заголовки экспорта AppMetrica (после normalizeHeader) и короткие имена для ручных CSV.
var headerAliases = map[string][]string{
	"date":       {"date", "дата", "день"},
	MetricActive: {"active", "активные", "активные пользователи", "пользователи", "users", "dau"},
	MetricNew:    {"new", "новые", "новые пользователи", "new users"},
	MetricD1:     {"d1", "d1, %", "retention d1", "retention 1 день", "retention day 1", "удержание d1", "удержание 1 день"},
}

// ParseAppMetricaCSV читает экспорт отчёта «Аудитория» по дням: дата, активные, новые и, если есть, retention
// первого дня в процентах. Разделитель (запятая, точка с запятой, табуляция) определяется по заголовку; строка
// «Итого» пропускается; числа допускают пробелы-разделители тысяч, десятичную запятую и знак %.
func ParseAppMetricaCSV(r io.Reader) ([]Day, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("чтение CSV: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("CSV пустой")
	}

	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = detectDelimiter(data)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("разбор CSV: %w", err)
	}

	cols, err := mapColumns(records[0])
	if err != nil {
		return nil, err
	}

	var days []Day
	seen := map[string]bool{}
	for i, rec := range records[1:] {
		line := i + 2
		cell := func(name string) string {
			idx, ok := cols[name]
			if !ok || idx >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[idx])
		}
		rawDate := cell("date")
		if rawDate == "" || isTotalRow(rawDate) {
			continue
		}
		day, err := parseDay(line, rawDate, cell)
		if err != nil {
			return nil, err
		}
		if seen[day.Date] {
			return nil, fmt.Errorf("строка %d: дата %s повторяется", line, day.Date)
		}
		seen[day.Date] = true
		days = append(days, day)
	}
	if len(days) == 0 {
		return nil, errors.New("в CSV нет строк с данными")
	}
	slices.SortFunc(days, func(a, b Day) int { return strings.Compare(a.Date, b.Date) })
	return days, nil
}

func parseDay(line int, rawDate string, cell func(string) string) (Day, error) {
	date, err := parseDate(rawDate)
	if err != nil {
		return Day{}, fmt.Errorf("строка %d: %w", line, err)
	}
	day := Day{Date: date}
	for _, f := range []struct {
		name string
		dst  *float64
	}{{MetricActive, &day.Active}, {MetricNew, &day.New}} {
		v, err := parseNumber(cell(f.name))
		if err != nil {
			return Day{}, fmt.Errorf("строка %d, %s: %w", line, f.name, err)
		}
		if v == nil {
			return Day{}, fmt.Errorf("строка %d: пустое значение %s", line, f.name)
		}
		*f.dst = *v
	}
	if day.D1, err = parseNumber(cell(MetricD1)); err != nil {
		return Day{}, fmt.Errorf("строка %d, %s: %w", line, MetricD1, err)
	}
	return day, nil
}

func detectDelimiter(data []byte) rune {
	header, _, _ := bytes.Cut(data, []byte("\n"))
	best, bestCount := ',', bytes.Count(header, []byte(","))
	for _, d := range []rune{';', '\t'} {
		if n := bytes.Count(header, []byte(string(d))); n > bestCount {
			best, bestCount = d, n
		}
	}
	return best
}

func normalizeHeader(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "ё", "е")
	return strings.Join(strings.Fields(s), " ")
}

func mapColumns(header []string) (map[string]int, error) {
	cols := map[string]int{}
	for i, h := range header {
		h = normalizeHeader(h)
		for name, aliases := range headerAliases {
			if _, taken := cols[name]; !taken && slices.Contains(aliases, h) {
				cols[name] = i
			}
		}
	}
	for _, name := range []string{"date", MetricActive, MetricNew} {
		if _, ok := cols[name]; !ok {
			return nil, fmt.Errorf("в CSV нет колонки %s (подходящие заголовки: %s)",
				name, strings.Join(headerAliases[name], ", "))
		}
	}
	return cols, nil
}

func isTotalRow(s string) bool {
	return slices.Contains([]string{"итого", "всего", "total"}, normalizeHeader(s))
}

func parseDate(s string) (string, error) {
	for _, layout := range []string{dateLayout, "02.01.2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(dateLayout), nil
		}
	}
	return "", fmt.Errorf("дата %q: ожидается YYYY-MM-DD или ДД.ММ.ГГГГ", s)
}

// parseNumber возвращает nil для пустой ячейки.
func parseNumber(s string) (*float64, error) {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '%' {
			return -1
		}
		if r == ',' {
			return '.'
		}
		return r
	}, s)
	if clean == "" {
		return nil, nil //nolint:nilnil // пустая ячейка — нет значения, это не ошибка
	}
	v, err := strconv.ParseFloat(clean, 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return nil, fmt.Errorf("значение %q не является неотрицательным числом", s)
	}
	return &v, nil
}

// Snapshot — ответы Read API, по которым идёт сверка. Поля — тела ответов getAudienceTimeseries (metric=active,
// metric=new) и getRetention как есть; -save сохраняет снимок, чтобы сверку можно было повторить без API.
type Snapshot struct {
	Active    *TimeSeries      `json:"active"`
	New       *TimeSeries      `json:"new"`
	Retention *RetentionMatrix `json:"retention,omitempty"`
}

type TimeSeries struct {
	Metric      string            `json:"metric"`
	Granularity string            `json:"granularity"`
	Points      []TimeSeriesPoint `json:"points"`
}

type TimeSeriesPoint struct {
	TS    string   `json:"ts"`
	Value *float64 `json:"value"`
}

type RetentionMatrix struct {
	CohortGranularity string            `json:"cohort_granularity"`
	Periods           int               `json:"periods"`
	Cohorts           []RetentionCohort `json:"cohorts"`
	Average           []*float64        `json:"average"`
}

type RetentionCohort struct {
	CohortStart string     `json:"cohort_start"`
	Size        int64      `json:"size"`
	Values      []*float64 `json:"values"`
}

// Ours переводит снимок в значения по датам. Дата точки — дата ts в его offset (TZ приложения), как и
// в экспорте AppMetrica; null — нет значения. D1 — values[1] когорты в процентах.
func (s Snapshot) Ours() (Ours, error) {
	ours := Ours{D1: map[string]float64{}}
	var err error
	if ours.Active, err = s.Active.byDate(MetricActive); err != nil {
		return Ours{}, err
	}
	if ours.New, err = s.New.byDate(MetricNew); err != nil {
		return Ours{}, err
	}
	if s.Retention == nil {
		return ours, nil
	}
	if s.Retention.CohortGranularity != "day" {
		return Ours{}, fmt.Errorf("retention: cohort_granularity %q, нужен day", s.Retention.CohortGranularity)
	}
	for _, c := range s.Retention.Cohorts {
		if _, err := time.Parse(dateLayout, c.CohortStart); err != nil {
			return Ours{}, fmt.Errorf("retention: cohort_start %q: %w", c.CohortStart, err)
		}
		if len(c.Values) > 1 && c.Values[1] != nil {
			ours.D1[c.CohortStart] = *c.Values[1] * 100
		}
	}
	return ours, nil
}

func (ts *TimeSeries) byDate(metric string) (map[string]float64, error) {
	if ts == nil {
		return nil, fmt.Errorf("в снимке нет ряда %s", metric)
	}
	if ts.Granularity != "day" {
		return nil, fmt.Errorf("ряд %s: granularity %q, нужен day", metric, ts.Granularity)
	}
	out := map[string]float64{}
	for _, p := range ts.Points {
		t, err := time.Parse(time.RFC3339, p.TS)
		if err != nil {
			return nil, fmt.Errorf("ряд %s: ts %q: %w", metric, p.TS, err)
		}
		if p.Value != nil {
			out[t.Format(dateLayout)] = *p.Value
		}
	}
	return out, nil
}

// readAPI запрашивает отчёты Read API с unit=installs — режим сверки NFR-010.
type readAPI struct {
	client      *http.Client
	baseURL     string
	appID       string
	environment string
	token       string
}

func (a readAPI) snapshot(ctx context.Context, days []Day) (Snapshot, error) {
	var s Snapshot
	from, to := days[0].Date, days[len(days)-1].Date
	for _, m := range []struct {
		metric string
		dst    **TimeSeries
	}{{MetricActive, &s.Active}, {MetricNew, &s.New}} {
		q := a.query(from, to)
		q.Set("granularity", "day")
		q.Set("metric", m.metric)
		q.Set("compare", "none")
		if err := a.get(ctx, "/audience/timeseries", q, m.dst); err != nil {
			return Snapshot{}, err
		}
	}

	var d1Dates []string
	for _, d := range days {
		if d.D1 != nil {
			d1Dates = append(d1Dates, d.Date)
		}
	}
	if len(d1Dates) == 0 {
		return s, nil
	}
	q := a.query(d1Dates[0], d1Dates[len(d1Dates)-1])
	q.Set("cohort_granularity", "day")
	q.Set("periods", "1")
	if err := a.get(ctx, "/retention", q, &s.Retention); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

func (a readAPI) query(from, to string) url.Values {
	return url.Values{"from": {from}, "to": {to}, "unit": {"installs"}, "environment": {a.environment}}
}

func (a readAPI) get(ctx context.Context, path string, q url.Values, dst any) error {
	endpoint := strings.TrimRight(a.baseURL, "/") + "/apps/" + url.PathEscape(a.appID) + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("запрос %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("GET %s: чтение ответа: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("GET %s: %d %s: %s", path, resp.StatusCode, e.Error.Code, e.Error.Message)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("GET %s: разбор ответа: %w", path, err)
	}
	return nil
}

type options struct {
	csvPath     string
	api         string
	appID       string
	environment string
	savePath    string
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.csvPath, "csv", "", "экспорт AppMetrica «Аудитория» по дням: дата, активные, новые[, retention D1 %]")
	fs.StringVar(&o.api, "api", "", "базовый URL Read API (https://api.appmetrica.online/v1, ключ sk_ из "+tokenEnv+") или файл снимка из -save")
	fs.StringVar(&o.appID, "app", "", "app_id приложения (нужен, если -api — URL)")
	fs.StringVar(&o.environment, "environment", "production", "окружение событий: production или development")
	fs.StringVar(&o.savePath, "save", "", "сохранить снимок ответов Read API в файл")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	switch {
	case o.csvPath == "":
		return options{}, errors.New("нужен -csv: файл экспорта AppMetrica")
	case o.api == "":
		return options{}, errors.New("нужен -api: URL Read API или файл снимка")
	}
	return o, nil
}

func isURL(s string) bool { return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") }

func loadSnapshot(ctx context.Context, o options, days []Day, getenv func(string) string) (Snapshot, error) {
	if !isURL(o.api) {
		data, err := os.ReadFile(o.api)
		if err != nil {
			return Snapshot{}, fmt.Errorf("снимок Read API: %w", err)
		}
		var s Snapshot
		if err := json.Unmarshal(data, &s); err != nil {
			return Snapshot{}, fmt.Errorf("снимок Read API %s: %w", o.api, err)
		}
		return s, nil
	}
	if o.appID == "" {
		return Snapshot{}, errors.New("нужен -app: app_id приложения в Read API")
	}
	token := getenv(tokenEnv)
	if token == "" {
		return Snapshot{}, fmt.Errorf("нужна переменная окружения %s с ключом Read API sk_ (scope read)", tokenEnv)
	}
	if !strings.HasPrefix(token, "sk_") {
		return Snapshot{}, fmt.Errorf("%s должна содержать ключ Read API sk_", tokenEnv)
	}
	api := readAPI{
		client:      &http.Client{Timeout: requestTimeout},
		baseURL:     o.api,
		appID:       o.appID,
		environment: o.environment,
		token:       token,
	}
	return api.snapshot(ctx, days)
}

func saveSnapshot(path string, s Snapshot) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("снимок Read API: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil { //nolint:gosec // путь задаёт владелец флагом -save
		return fmt.Errorf("сохранение снимка: %w", err)
	}
	return nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fail := func(err error) int {
		_, _ = fmt.Fprintf(stderr, "reconcile: %v\n", err)
		return 2
	}
	o, err := parseFlags(args, stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case err != nil:
		return fail(err)
	}

	f, err := os.Open(o.csvPath)
	if err != nil {
		return fail(fmt.Errorf("экспорт AppMetrica: %w", err))
	}
	days, err := ParseAppMetricaCSV(f)
	_ = f.Close()
	if err != nil {
		return fail(fmt.Errorf("экспорт AppMetrica %s: %w", o.csvPath, err))
	}

	snap, err := loadSnapshot(ctx, o, days, getenv)
	if err != nil {
		return fail(err)
	}
	ours, err := snap.Ours()
	if err != nil {
		return fail(fmt.Errorf("снимок Read API: %w", err))
	}
	if o.savePath != "" {
		if err := saveSnapshot(o.savePath, snap); err != nil {
			return fail(err)
		}
	}

	report := Compare(days, ours)
	if err := WriteTable(stdout, report); err != nil {
		return fail(fmt.Errorf("вывод отчёта: %w", err))
	}
	if !report.Pass() {
		return 1
	}
	return 0
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(code)
}
