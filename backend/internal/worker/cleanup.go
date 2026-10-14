package worker

import (
	"context"
	"errors"
	"fmt"
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

// jobRunsRetention — сколько хранится журнал job_runs (05-data-model.md §2.4.6).
const jobRunsRetention = 90 * 24 * time.Hour

// SessionCleanupJob — job session_cleanup раз в час: удаляет истёкшие, отозванные и простаивающие сессии,
// истёкшие WebAuthn-церемонии, использованные и истёкшие setup-токены (их церемонии — каскадом) и job_runs
// старше 90 дней. Ошибка одного удаления не мешает остальным.
func SessionCleanupJob(store CleanupStore, c clock.Clock) Job {
	return Job{Name: "session_cleanup", Every: time.Hour, Run: func(ctx context.Context, _ uuid.UUID) (Details, error) {
		now := c.Now()
		steps := []struct {
			name string
			run  func() (int64, error)
		}{
			{"sessions", func() (int64, error) { return store.DeleteExpiredSessions(ctx, now) }},
			{"ceremonies", func() (int64, error) { return store.DeleteExpiredCeremonies(ctx, now) }},
			{"setup_tokens", func() (int64, error) { return store.DeleteSpentSetupTokens(ctx, now) }},
			{"job_runs", func() (int64, error) { return store.DeleteOldJobRuns(ctx, now.Add(-jobRunsRetention)) }},
		}
		details := Details{}
		var errs []error
		for _, s := range steps {
			n, err := s.run()
			if err != nil {
				errs = append(errs, fmt.Errorf("session_cleanup: %s: %w", s.name, err))
				continue
			}
			details[s.name] = n
		}
		return details, errors.Join(errs...)
	}}
}
