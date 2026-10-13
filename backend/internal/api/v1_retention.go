package api

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// GetRetention — классический retention по дневным или недельным когортам (FR-014).
func (s *Server) GetRetention(ctx context.Context, req gen.GetRetentionRequestObject) (gen.GetRetentionResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetRetention404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	p, q, perr := parseRetentionParams(req.Params, req.AppId, loc)
	if perr != nil {
		return gen.GetRetention400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	r, err := s.reports.Retention(ctx, p, q)
	if err != nil {
		return nil, err
	}
	out := gen.GetRetention200JSONResponse{
		CohortGranularity: gen.CohortGranularity(r.CohortGranularity),
		Periods:           r.Periods,
		Cohorts:           make([]gen.RetentionCohort, 0, len(r.Cohorts)),
		Average:           nullableFloats(r.Average),
	}
	for _, c := range r.Cohorts {
		start, err := parseDay(c.CohortStart)
		if err != nil {
			return nil, err
		}
		out.Cohorts = append(out.Cohorts, gen.RetentionCohort{
			CohortStart: start, Size: toInt64(c.Size), Values: nullableFloats(c.Values),
		})
	}
	return out, nil
}

// parseRetentionParams — общие параметры отчёта, cohort_granularity (day по умолчанию) и periods: day 1..90
// (30 по умолчанию), week 1..52 (12 по умолчанию).
func parseRetentionParams(in gen.GetRetentionParams, appID uuid.UUID, loc *time.Location) (reports.Params, reports.RetentionQuery, *paramError) {
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, appID, loc)
	if perr != nil {
		return reports.Params{}, reports.RetentionQuery{}, perr
	}

	q := reports.RetentionQuery{Granularity: reports.GranularityDay, Periods: reports.DefaultDayPeriods}
	maxPeriods := reports.MaxDayPeriods
	if in.CohortGranularity != nil {
		switch *in.CohortGranularity {
		case gen.CohortGranularityDay:
		case gen.CohortGranularityWeek:
			q = reports.RetentionQuery{Granularity: reports.GranularityWeek, Periods: reports.DefaultWeekPeriods}
			maxPeriods = reports.MaxWeekPeriods
		default:
			return reports.Params{}, reports.RetentionQuery{},
				invalidParam("cohort_granularity", "cohort_granularity: допустимо day или week.")
		}
	}
	if in.Periods != nil {
		if *in.Periods < 1 || *in.Periods > maxPeriods {
			return reports.Params{}, reports.RetentionQuery{}, invalidParam("periods",
				fmt.Sprintf("periods: от 1 до %d для cohort_granularity=%s.", maxPeriods, q.Granularity))
		}
		q.Periods = *in.Periods
	}
	return p.Params, q, nil
}

func nullableFloats(in []*float64) []nullable.Nullable[float32] {
	out := make([]nullable.Nullable[float32], 0, len(in))
	for _, v := range in {
		out = append(out, nullableFloat(v))
	}
	return out
}
