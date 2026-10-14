package worker

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// deletionTimeout — предел цикла deletion: до 10 запросов с мутациями ClickHouse (mutations_sync=2).
// Меньше часа, после которого запрос running считается брошенным (deletion.staleRunning).
const deletionTimeout = 30 * time.Minute

// DeletionRunner — deletion.Executor.
type DeletionRunner interface {
	RunOnce(ctx context.Context, jobRunID uuid.UUID) (deletion.Result, error)
}

// DeletionJob — job deletion раз в минуту (FR-019).
func DeletionJob(r DeletionRunner) Job {
	return Job{
		Name: "deletion", Every: time.Minute, Timeout: deletionTimeout,
		Run: func(ctx context.Context, runID uuid.UUID) (Details, error) {
			res, err := r.RunOnce(ctx, runID)
			return Details{"requeued": res.Requeued, "done": res.Done, "retried": res.Retried, "failed": res.Failed}, err
		},
	}
}

// SettingsReader — настройки владельца (chat_id и флаг алертов).
type SettingsReader interface {
	GetSettings(ctx context.Context) (sqlc.GetSettingsRow, error)
}

// TelegramSender — telegram.Client; nil — TELEGRAM_BOT_TOKEN не задан.
type TelegramSender interface {
	Send(ctx context.Context, chatID, text string) (int64, error)
}

// DeletionFailedNotifier — алерт «Удаление failed после 5 попыток» (03-architecture.md, алерты) в чат
// владельца, если алерты включены и chat_id задан. subject_value (user_id, install_id) в сообщение не входит.
func DeletionFailedNotifier(settings SettingsReader, tg TelegramSender, logger *slog.Logger) func(context.Context, sqlc.DeletionRequest, string) {
	return func(ctx context.Context, dr sqlc.DeletionRequest, cause string) {
		if tg == nil {
			return
		}
		st, err := settings.GetSettings(ctx)
		if err != nil {
			logger.ErrorContext(ctx, "deletion alert skipped: settings unavailable", slog.Any("error", err))
			return
		}
		if !st.AlertsEnabled || st.TelegramChatID == nil || *st.TelegramChatID == "" {
			return
		}
		text := fmt.Sprintf("⚠️ <b>Удаление данных не выполнено</b>\n"+
			"Запрос <code>%s</code>, приложение <code>%s</code>, субъект: %s.\n"+
			"Попыток: %d. Последняя ошибка: %s\n"+
			"Создайте запрос заново после устранения причины.",
			dr.ID, dr.AppID, html.EscapeString(string(dr.SubjectType)), dr.Attempts, html.EscapeString(cause))
		if _, err := tg.Send(ctx, *st.TelegramChatID, text); err != nil {
			logger.ErrorContext(ctx, "deletion alert not sent",
				slog.String("deletion_request_id", dr.ID.String()), slog.Any("error", err))
		}
	}
}
