package worker

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// SystemCheckStore — настройки и alert_states (pgstore/sqlc.Queries).
type SystemCheckStore interface {
	SettingsReader
	ListAlertStates(ctx context.Context) ([]sqlc.AlertState, error)
	UpsertAlertState(ctx context.Context, arg sqlc.UpsertAlertStateParams) (sqlc.AlertState, error)
}

// ClickHouseProbe — chstore.Store.
type ClickHouseProbe interface {
	Ping(ctx context.Context) error
	DiskFreePct(ctx context.Context) (float64, error)
}

// BackupLister — chstore.BackupClient.
type BackupLister interface {
	Latest(ctx context.Context) (chstore.Backup, bool, error)
}

// SystemCheckConfig — зависимости и пороги system_check.
type SystemCheckConfig struct {
	Store      SystemCheckStore
	ClickHouse ClickHouseProbe
	// Backups — nil: BACKUP_API_URL не задан, свежесть бэкапа не проверяется.
	Backups BackupLister
	// Telegram — nil: TELEGRAM_BOT_TOKEN не задан, алерты пишутся в лог.
	Telegram TelegramSender
	Lock     Locker
	Clock    clock.Clock
	Logger   *slog.Logger
	// HTTP — клиент для /health, /metrics и healthchecks.io; nil — http.DefaultClient.
	HTTP *http.Client

	HealthchecksPingURL string
	DiskFreeAlertPct    int
	IngestHealthURL     string
	APIHealthURL        string
	IngestMetricsURL    string
}

// SystemCheck — job system_check (FR-021).
type SystemCheck struct {
	cfg SystemCheckConfig
}

func NewSystemCheck(cfg SystemCheckConfig) *SystemCheck {
	return &SystemCheck{cfg: cfg}
}

// Job — system_check раз в 5 минут.
func (s *SystemCheck) Job() Job {
	return Job{Name: "system_check", Every: 5 * time.Minute, Run: s.run}
}

// RegisterMetrics регистрирует worker_alerts_firing.
func (s *SystemCheck) RegisterMetrics(_ prometheus.Registerer) error {
	return nil
}

func (s *SystemCheck) run(_ context.Context, _ uuid.UUID) (Details, error) {
	return nil, errors.New("system_check: not implemented")
}
