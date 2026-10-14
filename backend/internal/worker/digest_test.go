//go:build integration

package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/telegram"
	"appmetrica.online/backend/internal/testutil"
	"appmetrica.online/backend/internal/worker"
)

// digestNow — 2026-10-16 09:00:30 по Москве: дайджест за четверг 2026-10-15 — в сценарии у приложений есть
// D1 (когорты 2026-10-14) и Δ к 2026-10-08 в обе стороны.
var digestNow = time.Date(2026, 10, 16, 6, 0, 30, 0, time.UTC)

const (
	digestToken   = "123456:TEST-TOKEN"
	digestBaseURL = "https://appmetrica.online"
)

// tgRequest — запрос sendMessage, принятый fakeBotAPI.
type tgRequest struct {
	Path      string
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// tgReply — ответ fakeBotAPI: HTTP-статус и тело.
type tgReply struct {
	status int
	body   string
}

// fakeBotAPI — httptest вместо api.telegram.org: отвечает из очереди replies, после неё — ok.
type fakeBotAPI struct {
	srv     *httptest.Server
	mu      sync.Mutex
	replies []tgReply
	got     []tgRequest
}

func newFakeBotAPI(t *testing.T, replies ...tgReply) *fakeBotAPI {
	t.Helper()
	f := &fakeBotAPI{replies: replies}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req tgRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Path = r.URL.Path
		f.mu.Lock()
		f.got = append(f.got, req)
		reply := tgReply{http.StatusOK, `{"ok":true,"result":{"message_id":` + strconv.Itoa(len(f.got)+100) + `}}`}
		if len(f.replies) > 0 {
			reply, f.replies = f.replies[0], f.replies[1:]
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBotAPI) requests() []tgRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tgRequest(nil), f.got...)
}

var (
	reply429 = tgReply{
		http.StatusTooManyRequests,
		`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`,
	}
	reply400 = tgReply{http.StatusBadRequest, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`}
)

type digestHarness struct {
	env    *testutil.Env
	clock  *clock.Fixed
	bot    *fakeBotAPI
	tg     *telegram.Client
	reg    *prometheus.Registry
	sleeps []time.Duration
	logs   *bytes.Buffer
	sched  *worker.Scheduler
}

type digestOption func(*digestHarness)

// withoutTelegram — TELEGRAM_BOT_TOKEN не задан: дайджест только пишется в лог.
func withoutTelegram() digestOption { return func(h *digestHarness) { h.tg = nil } }

func newDigestHarness(t *testing.T, bot *fakeBotAPI, opts ...digestOption) *digestHarness {
	t.Helper()
	env := testutil.Start(t)
	env.LoadNDJSON(t, testutil.ScenarioPath())
	h := &digestHarness{env: env, clock: clock.NewFixed(digestNow), bot: bot, reg: prometheus.NewRegistry(), logs: &bytes.Buffer{}}
	h.tg = telegram.New(digestToken, telegram.WithBaseURL(bot.srv.URL), telegram.WithSleep(func(_ context.Context, d time.Duration) error {
		h.sleeps = append(h.sleeps, d)
		return nil
	}))
	require.NoError(t, h.tg.RegisterMetrics(h.reg))
	for _, o := range opts {
		o(h)
	}
	logger := slog.New(slog.NewJSONHandler(h.logs, nil))
	var tg worker.TelegramSender
	if h.tg != nil {
		tg = h.tg
	}
	h.sched = worker.NewScheduler(env.PG.Queries(), h.clock, logger, []string{"digest"},
		worker.DigestJob(env.PG.Queries(), reports.NewService(env.CH.Conn(), h.clock), tg, h.clock, logger, digestBaseURL))

	// Портфель: московское и нью-йоркское приложения сценария, приложение без событий; архивное не входит.
	h.addApp(t, testutil.ScenarioApps[0].Name, testutil.ScenarioApps[0], "active")
	h.addApp(t, testutil.ScenarioApps[1].Name, testutil.ScenarioApps[1], "active")
	h.addApp(t, testutil.ScenarioApps[2].Name, testutil.ScenarioApps[2], "archived")
	_, err := env.PG.Pool().Exec(t.Context(),
		`INSERT INTO apps (name, platform, timezone) VALUES ('Бот поддержки & <FAQ>', 'bot', 'Europe/Moscow')`)
	require.NoError(t, err)
	h.setSettings(t, "UPDATE settings SET telegram_chat_id = '123', digest_enabled = true, digest_time = '09:00', owner_timezone = 'Europe/Moscow'")
	return h
}

func (h *digestHarness) addApp(t *testing.T, name string, app testutil.ScenarioApp, status string) {
	t.Helper()
	var archivedAt *time.Time
	if status == "archived" {
		archivedAt = &digestNow
	}
	_, err := h.env.PG.Pool().Exec(t.Context(),
		`INSERT INTO apps (id, name, platform, timezone, status, archived_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		app.ID, name, app.Platform, app.Timezone, status, archivedAt)
	require.NoError(t, err)
}

func (h *digestHarness) setSettings(t *testing.T, stmt string) {
	t.Helper()
	_, err := h.env.PG.Pool().Exec(t.Context(), stmt)
	require.NoError(t, err)
}

// runAt — RunOnce("digest") в момент at.
func (h *digestHarness) runAt(t *testing.T, at time.Time) error {
	t.Helper()
	h.clock.Set(at)
	return h.sched.RunOnce(t.Context(), "digest")
}

func (h *digestHarness) runs(t *testing.T) []jobRun {
	t.Helper()
	rows, err := h.env.PG.Pool().Query(t.Context(),
		`SELECT status::text, details FROM job_runs WHERE job = 'digest' ORDER BY started_at, id`)
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (jobRun, error) {
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
	return out
}

func (h *digestHarness) lastRun(t *testing.T) jobRun {
	t.Helper()
	runs := h.runs(t)
	require.NotEmpty(t, runs)
	return runs[len(runs)-1]
}

// AC-051.1 (AC-8.1.1, AC-8.1.2): в 09:00 по Москве отправлен один дайджест за вчера — текст совпадает с golden
// (формат OQ-012); в 09:05 того же дня и после переноса digest_time на более раннее сообщений нет; до 09:00 —
// тоже нет. На следующий день — дайджест за новый вчерашний день.
func TestDigest_SendsOncePerDay(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newDigestHarness(t, bot)

	require.NoError(t, h.runAt(t, digestNow.Add(-time.Minute)))
	require.Empty(t, bot.requests(), "08:59:30 — ещё рано")
	require.Equal(t, map[string]any{"skipped": "not_yet"}, h.lastRun(t).Details)

	require.NoError(t, h.runAt(t, digestNow))
	got := bot.requests()
	require.Len(t, got, 1)
	require.Equal(t, "/bot"+digestToken+"/sendMessage", got[0].Path)
	require.Equal(t, "123", got[0].ChatID)
	require.Equal(t, "HTML", got[0].ParseMode)
	testutil.AssertGoldenText(t, filepath.Join("testdata", "digest.golden.txt"), got[0].Text)
	run := h.lastRun(t)
	require.Equal(t, "ok", run.Status)
	require.Equal(t, map[string]any{
		"sent_on": "2026-10-16", "date": "2026-10-15", "apps": float64(3), "telegram_message_id": float64(101),
	}, run.Details)

	require.NoError(t, h.runAt(t, digestNow.Add(4*time.Minute+30*time.Second)))
	require.Len(t, bot.requests(), 1, "09:05 — дайджест уже отправлен сегодня")
	require.Equal(t, map[string]any{"skipped": "already_sent"}, h.lastRun(t).Details)

	h.setSettings(t, "UPDATE settings SET digest_time = '08:00'")
	require.NoError(t, h.runAt(t, digestNow.Add(10*time.Minute)))
	require.Len(t, bot.requests(), 1, "перенос digest_time раньше не даёт повторной отправки")

	require.NoError(t, h.runAt(t, digestNow.Add(24*time.Hour)))
	got = bot.requests()
	require.Len(t, got, 2)
	require.Contains(t, got[1].Text, "<b>Портфель за 16.10.2026</b>")
	require.Equal(t, "2026-10-17", h.lastRun(t).Details["sent_on"])

	for _, r := range h.runs(t) {
		require.Equal(t, "ok", r.Status)
	}
}

// AC-051.2 (AC-8.1.3): 429 с retry_after=1, затем 200 — ok после 2 попыток; 400 chat not found — job_runs failed,
// worker_telegram_send_errors_total = 1.
func TestDigest_TelegramErrors(t *testing.T) {
	t.Run("429 then 200", func(t *testing.T) {
		bot := newFakeBotAPI(t, reply429)
		h := newDigestHarness(t, bot)

		require.NoError(t, h.runAt(t, digestNow))
		require.Len(t, bot.requests(), 2)
		require.Equal(t, []time.Duration{time.Second}, h.sleeps)
		run := h.lastRun(t)
		require.Equal(t, "ok", run.Status)
		require.Equal(t, "2026-10-16", run.Details["sent_on"])
		require.InDelta(t, 0, h.tgErrors(t), 0)
	})

	t.Run("400 chat not found", func(t *testing.T) {
		bot := newFakeBotAPI(t, reply400)
		h := newDigestHarness(t, bot)

		require.ErrorContains(t, h.runAt(t, digestNow), "chat not found")
		require.Len(t, bot.requests(), 1, "400 не повторяется")
		run := h.lastRun(t)
		require.Equal(t, "failed", run.Status)
		require.Contains(t, run.Details["error"], "chat not found")
		require.Equal(t, "2026-10-15", run.Details["date"])
		require.NotContains(t, run.Details, "sent_on")
		require.InDelta(t, 1, h.tgErrors(t), 0)
		require.NotContains(t, h.logs.String(), digestToken, "токен бота не попадает в лог")
	})
}

// tgErrors — значение worker_telegram_send_errors_total.
func (h *digestHarness) tgErrors(t *testing.T) float64 {
	t.Helper()
	families, err := h.reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == "worker_telegram_send_errors_total" {
			return f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatal("worker_telegram_send_errors_total не зарегистрирована")
	return 0
}

// 03-architecture.md: digest — 3 попытки через 10 минут; после трёх неудач до конца суток владельца
// сообщений нет, на следующий день — снова.
func TestDigest_RetriesThreeTimesTenMinutesApart(t *testing.T) {
	bot := newFakeBotAPI(t, reply400, reply400, reply400, reply400)
	h := newDigestHarness(t, bot)

	require.Error(t, h.runAt(t, digestNow))
	require.NoError(t, h.runAt(t, digestNow.Add(5*time.Minute)))
	require.Equal(t, map[string]any{"skipped": "retry_wait"}, h.lastRun(t).Details)
	require.Len(t, bot.requests(), 1)

	require.Error(t, h.runAt(t, digestNow.Add(10*time.Minute)))
	require.Error(t, h.runAt(t, digestNow.Add(20*time.Minute)))
	require.Len(t, bot.requests(), 3)

	require.NoError(t, h.runAt(t, digestNow.Add(30*time.Minute)))
	require.NoError(t, h.runAt(t, digestNow.Add(5*time.Hour)))
	require.Len(t, bot.requests(), 3, "три попытки за сутки исчерпаны")
	require.Equal(t, map[string]any{"skipped": "attempts_exhausted"}, h.lastRun(t).Details)

	require.Error(t, h.runAt(t, digestNow.Add(24*time.Hour)), "новые сутки — новые попытки")
	require.Len(t, bot.requests(), 4)
	require.NoError(t, h.runAt(t, digestNow.Add(24*time.Hour+10*time.Minute)))
	require.Len(t, bot.requests(), 5)
	require.Equal(t, "2026-10-17", h.lastRun(t).Details["sent_on"])
}

// FR-020: без digest_enabled или chat_id и без активных приложений сообщение не отправляется.
func TestDigest_SkipsWhenNothingToSend(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		bot := newFakeBotAPI(t)
		h := newDigestHarness(t, bot)
		h.setSettings(t, "UPDATE settings SET digest_enabled = false")

		require.NoError(t, h.runAt(t, digestNow))
		require.Empty(t, bot.requests())
		require.Equal(t, map[string]any{"skipped": "disabled"}, h.lastRun(t).Details)
	})

	t.Run("no chat id", func(t *testing.T) {
		bot := newFakeBotAPI(t)
		h := newDigestHarness(t, bot)
		h.setSettings(t, "UPDATE settings SET digest_enabled = false, telegram_chat_id = NULL")

		require.NoError(t, h.runAt(t, digestNow))
		require.Empty(t, bot.requests())
		require.Equal(t, map[string]any{"skipped": "disabled"}, h.lastRun(t).Details)
	})

	t.Run("no active apps", func(t *testing.T) {
		bot := newFakeBotAPI(t)
		h := newDigestHarness(t, bot)
		h.setSettings(t, "UPDATE apps SET status = 'archived', archived_at = now()")

		require.NoError(t, h.runAt(t, digestNow))
		require.Empty(t, bot.requests())
		run := h.lastRun(t)
		require.Equal(t, "ok", run.Status)
		require.Equal(t, map[string]any{"skipped": "no_apps", "sent_on": "2026-10-16"}, run.Details)

		require.NoError(t, h.runAt(t, digestNow.Add(5*time.Minute)))
		require.Equal(t, map[string]any{"skipped": "already_sent"}, h.lastRun(t).Details)
	})
}

// digest_time и «вчера» считаются в settings.owner_timezone, а не в UTC и не в TZ приложений.
func TestDigest_OwnerTimezone(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newDigestHarness(t, bot)
	h.setSettings(t, "UPDATE settings SET owner_timezone = 'America/New_York'")

	require.NoError(t, h.runAt(t, digestNow), "02:00 в Нью-Йорке")
	require.Empty(t, bot.requests())

	require.NoError(t, h.runAt(t, time.Date(2026, 10, 16, 13, 0, 0, 0, time.UTC)))
	got := bot.requests()
	require.Len(t, got, 1)
	require.Contains(t, got[0].Text, "<b>Портфель за 15.10.2026</b>")
	require.Equal(t, "2026-10-16", h.lastRun(t).Details["sent_on"])

	// 23:30 по Нью-Йорку — те же сутки владельца, хотя в UTC уже 17-е.
	require.NoError(t, h.runAt(t, time.Date(2026, 10, 17, 3, 30, 0, 0, time.UTC)))
	require.Len(t, bot.requests(), 1)
}

// В день перевода часов digest_time сравнивается с часами владельца: 25.10.2026 в Берлине с полуночи до 08:30
// проходит 9 ч 30 мин, но дайджест уходит в 09:00 по местным часам.
func TestDigest_DSTUsesWallClock(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newDigestHarness(t, bot)
	h.setSettings(t, "UPDATE settings SET owner_timezone = 'Europe/Berlin'")

	require.NoError(t, h.runAt(t, time.Date(2026, 10, 25, 7, 30, 0, 0, time.UTC)), "08:30 CET")
	require.Empty(t, bot.requests())
	require.Equal(t, map[string]any{"skipped": "not_yet"}, h.lastRun(t).Details)

	require.NoError(t, h.runAt(t, time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)), "09:00 CET")
	got := bot.requests()
	require.Len(t, got, 1)
	require.Contains(t, got[0].Text, "<b>Портфель за 24.10.2026</b>")
}

// Без TELEGRAM_BOT_TOKEN дайджест пишется в лог раз в сутки (12-dev-runbook.md §3.2).
func TestDigest_WithoutTokenLogsMessage(t *testing.T) {
	bot := newFakeBotAPI(t)
	h := newDigestHarness(t, bot, withoutTelegram())

	require.NoError(t, h.runAt(t, digestNow))
	require.Empty(t, bot.requests())
	require.Contains(t, h.logs.String(), "Портфель за 15.10.2026")
	run := h.lastRun(t)
	require.Equal(t, "ok", run.Status)
	require.Equal(t, map[string]any{
		"sent_on": "2026-10-16", "date": "2026-10-15", "apps": float64(3), "delivery": "log",
	}, run.Details)

	h.logs.Reset()
	require.NoError(t, h.runAt(t, digestNow.Add(time.Minute)))
	require.NotContains(t, h.logs.String(), "Портфель за")
}
