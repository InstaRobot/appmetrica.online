package worker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// SystemCheckStore — настройки и alert_states (pgstore/sqlc.Queries).
type SystemCheckStore interface {
	SettingsReader
	ListAlertStates(ctx context.Context) ([]sqlc.AlertState, error)
	UpsertAlertState(ctx context.Context, arg sqlc.UpsertAlertStateParams) (sqlc.AlertState, error)
}

// ClickHouseProbe — chstore.Store.
type ClickHouseProbe interface {
	Ping(ctx context.Context) error
	DiskFreePct(ctx context.Context) (float64, error)
}

// BackupLister — chstore.BackupClient.
type BackupLister interface {
	Latest(ctx context.Context) (chstore.Backup, bool, error)
}

// SystemCheckConfig — зависимости и пороги system_check.
type SystemCheckConfig struct {
	Store      SystemCheckStore
	ClickHouse ClickHouseProbe
	// Backups — nil: BACKUP_API_URL не задан, свежесть бэкапа не проверяется.
	Backups BackupLister
	// Telegram — nil: TELEGRAM_BOT_TOKEN не задан, алерты пишутся в лог.
	Telegram TelegramSender
	Lock     Locker
	Clock    clock.Clock
	Logger   *slog.Logger
	// HTTP — клиент для /health, /metrics и healthchecks.io; nil — http.DefaultClient. Таймауты задают контексты.
	HTTP *http.Client

	// HealthchecksPingURL — пусто: пинг отключён.
	HealthchecksPingURL string
	DiskFreeAlertPct    int
	// IngestHealthURL, APIHealthURL, IngestMetricsURL — пусто: проверка отключена.
	IngestHealthURL  string
	APIHealthURL     string
	IngestMetricsURL string
}

const (
	systemCheckEvery   = 5 * time.Minute
	systemCheckTimeout = time.Minute
	systemCheckLockKey = "system_check"
	// Таймауты проверок (02-tech-spec.md FR-021): цикл укладывается в 30 с.
	clickhouseCheckTimeout = 5 * time.Second
	httpCheckTimeout       = 3 * time.Second
	backupCheckTimeout     = 5 * time.Second
	pingTimeout            = 10 * time.Second
	// failuresToFire — неудачных циклов подряд до алерта: одиночный сбой не будит владельца.
	failuresToFire = 2
	alertRepeat    = 6 * time.Hour
	backupMaxAge   = 26 * time.Hour
	// insertErrorsMaxPct — доля неудачных вставок с прошлого цикла, выше которой поднимается insert_errors.
	insertErrorsMaxPct = 1.0
	maxMetricsBytes    = 4 << 20
)

// alertKeys — ключи alert_states в порядке проверок и сообщений.
var alertKeys = []string{"clickhouse_down", "disk_free", "insert_errors", "backup_stale", "ingest_down", "api_down"}

// observation — результат проверки: bad — условие алерта, value — alert_states.last_value, problem — текст 🔴,
// recovered — что восстановилось (текст 🟢).
type observation struct {
	bad       bool
	value     string
	problem   string
	recovered string
}

// insertCounters — счётчики /metrics ingest на момент прошлого цикла.
type insertCounters struct {
	errors, inserts float64
}

// SystemCheck — job system_check (FR-021): проверки ClickHouse, ingest, api и бэкапа, машина состояний alert_states
// с сообщениями в Telegram и dead-man ping healthchecks.io.
type SystemCheck struct {
	cfg    SystemCheckConfig
	http   *http.Client
	firing *prometheus.GaugeVec

	mu sync.Mutex
	// prevInserts — счётчики прошлого цикла для доли ошибок вставки; nil — замера ещё не было.
	prevInserts *insertCounters
}

func NewSystemCheck(cfg SystemCheckConfig) *SystemCheck {
	s := &SystemCheck{
		cfg:  cfg,
		http: cfg.HTTP,
		firing: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "worker_alerts_firing",
			Help: "Системный алерт сработал (1) или в норме (0), по alert_states.",
		}, []string{"key"}),
	}
	if s.http == nil {
		s.http = http.DefaultClient
	}
	return s
}

// Job — system_check раз в 5 минут.
func (s *SystemCheck) Job() Job {
	return Job{Name: "system_check", Every: systemCheckEvery, Timeout: systemCheckTimeout, Run: s.run}
}

// RegisterMetrics регистрирует worker_alerts_firing.
func (s *SystemCheck) RegisterMetrics(reg prometheus.Registerer) error {
	if err := reg.Register(s.firing); err != nil {
		return fmt.Errorf("worker: register metrics: %w", err)
	}
	return nil
}

// run — цикл под advisory lock (второй worker пропускает цикл). После цикла без ошибок — GET HEALTHCHECKS_PING_URL,
// при ошибке или панике проверки — GET …/fail; проблемы, найденные проверками, ошибками цикла не считаются.
func (s *SystemCheck) run(ctx context.Context, _ uuid.UUID) (details Details, err error) {
	release, ok, err := s.cfg.Lock.TryAdvisoryLock(ctx, systemCheckLockKey)
	if err != nil {
		return Details{"healthchecks": s.ping(ctx, true)}, fmt.Errorf("system_check: %w", err)
	}
	if !ok {
		return Details{"skipped": "locked"}, nil
	}
	defer release()
	s.mu.Lock()
	defer s.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			details, err = Details{}, fmt.Errorf("system_check: panic: %v", r)
		}
		details["healthchecks"] = s.ping(ctx, err != nil)
	}()
	return s.cycle(ctx)
}

func (s *SystemCheck) cycle(ctx context.Context) (Details, error) {
	obs := s.observe(ctx)

	st, err := s.cfg.Store.GetSettings(ctx)
	if err != nil {
		return Details{}, fmt.Errorf("system_check: settings: %w", err)
	}
	rows, err := s.cfg.Store.ListAlertStates(ctx)
	if err != nil {
		return Details{}, fmt.Errorf("system_check: alert states: %w", err)
	}
	states := make(map[string]sqlc.AlertState, len(rows))
	for _, a := range rows {
		states[a.Key] = a
	}
	n := &notifier{check: s}
	if st.AlertsEnabled && st.TelegramChatID != nil {
		n.chatID = *st.TelegramChatID
	}

	values := map[string]string{}
	firing := []string{}
	var errs []error
	for _, key := range alertKeys {
		prev, known := states[key]
		o, observed := obs[key]
		if !observed {
			if known {
				if prev.Firing {
					firing = append(firing, key)
				}
				s.firing.WithLabelValues(key).Set(boolGauge(prev.Firing))
			}
			continue
		}
		next := s.transition(ctx, n, prev, o)
		next.Key = key
		if _, err := s.cfg.Store.UpsertAlertState(ctx, sqlc.UpsertAlertStateParams(next)); err != nil {
			errs = append(errs, fmt.Errorf("system_check: alert state %s: %w", key, err))
			continue
		}
		values[key] = o.value
		if next.Firing {
			firing = append(firing, key)
		}
		s.firing.WithLabelValues(key).Set(boolGauge(next.Firing))
	}
	return Details{"values": values, "firing": firing, "sent": n.sent}, errors.Join(errs...)
}

// transition — машина состояний алерта (alert_states):
//   - норма: firing = false, first_fired_at = NULL;
//   - первая неудача: first_fired_at = начало проблемы, firing ещё false;
//   - вторая неудача подряд: firing = true и 🔴; пока firing — повтор раз в 6 ч;
//   - норма после firing: firing = false и 🟢, если о проблеме сообщали.
//
// last_notified_at — когда владелец последний раз получил 🔴 этого эпизода: NULL у firing — сообщение ещё не
// доставлено (Telegram недоступен или алерты выключены) и уйдёт в следующем цикле; не NULL в норме — 🟢 ещё не
// доставлено.
func (s *SystemCheck) transition(ctx context.Context, n *notifier, prev sqlc.AlertState, o observation) sqlc.AlertState {
	now := s.cfg.Clock.Now()
	next := prev
	next.LastValue = o.value
	switch {
	case o.bad && !prev.Firing && prev.FirstFiredAt == nil:
		next.FirstFiredAt = &now
	case o.bad && !prev.Firing:
		next.Firing, next.LastNotifiedAt = true, nil
		if n.send(ctx, "🔴 "+o.problem, true) {
			next.LastNotifiedAt = &now
		}
	case o.bad:
		if prev.LastNotifiedAt == nil || now.Sub(*prev.LastNotifiedAt) >= alertRepeat {
			text := "🔴 " + o.problem
			if prev.LastNotifiedAt != nil && prev.FirstFiredAt != nil {
				text += "\nНе устранено " + formatDuration(now.Sub(*prev.FirstFiredAt))
			}
			if n.send(ctx, text, true) {
				next.LastNotifiedAt = &now
			}
		}
	default:
		next.Firing, next.FirstFiredAt = false, nil
		if prev.LastNotifiedAt != nil && n.send(ctx, "🟢 Восстановлено: "+o.recovered, false) {
			next.LastNotifiedAt = nil
		}
	}
	return next
}

// notifier — отправка алертов цикла в чат владельца; chatID пуст — алерты выключены или чат не задан.
type notifier struct {
	check  *SystemCheck
	chatID string
	sent   int
}

// send — true, если сообщение доставлено (или записано в лог без TELEGRAM_BOT_TOKEN). Для 🟢 недоставка из-за
// выключенных алертов тоже true: откладывать «восстановлено» до их включения незачем.
func (n *notifier) send(ctx context.Context, text string, problem bool) bool {
	logger := n.check.cfg.Logger
	if n.chatID == "" {
		return !problem
	}
	if n.check.cfg.Telegram == nil {
		logger.InfoContext(ctx, "alert not sent: TELEGRAM_BOT_TOKEN is not set", slog.String("text", text))
		n.sent++
		return true
	}
	if _, err := n.check.cfg.Telegram.Send(ctx, n.chatID, text); err != nil {
		logger.WarnContext(ctx, "alert not sent", slog.Any("error", err))
		return false
	}
	n.sent++
	return true
}

// observe — проверки цикла по очереди. Нет ключа в результате — проверка не выполнялась (отключена или нечем
// судить), состояние алерта не меняется.
func (s *SystemCheck) observe(ctx context.Context) map[string]observation {
	out := map[string]observation{}
	chCtx, cancel := context.WithTimeout(ctx, clickhouseCheckTimeout)
	pingErr := s.cfg.ClickHouse.Ping(chCtx)
	cancel()
	if pingErr != nil {
		s.cfg.Logger.WarnContext(ctx, "clickhouse ping failed", slog.Any("error", pingErr))
		out["clickhouse_down"] = observation{bad: true, value: "error", problem: "ClickHouse не отвечает"}
	} else {
		out["clickhouse_down"] = observation{value: "ok", recovered: "ClickHouse отвечает"}
		if o, ok := s.checkDisk(ctx); ok {
			out["disk_free"] = o
		}
	}
	if o, ok := s.checkInserts(ctx); ok {
		out["insert_errors"] = o
	}
	if o, ok := s.checkBackup(ctx); ok {
		out["backup_stale"] = o
	}
	if s.cfg.IngestHealthURL != "" {
		out["ingest_down"] = s.checkHealth(ctx, s.cfg.IngestHealthURL, "Ingest", "ingest")
	}
	if s.cfg.APIHealthURL != "" {
		out["api_down"] = s.checkHealth(ctx, s.cfg.APIHealthURL, "API", "API")
	}
	return out
}

func (s *SystemCheck) checkDisk(ctx context.Context) (observation, bool) {
	diskCtx, cancel := context.WithTimeout(ctx, clickhouseCheckTimeout)
	defer cancel()
	pct, err := s.cfg.ClickHouse.DiskFreePct(diskCtx)
	if err != nil {
		s.cfg.Logger.WarnContext(ctx, "disk free check failed", slog.Any("error", err))
		return observation{}, false
	}
	return observation{
		bad:       pct < float64(s.cfg.DiskFreeAlertPct),
		value:     strconv.FormatFloat(pct, 'f', 1, 64),
		problem:   "На диске ClickHouse свободно " + formatPct(pct),
		recovered: "свободное место на диске ClickHouse (" + formatPct(pct) + ")",
	}, true
}

// checkInserts — доля неудачных вставок в ClickHouse с прошлого цикла по счётчикам /metrics ingest. Первый замер и
// сброс счётчиков (рестарт ingest) только запоминаются; /metrics недоступен — судить не по чему, это видно по
// ingest_down.
func (s *SystemCheck) checkInserts(ctx context.Context) (observation, bool) {
	if s.cfg.IngestMetricsURL == "" {
		return observation{}, false
	}
	cur, err := s.scrapeInserts(ctx)
	if err != nil {
		s.cfg.Logger.WarnContext(ctx, "ingest metrics unavailable", slog.Any("error", err))
		return observation{}, false
	}
	prev := s.prevInserts
	s.prevInserts = &cur
	if prev == nil || cur.errors < prev.errors || cur.inserts < prev.inserts {
		return observation{}, false
	}
	failed, total := cur.errors-prev.errors, cur.inserts-prev.inserts
	pct := 0.0
	if total > 0 {
		pct = failed / total * 100
	}
	return observation{
		bad:   pct > insertErrorsMaxPct,
		value: strconv.FormatFloat(pct, 'f', 1, 64),
		problem: fmt.Sprintf("Ошибки вставки в ClickHouse: %s вставок (%s из %s)",
			formatPct(pct), strconv.FormatFloat(failed, 'f', 0, 64), strconv.FormatFloat(total, 'f', 0, 64)),
		recovered: "вставка в ClickHouse (ошибок " + formatPct(pct) + ")",
	}, true
}

// scrapeInserts читает из текстового формата Prometheus ingest_clickhouse_insert_errors_total и число вставок —
// _count гистограммы ingest_clickhouse_insert_duration_seconds (она наблюдает каждую вставку, и удачную, и нет).
func (s *SystemCheck) scrapeInserts(ctx context.Context) (insertCounters, error) {
	body, status, err := s.get(ctx, s.cfg.IngestMetricsURL, httpCheckTimeout, maxMetricsBytes)
	if err != nil {
		return insertCounters{}, err
	}
	if status != http.StatusOK {
		return insertCounters{}, fmt.Errorf("ingest metrics: HTTP %d", status)
	}
	var c insertCounters
	var seenErrors, seenInserts bool
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		name, value, ok := parseSample(sc.Text())
		if !ok {
			continue
		}
		switch name {
		case "ingest_clickhouse_insert_errors_total":
			c.errors, seenErrors = c.errors+value, true
		case "ingest_clickhouse_insert_duration_seconds_count":
			c.inserts, seenInserts = c.inserts+value, true
		}
	}
	if !seenErrors || !seenInserts {
		return insertCounters{}, errors.New("ingest metrics: insert counters not found")
	}
	return c, nil
}

// parseSample — имя метрики без меток и значение строки сэмпла «name{labels} value [timestamp]».
func parseSample(line string) (string, float64, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return "", 0, false
	}
	name, rest, _ := strings.Cut(line, " ")
	if i := strings.IndexByte(name, '{'); i >= 0 {
		name = line[:i]
		end := strings.LastIndexByte(line, '}')
		if end < 0 {
			return "", 0, false
		}
		rest = strings.TrimSpace(line[end+1:])
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", 0, false
	}
	return name, v, true
}

func (s *SystemCheck) checkBackup(ctx context.Context) (observation, bool) {
	if s.cfg.Backups == nil {
		return observation{}, false
	}
	backupCtx, cancel := context.WithTimeout(ctx, backupCheckTimeout)
	defer cancel()
	b, ok, err := s.cfg.Backups.Latest(backupCtx)
	switch {
	case err != nil:
		s.cfg.Logger.WarnContext(ctx, "backup list unavailable", slog.Any("error", err))
		return observation{bad: true, value: "error", problem: "Список бэкапов ClickHouse недоступен", recovered: "список бэкапов ClickHouse"}, true
	case !ok:
		return observation{bad: true, value: "none", problem: "Бэкапов ClickHouse нет", recovered: "бэкап ClickHouse создан"}, true
	}
	age := max(s.cfg.Clock.Now().Sub(b.CreatedAt), 0)
	hours := strconv.Itoa(int(age / time.Hour))
	return observation{
		bad:       age > backupMaxAge,
		value:     hours,
		problem:   "Последний бэкап ClickHouse " + hours + " ч назад",
		recovered: "бэкап ClickHouse (" + hours + " ч назад)",
	}, true
}

// checkHealth — GET /health за 3 с: не 200 или нет ответа — неудача.
func (s *SystemCheck) checkHealth(ctx context.Context, rawURL, title, name string) observation {
	_, status, err := s.get(ctx, rawURL, httpCheckTimeout, 1<<10)
	path := "/health"
	if u, perr := url.Parse(rawURL); perr == nil {
		path = u.Path
	}
	switch {
	case err != nil:
		s.cfg.Logger.WarnContext(ctx, "health check failed", slog.String("service", name), slog.Any("error", err))
		return observation{bad: true, value: "error", problem: title + " не отвечает: " + path + " недоступен"}
	case status != http.StatusOK:
		code := strconv.Itoa(status)
		return observation{bad: true, value: code, problem: title + " не отвечает: " + path + " → " + code}
	}
	return observation{value: "200", recovered: name + " отвечает"}
}

// get — GET с таймаутом: тело (не больше limit байт) и статус. Ошибка без URL: в адресе пинга — секрет.
func (s *SystemCheck) get(ctx context.Context, rawURL string, timeout time.Duration, limit int64) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, errors.New("invalid URL")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return string(body), resp.StatusCode, nil
}

// ping — GET HEALTHCHECKS_PING_URL (или …/fail): "ok", "error" или "off" без URL. Пинг не прерывается отменой
// цикла: healthchecks.io должен узнать и о сбое по таймауту.
func (s *SystemCheck) ping(ctx context.Context, failed bool) string {
	if s.cfg.HealthchecksPingURL == "" {
		return "off"
	}
	u := s.cfg.HealthchecksPingURL
	if failed {
		u = strings.TrimRight(u, "/") + "/fail"
	}
	_, status, err := s.get(context.WithoutCancel(ctx), u, pingTimeout, 1<<10)
	switch {
	case err != nil:
		s.cfg.Logger.WarnContext(ctx, "healthchecks ping failed", slog.Any("error", err))
		return "error"
	case status/100 != 2:
		s.cfg.Logger.WarnContext(ctx, "healthchecks ping failed", slog.Int("status", status))
		return "error"
	}
	return "ok"
}

// formatPct — процент с одним знаком без лишнего нуля, с запятой: 15 %, 14,2 %.
func formatPct(v float64) string {
	v = math.Round(v*10) / 10
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1) + " %"
}

// formatDuration — сколько длится проблема: «45 мин», «6 ч».
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return strconv.Itoa(int(d/time.Minute)) + " мин"
	}
	return strconv.Itoa(int(d/time.Hour)) + " ч"
}

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
