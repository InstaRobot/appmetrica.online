package worker

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	// mergeTimeout — предел запуска app_merge: копирование источника с 10 млн событий укладывается в 30 минут
	// (FR-8); следующий запуск ждёт окончания текущего.
	mergeTimeout = 30 * time.Minute
	mergeLockKey = "app_merge"
)

// MergeRunner — merge.Executor.
type MergeRunner interface {
	RunOnce(ctx context.Context, runID uuid.UUID) (merge.Result, error)
}

// MergeJob — job app_merge раз в минуту (FR-8) под advisory lock: два worker-а не выполняют шаги одного
// слияния одновременно.
func MergeJob(r MergeRunner, lock Locker) Job {
	return Job{
		Name: "app_merge", Every: time.Minute, Timeout: mergeTimeout,
		Run: func(ctx context.Context, runID uuid.UUID) (Details, error) {
			release, ok, err := lock.TryAdvisoryLock(ctx, mergeLockKey)
			if err != nil {
				return nil, fmt.Errorf("app_merge: %w", err)
			}
			if !ok {
				return Details{"skipped": "locked"}, nil
			}
			defer release()
			res, err := r.RunOnce(ctx, runID)
			if res.MergeID == uuid.Nil {
				return Details{}, err
			}
			return Details{"merge_id": res.MergeID.String(), "status": string(res.Status), "step": string(res.Step)}, err
		},
	}
}

// MergeFailedNotifier — алерт «Слияние упало» (05-impact-and-regression.md §7.4) в чат владельца, если алерты
// включены и chat_id задан.
func MergeFailedNotifier(settings SettingsReader, tg TelegramSender, logger *slog.Logger) merge.OnFailed {
	return func(ctx context.Context, m sqlc.AppMerge, cause string) {
		if tg == nil {
			return
		}
		st, err := settings.GetSettings(ctx)
		if err != nil {
			logger.ErrorContext(ctx, "merge alert skipped: settings unavailable", slog.Any("error", err))
			return
		}
		if !st.AlertsEnabled || st.TelegramChatID == nil || *st.TelegramChatID == "" {
			return
		}
		text := fmt.Sprintf("⚠️ <b>Слияние приложений не выполнено</b>\n"+
			"Слияние <code>%s</code>: «%s» → приложение <code>%s</code>, шаг %s.\n"+
			"Ошибка: %s\n"+
			"Нажмите «Повторить» в баннере приложения после устранения причины: слияние продолжится с этого шага.",
			m.ID, html.EscapeString(m.SourceName), m.TargetAppID, html.EscapeString(string(m.Step)), html.EscapeString(cause))
		if _, err := tg.Send(ctx, *st.TelegramChatID, text); err != nil {
			logger.ErrorContext(ctx, "merge alert not sent", slog.String("merge_id", m.ID.String()), slog.Any("error", err))
		}
	}
}
