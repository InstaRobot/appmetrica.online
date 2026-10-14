package api

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/oapi-codegen/nullable"

	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	// systemCheckTimeout — на все проверки статуса: ответ укладывается в 2 с, даже если что-то зависло.
	systemCheckTimeout = 1500 * time.Millisecond
	maxStatusJobs      = 20
)

// ClickHouseStatus — проверки ClickHouse для статуса системы.
type ClickHouseStatus interface {
	Ping(ctx context.Context) error
	DiskFreePct(ctx context.Context) (float64, error)
	EventsRows(ctx context.Context) (int64, error)
}

// OpsReader — служебные таблицы Postgres: состояния алертов и запуски jobs.
type OpsReader interface {
	ListAlertStates(ctx context.Context) ([]sqlc.AlertState, error)
	ListLatestJobRuns(ctx context.Context) ([]sqlc.JobRun, error)
}

// BackupLister — последний удалённый бэкап clickhouse-backup.
type BackupLister interface {
	Latest(ctx context.Context) (chstore.Backup, bool, error)
}

// SystemSources — откуда getSystemStatus собирает статус.
type SystemSources struct {
	ClickHouse ClickHouseStatus
	Postgres   func(ctx context.Context) error
	Ops        OpsReader
	// Backup nil — BACKUP_API_URL не задан: backup в ответе пустой.
	Backup BackupLister
}

// WithSystemStatus подключает getSystemStatus.
func (s *Server) WithSystemStatus(src SystemSources) *Server {
	s.system = &src
	return s
}

// GetSystemStatus проверяет ClickHouse, Postgres и clickhouse-backup параллельно. Недоступный компонент —
// ok=false или null в его блоке, а не ошибка ответа: статус нужен как раз когда что-то сломалось.
func (s *Server) GetSystemStatus(ctx context.Context, _ gen.GetSystemStatusRequestObject) (gen.GetSystemStatusResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, systemCheckTimeout)
	defer cancel()

	var st gen.SystemStatus
	var wg sync.WaitGroup
	wg.Go(func() { st.Clickhouse = s.clickhouseStatus(ctx) })
	wg.Go(func() { st.Postgres.Ok, st.Alerts, st.Jobs = s.postgresStatus(ctx) })
	wg.Go(func() { st.Backup = s.backupStatus(ctx) })
	wg.Wait()
	return gen.GetSystemStatus200JSONResponse(st), nil
}

type clickhouseStatus = struct {
	DiskFreePct nullable.Nullable[float32] `json:"disk_free_pct"`
	EventsRows  nullable.Nullable[int64]   `json:"events_rows"`
	Ok          bool                       `json:"ok"`
}

func (s *Server) clickhouseStatus(ctx context.Context) clickhouseStatus {
	out := clickhouseStatus{
		DiskFreePct: nullable.NewNullNullable[float32](),
		EventsRows:  nullable.NewNullNullable[int64](),
	}
	ch := s.system.ClickHouse
	if err := ch.Ping(ctx); err != nil {
		s.statusWarn(ctx, "clickhouse", err)
		return out
	}
	out.Ok = true
	if pct, err := ch.DiskFreePct(ctx); err != nil {
		s.statusWarn(ctx, "clickhouse disk", err)
	} else {
		out.DiskFreePct.Set(float32(pct))
	}
	if rows, err := ch.EventsRows(ctx); err != nil {
		s.statusWarn(ctx, "clickhouse events rows", err)
	} else {
		out.EventsRows.Set(rows)
	}
	return out
}

// postgresStatus — ok и служебные таблицы; jobs — последний запуск каждого job, свежие сверху.
func (s *Server) postgresStatus(ctx context.Context) (ok bool, alerts []gen.AlertState, jobs []gen.JobRun) {
	alerts, jobs = []gen.AlertState{}, []gen.JobRun{}
	if err := s.system.Postgres(ctx); err != nil {
		s.statusWarn(ctx, "postgres", err)
		return false, alerts, jobs
	}
	if rows, err := s.system.Ops.ListAlertStates(ctx); err != nil {
		s.statusWarn(ctx, "alert states", err)
	} else {
		for _, a := range rows {
			alerts = append(alerts, gen.AlertState{
				Key:            gen.AlertKey(a.Key),
				Firing:         a.Firing,
				FirstFiredAt:   nullableValue(a.FirstFiredAt),
				LastNotifiedAt: nullableValue(a.LastNotifiedAt),
				LastValue:      a.LastValue,
			})
		}
	}
	runs, err := s.system.Ops.ListLatestJobRuns(ctx)
	if err != nil {
		s.statusWarn(ctx, "job runs", err)
		return true, alerts, jobs
	}
	slices.SortFunc(runs, func(a, b sqlc.JobRun) int { return cmp.Compare(b.StartedAt.UnixNano(), a.StartedAt.UnixNano()) })
	for _, r := range runs[:min(len(runs), maxStatusJobs)] {
		details := map[string]any{}
		if err := json.Unmarshal(r.Details, &details); err != nil {
			details = map[string]any{}
		}
		jobs = append(jobs, gen.JobRun{
			Id:         r.ID,
			Job:        gen.JobName(r.Job),
			StartedAt:  r.StartedAt.UTC(),
			FinishedAt: nullableValue(r.FinishedAt),
			Status:     gen.JobStatus(r.Status),
			Details:    details,
		})
	}
	return true, alerts, jobs
}

type backupStatus = struct {
	LastBackupAt   nullable.Nullable[time.Time] `json:"last_backup_at"`
	LastBackupName nullable.Nullable[string]    `json:"last_backup_name"`
}

func (s *Server) backupStatus(ctx context.Context) backupStatus {
	out := backupStatus{
		LastBackupAt:   nullable.NewNullNullable[time.Time](),
		LastBackupName: nullable.NewNullNullable[string](),
	}
	if s.system.Backup == nil {
		return out
	}
	b, ok, err := s.system.Backup.Latest(ctx)
	if err != nil {
		s.statusWarn(ctx, "clickhouse-backup", err)
		return out
	}
	if ok {
		out.LastBackupAt.Set(b.CreatedAt.UTC())
		out.LastBackupName.Set(b.Name)
	}
	return out
}

func (s *Server) statusWarn(ctx context.Context, component string, err error) {
	s.logger.WarnContext(ctx, "system status check failed", slog.String("component", component),
		slog.String("request_id", httpx.RequestIDFrom(ctx)), slog.Any("error", err))
}
