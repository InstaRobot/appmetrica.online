//go:build integration

package reports_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/ratelimit"
	"appmetrica.online/backend/internal/reports"
	"appmetrica.online/backend/internal/testutil"
)

const realtimeKey = "pk_Rt7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0J"

// realtimeIngest — ingest поверх тех же Postgres (ключи) и ClickHouse, что читает отчёт; часы системные.
func realtimeIngest(t *testing.T, env *testutil.Env) (http.Handler, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	q := env.PG.Queries()
	app, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Notes iOS", Platform: sqlc.AppPlatformIos, Timezone: "Europe/Moscow"})
	require.NoError(t, err)
	_, err = q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: realtimeKey, Label: "Основной"})
	require.NoError(t, err)

	geoDB, err := geo.Open(testutil.GeoDBPath())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, geoDB.Close()) })
	metrics, err := ingest.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	clk := clock.System{}
	logger := slog.New(slog.DiscardHandler)
	cache := ingest.NewKeyCache(ingest.KeyCacheConfig{Store: q, Clock: clk, Logger: logger, Metrics: metrics})
	require.NoError(t, cache.Refresh(ctx))
	rl := ingest.RejectedLogger{Sink: chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: env.CH}), Keys: cache, Metrics: metrics}
	p := ingest.NewPipeline(ingest.PipelineConfig{
		Limits: ingest.RateLimits{
			Apps:     ratelimit.NewAppLimiter(2000, 4000, clk),
			Installs: ratelimit.NewInstallLimiter(100, 1000, clk),
			Metrics:  metrics,
		},
		Enricher: ingest.Enricher{Geo: geoDB},
		Time:     ingest.DefaultTimeConfig,
		Writer:   env.CH,
		Rejected: rl,
		Clock:    clk,
		Logger:   logger,
		Metrics:  metrics,
	})
	h := httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: p, Keys: cache, Clock: clk, Logger: logger, Metrics: metrics, BatchErrors: rl,
	}), httpx.RequestID(clk))
	return h, app.ID
}

// AC-031.2 (AC-5.4.2, NFR-004): событие, на которое ingest ответил 200, появляется в recent_events при опросе
// getRealtime каждые 500 мс не позже чем через 10 с.
func TestRealtime_VisibleWithin10s(t *testing.T) {
	env := testutil.Start(t)
	handler, appID := realtimeIngest(t, env)
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	svc := reports.NewService(env.CH.Conn(), clock.System{})

	insertID, installID, sessionID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.Marshal(map[string]any{
		"sent_at": now,
		"context": map[string]any{"platform": "ios", "os_name": "iOS", "app_version": "3.4.0", "environment": "production"},
		"events": []map[string]any{{
			"insert_id": insertID, "name": "session_start", "timestamp": now,
			"install_id": installID, "session_id": sessionID,
		}},
	})
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Key", realtimeKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	acked := time.Now()

	p := reports.Params{AppID: appID, Location: msk, Env: reports.EnvProduction}
	var seen reports.RecentEvent
	require.Eventually(t, func() bool {
		r, err := svc.Realtime(t.Context(), p)
		if err != nil {
			return false
		}
		for _, e := range r.RecentEvents {
			if e.InsertID == insertID {
				seen = e
				return true
			}
		}
		return false
	}, 10*time.Second, 500*time.Millisecond, "событие не появилось в recent_events за 10 с")
	t.Logf("событие в ленте через %s после ответа 200", time.Since(acked).Round(time.Millisecond))

	require.Equal(t, installID, seen.InstallID)
	require.Equal(t, "session_start", seen.Name)
	require.Equal(t, "i:"+installID.String(), seen.PersonKey)
	r, err := svc.Realtime(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, uint64(1), r.ActiveNow, "сессия начата и не завершена")
}
