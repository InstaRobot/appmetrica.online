package api

import (
	"context"
	"errors"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

var errNotImplemented = errors.New("api: not implemented")

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

func (s *Server) GetSettings(_ context.Context, _ gen.GetSettingsRequestObject) (gen.GetSettingsResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) UpdateSettings(_ context.Context, _ gen.UpdateSettingsRequestObject) (gen.UpdateSettingsResponseObject, error) {
	return nil, errNotImplemented
}

func (s *Server) SendTelegramTest(_ context.Context, _ gen.SendTelegramTestRequestObject) (gen.SendTelegramTestResponseObject, error) {
	return nil, errNotImplemented
}
