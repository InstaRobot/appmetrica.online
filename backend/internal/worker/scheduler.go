// Package worker — фоновые задачи `appmetrica worker`: планировщик с журналом job_runs и сами задачи
// (deletion, close_day, digest; system_check и session_cleanup — по мере реализации).
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// ErrJobDisabled — задача не включена в WORKER_JOBS_ENABLED или не реализована.
var ErrJobDisabled = errors.New("worker: job is not enabled")

// Details — job_runs.details цикла.
type Details map[string]any

// Job — периодическая задача. Run получает id своей строки job_runs.
type Job struct {
	Name  string // из config.Jobs
	Every time.Duration
	// Timeout — предел одного запуска; 0 — Every.
	Timeout time.Duration
	Run     func(ctx context.Context, runID uuid.UUID) (Details, error)
}

// Journal — запись job_runs (pgstore/sqlc.Queries).
type Journal interface {
	StartJobRun(ctx context.Context, arg sqlc.StartJobRunParams) (uuid.UUID, error)
	FinishJobRun(ctx context.Context, arg sqlc.FinishJobRunParams) error
}

// Scheduler запускает включённые задачи: первый раз сразу при старте, затем раз в Every. Запуски одной
// задачи не перекрываются: пропущенные за время долгого запуска тики отбрасываются.
type Scheduler struct {
	journal  Journal
	clock    clock.Clock
	logger   *slog.Logger
	jobs     []Job
	duration *prometheus.HistogramVec
	failures *prometheus.CounterVec
}

// NewScheduler оставляет из jobs только перечисленные в enabled (WORKER_JOBS_ENABLED).
func NewScheduler(journal Journal, c clock.Clock, logger *slog.Logger, enabled []string, jobs ...Job) *Scheduler {
	s := &Scheduler{
		journal: journal, clock: c, logger: logger,
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "worker_job_duration_seconds",
			Help:    "Длительность запуска задачи worker.",
			Buckets: []float64{0.1, 0.5, 1, 5, 15, 60, 300, 900},
		}, []string{"job"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "worker_job_failures_total",
			Help: "Запуски задачи worker, завершившиеся ошибкой.",
		}, []string{"job"}),
	}
	for _, j := range jobs {
		if slices.Contains(enabled, j.Name) {
			s.jobs = append(s.jobs, j)
		}
	}
	return s
}

// Jobs — имена задач, которые запускает планировщик.
func (s *Scheduler) Jobs() []string {
	out := make([]string, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j.Name)
	}
	return out
}

// RegisterMetrics регистрирует worker_job_duration_seconds и worker_job_failures_total.
func (s *Scheduler) RegisterMetrics(reg prometheus.Registerer) error {
	for _, c := range []prometheus.Collector{s.duration, s.failures} {
		if err := reg.Register(c); err != nil {
			return fmt.Errorf("worker: register metrics: %w", err)
		}
	}
	return nil
}

// Run запускает задачи до отмены ctx и ждёт завершения текущих запусков.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, j := range s.jobs {
		wg.Go(func() { s.loop(ctx, j) })
	}
	wg.Wait()
}

func (s *Scheduler) loop(ctx context.Context, j Job) {
	ticker := time.NewTicker(j.Every)
	defer ticker.Stop()
	for {
		// Ошибка уже записана в job_runs, лог и метрики.
		_ = s.run(ctx, j)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce — один запуск задачи name (тесты и ручной запуск).
func (s *Scheduler) RunOnce(ctx context.Context, name string) error {
	i := slices.IndexFunc(s.jobs, func(j Job) bool { return j.Name == name })
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrJobDisabled, name)
	}
	return s.run(ctx, s.jobs[i])
}

// run — запуск с журналом: строка job_runs running → ok / failed с details. Без строки job_runs
// (Postgres недоступен) задача не запускается.
func (s *Scheduler) run(ctx context.Context, j Job) error {
	logger := s.logger.With(slog.String("job", j.Name))
	started := s.clock.Now()
	runID, err := s.journal.StartJobRun(ctx, sqlc.StartJobRunParams{Job: sqlc.JobName(j.Name), Now: started})
	if err != nil {
		s.failures.WithLabelValues(j.Name).Inc()
		logger.ErrorContext(ctx, "job skipped: job_runs unavailable", slog.Any("error", err))
		return fmt.Errorf("worker: start %s: %w", j.Name, err)
	}

	timeout := j.Timeout
	if timeout <= 0 {
		timeout = j.Every
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	details, jobErr := s.call(runCtx, j, runID)
	cancel()

	finished := s.clock.Now()
	s.duration.WithLabelValues(j.Name).Observe(finished.Sub(started).Seconds())
	status := sqlc.JobStatusOk
	if details == nil {
		details = Details{}
	}
	if jobErr != nil {
		status = sqlc.JobStatusFailed
		details["error"] = jobErr.Error()
		s.failures.WithLabelValues(j.Name).Inc()
	}
	raw, err := json.Marshal(details)
	if err != nil {
		raw = []byte(`{}`)
		logger.ErrorContext(ctx, "job details not encodable", slog.Any("error", err))
	}
	// Запуск уже закончился: запись итога не прерывается отменой ctx при остановке worker-а.
	finishErr := s.journal.FinishJobRun(context.WithoutCancel(ctx), sqlc.FinishJobRunParams{
		Now: &finished, Status: status, Details: raw, ID: runID,
	})
	if finishErr != nil {
		logger.ErrorContext(ctx, "job_runs not updated", slog.String("job_run_id", runID.String()), slog.Any("error", finishErr))
	}
	attrs := []any{slog.String("job_run_id", runID.String()), slog.Int64("duration_ms", finished.Sub(started).Milliseconds())}
	if jobErr != nil {
		logger.WarnContext(ctx, "job failed", append(attrs, slog.Any("error", jobErr))...)
		return fmt.Errorf("worker: %s: %w", j.Name, jobErr)
	}
	logger.InfoContext(ctx, "job done", attrs...)
	if finishErr != nil {
		return fmt.Errorf("worker: finish %s: %w", j.Name, finishErr)
	}
	return nil
}

// call — Run задачи; паника задачи становится ошибкой запуска и не останавливает worker.
func (s *Scheduler) call(ctx context.Context, j Job, runID uuid.UUID) (details Details, err error) {
	defer func() {
		if r := recover(); r != nil {
			details, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return j.Run(ctx, runID)
}
