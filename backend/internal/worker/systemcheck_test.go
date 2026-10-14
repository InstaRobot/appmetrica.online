//go:build integration

package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/telegram"
	"appmetrica.online/backend/internal/testutil"
	"appmetrica.online/backend/internal/worker"
)

var checkNow = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

const checkPingPath = "/ping/0b5e6c1e-0000-4000-8000-000000000001"

// fakeClickHouse — ClickHouseProbe: свободное место и доступность задаёт тест; panicDisk — паника проверки диска.
type fakeClickHouse struct {
	mu        sync.Mutex
	freePct   float64
	pingErr   error
	panicDisk bool
}

func (f *fakeClickHouse) Ping(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingErr
}

func (f *fakeClickHouse) DiskFreePct(context.Context) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.panicDisk {
		panic("disk probe exploded")
	}
	return f.freePct, nil
}

func (f *fakeClickHouse) set(fn func(f *fakeClickHouse)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// fakeBackups — BackupLister: последний бэкап задаёт тест.
type fakeBackups struct {
	mu     sync.Mutex
	latest chstore.Backup
	ok     bool
	err    error
}

func (f *fakeBackups) Latest(context.Context) (chstore.Backup, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.latest, f.ok, f.err
}

func (f *fakeBackups) set(created time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latest, f.ok, f.err = chstore.Backup{Name: "daily-" + created.Format("20060102"), CreatedAt: created}, true, nil
}

// fakeHTTP — httptest с изменяемым ответом и журналом путей запросов.
type fakeHTTP struct {
	srv    *httptest.Server
	mu     sync.Mutex
	status int
	body   string
	paths  []string
}

func newFakeHTTP(t *testing.T, status int, body string) *fakeHTTP {
	t.Helper()
	f := &fakeHTTP{status: status, body: body}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		status, body := f.status, f.body
		f.mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeHTTP) reply(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *fakeHTTP) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

// ingestMetrics — фрагмент /metrics ingest: счётчик ошибок вставки и число вставок (count гистограммы).
func ingestMetrics(errorsTotal, inserts int) string {
	return "# HELP ingest_clickhouse_insert_errors_total Неудачные вставки.\n" +
		"# TYPE ingest_clickhouse_insert_errors_total counter\n" +
		"ingest_clickhouse_insert_errors_total " + strconv.Itoa(errorsTotal) + "\n" +
		"# TYPE ingest_clickhouse_insert_duration_seconds histogram\n" +
		"ingest_clickhouse_insert_duration_seconds_bucket{le=\"+Inf\"} " + strconv.Itoa(inserts) + "\n" +
		"ingest_clickhouse_insert_duration_seconds_sum 1.5\n" +
		"ingest_clickhouse_insert_duration_seconds_count " + strconv.Itoa(inserts) + "\n"
}

type checkHarness struct {
	env     *testutil.Env
	clock   *clock.Fixed
	ch      *fakeClickHouse
	backups *fakeBackups
	ingest  *fakeHTTP
	api     *fakeHTTP
	metrics *fakeHTTP
	hc      *fakeHTTP
	bot     *fakeBotAPI
	reg     *prometheus.Registry
	logs    *bytes.Buffer
	sched   *worker.Scheduler
}

type checkOption func(*worker.SystemCheckConfig)

func withoutPingURL() checkOption {
	return func(c *worker.SystemCheckConfig) { c.HealthchecksPingURL = "" }
}

func newCheckHarness(t *testing.T, bot *fakeBotAPI, opts ...checkOption) *checkHarness {
	t.Helper()
	env := testutil.Start(t)
	h := &checkHarness{
		env: env, clock: clock.NewFixed(checkNow), bot: bot,
		ch:      &fakeClickHouse{freePct: 80},
		backups: &fakeBackups{},
		ingest:  newFakeHTTP(t, http.StatusOK, "ok"),
		api:     newFakeHTTP(t, http.StatusOK, "ok"),
		metrics: newFakeHTTP(t, http.StatusOK, ingestMetrics(0, 100)),
		hc:      newFakeHTTP(t, http.StatusOK, "OK"),
		reg:     prometheus.NewRegistry(),
		logs:    &bytes.Buffer{},
	}
	h.backups.set(checkNow.Add(-2 * time.Hour))
	tg := telegram.New(digestToken, telegram.WithBaseURL(bot.srv.URL),
		telegram.WithSleep(func(context.Context, time.Duration) error { return nil }))
	logger := slog.New(slog.NewJSONHandler(h.logs, nil))
	cfg := worker.SystemCheckConfig{
		Store: env.PG.Queries(), ClickHouse: h.ch, Backups: h.backups, Telegram: tg, Lock: env.PG,
		Clock: h.clock, Logger: logger,
		HealthchecksPingURL: h.hc.srv.URL + checkPingPath,
		DiskFreeAlertPct:    20,
		IngestHealthURL:     h.ingest.srv.URL + "/health",
		APIHealthURL:        h.api.srv.URL + "/health",
		IngestMetricsURL:    h.metrics.srv.URL + "/metrics",
	}
	for _, o := range opts {
		o(&cfg)
	}
	check := worker.NewSystemCheck(cfg)
	require.NoError(t, check.RegisterMetrics(h.reg))
	h.sched = worker.NewScheduler(env.PG.Queries(), h.clock, logger, []string{"system_check"}, check.Job())
	h.exec(t, "UPDATE settings SET telegram_chat_id = '123', alerts_enabled = true")
	return h
}

func (h *checkHarness) exec(t *testing.T, stmt string, args ...any) {
	t.Helper()
	_, err := h.env.PG.Pool().Exec(t.Context(), stmt, args...)
	require.NoError(t, err)
}

// cycle — RunOnce("system_check"); следующий цикл — через 5 минут.
func (h *checkHarness) cycle(t *testing.T) error {
	t.Helper()
	err := h.sched.RunOnce(t.Context(), "system_check")
	h.clock.Advance(5 * time.Minute)
	return err
}

func (h *checkHarness) texts() []string {
	var out []string
	for _, r := range h.bot.requests() {
		out = append(out, r.Text)
	}
	return out
}

type alertRow struct {
	Firing         bool
	FirstFiredAt   *time.Time
	LastNotifiedAt *time.Time
	LastValue      string
}

func (h *checkHarness) alert(t *testing.T, key string) alertRow {
	t.Helper()
	var a alertRow
	err := h.env.PG.Pool().QueryRow(t.Context(),
		`SELECT firing, first_fired_at, last_notified_at, last_value FROM alert_states WHERE key = $1`, key).
		Scan(&a.Firing, &a.FirstFiredAt, &a.LastNotifiedAt, &a.LastValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return alertRow{}
	}
	require.NoError(t, err)
	return a
}

func (h *checkHarness) lastRun(t *testing.T) jobRun {
	t.Helper()
	rows, err := h.env.PG.Pool().Query(t.Context(),
		`SELECT status::text, details FROM job_runs WHERE job = 'system_check' ORDER BY started_at, id`)
	require.NoError(t, err)
	runs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (jobRun, error) {
		var (
			jr  jobRun
			raw []byte
		)
		if err := r.Scan(&jr.Status, &raw); err != nil {
			return jr, err
		}
		return jr, json.Unmarshal(raw, &jr.Details)
	})
	require.NoError(t, err)
	require.NotEmpty(t, runs)
	return runs[len(runs)-1]
}

// firingGauge — worker_alerts_firing{key}.
func (h *checkHarness) firingGauge(t *testing.T, key string) float64 {
	t.Helper()
	families, err := h.reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != "worker_alerts_firing" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "key" && l.GetValue() == key {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	t.Fatalf("worker_alerts_firing{key=%q} не найдена", key)
	return 0
}

// AC-052.1: свободно 15 % два цикла подряд — одно сообщение после второго цикла; 30 % — «восстановлено».
func TestSystemCheck_DiskAlertAndRecovery(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)
	h.ch.set(func(f *fakeClickHouse) { f.freePct = 15 })

	require.NoError(t, h.cycle(t))
	require.Empty(t, h.texts(), "первая неудача — ещё не алерт")
	a := h.alert(t, "disk_free")
	require.False(t, a.Firing)
	require.NotNil(t, a.FirstFiredAt)
	require.Equal(t, "15.0", a.LastValue)

	require.NoError(t, h.cycle(t))
	require.Equal(t, []string{"🔴 На диске ClickHouse свободно 15 %"}, h.texts())
	a = h.alert(t, "disk_free")
	require.True(t, a.Firing)
	require.Equal(t, checkNow, a.FirstFiredAt.UTC(), "начало проблемы — первый неудачный цикл")
	require.Equal(t, checkNow.Add(5*time.Minute), a.LastNotifiedAt.UTC())
	require.InDelta(t, 1, h.firingGauge(t, "disk_free"), 0)
	require.Equal(t, "123", bot.requests()[0].ChatID)

	require.NoError(t, h.cycle(t))
	require.Len(t, h.texts(), 1, "повтор не раньше чем через 6 ч")

	h.ch.set(func(f *fakeClickHouse) { f.freePct = 30 })
	require.NoError(t, h.cycle(t))
	require.Equal(t, "🟢 Восстановлено: свободное место на диске ClickHouse (30 %)", h.texts()[1])
	a = h.alert(t, "disk_free")
	require.False(t, a.Firing)
	require.Nil(t, a.FirstFiredAt)
	require.Equal(t, "30.0", a.LastValue)
	require.InDelta(t, 0, h.firingGauge(t, "disk_free"), 0)

	require.NoError(t, h.cycle(t))
	require.Len(t, h.texts(), 2, "после восстановления сообщений нет")
}

// AC-052.2 (AC-8.2.2): firing, last_notified_at = now − 5 ч — повтора нет; через 1 ч 1 мин — повтор.
func TestSystemCheck_RepeatEvery6h(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)
	h.ch.set(func(f *fakeClickHouse) { f.freePct = 15 })
	h.exec(t, `INSERT INTO alert_states (key, firing, first_fired_at, last_notified_at, last_value)
		VALUES ('disk_free', true, $1, $2, '15.0')`, checkNow.Add(-5*time.Hour-5*time.Minute), checkNow.Add(-5*time.Hour))

	require.NoError(t, h.sched.RunOnce(t.Context(), "system_check"))
	require.Empty(t, h.texts())

	h.clock.Advance(time.Hour + time.Minute)
	require.NoError(t, h.sched.RunOnce(t.Context(), "system_check"))
	require.Equal(t, []string{"🔴 На диске ClickHouse свободно 15 %\nНе устранено 6 ч"}, h.texts())
	a := h.alert(t, "disk_free")
	require.True(t, a.Firing)
	require.Equal(t, checkNow.Add(time.Hour+time.Minute), a.LastNotifiedAt.UTC())
}

// AC-052.3 (AC-8.2.3): успешный цикл — GET HEALTHCHECKS_PING_URL; паника проверки — GET …/fail и failed.
func TestSystemCheck_HealthchecksPing(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t))
		require.NoError(t, h.cycle(t))
		require.Equal(t, []string{"GET " + checkPingPath}, h.hc.requests())
		run := h.lastRun(t)
		require.Equal(t, "ok", run.Status)
		require.Equal(t, "ok", run.Details["healthchecks"])
	})

	t.Run("panic", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t))
		h.ch.set(func(f *fakeClickHouse) { f.panicDisk = true })
		require.ErrorContains(t, h.cycle(t), "disk probe exploded")
		require.Equal(t, []string{"GET " + checkPingPath + "/fail"}, h.hc.requests())
		run := h.lastRun(t)
		require.Equal(t, "failed", run.Status)
		require.Contains(t, run.Details["error"], "panic")
	})

	t.Run("healthchecks unavailable", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t))
		h.hc.reply(http.StatusInternalServerError, "")
		require.NoError(t, h.cycle(t), "сбой healthchecks.io не роняет цикл")
		require.Equal(t, "error", h.lastRun(t).Details["healthchecks"])
		require.NotContains(t, h.logs.String(), checkPingPath, "ping URL — секрет, в лог не попадает")
	})

	t.Run("no ping url", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t), withoutPingURL())
		require.NoError(t, h.cycle(t))
		require.Empty(t, h.hc.requests())
		require.Equal(t, "off", h.lastRun(t).Details["healthchecks"])
	})
}

// AC-052.4 (AC-8.2.4): последний бэкап 27 ч назад — алерт backup_stale.
func TestSystemCheck_BackupStale(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)
	h.backups.set(checkNow.Add(-27 * time.Hour))

	require.NoError(t, h.cycle(t))
	require.NoError(t, h.cycle(t))
	require.Equal(t, []string{"🔴 Последний бэкап ClickHouse 27 ч назад"}, h.texts())
	a := h.alert(t, "backup_stale")
	require.True(t, a.Firing)
	require.Equal(t, "27", a.LastValue)

	h.backups.set(h.clock.Now().Add(-time.Hour))
	require.NoError(t, h.cycle(t))
	require.Equal(t, "🟢 Восстановлено: бэкап ClickHouse (1 ч назад)", h.texts()[1])
}

// Ошибка clickhouse-backup и пустой список — тоже backup_stale; без BACKUP_API_URL проверки нет.
func TestSystemCheck_BackupUnavailable(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t))
		h.backups.mu.Lock()
		h.backups.err = errors.New("chstore: backup list: HTTP 502")
		h.backups.mu.Unlock()
		require.NoError(t, h.cycle(t))
		require.NoError(t, h.cycle(t))
		require.Equal(t, []string{"🔴 Список бэкапов ClickHouse недоступен"}, h.texts())
	})

	t.Run("no backups", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t))
		h.backups.mu.Lock()
		h.backups.ok = false
		h.backups.mu.Unlock()
		require.NoError(t, h.cycle(t))
		require.NoError(t, h.cycle(t))
		require.Equal(t, []string{"🔴 Бэкапов ClickHouse нет"}, h.texts())
	})

	t.Run("not configured", func(t *testing.T) {
		h := newCheckHarness(t, newFakeBotAPI(t), func(c *worker.SystemCheckConfig) { c.Backups = nil })
		require.NoError(t, h.cycle(t))
		require.Equal(t, alertRow{}, h.alert(t, "backup_stale"))
	})
}

// AC-8.2.5: ingest /health 503, затем 200 — алерта нет; два 503 подряд — ingest_down; так же api_down.
func TestSystemCheck_RequiresTwoFailures(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)

	h.ingest.reply(http.StatusServiceUnavailable, "")
	require.NoError(t, h.cycle(t))
	h.ingest.reply(http.StatusOK, "ok")
	require.NoError(t, h.cycle(t))
	require.Empty(t, h.texts(), "флаппинг — не алерт")
	require.Equal(t, alertRow{LastValue: "200"}, h.alert(t, "ingest_down"))

	h.ingest.reply(http.StatusServiceUnavailable, "")
	h.api.reply(http.StatusBadGateway, "")
	require.NoError(t, h.cycle(t))
	require.NoError(t, h.cycle(t))
	require.Equal(t, []string{
		"🔴 Ingest не отвечает: /health → 503",
		"🔴 API не отвечает: /health → 502",
	}, h.texts())
	require.True(t, h.alert(t, "ingest_down").Firing)
	require.Equal(t, "502", h.alert(t, "api_down").LastValue)

	h.ingest.srv.Close()
	require.NoError(t, h.cycle(t))
	require.Equal(t, "error", h.alert(t, "ingest_down").LastValue, "нет соединения — тоже неудача")
	require.Len(t, h.texts(), 2)
}

// ClickHouse не отвечает два цикла — clickhouse_down; диск в это время не проверяется и его состояние не меняется.
func TestSystemCheck_ClickHouseDown(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)
	h.ch.set(func(f *fakeClickHouse) { f.freePct = 10; f.pingErr = errors.New("chstore: ping: connection refused") })

	require.NoError(t, h.cycle(t), "проблема ClickHouse — не исключение цикла")
	require.NoError(t, h.cycle(t))
	require.Equal(t, []string{"🔴 ClickHouse не отвечает"}, h.texts())
	require.Equal(t, alertRow{}, h.alert(t, "disk_free"))
	require.Equal(t, []string{"GET " + checkPingPath, "GET " + checkPingPath}, h.hc.requests())

	h.ch.set(func(f *fakeClickHouse) { f.freePct = 50; f.pingErr = nil })
	require.NoError(t, h.cycle(t))
	require.Equal(t, "🟢 Восстановлено: ClickHouse отвечает", h.texts()[1])
}

// insert_errors — доля неудачных вставок с прошлого цикла по /metrics ingest: > 1 % два цикла — алерт.
// Первый замер и сброс счётчиков (рестарт ingest) только запоминаются.
func TestSystemCheck_InsertErrors(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)

	require.NoError(t, h.cycle(t))
	require.Equal(t, alertRow{}, h.alert(t, "insert_errors"), "первый замер — без оценки")

	h.metrics.reply(http.StatusOK, ingestMetrics(1, 200))
	require.NoError(t, h.cycle(t))
	require.Equal(t, alertRow{LastValue: "1.0"}, h.alert(t, "insert_errors"), "1 % — ещё норма")

	h.metrics.reply(http.StatusOK, ingestMetrics(6, 300))
	require.NoError(t, h.cycle(t))
	h.metrics.reply(http.StatusOK, ingestMetrics(9, 400))
	require.NoError(t, h.cycle(t))
	require.Equal(t, []string{"🔴 Ошибки вставки в ClickHouse: 3 % вставок (3 из 100)"}, h.texts())

	h.metrics.reply(http.StatusOK, ingestMetrics(0, 10))
	require.NoError(t, h.cycle(t))
	require.True(t, h.alert(t, "insert_errors").Firing, "сброс счётчиков — новый базовый замер")

	h.metrics.reply(http.StatusOK, ingestMetrics(0, 10))
	require.NoError(t, h.cycle(t))
	require.Equal(t, "🟢 Восстановлено: вставка в ClickHouse (ошибок 0 %)", h.texts()[1], "вставок не было — ошибок нет")

	h.metrics.reply(http.StatusServiceUnavailable, "")
	require.NoError(t, h.cycle(t))
	require.Equal(t, "0.0", h.alert(t, "insert_errors").LastValue, "/metrics недоступен — состояние не меняется")
}

// alerts_enabled = false: состояния пишутся, сообщений нет; восстановление без отправленного алерта — тоже без
// сообщения. Без chat_id сообщений нет.
func TestSystemCheck_AlertsDisabled(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newCheckHarness(t, bot)
	h.exec(t, "UPDATE settings SET alerts_enabled = false")
	h.ch.set(func(f *fakeClickHouse) { f.freePct = 15 })

	require.NoError(t, h.cycle(t))
	require.NoError(t, h.cycle(t))
	require.Empty(t, h.texts())
	a := h.alert(t, "disk_free")
	require.True(t, a.Firing)
	require.Nil(t, a.LastNotifiedAt)

	h.exec(t, "UPDATE settings SET alerts_enabled = true, telegram_chat_id = NULL")
	require.NoError(t, h.cycle(t))
	require.Empty(t, h.texts(), "нет chat_id")

	h.exec(t, "UPDATE settings SET telegram_chat_id = '123'")
	require.NoError(t, h.cycle(t))
	require.Equal(t, []string{"🔴 На диске ClickHouse свободно 15 %"}, h.texts(), "алерты включены — горящий алерт отправлен")

	h.exec(t, "UPDATE settings SET alerts_enabled = false")
	h.ch.set(func(f *fakeClickHouse) { f.freePct = 30 })
	require.NoError(t, h.cycle(t))
	require.Len(t, h.texts(), 1, "алерты выключены — «восстановлено» не отправляется и не откладывается")
	a = h.alert(t, "disk_free")
	require.False(t, a.Firing)
	require.Nil(t, a.LastNotifiedAt)

	h.exec(t, "UPDATE settings SET alerts_enabled = true")
	require.NoError(t, h.cycle(t))
	require.Len(t, h.texts(), 1)
}

// Telegram недоступен: состояние фиксируется, last_notified_at не обновляется, отправка повторяется в следующем
// цикле — и для алерта, и для «восстановлено». Без TELEGRAM_BOT_TOKEN алерт пишется в лог.
func TestSystemCheck_TelegramUnavailable(t *testing.T) {
	t.Run("retry next cycle", func(t *testing.T) {
		bot := newFakeBotAPI(t, reply400)
		h := newCheckHarness(t, bot)
		h.ch.set(func(f *fakeClickHouse) { f.freePct = 15 })

		require.NoError(t, h.cycle(t))
		require.NoError(t, h.cycle(t), "сбой Telegram не роняет цикл")
		a := h.alert(t, "disk_free")
		require.True(t, a.Firing)
		require.Nil(t, a.LastNotifiedAt)
		require.Equal(t, "ok", h.lastRun(t).Details["healthchecks"])

		require.NoError(t, h.cycle(t))
		require.Len(t, bot.requests(), 2)
		require.NotNil(t, h.alert(t, "disk_free").LastNotifiedAt)

		bot.mu.Lock()
		bot.replies = []tgReply{reply400}
		bot.mu.Unlock()
		h.ch.set(func(f *fakeClickHouse) { f.freePct = 30 })
		require.NoError(t, h.cycle(t))
		require.NoError(t, h.cycle(t))
		got := h.texts()
		require.Len(t, got, 4)
		require.Equal(t, got[2], got[3], "«восстановлено» отправлено повторно")
		require.Nil(t, h.alert(t, "disk_free").LastNotifiedAt)

		require.NoError(t, h.cycle(t))
		require.Len(t, h.texts(), 4)
	})

	t.Run("no token", func(t *testing.T) {
		bot := newFakeBotAPI(t)
		h := newCheckHarness(t, bot, func(c *worker.SystemCheckConfig) { c.Telegram = nil })
		h.ch.set(func(f *fakeClickHouse) { f.freePct = 15 })
		require.NoError(t, h.cycle(t))
		require.NoError(t, h.cycle(t))
		require.Empty(t, bot.requests())
		require.Contains(t, h.logs.String(), "На диске ClickHouse свободно 15 %")
		require.NotNil(t, h.alert(t, "disk_free").LastNotifiedAt)
	})
}

// Цикл, начатый при занятом advisory lock (второй worker), ничего не проверяет и не пингует.
func TestSystemCheck_SkipsWhenLocked(t *testing.T) {
	h := newCheckHarness(t, newFakeBotAPI(t))
	release, ok, err := h.env.PG.TryAdvisoryLock(t.Context(), "system_check")
	require.NoError(t, err)
	require.True(t, ok)
	defer release()

	require.NoError(t, h.cycle(t))
	require.Equal(t, map[string]any{"skipped": "locked"}, h.lastRun(t).Details)
	require.Empty(t, h.hc.requests())
}
