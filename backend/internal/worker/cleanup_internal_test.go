package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
)

// fakeCleanupStore — CleanupStore: запоминает аргументы, sessions падает при failSessions.
type fakeCleanupStore struct {
	failSessions  bool
	calls         []string
	jobRunsBefore time.Time
}

func (f *fakeCleanupStore) DeleteExpiredSessions(context.Context, time.Time) (int64, error) {
	f.calls = append(f.calls, "sessions")
	if f.failSessions {
		return 0, errors.New("pg down")
	}
	return 1, nil
}

func (f *fakeCleanupStore) DeleteExpiredCeremonies(context.Context, time.Time) (int64, error) {
	f.calls = append(f.calls, "ceremonies")
	return 2, nil
}

func (f *fakeCleanupStore) DeleteSpentSetupTokens(context.Context, time.Time) (int64, error) {
	f.calls = append(f.calls, "setup_tokens")
	return 3, nil
}

func (f *fakeCleanupStore) DeleteOldJobRuns(_ context.Context, before time.Time) (int64, error) {
	f.calls = append(f.calls, "job_runs")
	f.jobRunsBefore = before
	return 4, nil
}

// Ошибка одного удаления не мешает остальным; запуск — failed с причиной.
func TestSessionCleanup_ContinuesAfterError(t *testing.T) {
	store := &fakeCleanupStore{failSessions: true}
	job := SessionCleanupJob(store, clock.NewFixed(testNow))
	require.Equal(t, time.Hour, job.Every)

	details, err := job.Run(t.Context(), uuid.Nil)
	require.ErrorContains(t, err, "session_cleanup: sessions: pg down")
	require.Equal(t, []string{"sessions", "ceremonies", "setup_tokens", "job_runs"}, store.calls)
	require.Equal(t, Details{"ceremonies": int64(2), "setup_tokens": int64(3), "job_runs": int64(4)}, details)
	require.Equal(t, testNow.Add(-90*24*time.Hour), store.jobRunsBefore)
}
