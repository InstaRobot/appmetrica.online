package api

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"appmetrica.online/backend/internal/auth"
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

// GetPortfolio — портфель на дату (FR-012): строка на приложение, сортировка по active_date DESC, при равенстве —
// по названию. Дата по умолчанию — вчера в owner_timezone; для каждого приложения она считается в его TZ.
func (s *Server) GetPortfolio(ctx context.Context, req gen.GetPortfolioRequestObject) (gen.GetPortfolioResponseObject, error) {
	unit, perr := parseUnit(req.Params.Unit)
	if perr != nil {
		return gen.GetPortfolio400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}
	env, perr := parseEnvironment(req.Params.Environment)
	if perr != nil {
		return gen.GetPortfolio400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	q := reports.PortfolioQuery{Env: env, Unit: unit}
	if req.Params.Date != nil {
		q.Date = req.Params.Date.Time
	} else {
		settings, err := s.settings.GetSettings(ctx)
		if err != nil {
			return nil, fmt.Errorf("api: portfolio settings: %w", err)
		}
		if q.OwnerLocation, err = time.LoadLocation(settings.OwnerTimezone); err != nil {
			return nil, fmt.Errorf("api: portfolio owner timezone: %w", err)
		}
	}

	list, err := s.apps.List(ctx, req.Params.IncludeArchived != nil && *req.Params.IncludeArchived)
	if err != nil {
		return nil, err
	}
	only, restricted := auth.APIKeyAppFrom(ctx)
	byID := make(map[openapi_types.UUID]sqlc.App, len(list))
	for _, a := range list {
		if restricted && a.ID != only {
			continue
		}
		loc, err := time.LoadLocation(a.Timezone)
		if err != nil {
			return nil, fmt.Errorf("api: portfolio app %s timezone: %w", a.ID, err)
		}
		byID[a.ID] = a
		q.Apps = append(q.Apps, reports.PortfolioApp{ID: a.ID, Location: loc})
	}

	p, err := s.reports.Portfolio(ctx, q)
	if err != nil {
		return nil, err
	}
	date, err := parseDay(p.Date)
	if err != nil {
		return nil, err
	}
	out := gen.GetPortfolio200JSONResponse{
		Date:        date,
		Environment: gen.Environment(p.Environment),
		Unit:        gen.Unit(p.Unit),
		Rows:        make([]gen.PortfolioRow, 0, len(p.Rows)),
	}
	for _, r := range p.Rows {
		row, err := portfolioRowModel(byID[r.AppID], r)
		if err != nil {
			return nil, err
		}
		out.Rows = append(out.Rows, row)
	}
	slices.SortStableFunc(out.Rows, func(a, b gen.PortfolioRow) int {
		return cmp.Or(cmp.Compare(b.ActiveDate, a.ActiveDate), cmp.Compare(a.App.Name, b.App.Name))
	})
	return out, nil
}

func portfolioRowModel(app sqlc.App, r reports.PortfolioRow) (gen.PortfolioRow, error) {
	row := gen.PortfolioRow{
		App: gen.AppSummary{
			Id:        app.ID,
			Name:      app.Name,
			Platform:  gen.Platform(app.Platform),
			Platforms: platformListModel(app.Platforms),
			Status:    gen.AppStatus(app.Status),
		},
		ActiveToday:           toInt64(r.ActiveToday),
		ActiveDate:            toInt64(r.ActiveDate),
		ActiveSameDayPrevWeek: toInt64(r.ActiveSameDayPrevWeek),
		ActiveDeltaPct:        nullableFloat(r.ActiveDeltaPct),
		NewDate:               toInt64(r.NewDate),
		SessionsDate:          toInt64(r.SessionsDate),
		RetentionD1:           nullableFloat(r.RetentionD1),
		RetentionD7:           nullableFloat(r.RetentionD7),
		Sparkline:             make([]gen.SparklinePoint, 0, len(r.Sparkline)),
		Platforms:             make([]gen.PlatformActive, 0, len(r.Platforms)),
	}
	for _, p := range r.Platforms {
		row.Platforms = append(row.Platforms, gen.PlatformActive{Platform: p.Platform, Active: toInt64(p.Active)})
	}
	for _, p := range r.Sparkline {
		d, err := parseDay(p.Day)
		if err != nil {
			return gen.PortfolioRow{}, err
		}
		row.Sparkline = append(row.Sparkline, gen.SparklinePoint{Day: d, Active: toInt64(p.Active)})
	}
	return row, nil
}

// toInt64 — счётчик отчёта в int64 контракта: числа людей и сессий далеки от MaxInt64.
func toInt64(v uint64) int64 {
	return int64(v) //nolint:gosec // G115: см. выше
}

func parseDay(s string) (openapi_types.Date, error) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return openapi_types.Date{}, fmt.Errorf("api: report day: %w", err)
	}
	return openapi_types.Date{Time: d}, nil
}

// nullableFloat — значение или null контракта (number в api.yaml генерируется как float32).
func nullableFloat(v *float64) nullable.Nullable[float32] {
	if v == nil {
		return nullable.NewNullNullable[float32]()
	}
	return nullable.NewNullableWithValue(float32(*v))
}
