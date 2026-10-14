package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

var testNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type journalRun struct {
	id       uuid.UUID
	job      sqlc.JobName
	started  time.Time
	finished *time.Time
	status   sqlc.JobStatus
	details  map[string]any
}

// fakeJournal — job_runs в памяти.
type fakeJournal struct {
	mu       sync.Mutex
	runs     []*journalRun
	startErr error
	// finishErr — FinishJobRun падает, строка остаётся running.
	finishErr error
}

func (j *fakeJournal) StartJobRun(_ context.Context, arg sqlc.StartJobRunParams) (uuid.UUID, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.startErr != nil {
		return uuid.Nil, j.startErr
	}
	r := &journalRun{id: uuid.New(), job: arg.Job, started: arg.Now, status: sqlc.JobStatusRunning}
	j.runs = append(j.runs, r)
	return r.id, nil
}

func (j *fakeJournal) FinishJobRun(_ context.Context, arg sqlc.FinishJobRunParams) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.finishErr != nil {
		return j.finishErr
	}
	for _, r := range j.runs {
		if r.id == arg.ID {
			r.finished = arg.Now
			r.status = arg.Status
			if err := json.Unmarshal(arg.Details, &r.details); err != nil {
				return err
			}
		}
	}
	return nil
}

func (j *fakeJournal) byJob(name sqlc.JobName) []journalRun {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []journalRun
	for _, r := range j.runs {
		if r.job == name {
			out = append(out, *r)
		}
	}
	return out
}

// countingJob — задача, считающая свои запуски.
type countingJob struct {
	mu   sync.Mutex
	runs []uuid.UUID
}

func (c *countingJob) job(name string, every time.Duration) Job {
	return Job{Name: name, Every: every, Run: func(_ context.Context, runID uuid.UUID) (Details, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.runs = append(c.runs, runID)
		return Details{"n": len(c.runs)}, nil
	}}
}

func (c *countingJob) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.runs)
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// AC-046.4: WORKER_JOBS_ENABLED без deletion — job deletion не запускается, остальные работают.
func TestScheduler_RespectsEnabledJobs(t *testing.T) {
	journal := &fakeJournal{}
	var deletionJob, cleanupJob countingJob
	s := NewScheduler(journal, clock.NewFixed(testNow), discardLogger(), []string{"session_cleanup", "digest"},
		deletionJob.job("deletion", time.Hour),
		cleanupJob.job("session_cleanup", time.Hour),
	)
	require.Equal(t, []string{"session_cleanup"}, s.Jobs())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	require.Eventually(t, func() bool {
		return len(journal.byJob(sqlc.JobNameSessionCleanup)) == 1 &&
			journal.byJob(sqlc.JobNameSessionCleanup)[0].status == sqlc.JobStatusOk
	}, 5*time.Second, 5*time.Millisecond)
	cancel()
	<-done

	require.Equal(t, 1, cleanupJob.count(), "первый запуск — сразу при старте")
	require.Zero(t, deletionJob.count())
	require.Empty(t, journal.byJob(sqlc.JobNameDeletion))

	err := s.RunOnce(t.Context(), "deletion")
	require.ErrorIs(t, err, ErrJobDisabled)
	require.Zero(t, deletionJob.count())
}

// Задача повторяется с периодом Every; Run возвращается после отмены контекста.
func TestScheduler_RunRepeatsEvery(t *testing.T) {
	journal := &fakeJournal{}
	var job countingJob
	s := NewScheduler(journal, clock.NewFixed(testNow), discardLogger(), []string{"deletion"},
		job.job("deletion", 10*time.Millisecond))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	require.Eventually(t, func() bool { return job.count() >= 3 }, 5*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
	}
}

func TestScheduler_RunOnceRecordsJobRun(t *testing.T) {
	c := clock.NewFixed(testNow)
	cases := []struct {
		name        string
		run         func(context.Context, uuid.UUID) (Details, error)
		wantStatus  sqlc.JobStatus
		wantDetails map[string]any
		wantErr     string
	}{
		{
			name: "ok",
			run: func(context.Context, uuid.UUID) (Details, error) {
				c.Advance(3 * time.Second)
				return Details{"done": 2}, nil
			},
			wantStatus: sqlc.JobStatusOk, wantDetails: map[string]any{"done": float64(2)},
		},
		{
			name:       "ok_nil_details",
			run:        func(context.Context, uuid.UUID) (Details, error) { return nil, nil },
			wantStatus: sqlc.JobStatusOk, wantDetails: map[string]any{},
		},
		{
			name: "failed_keeps_details",
			run: func(context.Context, uuid.UUID) (Details, error) {
				return Details{"retried": 1}, errors.New("clickhouse: timeout")
			},
			wantStatus: sqlc.JobStatusFailed, wantDetails: map[string]any{"retried": float64(1), "error": "clickhouse: timeout"},
			wantErr: "clickhouse: timeout",
		},
		{
			name:       "panic",
			run:        func(context.Context, uuid.UUID) (Details, error) { panic("nil map") },
			wantStatus: sqlc.JobStatusFailed, wantDetails: map[string]any{"error": "panic: nil map"},
			wantErr: "panic: nil map",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.Set(testNow)
			journal := &fakeJournal{}
			var gotRunID uuid.UUID
			s := NewScheduler(journal, c, discardLogger(), []string{"deletion"}, Job{
				Name: "deletion", Every: time.Minute,
				Run: func(ctx context.Context, runID uuid.UUID) (Details, error) {
					gotRunID = runID
					return tc.run(ctx, runID)
				},
			})

			err := s.RunOnce(t.Context(), "deletion")
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			runs := journal.byJob(sqlc.JobNameDeletion)
			require.Len(t, runs, 1)
			require.Equal(t, gotRunID, runs[0].id, "задача получает id своей строки job_runs")
			require.Equal(t, testNow, runs[0].started)
			require.NotNil(t, runs[0].finished)
			require.Equal(t, c.Now(), *runs[0].finished)
			require.Equal(t, tc.wantStatus, runs[0].status)
			require.Equal(t, tc.wantDetails, runs[0].details)
		})
	}
}

// Postgres недоступен: строку job_runs не завести — задача пропускается (03-architecture.md, сбои).
func TestScheduler_SkipsRunWhenJournalUnavailable(t *testing.T) {
	journal := &fakeJournal{startErr: errors.New("pg down")}
	var job countingJob
	s := NewScheduler(journal, clock.NewFixed(testNow), discardLogger(), []string{"deletion"}, job.job("deletion", time.Minute))

	err := s.RunOnce(t.Context(), "deletion")
	require.ErrorContains(t, err, "pg down")
	require.Zero(t, job.count())
}

func TestScheduler_UnknownJob(t *testing.T) {
	s := NewScheduler(&fakeJournal{}, clock.NewFixed(testNow), discardLogger(), []string{"deletion"})
	require.Empty(t, s.Jobs(), "включённая, но не реализованная задача не запускается")
	require.ErrorIs(t, s.RunOnce(t.Context(), "deletion"), ErrJobDisabled)
	require.ErrorIs(t, s.RunOnce(t.Context(), "nope"), ErrJobDisabled)
}

// Задача, не уложившаяся в Timeout, получает отменённый контекст.
func TestScheduler_RunOnceTimeout(t *testing.T) {
	journal := &fakeJournal{}
	s := NewScheduler(journal, clock.NewFixed(testNow), discardLogger(), []string{"deletion"}, Job{
		Name: "deletion", Every: time.Minute, Timeout: 20 * time.Millisecond,
		Run: func(ctx context.Context, _ uuid.UUID) (Details, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	err := s.RunOnce(t.Context(), "deletion")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	runs := journal.byJob(sqlc.JobNameDeletion)
	require.Len(t, runs, 1)
	require.Equal(t, sqlc.JobStatusFailed, runs[0].status)
}

func TestScheduler_Metrics(t *testing.T) {
	c := clock.NewFixed(testNow)
	fail := true
	s := NewScheduler(&fakeJournal{}, c, discardLogger(), []string{"deletion"}, Job{
		Name: "deletion", Every: time.Minute,
		Run: func(context.Context, uuid.UUID) (Details, error) {
			c.Advance(2 * time.Second)
			if fail {
				return nil, errors.New("boom")
			}
			return nil, nil
		},
	})
	reg := prometheus.NewRegistry()
	require.NoError(t, s.RegisterMetrics(reg))

	require.Error(t, s.RunOnce(t.Context(), "deletion"))
	fail = false
	require.NoError(t, s.RunOnce(t.Context(), "deletion"))

	families, err := reg.Gather()
	require.NoError(t, err)
	got := map[string]bool{}
	for _, f := range families {
		got[f.GetName()] = true
		m := f.GetMetric()
		require.Len(t, m, 1, f.GetName())
		require.Equal(t, "job", m[0].GetLabel()[0].GetName())
		require.Equal(t, "deletion", m[0].GetLabel()[0].GetValue())
		switch f.GetName() {
		case "worker_job_failures_total":
			require.InDelta(t, 1, m[0].GetCounter().GetValue(), 0)
		case "worker_job_duration_seconds":
			require.Equal(t, uint64(2), m[0].GetHistogram().GetSampleCount())
			require.InDelta(t, 4, m[0].GetHistogram().GetSampleSum(), 1e-9)
		}
	}
	require.Equal(t, map[string]bool{"worker_job_failures_total": true, "worker_job_duration_seconds": true}, got)
}

// job_runs не обновился: при успешной задаче ошибка записи возвращается наружу (строка остаётся running
// и видна на S-12), при упавшей задаче наружу уходит ошибка самой задачи.
func TestScheduler_FinishJobRunFails(t *testing.T) {
	cases := []struct {
		name    string
		jobErr  error
		wantErr string
	}{
		{"job ok", nil, "finish deletion: pg down"},
		{"job failed", errors.New("boom"), "deletion: boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			journal := &fakeJournal{finishErr: errors.New("pg down")}
			s := NewScheduler(journal, clock.NewFixed(testNow), discardLogger(), []string{"deletion"}, Job{
				Name: "deletion", Every: time.Minute,
				Run: func(context.Context, uuid.UUID) (Details, error) { return Details{"n": 1}, tc.jobErr },
			})

			err := s.RunOnce(t.Context(), "deletion")
			require.ErrorContains(t, err, tc.wantErr)
			runs := journal.byJob(sqlc.JobNameDeletion)
			require.Len(t, runs, 1)
			require.Equal(t, sqlc.JobStatusRunning, runs[0].status)
			require.Nil(t, runs[0].finished)
		})
	}
}

// Детали, которые нельзя закодировать в JSON, не роняют запись итога: в job_runs уходит {}.
func TestScheduler_UnencodableDetails(t *testing.T) {
	journal := &fakeJournal{}
	s := NewScheduler(journal, clock.NewFixed(testNow), discardLogger(), []string{"deletion"}, Job{
		Name: "deletion", Every: time.Minute,
		Run: func(context.Context, uuid.UUID) (Details, error) { return Details{"ch": make(chan int)}, nil },
	})

	require.NoError(t, s.RunOnce(t.Context(), "deletion"))
	runs := journal.byJob(sqlc.JobNameDeletion)
	require.Len(t, runs, 1)
	require.Equal(t, sqlc.JobStatusOk, runs[0].status)
	require.Equal(t, map[string]any{}, runs[0].details)
}
