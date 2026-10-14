package api

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/clock"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	telegramTestText = "Тестовое сообщение appmetrica.online"

	msgNoChatID   = "Сначала укажите Telegram chat ID"
	msgNoBotToken = "Токен Telegram-бота не задан: укажите TELEGRAM_BOT_TOKEN" //nolint:gosec // имя переменной, не секрет
)

var (
	digestTimeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	chatIDRe     = regexp.MustCompile(`^-?[0-9]{1,20}$`)
)

// SettingsStore — строка settings в Postgres.
type SettingsStore interface {
	SettingsReader
	UpdateSettings(ctx context.Context, arg sqlc.UpdateSettingsParams) (sqlc.UpdateSettingsRow, error)
}

// TelegramSender отправляет сообщение бота и возвращает message_id; ошибка Telegram — текст его description.
type TelegramSender interface {
	Send(ctx context.Context, chatID, text string) (int64, error)
}

// WithSettings подключает getSettings, updateSettings и sendTelegramTest. tg nil — TELEGRAM_BOT_TOKEN не задан.
func (s *Server) WithSettings(store SettingsStore, tg TelegramSender, c clock.Clock) *Server {
	s.settingsStore = store
	s.telegram = tg
	s.clock = c
	return s
}

func (s *Server) GetSettings(ctx context.Context, _ gen.GetSettingsRequestObject) (gen.GetSettingsResponseObject, error) {
	row, err := s.settingsStore.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: get settings: %w", err)
	}
	return gen.GetSettings200JSONResponse(settingsModel(row)), nil
}

// UpdateSettings меняет переданные поля. telegram_chat_id: null выключает дайджест: без чата его некуда слать
// (CHECK settings_digest_requires_chat_chk); включить дайджест без чата нельзя — 400 по digest_enabled.
func (s *Server) UpdateSettings(ctx context.Context, req gen.UpdateSettingsRequestObject) (gen.UpdateSettingsResponseObject, error) {
	if req.Body == nil {
		return gen.UpdateSettings400JSONResponse{BadRequestJSONResponse: invalidBody(ctx)}, nil
	}
	p, specified, ferr := parseSettingsBody(req.Body)
	switch {
	case !specified:
		return gen.UpdateSettings400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(
			apiError(ctx, "invalid_request", "Укажите хотя бы одну настройку", nil))}, nil
	case ferr.any():
		return gen.UpdateSettings400JSONResponse{BadRequestJSONResponse: ferr.response(ctx)}, nil
	}

	current, err := s.settingsStore.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: update settings: %w", err)
	}
	hasChat := p.TelegramChatID != nil || (current.TelegramChatID != nil && !p.ClearTelegramChatID)
	switch {
	case hasChat:
	case p.DigestEnabled != nil && *p.DigestEnabled:
		ferr.add("digest_enabled", msgNoChatID)
		return gen.UpdateSettings400JSONResponse{BadRequestJSONResponse: ferr.response(ctx)}, nil
	case current.DigestEnabled:
		off := false
		p.DigestEnabled = &off
	}

	p.Now = s.clock.Now()
	row, err := s.settingsStore.UpdateSettings(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("api: update settings: %w", err)
	}
	return gen.UpdateSettings200JSONResponse(settingsModel(sqlc.GetSettingsRow(row))), nil
}

// parseSettingsBody проверяет поля тела по порядку схемы UpdateSettingsBody; specified=false — ни одного поля.
func parseSettingsBody(b *gen.UpdateSettingsBody) (p sqlc.UpdateSettingsParams, specified bool, ferr fieldErrors) {
	if b.OwnerTimezone != nil {
		specified = true
		if apps.ValidTimezone(*b.OwnerTimezone) {
			p.OwnerTimezone = b.OwnerTimezone
		} else {
			ferr.add("owner_timezone", "Неизвестный часовой пояс")
		}
	}
	if b.TelegramChatId.IsSpecified() {
		specified = true
		if b.TelegramChatId.IsNull() {
			p.ClearTelegramChatID = true
		} else if v, _ := b.TelegramChatId.Get(); chatIDRe.MatchString(v) {
			p.TelegramChatID = &v
		} else {
			ferr.add("telegram_chat_id", "Chat ID — число, например 123456789 или -1001234567890")
		}
	}
	if b.DigestEnabled != nil {
		specified = true
		p.DigestEnabled = b.DigestEnabled
	}
	if b.DigestTime != nil {
		specified = true
		if t, ok := parseClock(*b.DigestTime); ok {
			p.DigestTime = t
		} else {
			ferr.add("digest_time", "Время в формате ЧЧ:ММ, от 00:00 до 23:59")
		}
	}
	if b.AlertsEnabled != nil {
		specified = true
		p.AlertsEnabled = b.AlertsEnabled
	}
	return p, specified, ferr
}

// SendTelegramTest — ошибка Telegram и отсутствие токена бота — 200 sent=false, а не 5xx: это не сбой API.
func (s *Server) SendTelegramTest(ctx context.Context, _ gen.SendTelegramTestRequestObject) (gen.SendTelegramTestResponseObject, error) {
	settings, err := s.settingsStore.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: telegram test: %w", err)
	}
	if settings.TelegramChatID == nil {
		return gen.SendTelegramTest400JSONResponse{
			BadRequestJSONResponse: invalidParam("telegram_chat_id", msgNoChatID).response(ctx),
		}, nil
	}
	if s.telegram == nil {
		msg := msgNoBotToken
		return gen.SendTelegramTest200JSONResponse{Sent: false, Error: &msg}, nil
	}
	id, err := s.telegram.Send(ctx, *settings.TelegramChatID, telegramTestText)
	if err != nil {
		s.logger.WarnContext(ctx, "telegram test message failed",
			slog.String("request_id", httpx.RequestIDFrom(ctx)), slog.Any("error", err))
		msg := err.Error()
		return gen.SendTelegramTest200JSONResponse{Sent: false, Error: &msg}, nil
	}
	return gen.SendTelegramTest200JSONResponse{Sent: true, TelegramMessageId: &id}, nil
}

func settingsModel(row sqlc.GetSettingsRow) gen.Settings {
	return gen.Settings{
		OwnerTimezone:  row.OwnerTimezone,
		TelegramChatId: nullableValue(row.TelegramChatID),
		DigestEnabled:  row.DigestEnabled,
		DigestTime:     formatClock(row.DigestTime),
		AlertsEnabled:  row.AlertsEnabled,
		UpdatedAt:      row.UpdatedAt.UTC(),
	}
}

// parseClock — «ЧЧ:ММ» в Postgres time.
func parseClock(s string) (pgtype.Time, bool) {
	if !digestTimeRe.MatchString(s) {
		return pgtype.Time{}, false
	}
	h, _ := strconv.Atoi(s[:2])
	m, _ := strconv.Atoi(s[3:])
	d := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute
	return pgtype.Time{Microseconds: d.Microseconds(), Valid: true}, true
}

// formatClock — Postgres time в «ЧЧ:ММ» (секунды в digest_time не хранятся).
func formatClock(t pgtype.Time) string {
	d := time.Duration(t.Microseconds) * time.Microsecond
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// fieldErrors — ошибки нескольких полей тела: details.field — первое, details.fields — все с текстами.
type fieldErrors struct {
	order []string
	msgs  map[string]any
}

func (f *fieldErrors) add(field, message string) {
	if f.msgs == nil {
		f.msgs = map[string]any{}
	}
	f.order = append(f.order, field)
	f.msgs[field] = message
}

func (f *fieldErrors) any() bool { return len(f.order) > 0 }

func (f *fieldErrors) response(ctx context.Context) gen.BadRequestJSONResponse {
	first := f.order[0]
	message, _ := f.msgs[first].(string)
	return gen.BadRequestJSONResponse(apiError(ctx, "invalid_request", message,
		map[string]any{"field": first, "fields": f.msgs}))
}
