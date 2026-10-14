package api

import (
	"context"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// GetRealtime — срез «прямо сейчас» приложения (FR-015), при platform — только этих платформ: минуты — с offset
// TZ приложения, время событий — UTC (06-api-contracts.md §1).
func (s *Server) GetRealtime(ctx context.Context, req gen.GetRealtimeRequestObject) (gen.GetRealtimeResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetRealtime404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	env, perr := parseEnvironment(req.Params.Environment)
	if perr != nil {
		return gen.GetRealtime400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}
	// Из фильтров отчётов realtime принимает только platform (переключатель платформ дашборда).
	filters, perr := parseFilters(commonParams{Platform: req.Params.Platform})
	if perr != nil {
		return gen.GetRealtime400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	r, err := s.reports.Realtime(ctx, reports.Params{AppID: req.AppId, Location: loc, Env: env, Filters: filters})
	if err != nil {
		return nil, err
	}
	out := gen.GetRealtime200JSONResponse{
		ActiveNow:    toInt64(r.ActiveNow),
		PerMinute:    make([]gen.RealtimeMinute, 0, len(r.PerMinute)),
		RecentEvents: make([]gen.RecentEvent, 0, len(r.RecentEvents)),
		GeneratedAt:  r.GeneratedAt.UTC(),
	}
	for _, m := range r.PerMinute {
		out.PerMinute = append(out.PerMinute, gen.RealtimeMinute{Minute: m.Minute.In(loc), SessionsStarted: toInt64(m.SessionsStarted)})
	}
	for _, e := range r.RecentEvents {
		out.RecentEvents = append(out.RecentEvents, gen.RecentEvent{
			InsertId:    e.InsertID,
			EventTime:   e.EventTime.UTC(),
			ReceivedAt:  e.ReceivedAt.UTC(),
			Name:        gen.RecentEventName(e.Name),
			Environment: gen.Environment(e.Environment),
			Platform:    e.Platform,
			AppVersion:  e.AppVersion,
			OsName:      e.OSName,
			OsVersion:   e.OSVersion,
			DeviceModel: e.DeviceModel,
			Country:     e.Country,
			City:        e.City,
			InstallId:   e.InstallID,
			PersonKey:   e.PersonKey,
		})
	}
	return out, nil
}
