package api

import (
	"context"
	"errors"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/reports"
)

// SettingsReader — настройки владельца: owner_timezone задаёт дату портфеля по умолчанию.
type SettingsReader interface {
	GetSettings(ctx context.Context) (sqlc.GetSettingsRow, error)
}

// WithReports подключает отчёты Read API по ClickHouse.
func (s *Server) WithReports(r *reports.Service, settings SettingsReader) *Server {
	s.reports = r
	s.settings = settings
	return s
}

func (s *Server) GetPortfolio(_ context.Context, _ gen.GetPortfolioRequestObject) (gen.GetPortfolioResponseObject, error) {
	return nil, errors.New("api: getPortfolio not implemented")
}
