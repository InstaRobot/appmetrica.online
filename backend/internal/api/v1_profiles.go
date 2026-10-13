package api

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/oapi-codegen/nullable"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
)

const msgProfileNotFound = "Профиль не найден"

// maxProfileQuery — предел q в символах (api.yaml: maxLength 128).
const maxProfileQuery = 128

// SearchProfiles — поиск профилей по точному install_id или user_id (FR-016); время — UTC.
func (s *Server) SearchProfiles(ctx context.Context, req gen.SearchProfilesRequestObject) (gen.SearchProfilesResponseObject, error) {
	_, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.SearchProfiles404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	if q := req.Params.Q; q == "" || utf8.RuneCountInString(q) > maxProfileQuery {
		perr := invalidParam("q", "q — точный install_id или user_id длиной от 1 до 128 символов.")
		return gen.SearchProfiles400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	}

	found, err := s.reports.SearchProfiles(ctx, req.AppId, req.Params.Q)
	if err != nil {
		return nil, err
	}
	out := gen.SearchProfiles200JSONResponse{Items: make([]gen.ProfileSummary, 0, len(found))}
	for _, p := range found {
		out.Items = append(out.Items, gen.ProfileSummary{
			PersonKey:     p.PersonKey.String(),
			UserId:        profileUserID(p.PersonKey),
			MatchedBy:     gen.ProfileSummaryMatchedBy(p.MatchedBy),
			InstallsCount: p.InstallsCount,
			FirstSeen:     p.FirstSeen.UTC(),
			LastSeen:      p.LastSeen.UTC(),
		})
	}
	return out, nil
}

// GetProfile — карточка профиля (FR-016): установки, сессии по started_at DESC с курсором before; время — UTC.
func (s *Server) GetProfile(ctx context.Context, req gen.GetProfileRequestObject) (gen.GetProfileResponseObject, error) {
	_, ok, err := s.appLocation(ctx, req.AppId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return gen.GetProfile404JSONResponse{NotFoundJSONResponse: appNotFound(ctx)}, nil
	}
	q := reports.ProfileQuery{PersonKey: req.PersonKey}
	if l := req.Params.SessionsLimit; l != nil {
		if *l < 1 || *l > reports.MaxSessionsLimit {
			perr := invalidParam("sessions_limit", "sessions_limit — целое от 1 до 500.")
			return gen.GetProfile400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
		}
		q.SessionsLimit = *l
	}
	if req.Params.Before != nil {
		q.Before = *req.Params.Before
	}

	p, err := s.reports.Profile(ctx, req.AppId, q)
	switch {
	case errors.Is(err, identity.ErrInvalidPersonKey):
		perr := invalidParam("person_key", "person_key — u:<user_id> или i:<install_id>.")
		return gen.GetProfile400JSONResponse{BadRequestJSONResponse: perr.response(ctx)}, nil
	case errors.Is(err, reports.ErrProfileNotFound):
		return gen.GetProfile404JSONResponse{
			NotFoundJSONResponse: gen.NotFoundJSONResponse(apiError(ctx, "profile_not_found", msgProfileNotFound, nil)),
		}, nil
	case err != nil:
		return nil, err
	}

	out := gen.GetProfile200JSONResponse{
		PersonKey:  p.PersonKey.String(),
		UserId:     profileUserID(p.PersonKey),
		FirstSeen:  p.FirstSeen.UTC(),
		LastSeen:   p.LastSeen.UTC(),
		Installs:   make([]gen.ProfileInstall, 0, len(p.Installs)),
		Sessions:   make([]gen.ProfileSession, 0, len(p.Sessions)),
		NextBefore: nullable.NewNullNullable[time.Time](),
	}
	if p.NextBefore != nil {
		out.NextBefore = nullable.NewNullableWithValue(p.NextBefore.UTC())
	}
	for _, i := range p.Installs {
		out.Installs = append(out.Installs, gen.ProfileInstall{
			InstallId:   i.InstallID,
			Platform:    i.Platform,
			OsName:      i.OSName,
			OsVersion:   i.OSVersion,
			AppVersion:  i.AppVersion,
			DeviceModel: i.DeviceModel,
			DeviceType:  i.DeviceType,
			Locale:      i.Locale,
			Country:     i.Country,
			City:        i.City,
			FirstSeen:   i.FirstSeen.UTC(),
			LastSeen:    i.LastSeen.UTC(),
		})
	}
	for _, ss := range p.Sessions {
		session := gen.ProfileSession{
			SessionId:   ss.SessionID,
			InstallId:   ss.InstallID,
			StartedAt:   ss.StartedAt.UTC(),
			EndedAt:     nullable.NewNullNullable[time.Time](),
			DurationMs:  nullable.NewNullNullable[int64](),
			AppVersion:  ss.AppVersion,
			Platform:    ss.Platform,
			Environment: gen.Environment(ss.Environment),
			Country:     ss.Country,
			City:        ss.City,
		}
		if ss.EndedAt != nil {
			session.EndedAt = nullable.NewNullableWithValue(ss.EndedAt.UTC())
		}
		if ss.DurationMs != nil {
			session.DurationMs = nullable.NewNullableWithValue(int64(*ss.DurationMs))
		}
		out.Sessions = append(out.Sessions, session)
	}
	return out, nil
}

// profileUserID — user_id профиля u:<user_id>; у анонимной установки — null.
func profileUserID(key identity.PersonKey) nullable.Nullable[string] {
	if !key.IsUser() {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(key.UserID)
}
