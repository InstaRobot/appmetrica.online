//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

func integrationPath(app uuid.UUID, query string) string {
	return "/v1/apps/" + app.String() + "/integration/health?" + query
}

// getIntegrationHealth отдаёт срез reports.IntegrationHealth: часы — с offset TZ приложения, остальное время — UTC.
func TestGetIntegrationHealth_HTTP(t *testing.T) {
	e := newPortfolioEnv(t)
	msk, nyc, tyo := testutil.ScenarioApps[0], testutil.ScenarioApps[1], testutil.ScenarioApps[2]
	now := time.Date(2026, 10, 13, 5, 0, 0, 0, time.UTC) // часы newPortfolioEnv
	ctx := t.Context()
	// Отказы теста старше 7 дней от настоящих часов: TTL-слияния не должны их удалить.
	require.NoError(t, e.env.CH.Conn().Exec(ctx, "SYSTEM STOP TTL MERGES rejected_events"))

	require.NoError(t, e.env.CH.InsertEvents(ctx, []chstore.EventRow{
		liveSession(msk, uuid.New(), reports.EnvProduction, now.Add(-10*time.Minute)),
		liveSession(msk, uuid.New(), reports.EnvDevelopment, now.Add(-2*time.Hour)),
	}))
	rejected := []chstore.RejectedRow{
		{
			ReceivedAt: now.Add(-30 * time.Minute), AppID: msk.ID, KeyPrefix: "pk_live_5ce0", Code: "duration_required",
			Field: "events[0].duration_ms", Message: "Для session_end обязательно поле duration_ms (0..86400000).",
			EventName: "session_end", AppVersion: "3.3.2", Platform: "ios",
			PayloadSample: `{"insert_id":"0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b","name":"session_end"}`,
		},
		{ReceivedAt: now.Add(-3 * time.Hour), AppID: msk.ID, Code: "invalid_json", Message: "Тело запроса не является JSON."},
		{ReceivedAt: now.Add(-time.Hour), AppID: uuid.Nil, Code: "invalid_app_key", Message: "Неизвестный ключ."},
	}
	require.NoError(t, e.env.CH.InsertRejected(ctx, rejected))

	health := func(t *testing.T, app uuid.UUID, query string) (gen.IntegrationHealth, string) {
		t.Helper()
		rec := e.get(t, integrationPath(app, query), e.all)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got gen.IntegrationHealth
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return got, rec.Body.String()
	}
	requireSame := func(t *testing.T, want reports.IntegrationHealth, got gen.IntegrationHealth) {
		t.Helper()
		require.Equal(t, want.Hours, got.Hours)
		require.Equal(t, int64(want.AcceptedTotal), got.AcceptedTotal)
		require.Equal(t, int64(want.RejectedTotal), got.RejectedTotal)
		require.Len(t, got.Hourly, len(want.Hourly))
		for i, h := range want.Hourly {
			require.True(t, h.Hour.Equal(got.Hourly[i].Hour), "час %d", i)
			require.Equal(t, int64(h.Accepted), got.Hourly[i].Accepted, "час %d", i)
			require.Equal(t, int64(h.Rejected), got.Hourly[i].Rejected, "час %d", i)
		}
		require.Len(t, got.Reasons, len(want.Reasons))
		for i, r := range want.Reasons {
			g := got.Reasons[i]
			require.Equal(t, r.Code, g.Code)
			require.Equal(t, int64(r.Count), g.Count)
			require.True(t, r.LastSeen.Equal(g.LastSeen))
			require.Equal(t, r.ExamplePayload, g.ExamplePayload)
			require.Equal(t, r.ExampleMessage, g.ExampleMessage)
		}
		require.Len(t, got.Versions, len(want.Versions))
		for i, v := range want.Versions {
			g := got.Versions[i]
			require.Equal(t, v.AppVersion, g.AppVersion)
			require.Equal(t, int64(v.Accepted), g.Accepted)
			require.Equal(t, int64(v.Rejected), g.Rejected)
			require.True(t, v.LastSeen.Equal(g.LastSeen))
		}
		require.NotNil(t, want.FirstEventAt)
		first, err := got.FirstEventAt.Get()
		require.NoError(t, err)
		require.True(t, want.FirstEventAt.Equal(first))
	}

	t.Run("default_24_hours", func(t *testing.T) {
		want, err := e.reports.IntegrationHealth(ctx, reports.Params{AppID: msk.ID, Location: loadLocation(t, msk)}, 24)
		require.NoError(t, err)
		got, raw := health(t, msk.ID, "")
		requireSame(t, want, got)
		require.Equal(t, 24, got.Hours)
		require.GreaterOrEqual(t, got.AcceptedTotal, int64(2), "production и development")
		require.Equal(t, int64(2), got.RejectedTotal, "отказ с nil app_id не учитывается")
		require.Equal(t, "duration_required", got.Reasons[0].Code)
		require.Equal(t, rejected[0].PayloadSample, got.Reasons[0].ExamplePayload)
		require.Equal(t, rejected[0].Message, got.Reasons[0].ExampleMessage)
		require.Contains(t, raw, `"hour":"2026-10-12T09:00:00+03:00"`, "первый час окна в TZ приложения")
		require.Contains(t, raw, `"hour":"2026-10-13T08:00:00+03:00"`)
		require.Contains(t, raw, `"last_seen":"2026-10-13T04:30:00Z"`)
		require.Regexp(t, `"first_event_at":"[^"]+Z"`, raw)
		require.NotContains(t, raw, "invalid_app_key")
	})

	t.Run("hours", func(t *testing.T) {
		for _, h := range []int{1, 168} {
			want, err := e.reports.IntegrationHealth(ctx, reports.Params{AppID: msk.ID, Location: loadLocation(t, msk)}, h)
			require.NoError(t, err)
			got, _ := health(t, msk.ID, "hours="+strconv.Itoa(h))
			requireSame(t, want, got)
			require.Equal(t, h, got.Hours)
			require.Len(t, got.Hourly, h)
		}
	})

	t.Run("waiting_for_first_event", func(t *testing.T) {
		fresh := uuid.MustParse("5ce0a000-0000-4000-8000-0000000000aa")
		_, err := e.env.PG.Pool().Exec(ctx,
			`INSERT INTO apps (id, name, platform, timezone) VALUES ($1, 'Новое', 'web', 'Europe/Moscow')`, fresh)
		require.NoError(t, err)
		got, raw := health(t, fresh, "")
		require.Contains(t, raw, `"first_event_at":null`)
		require.Zero(t, got.AcceptedTotal)
		require.Zero(t, got.RejectedTotal)
		require.Len(t, got.Hourly, 24)
		require.Contains(t, raw, `"reasons":[]`)
		require.Contains(t, raw, `"versions":[]`)
	})

	t.Run("archived_and_restricted", func(t *testing.T) {
		require.Equal(t, http.StatusOK, e.get(t, integrationPath(tyo.ID, ""), e.all).Code)
		require.Equal(t, http.StatusOK, e.get(t, integrationPath(nyc.ID, ""), e.restricted).Code)
		body := errorBody(t, e.get(t, integrationPath(msk.ID, ""), e.restricted), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})

	t.Run("app_not_found", func(t *testing.T) {
		unknown := uuid.MustParse("5ce0a000-0000-4000-8000-0000000000ff")
		body := errorBody(t, e.get(t, integrationPath(unknown, ""), e.all), http.StatusNotFound)
		require.Equal(t, "app_not_found", body.Code)
	})
}

// Тест-вектор hours_limit (AC-033.3): hours вне 1..168 и не число — 400 invalid_request, field=hours, присланное
// значение не цитируется.
func TestHealth_HoursLimit(t *testing.T) {
	e := newPortfolioEnv(t)
	app := testutil.ScenarioApps[0].ID
	for _, q := range []string{"0", "169", "-1", "abc", "1.5", ""} {
		t.Run("hours="+q, func(t *testing.T) {
			rec := e.get(t, integrationPath(app, "hours="+q), e.all)
			body := errorBody(t, rec, http.StatusBadRequest)
			require.Equal(t, "invalid_request", body.Code)
			require.Equal(t, "hours", body.Details["field"])
			if q != "" {
				require.NotContains(t, body.Message, q)
			}
		})
	}
}
