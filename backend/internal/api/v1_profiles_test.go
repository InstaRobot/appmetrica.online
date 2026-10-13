//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

func profilesPath(app testutil.ScenarioApp, query string) string {
	return "/v1/apps/" + app.ID.String() + "/profiles?" + query
}

// profilePath — путь карточки; escapedKey уже URL-encoded.
func profilePath(app testutil.ScenarioApp, escapedKey, query string) string {
	return "/v1/apps/" + app.ID.String() + "/profiles/" + escapedKey + "?" + query
}

// userSession — session_start пользователя user (user "" — анонимно) установки install в at и, если dur > 0,
// session_end через dur.
func userSession(app testutil.ScenarioApp, install uuid.UUID, user, env string, at time.Time, dur time.Duration) []chstore.EventRow {
	start := liveSession(app, install, env, at)
	start.UserID = user
	if dur == 0 {
		return []chstore.EventRow{start}
	}
	end := start
	end.InsertID, end.Name, end.DurationMs = uuid.New(), "session_end", uint32(dur/time.Millisecond)
	end.EventTime = at.Add(dur)
	end.ClientTime, end.SentAt, end.ReceivedAt = end.EventTime, end.EventTime, end.EventTime
	return []chstore.EventRow{start, end}
}

type profilesFixture struct {
	user                    string
	installA, installB      uuid.UUID // установки user
	anonymous               uuid.UUID // анонимная установка с открытой сессией
	sessionsA, sessionsB    []chstore.EventRow
	anonymousSession        chstore.EventRow
	slashUser, slashInstall string
}

func seedProfiles(t *testing.T, e *portfolioEnv) profilesFixture {
	t.Helper()
	msk := testutil.ScenarioApps[0]
	f := profilesFixture{
		user:     "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
		installA: uuid.New(), installB: uuid.New(), anonymous: uuid.New(),
		slashUser: "org/42", slashInstall: uuid.NewString(),
	}
	f.sessionsA = userSession(msk, f.installA, f.user, reports.EnvProduction,
		time.Date(2026, 9, 1, 7, 12, 0, 0, time.UTC), 2*time.Minute)
	f.sessionsB = userSession(msk, f.installB, f.user, reports.EnvDevelopment,
		time.Date(2026, 10, 13, 4, 11, 58, 4_000_000, time.UTC), 3*time.Minute+3*time.Second+986*time.Millisecond)
	anon := userSession(msk, f.anonymous, "", reports.EnvProduction, time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC), 0)
	f.anonymousSession = anon[0]
	slash := userSession(msk, uuid.MustParse(f.slashInstall), f.slashUser, reports.EnvProduction,
		time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC), time.Minute)
	var rows []chstore.EventRow
	for _, part := range [][]chstore.EventRow{f.sessionsA, f.sessionsB, anon, slash} {
		rows = append(rows, part...)
	}
	require.NoError(t, e.env.CH.InsertEvents(t.Context(), rows))
	return f
}

func searchHTTP(t *testing.T, e *portfolioEnv, app testutil.ScenarioApp, q string) (gen.ProfileSearchResult, string) {
	t.Helper()
	rec := e.get(t, profilesPath(app, "q="+url.QueryEscape(q)), e.all)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got gen.ProfileSearchResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got, rec.Body.String()
}

func profileHTTP(t *testing.T, e *portfolioEnv, app testutil.ScenarioApp, escapedKey, query string) (gen.Profile, string) {
	t.Helper()
	rec := e.get(t, profilePath(app, escapedKey, query), e.all)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got gen.Profile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got, rec.Body.String()
}

// searchProfiles: установка находится как профиль её человека, время — UTC, user_id анонимного профиля — null.
func TestSearchProfiles_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	f := seedProfiles(t, e)
	msk, nyc := testutil.ScenarioApps[0], testutil.ScenarioApps[1]

	t.Run("install_resolves_to_user", func(t *testing.T) {
		got, raw := searchHTTP(t, e, msk, f.installB.String())
		require.Len(t, got.Items, 1)
		item := got.Items[0]
		require.Equal(t, "u:"+f.user, item.PersonKey)
		require.Equal(t, f.user, item.UserId.MustGet())
		require.Equal(t, gen.ProfileSummaryMatchedByInstallId, item.MatchedBy)
		require.Equal(t, 2, item.InstallsCount)
		require.Contains(t, raw, `"first_seen":"2026-09-01T07:12:00Z"`)
		require.Contains(t, raw, `"last_seen":"2026-10-13T04:15:01.99Z"`)
	})

	t.Run("by_user_id", func(t *testing.T) {
		got, _ := searchHTTP(t, e, msk, f.user)
		require.Len(t, got.Items, 1)
		require.Equal(t, gen.ProfileSummaryMatchedByUserId, got.Items[0].MatchedBy)
		require.Equal(t, "u:"+f.user, got.Items[0].PersonKey)
	})

	t.Run("anonymous", func(t *testing.T) {
		got, raw := searchHTTP(t, e, msk, f.anonymous.String())
		require.Len(t, got.Items, 1)
		require.Equal(t, "i:"+f.anonymous.String(), got.Items[0].PersonKey)
		require.Equal(t, 1, got.Items[0].InstallsCount)
		require.Contains(t, raw, `"user_id":null`)
	})

	t.Run("no_match", func(t *testing.T) {
		_, raw := searchHTTP(t, e, msk, "unknown")
		require.JSONEq(t, `{"items":[]}`, raw)
		got, _ := searchHTTP(t, e, msk, strings.Repeat("я", 128))
		require.Empty(t, got.Items, "128 символов — допустимый q")
	})

	t.Run("invalid_q", func(t *testing.T) {
		for _, query := range []string{"", "q=", "q=" + url.QueryEscape(strings.Repeat("я", 129))} {
			rec := e.get(t, profilesPath(msk, query), e.all)
			body := errorBody(t, rec, http.StatusBadRequest)
			require.Equal(t, "invalid_request", body.Code, "query=%q", query)
			require.Equal(t, "q", body.Details["field"], "query=%q", query)
			require.NotContains(t, rec.Body.String(), "яяя")
		}
	})

	t.Run("app_not_found", func(t *testing.T) {
		body := errorBody(t, e.get(t, profilesPath(msk, "q=x"), e.restricted), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
		require.Equal(t, http.StatusOK, e.get(t, profilesPath(nyc, "q=x"), e.restricted).Code)
		unknown := testutil.ScenarioApp{ID: uuid.MustParse("5ce0a000-0000-4000-8000-0000000000ff")}
		body = errorBody(t, e.get(t, profilesPath(unknown, "q=x"), e.all), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})
}

// getProfile: карточка с установками и сессиями всех environment, время — UTC, пагинация по курсору before.
func TestGetProfile_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	f := seedProfiles(t, e)
	msk := testutil.ScenarioApps[0]
	key := url.PathEscape("u:" + f.user)

	got, raw := profileHTTP(t, e, msk, key, "")
	want, err := e.reports.Profile(t.Context(), msk.ID, reports.ProfileQuery{PersonKey: "u:" + f.user})
	require.NoError(t, err)
	require.Equal(t, want.PersonKey.String(), got.PersonKey)
	require.Equal(t, f.user, got.UserId.MustGet())
	require.Len(t, got.Installs, len(want.Installs))
	require.Equal(t, []uuid.UUID{f.installB, f.installA}, []uuid.UUID{got.Installs[0].InstallId, got.Installs[1].InstallId})
	inst := got.Installs[0]
	require.Equal(t, "ios", inst.Platform)
	require.Equal(t, "iOS", inst.OsName)
	require.Equal(t, "26.0.1", inst.OsVersion)
	require.Equal(t, "3.4.0", inst.AppVersion)
	require.Equal(t, "iPhone17,3", inst.DeviceModel)
	require.Equal(t, "RU", inst.Country)
	require.Equal(t, "Moscow", inst.City)
	require.Len(t, got.Sessions, 2)
	s := got.Sessions[0]
	require.Equal(t, f.sessionsB[0].SessionID, s.SessionId)
	require.Equal(t, f.installB, s.InstallId)
	require.Equal(t, gen.EnvironmentDevelopment, s.Environment)
	require.Equal(t, int64(183_986), s.DurationMs.MustGet())
	require.Equal(t, gen.EnvironmentProduction, got.Sessions[1].Environment)
	require.Contains(t, raw, `"started_at":"2026-10-13T04:11:58.004Z"`)
	require.Contains(t, raw, `"ended_at":"2026-10-13T04:15:01.99Z"`)
	require.Contains(t, raw, `"first_seen":"2026-09-01T07:12:00Z"`)
	require.Contains(t, raw, `"next_before":null`)

	t.Run("pagination", func(t *testing.T) {
		page, raw := profileHTTP(t, e, msk, key, "sessions_limit=1")
		require.Len(t, page.Sessions, 1)
		require.Contains(t, raw, `"next_before":"2026-10-13T04:11:58.004Z"`)
		before := page.NextBefore.MustGet()
		page, _ = profileHTTP(t, e, msk, key, "sessions_limit=1&before="+url.QueryEscape(before.Format(time.RFC3339Nano)))
		require.Len(t, page.Sessions, 1)
		require.Equal(t, f.sessionsA[0].SessionID, page.Sessions[0].SessionId)
		require.True(t, page.NextBefore.IsNull(), "вторая страница последняя")
	})

	t.Run("open_session", func(t *testing.T) {
		page, raw := profileHTTP(t, e, msk, url.PathEscape("i:"+f.anonymous.String()), "")
		require.Equal(t, "i:"+f.anonymous.String(), page.PersonKey)
		require.Len(t, page.Sessions, 1)
		require.Equal(t, f.anonymousSession.SessionID, page.Sessions[0].SessionId)
		require.True(t, page.Sessions[0].EndedAt.IsNull())
		require.True(t, page.Sessions[0].DurationMs.IsNull())
		require.Contains(t, raw, `"ended_at":null`)
		require.Contains(t, raw, `"duration_ms":null`)
		require.Contains(t, raw, `"user_id":null`)
	})

	t.Run("unescaped_colon", func(t *testing.T) {
		page, _ := profileHTTP(t, e, msk, "u:"+f.user, "")
		require.Equal(t, "u:"+f.user, page.PersonKey)
	})
}

// Тест-вектор not_found: у person_key нет событий — 404 profile_not_found.
func TestProfile_NotFound(t *testing.T) {
	e := newPortfolioEnv(t)
	f := seedProfiles(t, e)
	msk, nyc := testutil.ScenarioApps[0], testutil.ScenarioApps[1]

	for _, key := range []string{"i:" + uuid.NewString(), "u:nobody"} {
		body := errorBody(t, e.get(t, profilePath(msk, url.PathEscape(key), ""), e.all), http.StatusNotFound)
		require.Equal(t, "profile_not_found", body.Code, "person_key=%s", key)
		require.Equal(t, "Профиль не найден", body.Message)
		require.NotEmpty(t, body.RequestID)
	}

	body := errorBody(t, e.get(t, profilePath(nyc, url.PathEscape("u:"+f.user), ""), e.all), http.StatusNotFound)
	require.Equal(t, "profile_not_found", body.Code, "профиль другого приложения")
	body = errorBody(t, e.get(t, profilePath(msk, url.PathEscape("u:"+f.user), ""), e.restricted), http.StatusNotFound)
	require.Equal(t, "app_not_found", body.Code)
}

// Тест-вектор bad_person_key: неверный person_key и параметры — 400 invalid_request с details.field, присланное
// значение в ответ не попадает.
func TestProfile_BadPersonKey(t *testing.T) {
	e := newPortfolioEnv(t)
	f := seedProfiles(t, e)
	msk := testutil.ScenarioApps[0]
	key := url.PathEscape("u:" + f.user)

	for _, tc := range []struct {
		key, query, field string
	}{
		{"x:123", "", "person_key"},
		{"u%3A", "", "person_key"},
		{"i%3Anot-a-uuid", "", "person_key"},
		{url.PathEscape("u:" + strings.Repeat("я", 129)), "", "person_key"},
		{key, "sessions_limit=0", "sessions_limit"},
		{key, "sessions_limit=501", "sessions_limit"},
		{key, "sessions_limit=-1", "sessions_limit"},
		{key, "sessions_limit=abc", "sessions_limit"},
		{key, "before=yesterday", "before"},
	} {
		rec := e.get(t, profilePath(msk, tc.key, tc.query), e.all)
		body := errorBody(t, rec, http.StatusBadRequest)
		require.Equal(t, "invalid_request", body.Code, "%s?%s", tc.key, tc.query)
		require.Equal(t, tc.field, body.Details["field"], "%s?%s", tc.key, tc.query)
		require.NotContains(t, rec.Body.String(), "not-a-uuid")
		require.NotContains(t, rec.Body.String(), "yesterday")
	}

	page, _ := profileHTTP(t, e, msk, key, "sessions_limit=500")
	require.Len(t, page.Sessions, 2)
}

// Тест-вектор slash_in_user_id: user_id со слешем — путь u%3Aorg%2F42.
func TestProfile_EncodedSlash(t *testing.T) {
	e := newPortfolioEnv(t)
	f := seedProfiles(t, e)
	msk := testutil.ScenarioApps[0]

	got, _ := profileHTTP(t, e, msk, "u%3Aorg%2F42", "")
	require.Equal(t, "u:"+f.slashUser, got.PersonKey)
	require.Equal(t, f.slashUser, got.UserId.MustGet())
	require.Len(t, got.Installs, 1)
	require.Equal(t, f.slashInstall, got.Installs[0].InstallId.String())
	require.Len(t, got.Sessions, 1)
}
