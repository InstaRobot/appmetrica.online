package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/reports"
)

// DigestStore — настройки, активные приложения и журнал digest (pgstore/sqlc.Queries).
type DigestStore interface {
	SettingsReader
	ListApps(ctx context.Context, includeArchived bool) ([]sqlc.App, error)
	GetDigestDayState(ctx context.Context, since time.Time) (sqlc.GetDigestDayStateRow, error)
}

// PortfolioReader — reports.Service.
type PortfolioReader interface {
	Portfolio(ctx context.Context, q reports.PortfolioQuery) (reports.Portfolio, error)
}

// DigestJob — job digest (FR-020). Не реализован.
func DigestJob(_ DigestStore, _ PortfolioReader, _ TelegramSender, _ clock.Clock, _ *slog.Logger, _ string) Job {
	return Job{Name: "digest", Run: func(context.Context, uuid.UUID) (Details, error) { return nil, nil }}
}

// digestRow — строка портфеля и имя приложения.
type digestRow struct {
	Name string
	Row  reports.PortfolioRow
}

func formatDigest(time.Time, []digestRow, string) string { return "" }

func groupThousands(uint64) string { return "" }
