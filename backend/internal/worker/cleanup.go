package worker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
)

// CleanupStore — удаление отработавших строк (pgstore/sqlc.Queries).
type CleanupStore interface {
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
	DeleteExpiredCeremonies(ctx context.Context, now time.Time) (int64, error)
	DeleteSpentSetupTokens(ctx context.Context, now time.Time) (int64, error)
	DeleteOldJobRuns(ctx context.Context, before time.Time) (int64, error)
}

// SessionCleanupJob — job session_cleanup раз в час.
func SessionCleanupJob(_ CleanupStore, _ clock.Clock) Job {
	return Job{Name: "session_cleanup", Every: time.Hour, Run: func(context.Context, uuid.UUID) (Details, error) {
		return nil, errors.New("session_cleanup: not implemented")
	}}
}
