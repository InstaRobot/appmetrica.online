package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/apps"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// Пределы getAudienceBreakdown.limit (api.yaml).
const (
	minBreakdownLimit = 1
	maxBreakdownLimit = 200
)

// appLocation — TZ приложения отчёта; false — приложения нет (404 app_not_found). Отчёты архивного приложения
// доступны. Ключ, ограниченный другим приложением, получает 404 раньше — в auth.RequireAPIAccess.
func (s *Server) appLocation(ctx context.Context, id uuid.UUID) (*time.Location, bool, error) {
	app, err := s.apps.Get(ctx, id)
	switch {
	case errors.Is(err, apps.ErrNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	loc, err := time.LoadLocation(app.Timezone)
	if err != nil {
		return nil, false, fmt.Errorf("api: app %s timezone: %w", id, err)
	}
	return loc, true, nil
}

// parseCompare — compare=previous_period (по умолчанию) | none.
func parseCompare(in *string) (bool, *paramError) {
	if in == nil {
		return true, nil
	}
	switch gen.GetAudienceSummaryParamsCompare(*in) {
	case gen.GetAudienceSummaryParamsComparePreviousPeriod:
		return true, nil
	case gen.GetAudienceSummaryParamsCompareNone:
		return false, nil
	default:
		return false, invalidParam("compare", "compare: допустимо previous_period или none.")
	}
}

// GetAudienceSummary — KPI аудитории за период и сравнение с предыдущим (FR-013).
func (s *Server) GetAudienceSummary(ctx context.Context, req gen.GetAudienceSummaryRequestObject) (gen.GetAudienceSummaryResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetAudienceSummary404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	in := req.Params
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, req.AppId, loc)
	compare := false
	if perr == nil {
		compare, perr = parseCompare((*string)(in.Compare))
	}
	if perr != nil {
		return gen.GetAudienceSummary400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	sum, err := s.reports.AudienceSummary(ctx, p.Params, compare)
	if err != nil {
		return nil, err
	}
	out := gen.GetAudienceSummary200JSONResponse{
		Active:           toInt64(sum.Active),
		New:              toInt64(sum.New),
		Sessions:         toInt64(sum.Sessions),
		AvgDurationMs:    nullableFloat(sum.AvgDurationMs),
		MedianDurationMs: nullableFloat(sum.MedianDurationMs),
	}
	if out.From, err = parseDay(sum.From); err != nil {
		return nil, err
	}
	if out.To, err = parseDay(sum.To); err != nil {
		return nil, err
	}
	if c := sum.Compare; c != nil {
		cmp := gen.AudienceCompare{
			Active:           toInt64(c.Active),
			New:              toInt64(c.New),
			Sessions:         toInt64(c.Sessions),
			AvgDurationMs:    nullableFloat(c.AvgDurationMs),
			MedianDurationMs: nullableFloat(c.MedianDurationMs),
			DeltaPct: gen.AudienceDelta{
				Active:           nullableFloat(c.DeltaPct.Active),
				New:              nullableFloat(c.DeltaPct.New),
				Sessions:         nullableFloat(c.DeltaPct.Sessions),
				AvgDurationMs:    nullableFloat(c.DeltaPct.AvgDurationMs),
				MedianDurationMs: nullableFloat(c.DeltaPct.MedianDurationMs),
			},
		}
		if cmp.From, err = parseDay(c.From); err != nil {
			return nil, err
		}
		if cmp.To, err = parseDay(c.To); err != nil {
			return nil, err
		}
		out.Compare = &cmp
	}
	return out, nil
}

// GetAudienceTimeseries — метрика по корзинам hour/day/week/month в TZ приложения (FR-013).
func (s *Server) GetAudienceTimeseries(ctx context.Context, req gen.GetAudienceTimeseriesRequestObject) (gen.GetAudienceTimeseriesResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetAudienceTimeseries404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	in := req.Params
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Granularity: in.Granularity, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, req.AppId, loc)
	q := reports.TimeseriesQuery{Granularity: reports.Granularity(p.Granularity), Metric: reports.MetricActive}
	if perr == nil && in.Metric != nil {
		if !in.Metric.Valid() {
			perr = invalidParam("metric", "metric: допустимо active, new, sessions или avg_duration_ms.")
		}
		q.Metric = reports.Metric(*in.Metric)
	}
	if perr == nil {
		q.Compare, perr = parseCompare((*string)(in.Compare))
	}
	if perr != nil {
		return gen.GetAudienceTimeseries400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	ts, err := s.reports.AudienceTimeseries(ctx, p.Params, q)
	if err != nil {
		return nil, err
	}
	out := gen.GetAudienceTimeseries200JSONResponse{
		Metric:      gen.TimeSeriesMetric(ts.Metric),
		Granularity: gen.Granularity(ts.Granularity),
		Points:      timeseriesPoints(ts.Points),
	}
	if ts.ComparePoints != nil {
		points := timeseriesPoints(ts.ComparePoints)
		out.ComparePoints = &points
	}
	return out, nil
}

func timeseriesPoints(in []reports.TimeseriesPoint) []gen.TimeSeriesPoint {
	out := make([]gen.TimeSeriesPoint, 0, len(in))
	for _, p := range in {
		out = append(out, gen.TimeSeriesPoint{Ts: p.TS, Value: nullableFloat(p.Value)})
	}
	return out
}

// GetRollingActive — DAU/WAU/MAU для каждого дня периода (FR-013).
func (s *Server) GetRollingActive(ctx context.Context, req gen.GetRollingActiveRequestObject) (gen.GetRollingActiveResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetRollingActive404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	in := req.Params
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, req.AppId, loc)
	if perr != nil {
		return gen.GetRollingActive400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	r, err := s.reports.RollingActive(ctx, p.Params)
	if err != nil {
		return nil, err
	}
	out := gen.GetRollingActive200JSONResponse{Points: make([]gen.RollingActivePoint, 0, len(r.Points))}
	for _, pt := range r.Points {
		d, err := parseDay(pt.Day)
		if err != nil {
			return nil, err
		}
		out.Points = append(out.Points, gen.RollingActivePoint{
			Day: d, Dau: toInt64(pt.DAU), Wau: toInt64(pt.WAU), Mau: toInt64(pt.MAU),
		})
	}
	return out, nil
}

// GetAudienceBreakdown — метрика по значениям измерения, share — доля от метрики без разбивки (FR-013).
func (s *Server) GetAudienceBreakdown(ctx context.Context, req gen.GetAudienceBreakdownRequestObject) (gen.GetAudienceBreakdownResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetAudienceBreakdown404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	in := req.Params
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, req.AppId, loc)
	q, qerr := parseBreakdownQuery(in)
	if perr == nil {
		perr = qerr
	}
	if perr != nil {
		return gen.GetAudienceBreakdown400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	b, err := s.reports.AudienceBreakdown(ctx, p.Params, q)
	if err != nil {
		return nil, err
	}
	out := gen.GetAudienceBreakdown200JSONResponse{
		Dimension: gen.BreakdownDimension(b.Dimension),
		Metric:    gen.BreakdownMetric(b.Metric),
		Total:     toInt64(b.Total),
		Rows:      make([]gen.BreakdownRow, 0, len(b.Rows)),
	}
	for _, r := range b.Rows {
		out.Rows = append(out.Rows, gen.BreakdownRow{Value: r.Value, Count: toInt64(r.Count), Share: float32(r.Share)})
	}
	return out, nil
}

// parseBreakdownQuery — dimension (enum), metric (active по умолчанию), limit 1..200 (50 по умолчанию).
func parseBreakdownQuery(in gen.GetAudienceBreakdownParams) (reports.BreakdownQuery, *paramError) {
	q := reports.BreakdownQuery{Metric: reports.MetricActive, Limit: reports.DefaultBreakdownLimit}
	if !in.Dimension.Valid() {
		return q, invalidParam("dimension",
			"dimension: допустимо platform, os_name, os_version, app_version, device_model, device_type, country, city или locale.")
	}
	q.Dimension = reports.Dimension(in.Dimension)
	if in.Metric != nil {
		if !in.Metric.Valid() {
			return q, invalidParam("metric", "metric: допустимо active, new или sessions.")
		}
		q.Metric = reports.Metric(*in.Metric)
	}
	if in.Limit != nil {
		if *in.Limit < minBreakdownLimit || *in.Limit > maxBreakdownLimit {
			return q, invalidParam("limit", fmt.Sprintf("limit: от %d до %d.", minBreakdownLimit, maxBreakdownLimit))
		}
		q.Limit = *in.Limit
	}
	return q, nil
}
