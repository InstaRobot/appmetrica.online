//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

func realtimePath(app testutil.ScenarioApp, query string) string {
	return "/v1/apps/" + app.ID.String() + "/realtime?" + query
}

// liveSession — session_start установки install приложения app в at (принят через 300 мс).
func liveSession(app testutil.ScenarioApp, install uuid.UUID, env string, at time.Time) chstore.EventRow {
	return chstore.EventRow{
		AppID: app.ID, EventDate: time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
		EventTime: at, ClientTime: at, SentAt: at, ReceivedAt: at.Add(300 * time.Millisecond),
		InsertID: uuid.New(), Name: "session_start", InstallID: install, SessionID: uuid.New(),
		Environment: env, Platform: "ios", OSName: "iOS", OSVersion: "26.0.1", AppVersion: "3.4.0",
		DeviceModel: "iPhone17,3", Country: "RU", City: "Moscow",
	}
}

// getRealtime отдаёт срез reports.Realtime: минуты — с offset TZ приложения, время событий и generated_at — UTC.
func TestGetRealtime_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	msk, nyc, tyo := testutil.ScenarioApps[0], testutil.ScenarioApps[1], testutil.ScenarioApps[2]
	now := time.Date(2026, 10, 13, 5, 0, 0, 0, time.UTC) // часы newPortfolioEnv
	ctx := t.Context()

	installA, installB := uuid.New(), uuid.New()
	rows := []chstore.EventRow{
		liveSession(msk, installA, reports.EnvProduction, now.Add(-10*time.Minute)),
		liveSession(msk, installB, reports.EnvProduction, now.Add(-3*time.Minute)),
		liveSession(msk, uuid.New(), reports.EnvDevelopment, now.Add(-2*time.Minute)),
		liveSession(nyc, uuid.New(), reports.EnvProduction, now.Add(-time.Minute)),
	}
	require.NoError(t, e.env.CH.InsertEvents(ctx, rows))

	realtime := func(t *testing.T, app testutil.ScenarioApp, query string) (gen.Realtime, string) {
		t.Helper()
		rec := e.get(t, realtimePath(app, query), e.all)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got gen.Realtime
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return got, rec.Body.String()
	}
	requireSame := func(t *testing.T, want reports.Realtime, got gen.Realtime) {
		t.Helper()
		require.Equal(t, int64(want.ActiveNow), got.ActiveNow)
		require.Len(t, got.PerMinute, reports.RealtimeMinutes)
		for i, m := range want.PerMinute {
			require.True(t, m.Minute.Equal(got.PerMinute[i].Minute), "минута %d", i)
			require.Equal(t, int64(m.SessionsStarted), got.PerMinute[i].SessionsStarted, "минута %d", i)
		}
		require.Len(t, got.RecentEvents, len(want.RecentEvents))
		for i, ev := range want.RecentEvents {
			g := got.RecentEvents[i]
			require.Equal(t, ev.InsertID, g.InsertId)
			require.Equal(t, ev.PersonKey, g.PersonKey)
			require.Equal(t, ev.InstallID, g.InstallId)
			require.Equal(t, ev.Environment, string(g.Environment))
			require.True(t, ev.ReceivedAt.Equal(g.ReceivedAt))
		}
		require.True(t, now.Equal(got.GeneratedAt))
	}

	t.Run("production_default", func(t *testing.T) {
		want, err := e.reports.Realtime(ctx, reports.Params{AppID: msk.ID, Location: loadLocation(t, msk), Env: reports.EnvProduction})
		require.NoError(t, err)
		got, raw := realtime(t, msk, "")
		requireSame(t, want, got)
		require.Equal(t, int64(2), got.ActiveNow)
		require.Equal(t, rows[1].InsertID, got.RecentEvents[0].InsertId)
		ev := got.RecentEvents[0]
		require.Equal(t, gen.RecentEventNameSessionStart, ev.Name)
		require.Equal(t, gen.EnvironmentProduction, ev.Environment)
		require.Equal(t, "ios", ev.Platform)
		require.Equal(t, "3.4.0", ev.AppVersion)
		require.Equal(t, "iOS", ev.OsName)
		require.Equal(t, "26.0.1", ev.OsVersion)
		require.Equal(t, "iPhone17,3", ev.DeviceModel)
		require.Equal(t, "RU", ev.Country)
		require.Equal(t, "Moscow", ev.City)
		require.True(t, rows[1].EventTime.Equal(ev.EventTime))
		require.Contains(t, raw, `"minute":"2026-10-13T07:30:00+03:00"`, "первая минута окна в TZ приложения")
		require.Contains(t, raw, `"minute":"2026-10-13T07:59:00+03:00"`)
		require.Contains(t, raw, `"received_at":"2026-10-13T04:57:00.3Z"`)
		require.Contains(t, raw, `"generated_at":"2026-10-13T05:00:00Z"`)
	})

	t.Run("development", func(t *testing.T) {
		want, err := e.reports.Realtime(ctx, reports.Params{AppID: msk.ID, Location: loadLocation(t, msk), Env: reports.EnvDevelopment})
		require.NoError(t, err)
		got, _ := realtime(t, msk, "environment=development")
		requireSame(t, want, got)
		require.Equal(t, int64(1), got.ActiveNow)
		require.Equal(t, rows[2].InsertID, got.RecentEvents[0].InsertId)
	})

	t.Run("archived_and_restricted", func(t *testing.T) {
		require.Equal(t, http.StatusOK, e.get(t, realtimePath(tyo, ""), e.all).Code)
		require.Equal(t, http.StatusOK, e.get(t, realtimePath(nyc, ""), e.restricted).Code)
		body := errorBody(t, e.get(t, realtimePath(msk, ""), e.restricted), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})

	t.Run("app_not_found", func(t *testing.T) {
		unknown := testutil.ScenarioApp{ID: uuid.MustParse("5ce0a000-0000-4000-8000-0000000000ff")}
		body := errorBody(t, e.get(t, realtimePath(unknown, ""), e.all), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})

	t.Run("invalid_environment", func(t *testing.T) {
		rec := e.get(t, realtimePath(msk, "environment=staging"), e.all)
		body := errorBody(t, rec, http.StatusBadRequest)
		require.Equal(t, "invalid_request", body.Code)
		require.Equal(t, "environment", body.Details["field"])
		require.NotContains(t, rec.Body.String(), "staging")
	})
}

func loadLocation(t *testing.T, app testutil.ScenarioApp) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(app.Timezone)
	require.NoError(t, err)
	return loc
}
