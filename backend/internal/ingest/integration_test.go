//go:build integration

package ingest_test

import (
	"bytes"
	"io"
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
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/ratelimit"
	"appmetrica.online/backend/internal/testutil"
)

// liveIngest — ingest поверх настоящих Postgres (ключи) и ClickHouse (events, rejected_events).
type liveIngest struct {
	handler  http.Handler
	env      *testutil.Env
	rejected *chstore.RejectedLog
	appID    uuid.UUID
	keyID    uuid.UUID
}

// newLiveIngest заводит приложение «Notes iOS» с ключом testKey. writer выбирает хранилище событий
// (nil — env.CH).
func newLiveIngest(t *testing.T, writer func(env *testutil.Env) ingest.EventWriter) *liveIngest {
	t.Helper()
	ctx := t.Context()
	env := testutil.Start(t)
	q := env.PG.Queries()
	app, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Notes iOS", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}, Timezone: "Europe/Moscow"})
	require.NoError(t, err)
	key, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: testKey, Label: "Основной"})
	require.NoError(t, err)
	var w ingest.EventWriter = env.CH
	if writer != nil {
		w = writer(env)
	}

	metrics, err := ingest.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	clk := clock.NewFixed(receivedAt)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := ingest.NewKeyCache(ingest.KeyCacheConfig{Store: q, Clock: clk, Logger: logger, Metrics: metrics})
	require.NoError(t, cache.Refresh(ctx))
	rejected := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: env.CH})
	rl := ingest.RejectedLogger{Sink: rejected, Keys: cache, Metrics: metrics}
	p := ingest.NewPipeline(ingest.PipelineConfig{
		Limits: ingest.RateLimits{
			Apps:     ratelimit.NewAppLimiter(2000, 4000, clk),
			Installs: ratelimit.NewInstallLimiter(100, 1000, clk),
			Metrics:  metrics,
		},
		Enricher: ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true},
		Time:     ingest.DefaultTimeConfig,
		Writer:   w,
		Rejected: rl,
		Clock:    clk,
		Logger:   logger,
		Metrics:  metrics,
	})
	h := httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: p, Keys: cache, Clock: clk, Logger: logger, Metrics: metrics, BatchErrors: rl,
	}), httpx.CORS(), httpx.RequestID(clk))
	return &liveIngest{handler: h, env: env, rejected: rejected, appID: app.ID, keyID: key.ID}
}

// post отправляет батч с ключом testKey; адрес клиента — X-Forwarded-For от Traefik.
func (l *liveIngest) post(t *testing.T, body []byte, forwardedFor string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Key", testKey)
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	rec := httptest.NewRecorder()
	l.handler.ServeHTTP(rec, req)
	return rec
}

func (l *liveIngest) count(t *testing.T, query string) uint64 {
	t.Helper()
	var n uint64
	require.NoError(t, l.env.CH.Conn().QueryRow(t.Context(), query, l.appID).Scan(&n))
	return n
}

// TestIngest_HappyPath_InsertsTwoEvents — AC-014.1 (I-01): 200 только после ack, строки сразу видны в events.
func TestIngest_HappyPath_InsertsTwoEvents(t *testing.T) {
	l := newLiveIngest(t, nil)
	rec := l.post(t, []byte(batchI01), "10.0.0.1, "+londonIP)
	res := decodeResult(t, rec)
	require.Equal(t, 2, res.Accepted)
	require.Empty(t, res.Rejected)
	require.Empty(t, res.Warnings)

	var rows []chstore.EventRow
	require.NoError(t, l.env.CH.Conn().Select(t.Context(), &rows,
		"SELECT * FROM events WHERE app_id = ? ORDER BY event_time", l.appID))
	require.Len(t, rows, 2)
	wantTimes := []time.Time{ts("2026-10-13T09:11:57.884Z"), ts("2026-10-13T09:15:01.870Z")}
	for i, r := range rows {
		require.Equal(t, wantTimes[i], r.EventTime.UTC(), "event_time = received_at + (timestamp − sent_at)")
		require.Equal(t, "GB", r.Country)
		require.Equal(t, l.keyID, r.WriteKeyID)
		require.Equal(t, "ios", r.Platform)
		require.Equal(t, "3.4.0", r.AppVersion)
		require.Equal(t, receivedAt, r.ReceivedAt.UTC())
	}
	require.Equal(t, "session_start", rows[0].Name)
	require.Equal(t, uint32(183986), rows[1].DurationMs)
}

// TestIngest_RetryIsIdempotent — AC-014.5 (I-36): повтор того же батча не завышает число сессий.
func TestIngest_RetryIsIdempotent(t *testing.T) {
	l := newLiveIngest(t, nil)
	requireAccepted(t, l.post(t, []byte(batchI01), ""), 2)
	requireAccepted(t, l.post(t, []byte(batchI01), ""), 2)

	require.Equal(t, uint64(1), l.count(t, "SELECT uniqExact(session_id) FROM events WHERE app_id = ? AND name = 'session_start'"))
	require.Equal(t, uint64(2), l.count(t, "SELECT count() FROM events FINAL WHERE app_id = ?"),
		"ReplacingMergeTree схлопывает повтор по insert_id")
}

// TestIngest_ClickHouseDown — I-35: соединение с ClickHouse потеряно — 503 storage_unavailable, Retry-After: 5.
func TestIngest_ClickHouseDown(t *testing.T) {
	l := newLiveIngest(t, func(env *testutil.Env) ingest.EventWriter {
		down, err := chstore.Open(t.Context(), env.ClickHouse)
		require.NoError(t, err)
		require.NoError(t, down.Close())
		return down
	})
	rec := l.post(t, []byte(batchI01), "")
	requireError(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
	require.Equal(t, "5", rec.Header().Get("Retry-After"))
	require.Zero(t, l.count(t, "SELECT count() FROM events WHERE app_id = ?"))
}

// TestIngest_RejectedEventsLogged — I-38: отказ PII попадает в rejected_events без user_id и IP.
func TestIngest_RejectedEventsLogged(t *testing.T) {
	l := newLiveIngest(t, nil)
	body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"ip": londonIP, "app_version": "3.4.0"},
		eventJSON(t, map[string]any{"user_id": "ivan@example.com"}))
	res := decodeResult(t, l.post(t, body, ""))
	require.Zero(t, res.Accepted)
	require.NoError(t, l.rejected.Flush(t.Context()))

	var rows []chstore.RejectedRow
	require.NoError(t, l.env.CH.Conn().Select(t.Context(), &rows, "SELECT * FROM rejected_events WHERE app_id = ?", l.appID))
	require.Len(t, rows, 1)
	r := rows[0]
	require.Equal(t, "user_id_looks_like_pii", r.Code)
	require.Equal(t, "events[0].user_id", r.Field)
	require.Equal(t, testKey[:11], r.KeyPrefix)
	require.Equal(t, "3.4.0", r.AppVersion)
	require.LessOrEqual(t, len(r.PayloadSample), ingest.MaxPayloadSampleBytes)
	require.NotContains(t, r.PayloadSample, "ivan@example.com")
	require.NotContains(t, r.PayloadSample, londonIP)
	require.Zero(t, l.count(t, "SELECT count() FROM events WHERE app_id = ?"))
}
