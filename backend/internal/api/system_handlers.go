package api

import (
	"context"

	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/pgstore/sqlc"
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

func (s *Server) GetSystemStatus(_ context.Context, _ gen.GetSystemStatusRequestObject) (gen.GetSystemStatusResponseObject, error) {
	return nil, errNotImplemented
}
