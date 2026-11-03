package api

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// Пределы getEventBreakdown.limit (api.yaml) — границы reports.
const (
	minEventBreakdownLimit = 1
	maxEventBreakdownLimit = reports.MaxEventBreakdownLimit
)

// eventKeyPattern — шаблон имени события и ключа свойства (EventName в ingest.yaml, api.yaml).
var eventKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// eventNotFound — 404 event_not_found: системное имя (session_start, session_end) не относится к отчёту «События».
func eventNotFound(ctx context.Context) gen.NotFoundJSONResponse {
	return gen.NotFoundJSONResponse(apiError(ctx, "event_not_found",
		"Событие не входит в отчёт «События»: session_start и session_end показывают отчёты аудитории.", nil))
}

// parseEventName — имя события пути по шаблону EventName.
func parseEventName(name string) *paramError {
	if !eventKeyPattern.MatchString(name) {
		return invalidParam("event_name", "event_name: строчные латинские буквы, цифры и «_», первая — буква, до 64 символов.")
	}
	return nil
}

// ListEvents — свои события периода по именам с событиями и людьми (FR-102).
func (s *Server) ListEvents(ctx context.Context, req gen.ListEventsRequestObject) (gen.ListEventsResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.ListEvents404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
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
		return gen.ListEvents400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	list, err := s.reports.EventsList(ctx, p.Params, compare)
	if err != nil {
		return nil, err
	}
	out := gen.ListEvents200JSONResponse{Events: make([]gen.EventStat, 0, len(list.Events)), Truncated: list.Truncated}
	for _, e := range list.Events {
		st := gen.EventStat{Name: e.Name, Events: toInt64(e.Events), People: toInt64(e.People)}
		if e.PrevEvents != nil {
			v := toInt64(*e.PrevEvents)
			st.PrevEvents = &v
		}
		if e.PrevPeople != nil {
			v := toInt64(*e.PrevPeople)
			st.PrevPeople = &v
		}
		out.Events = append(out.Events, st)
	}
	return out, nil
}

// GetEventReport — страница события: сводка, ряд по корзинам и ключи свойств (FR-102).
func (s *Server) GetEventReport(ctx context.Context, req gen.GetEventReportRequestObject) (gen.GetEventReportResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetEventReport404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	in := req.Params
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Granularity: in.Granularity, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, req.AppId, loc)
	q := reports.EventReportQuery{Name: req.EventName}
	if perr == nil {
		perr = parseEventName(req.EventName)
	}
	if perr == nil {
		q.Compare, perr = parseCompare((*string)(in.Compare))
	}
	if perr != nil {
		return gen.GetEventReport400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}
	q.Granularity = reports.Granularity(p.Granularity)

	r, err := s.reports.EventReport(ctx, p.Params, q)
	if errors.Is(err, reports.ErrSystemEvent) {
		return gen.GetEventReport404JSONResponse{NotFoundJSONResponse: eventNotFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := gen.GetEventReport200JSONResponse{
		Name: r.Name,
		Summary: gen.EventSummary{
			Events:              toInt64(r.Summary.Events),
			People:              toInt64(r.Summary.People),
			EventsPerPerson:     float32(r.Summary.EventsPerPerson),
			PrevEventsPerPerson: float32Ptr(r.Summary.PrevEventsPerPerson),
		},
		Timeseries:   make([]gen.EventPoint, 0, len(r.Timeseries)),
		PropertyKeys: make([]gen.EventPropertyKey, 0, len(r.PropertyKeys)),
	}
	if r.Summary.PrevEvents != nil {
		v := toInt64(*r.Summary.PrevEvents)
		out.Summary.PrevEvents = &v
	}
	if r.Summary.PrevPeople != nil {
		v := toInt64(*r.Summary.PrevPeople)
		out.Summary.PrevPeople = &v
	}
	for _, pt := range r.Timeseries {
		out.Timeseries = append(out.Timeseries, gen.EventPoint{Bucket: pt.Bucket, Events: toInt64(pt.Events), People: toInt64(pt.People)})
	}
	for _, k := range r.PropertyKeys {
		out.PropertyKeys = append(out.PropertyKeys, gen.EventPropertyKey{Key: k.Key, Events: toInt64(k.Events)})
	}
	return out, nil
}

func float32Ptr(v *float64) *float32 {
	if v == nil {
		return nil
	}
	f := float32(*v)
	return &f
}

// GetEventBreakdown — события одного имени по значениям свойства: топ limit, остальные и без ключа (FR-102).
func (s *Server) GetEventBreakdown(ctx context.Context, req gen.GetEventBreakdownRequestObject) (gen.GetEventBreakdownResponseObject, error) {
	loc, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetEventBreakdown404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	in := req.Params
	p, perr := parseCommonParams(commonParams{
		From: in.From, To: in.To, Environment: in.Environment, Unit: in.Unit,
		Platform: in.Platform, AppVersion: in.AppVersion, OsName: in.OsName, Country: in.Country,
	}, req.AppId, loc)
	q := reports.EventBreakdownQuery{Name: req.EventName, Property: in.Property, Limit: reports.DefaultEventBreakdownLimit}
	if perr == nil {
		perr = parseEventName(req.EventName)
	}
	if perr == nil && !eventKeyPattern.MatchString(in.Property) {
		perr = invalidParam("property", "property: строчные латинские буквы, цифры и «_», первая — буква, до 64 символов.")
	}
	if perr == nil && in.Limit != nil {
		if *in.Limit < minEventBreakdownLimit || *in.Limit > maxEventBreakdownLimit {
			perr = invalidParam("limit", fmt.Sprintf("limit: от %d до %d.", minEventBreakdownLimit, maxEventBreakdownLimit))
		} else {
			q.Limit = *in.Limit
		}
	}
	if perr != nil {
		return gen.GetEventBreakdown400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	b, err := s.reports.EventBreakdown(ctx, p.Params, q)
	if errors.Is(err, reports.ErrSystemEvent) {
		return gen.GetEventBreakdown404JSONResponse{NotFoundJSONResponse: eventNotFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := gen.GetEventBreakdown200JSONResponse{
		Property: b.Property,
		Values:   make([]gen.EventValueCount, 0, len(b.Values)),
		Missing:  gen.EventCount{Events: toInt64(b.Missing.Events), People: toInt64(b.Missing.People)},
	}
	for _, v := range b.Values {
		out.Values = append(out.Values, gen.EventValueCount{Value: v.Value, Events: toInt64(v.Events), People: toInt64(v.People)})
	}
	if b.Other != nil {
		out.Other = &gen.EventCount{Events: toInt64(b.Other.Events), People: toInt64(b.Other.People)}
	}
	return out, nil
}
