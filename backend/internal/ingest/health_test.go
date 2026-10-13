package ingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

func getReady(t *testing.T, h http.Handler) (int, ingestapi.ReadyStatus) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", nil))
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	var st ingestapi.ReadyStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &st), rec.Body.String())
	for name, v := range st.Checks {
		require.True(t, v.Valid(), "checks.%s = %q вне enum ReadyStatus", name, v)
	}
	return rec.Code, st
}

func TestReady_WaitsForKeyCache(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	h.store.fail(errors.New("connection refused"), nil)
	ready := ingest.NewReadyHandler(h.cache.ReadyCheck())

	// Процесс только стартовал, Postgres недоступен: кэш не загружен.
	require.Error(t, h.cache.Refresh(t.Context()))
	code, st := getReady(t, ready)
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, ingestapi.ReadyStatus{
		Status: "not_ready",
		Checks: map[string]ingestapi.ReadyStatusChecks{"key_cache": "fail"},
	}, st)

	h.store.fail(nil, nil)
	require.NoError(t, h.cache.Refresh(t.Context()))
	code, st = getReady(t, ready)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, ingestapi.ReadyStatus{
		Status: "ready",
		Checks: map[string]ingestapi.ReadyStatusChecks{"key_cache": "ok"},
	}, st)

	// Снимок старше 15 минут: приём событий отвечает 503, значит процесс не готов.
	h.clock.Advance(16 * time.Minute)
	code, st = getReady(t, ready)
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, ingestapi.ReadyStatusChecks("fail"), st.Checks["key_cache"])
}

func TestReady_AllChecksReported(t *testing.T) {
	var gotDeadline bool
	ready := ingest.NewReadyHandler(
		ingest.ReadyCheck{Name: "postgres", Check: func(ctx context.Context) error {
			_, gotDeadline = ctx.Deadline()
			return nil
		}},
		ingest.ReadyCheck{Name: "clickhouse", Check: func(context.Context) error { return errors.New("down") }},
	)

	code, st := getReady(t, ready)
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, ingestapi.ReadyStatus{
		Status: "not_ready",
		Checks: map[string]ingestapi.ReadyStatusChecks{"postgres": "ok", "clickhouse": "fail"},
	}, st)
	require.True(t, gotDeadline, "проверка зависимости ограничена таймаутом")
}

func TestReady_NoChecksIsReady(t *testing.T) {
	code, st := getReady(t, ingest.NewReadyHandler())
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, ingestapi.ReadyStatus{Status: "ready", Checks: map[string]ingestapi.ReadyStatusChecks{}}, st)
}
