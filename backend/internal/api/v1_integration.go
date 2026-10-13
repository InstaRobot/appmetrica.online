package api

import (
	"context"
	"fmt"
	"time"

	"github.com/oapi-codegen/nullable"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// GetIntegrationHealth — здоровье интеграции приложения (FR-017): часы — с offset TZ приложения, first_event_at
// и last_seen — UTC (06-api-contracts.md §1).
func (s *Server) GetIntegrationHealth(ctx context.Context, req gen.GetIntegrationHealthRequestObject) (gen.GetIntegrationHealthResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetIntegrationHealth404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	hours, perr := parseIntegrationParams(req.Params)
	if perr != nil {
		return gen.GetIntegrationHealth400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	h, err := s.reports.IntegrationHealth(ctx, reports.Params{AppID: req.AppId, Location: loc}, hours)
	if err != nil {
		return nil, err
	}
	out := gen.GetIntegrationHealth200JSONResponse{
		Hours:         h.Hours,
		FirstEventAt:  nullable.NewNullNullable[time.Time](),
		AcceptedTotal: toInt64(h.AcceptedTotal),
		RejectedTotal: toInt64(h.RejectedTotal),
		Hourly:        make([]gen.IntegrationHour, 0, len(h.Hourly)),
		Reasons:       make([]gen.IntegrationReason, 0, len(h.Reasons)),
		Versions:      make([]gen.IntegrationVersion, 0, len(h.Versions)),
	}
	if h.FirstEventAt != nil {
		out.FirstEventAt = nullable.NewNullableWithValue(h.FirstEventAt.UTC())
	}
	for _, p := range h.Hourly {
		out.Hourly = append(out.Hourly, gen.IntegrationHour{
			Hour: p.Hour.In(loc), Accepted: toInt64(p.Accepted), Rejected: toInt64(p.Rejected),
		})
	}
	for _, r := range h.Reasons {
		out.Reasons = append(out.Reasons, gen.IntegrationReason{
			Code: r.Code, Count: toInt64(r.Count), LastSeen: r.LastSeen.UTC(),
			ExamplePayload: r.ExamplePayload, ExampleMessage: r.ExampleMessage,
		})
	}
	for _, v := range h.Versions {
		out.Versions = append(out.Versions, gen.IntegrationVersion{
			AppVersion: v.AppVersion, Accepted: toInt64(v.Accepted), Rejected: toInt64(v.Rejected),
			LastSeen: v.LastSeen.UTC(),
		})
	}
	return out, nil
}

// parseIntegrationParams — hours 1..168, по умолчанию 24 (rejected_events хранятся 7 дней).
func parseIntegrationParams(in gen.GetIntegrationHealthParams) (int, *paramError) {
	if in.Hours == nil {
		return reports.DefaultIntegrationHours, nil
	}
	if *in.Hours < 1 || *in.Hours > reports.MaxIntegrationHours {
		return 0, invalidParam("hours", fmt.Sprintf("hours: от 1 до %d.", reports.MaxIntegrationHours))
	}
	return *in.Hours, nil
}
