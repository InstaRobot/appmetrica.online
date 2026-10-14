package worker

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/reports"
)

// DigestStore — настройки, активные приложения и журнал digest (pgstore/sqlc.Queries).
type DigestStore interface {
	SettingsReader
	ListApps(ctx context.Context, includeArchived bool) ([]sqlc.App, error)
	GetDigestDayState(ctx context.Context, since time.Time) (sqlc.GetDigestDayStateRow, error)
}

// PortfolioReader — reports.Service.
type PortfolioReader interface {
	Portfolio(ctx context.Context, q reports.PortfolioQuery) (reports.Portfolio, error)
}

const (
	digestEvery = time.Minute
	// digestTimeout — до 3 попыток Telegram с retry_after до 60 с и таймаутом запроса 10 с, плюс портфель.
	digestTimeout = 5 * time.Minute
	// digestMaxAttempts и digestRetryAfter — неудачных запусков за сутки владельца и пауза между ними
	// (03-architecture.md, retry policy).
	digestMaxAttempts = 3
	digestRetryAfter  = 10 * time.Minute
	// digestMaxApps — строк приложений в сообщении (OQ-012).
	digestMaxApps = 30
	// digestMaxLineChars — видимых символов на строки приложений: лимит sendMessage 4096 символов без
	// разметки за вычетом заголовка, «и ещё N приложений» и ссылки.
	digestMaxLineChars = 4096 - 200
)

type digest struct {
	store     DigestStore
	portfolio PortfolioReader
	tg        TelegramSender
	clock     clock.Clock
	logger    *slog.Logger
	baseURL   string
}

// DigestJob — job digest раз в минуту (FR-020): когда в settings.owner_timezone наступило digest_time, а сегодня
// по этим суткам дайджест ещё не отправлен, шлёт в telegram_chat_id «Портфель за вчера» (production, unit
// persons) по активным приложениям. Отправленный дайджест отмечается в job_runs.details.sent_on; неудачный запуск
// повторяется через 10 минут, не больше 3 раз за сутки. tg = nil (нет TELEGRAM_BOT_TOKEN) — текст пишется в лог.
func DigestJob(store DigestStore, portfolio PortfolioReader, tg TelegramSender, c clock.Clock, logger *slog.Logger, baseURL string) Job {
	d := &digest{store: store, portfolio: portfolio, tg: tg, clock: c, logger: logger, baseURL: baseURL}
	return Job{Name: "digest", Every: digestEvery, Timeout: digestTimeout, Run: d.run}
}

func (d *digest) run(ctx context.Context, _ uuid.UUID) (Details, error) {
	st, err := d.store.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("digest: settings: %w", err)
	}
	if !st.DigestEnabled || st.TelegramChatID == nil || *st.TelegramChatID == "" {
		return Details{"skipped": "disabled"}, nil
	}
	loc, err := time.LoadLocation(st.OwnerTimezone)
	if err != nil {
		return nil, fmt.Errorf("digest: owner timezone %q: %w", st.OwnerTimezone, err)
	}
	now := d.clock.Now().In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	// Время суток по часам владельца, а не прошедшее с полуночи: в день перевода часов они расходятся.
	sinceMidnight := time.Duration(now.Hour())*time.Hour + time.Duration(now.Minute())*time.Minute +
		time.Duration(now.Second())*time.Second + time.Duration(now.Nanosecond())
	if sinceMidnight < time.Duration(st.DigestTime.Microseconds)*time.Microsecond {
		return Details{"skipped": "not_yet"}, nil
	}

	state, err := d.store.GetDigestDayState(ctx, dayStart)
	if err != nil {
		return nil, fmt.Errorf("digest: job_runs: %w", err)
	}
	switch {
	case state.Sent:
		return Details{"skipped": "already_sent"}, nil
	case state.FailedAttempts >= digestMaxAttempts:
		return Details{"skipped": "attempts_exhausted"}, nil
	case state.FailedAttempts > 0 && now.Sub(state.LastFailedAt) < digestRetryAfter:
		return Details{"skipped": "retry_wait"}, nil
	}

	sentOn := now.Format(time.DateOnly)
	apps, err := d.store.ListApps(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("digest: list apps: %w", err)
	}
	if len(apps) == 0 {
		return Details{"skipped": "no_apps", "sent_on": sentOn}, nil
	}

	q := reports.PortfolioQuery{OwnerLocation: loc, Env: reports.EnvProduction, Unit: identity.Persons}
	for _, app := range apps {
		appLoc, err := time.LoadLocation(app.Timezone)
		if err != nil {
			return nil, fmt.Errorf("digest: app %s timezone %q: %w", app.ID, app.Timezone, err)
		}
		q.Apps = append(q.Apps, reports.PortfolioApp{ID: app.ID, Location: appLoc})
	}
	p, err := d.portfolio.Portfolio(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("digest: %w", err)
	}
	date, err := time.Parse(time.DateOnly, p.Date)
	if err != nil {
		return nil, fmt.Errorf("digest: portfolio date %q: %w", p.Date, err)
	}
	rows := make([]digestRow, len(apps))
	for i, app := range apps {
		rows[i] = digestRow{Name: app.Name, Row: p.Rows[i]}
	}
	text := formatDigest(date, rows, d.baseURL)
	details := Details{"date": p.Date, "apps": len(apps)}

	if d.tg == nil {
		d.logger.InfoContext(ctx, "digest not sent: TELEGRAM_BOT_TOKEN is not set", slog.String("text", text))
		details["sent_on"], details["delivery"] = sentOn, "log"
		return details, nil
	}
	id, err := d.tg.Send(ctx, *st.TelegramChatID, text)
	if err != nil {
		return details, fmt.Errorf("digest: telegram: %w", err)
	}
	details["sent_on"], details["telegram_message_id"] = sentOn, id
	return details, nil
}

// digestRow — строка портфеля и имя приложения.
type digestRow struct {
	Name string
	Row  reports.PortfolioRow
}

// nbsp — неразрывный пробел между разрядами и перед «%»: Telegram не переносит число посередине.
const nbsp = " "

var weekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// formatDigest — сообщение дайджеста (OQ-012, parse_mode HTML): заголовок с датой и суммой DAU, строки
// приложений по убыванию DAU (не больше 30 и не длиннее лимита Telegram, остальные — «и ещё N приложений»),
// ссылка на портфель.
func formatDigest(date time.Time, rows []digestRow, baseURL string) string {
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b digestRow) int {
		return cmp.Or(cmp.Compare(b.Row.ActiveDate, a.Row.ActiveDate), strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)))
	})
	var total uint64
	for _, r := range rows {
		total += r.Row.ActiveDate
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Портфель за %s</b> · DAU %s\n", date.Format("02.01.2006"), groupThousands(total))
	weekday := weekdays[date.Weekday()]
	shown, budget := 0, digestMaxLineChars
	for _, r := range rows[:min(len(rows), digestMaxApps)] {
		stats := digestStats(r.Row, weekday)
		n := utf8.RuneCountInString(r.Name) + utf8.RuneCountInString(stats) + 1
		if n > budget {
			break
		}
		budget -= n
		fmt.Fprintf(&b, "<b>%s</b>%s\n", html.EscapeString(r.Name), stats)
		shown++
	}
	if rest := len(rows) - shown; rest > 0 {
		fmt.Fprintf(&b, "и ещё %d %s\n", rest, pluralApps(rest))
	}
	fmt.Fprintf(&b, `<a href="%s/">Открыть портфель</a>`, html.EscapeString(baseURL))
	return b.String()
}

// digestStats — метрики строки приложения после имени: « · DAU 12 480 (▲4,1 % к пн) · новые … · D1 38 %».
// Без активных неделю назад Δ не показывается, при пустой когорте D1 — «—».
func digestStats(r reports.PortfolioRow, weekday string) string {
	var b strings.Builder
	b.WriteString(" · DAU " + groupThousands(r.ActiveDate))
	if v := r.ActiveDeltaPct; v != nil {
		fmt.Fprintf(&b, " (%s к %s)", formatDelta(*v), weekday)
	}
	fmt.Fprintf(&b, " · новые %s · сессии %s · D1 ", groupThousands(r.NewDate), groupThousands(r.SessionsDate))
	if v := r.RetentionD1; v != nil {
		b.WriteString(strconv.FormatFloat(math.Round(*v*100), 'f', 0, 64) + nbsp + "%")
	} else {
		b.WriteString("—")
	}
	return b.String()
}

// formatDelta — Δ в процентах с одним знаком после запятой: ▲4,1 %, ▼1,2 %, 0,0 %.
func formatDelta(v float64) string {
	arrow := ""
	switch {
	case v > 0:
		arrow = "▲"
	case v < 0:
		arrow = "▼"
	}
	return arrow + strings.Replace(strconv.FormatFloat(math.Abs(v), 'f', 1, 64), ".", ",", 1) + nbsp + "%"
}

// groupThousands — число с разрядами через неразрывный пробел: 12 480.
func groupThousands(n uint64) string {
	s := strconv.FormatUint(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(nbsp)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// pluralApps — «приложение», «приложения» или «приложений» для n.
func pluralApps(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return "приложение"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "приложения"
	default:
		return "приложений"
	}
}
