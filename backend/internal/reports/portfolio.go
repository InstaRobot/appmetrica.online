package reports

import (
	"context"
	"errors"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
)

// Service — отчёты по ClickHouse.
type Service struct {
	conn  driver.Conn
	clock clock.Clock
}

func NewService(conn driver.Conn, c clock.Clock) *Service {
	return &Service{conn: conn, clock: c}
}

// PortfolioApp — приложение портфеля и его TZ (apps.timezone; nil — UTC).
type PortfolioApp struct {
	ID       uuid.UUID
	Location *time.Location
}

// PortfolioQuery — параметры GET /v1/portfolio.
type PortfolioQuery struct {
	Date          time.Time      // календарная дата; нулевая — вчера в OwnerLocation
	OwnerLocation *time.Location // settings.owner_timezone; nil — UTC
	Env           string
	Unit          identity.Unit
	Apps          []PortfolioApp
}

// Portfolio — портфель на дату; Rows — в порядке PortfolioQuery.Apps.
type Portfolio struct {
	Date        string         `json:"date"`
	Environment string         `json:"environment"`
	Unit        identity.Unit  `json:"unit"`
	Rows        []PortfolioRow `json:"rows"`
}

// PortfolioRow — строка портфеля (FR-012); дни — в TZ приложения.
type PortfolioRow struct {
	AppID                 uuid.UUID        `json:"app_id"`
	ActiveToday           uint64           `json:"active_today"`
	ActiveDate            uint64           `json:"active_date"`
	ActiveSameDayPrevWeek uint64           `json:"active_same_day_prev_week"`
	ActiveDeltaPct        *float64         `json:"active_delta_pct"`
	NewDate               uint64           `json:"new_date"`
	SessionsDate          uint64           `json:"sessions_date"`
	RetentionD1           *float64         `json:"retention_d1"`
	RetentionD7           *float64         `json:"retention_d7"`
	Sparkline             []SparklinePoint `json:"sparkline"`
}

// SparklinePoint — активные за день Day (YYYY-MM-DD в TZ приложения).
type SparklinePoint struct {
	Day    string `json:"day"`
	Active uint64 `json:"active"`
}

var errNotImplemented = errors.New("reports: portfolio not implemented")

// Portfolio считает строки портфеля.
func (s *Service) Portfolio(_ context.Context, _ PortfolioQuery) (Portfolio, error) {
	return Portfolio{}, errNotImplemented
}

func deltaPct(_, _ uint64) *float64 { return nil }

func ratio(_, _ uint64) *float64 { return nil }

// sparkline — точки from … from+29: закрытые дни из closed, остальные из live.
func sparkline(_ time.Time, _, _ map[string]uint64) []SparklinePoint { return nil }
