package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const ingestTestKey = "pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2"

type stubKeyStore struct{ err error }

func (s stubKeyStore) LoadWriteKeyCache(context.Context) ([]sqlc.LoadWriteKeyCacheRow, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []sqlc.LoadWriteKeyCacheRow{{
		WriteKeyID: uuid.New(), Key: ingestTestKey, AppID: uuid.New(),
		AppStatus: sqlc.AppStatusActive, AppPlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos},
	}}, nil
}

func (stubKeyStore) TouchWriteKeysLastUsed(context.Context, sqlc.TouchWriteKeysLastUsedParams) error {
	return nil
}

type stubEvents struct {
	mu   sync.Mutex
	rows []chstore.EventRow
}

func (s *stubEvents) InsertEvents(_ context.Context, rows []chstore.EventRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, rows...)
	return nil
}

type stubRejected struct {
	mu   sync.Mutex
	rows []chstore.RejectedRow
}

func (s *stubRejected) InsertRejected(_ context.Context, rows []chstore.RejectedRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, rows...)
	return nil
}

func (s *stubRejected) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

type noGeo struct{}

func (noGeo) Lookup(netip.Addr) geo.Location { return geo.Location{} }

func ingestTestConfig() config.Ingest {
	return config.Ingest{
		MaxBodyBytes: 1 << 20, MaxEvents: 500,
		LateWindow: 168 * time.Hour, FutureTolerance: 10 * time.Minute,
		RateAppEventsPerSec: 2000, RateAppBurst: 4000, RateInstallPerMin: 100, RateInstallBurst: 1000,
		AcceptDevelopment: true, KeyCacheRefresh: time.Hour,
	}
}

func newTestIngestServer(t *testing.T, keys ingest.KeyStore, ready ...ingest.ReadyCheck) (*ingestServer, *stubEvents, *stubRejected, *prometheus.Registry) {
	t.Helper()
	events, rejected := &stubEvents{}, &stubRejected{}
	reg := prometheus.NewRegistry()
	srv, err := newIngestServer(ingestTestConfig(), ingestDeps{
		Keys: keys, Events: events, Rejected: rejected, Geo: noGeo{}, Ready: ready,
	}, reg, slog.New(slog.NewTextHandler(io.Discard, nil)), clock.NewFixed(time.Now().UTC()))
	require.NoError(t, err)
	return srv, events, rejected, reg
}

func serve(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("X-App-Key", key)
	}
	req.Header.Set("Origin", "https://site.ru")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func testBatch() string {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return `{"sent_at":"` + now + `","events":[` +
		`{"insert_id":"` + uuid.NewString() + `","name":"session_start","timestamp":"` + now + `",` +
		`"install_id":"` + uuid.NewString() + `","session_id":"` + uuid.NewString() + `"},` +
		`{"insert_id":"` + uuid.NewString() + `","name":"purchase","timestamp":"` + now + `",` +
		`"install_id":"` + uuid.NewString() + `","session_id":"` + uuid.NewString() + `"}]}`
}

func TestIngestServer_Routes(t *testing.T) {
	srv, events, rejected, reg := newTestIngestServer(t, stubKeyStore{},
		ingest.ReadyCheck{Name: "clickhouse", Check: func(context.Context) error { return nil }})

	rec := serve(t, srv.handler, http.MethodGet, "/health", "", "")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = serve(t, srv.handler, http.MethodGet, "/ready", "", "")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "кэш ключей ещё не загружен")

	rec = serve(t, srv.handler, http.MethodOptions, "/v1/events", "", "")
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "X-App-Key")

	rec = serve(t, srv.handler, http.MethodPost, "/v1/events", ingestTestKey, testBatch())
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "без снимка ключей — 503")

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	for _, run := range srv.background {
		wg.Go(func() { run(ctx) })
	}
	require.Eventually(t, func() bool {
		return serve(t, srv.handler, http.MethodGet, "/ready", "", "").Code == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond)
	var ready map[string]any
	require.NoError(t, json.Unmarshal(serve(t, srv.handler, http.MethodGet, "/ready", "", "").Body.Bytes(), &ready))
	require.Equal(t, map[string]any{"key_cache": "ok", "clickhouse": "ok"}, ready["checks"])

	rec = serve(t, srv.handler, http.MethodPost, "/v1/events", ingestTestKey, testBatch())
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))
	require.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	var res struct {
		Accepted int `json:"accepted"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Equal(t, 1, res.Accepted)
	require.Len(t, events.rows, 1)

	rec = serve(t, srv.handler, http.MethodPost, "/v1/events", "", testBatch())
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = serve(t, srv.handler, http.MethodGet, "/v1/events", ingestTestKey, "")
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	cancel()
	wg.Wait()
	require.Equal(t, 1, rejected.len(), "при остановке журнал отказов сброшен в ClickHouse")

	families, err := reg.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, mf := range families {
		names[mf.GetName()] = true
	}
	for _, name := range []string{
		"ingest_requests_total", "ingest_events_accepted_total", "ingest_events_rejected_total",
		"ingest_batch_duration_seconds", "ingest_batch_events", "ingest_clickhouse_insert_duration_seconds",
		"ingest_install_limiters", "ingest_key_cache_size",
	} {
		require.True(t, names[name], "метрика %s зарегистрирована", name)
	}
}

func TestIngestServer_ReadyFailsWhenDependencyDown(t *testing.T) {
	srv, _, _, _ := newTestIngestServer(t, stubKeyStore{},
		ingest.ReadyCheck{Name: "postgres", Check: func(context.Context) error { return errors.New("down") }})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, run := range srv.background {
		go run(ctx)
	}
	require.Eventually(t, func() bool {
		rec := serve(t, srv.handler, http.MethodGet, "/ready", "", "")
		return rec.Code == http.StatusServiceUnavailable && strings.Contains(rec.Body.String(), `"key_cache":"ok"`)
	}, 2*time.Second, 10*time.Millisecond)
}

func TestIngestServer_MetricsHandler(t *testing.T) {
	srv, _, _, reg := newTestIngestServer(t, stubKeyStore{err: errors.New("pg down")})
	serve(t, srv.handler, http.MethodPost, "/v1/events", "", testBatch())
	rec := httptest.NewRecorder()
	metricsHandler(reg).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `ingest_requests_total{status="401"} 1`)
}

func TestIngestServer_DuplicateRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()
	deps := ingestDeps{Keys: stubKeyStore{}, Events: &stubEvents{}, Rejected: &stubRejected{}, Geo: noGeo{}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := newIngestServer(ingestTestConfig(), deps, reg, logger, clock.System{})
	require.NoError(t, err)
	_, err = newIngestServer(ingestTestConfig(), deps, reg, logger, clock.System{})
	require.Error(t, err)
}
