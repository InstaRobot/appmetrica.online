package ingest_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

var (
	appA = uuid.MustParse("0199d600-0000-7000-8000-00000000000a")
	appB = uuid.MustParse("0199d600-0000-7000-8000-00000000000b")
	wk1  = uuid.MustParse("0199d600-0000-7000-8000-000000000001")
	wk2  = uuid.MustParse("0199d600-0000-7000-8000-000000000002")
)

func keyRow(id uuid.UUID, key string, app uuid.UUID, status sqlc.AppStatus) sqlc.LoadWriteKeyCacheRow {
	return sqlc.LoadWriteKeyCacheRow{WriteKeyID: id, Key: key, AppID: app, AppStatus: status, AppPlatforms: []sqlc.AppPlatform{sqlc.AppPlatformIos}}
}

// fakeKeyStore — Postgres кэша ключей: снимок write_keys+apps, ошибка загрузки и журнал TouchWriteKeysLastUsed.
type fakeKeyStore struct {
	mu       sync.Mutex
	rows     []sqlc.LoadWriteKeyCacheRow
	loadErr  error
	touchErr error
	loads    int
	touches  []sqlc.TouchWriteKeysLastUsedParams
	onLoad   func() // вызывается внутри LoadWriteKeyCache: медленный запрос Postgres
}

func (s *fakeKeyStore) LoadWriteKeyCache(context.Context) ([]sqlc.LoadWriteKeyCacheRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.onLoad != nil {
		s.onLoad()
	}
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return slices.Clone(s.rows), nil
}

func (s *fakeKeyStore) TouchWriteKeysLastUsed(_ context.Context, p sqlc.TouchWriteKeysLastUsedParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.touchErr != nil {
		return s.touchErr
	}
	s.touches = append(s.touches, p)
	return nil
}

func (s *fakeKeyStore) set(rows ...sqlc.LoadWriteKeyCacheRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = rows
}

func (s *fakeKeyStore) fail(load, touch error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr, s.touchErr = load, touch
}

func (s *fakeKeyStore) touchCalls() []sqlc.TouchWriteKeysLastUsedParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.touches)
}

// keyHarness — хендлер POST /v1/events с проверкой ключа через кэш и fakeProcessor вместо конвейера.
// fakeProcessor стоит на месте записи в ClickHouse: батч, не дошедший до него, не пишет ни одной строки.
type keyHarness struct {
	harness
	store   *fakeKeyStore
	cache   *ingest.KeyCache
	clock   *clock.Fixed
	logger  *slog.Logger
	metrics *ingest.Metrics
}

func newKeyHarness(t *testing.T, cfg ingest.KeyCacheConfig) *keyHarness {
	t.Helper()
	reg := prometheus.NewRegistry()
	metrics, err := ingest.NewMetrics(reg)
	require.NoError(t, err)
	h := &keyHarness{
		harness: harness{proc: &fakeProcessor{}, reg: reg, logs: &bytes.Buffer{}},
		store:   &fakeKeyStore{},
		clock:   clock.NewFixed(receivedAt),
		metrics: metrics,
	}
	h.logger = slog.New(slog.NewJSONHandler(h.logs, nil))
	cfg.Store, cfg.Clock, cfg.Logger, cfg.Metrics = h.store, h.clock, h.logger, metrics
	h.cache = ingest.NewKeyCache(cfg)
	h.setPipeline(h.proc)
	return h
}

// setPipeline пересобирает хендлер вокруг другой цепочки шагов конвейера.
func (h *keyHarness) setPipeline(p ingest.Processor) {
	h.handler = httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: p,
		Keys:      h.cache,
		Clock:     h.clock,
		Logger:    h.logger,
		Metrics:   h.metrics,
	}), httpx.RequestID(h.clock))
}

func (h *keyHarness) post(t *testing.T, key string) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(t, request{contentType: "application/json", header: key, body: []byte(batchI01)})
}

// metricValue — значение метрики без меток (gauge или counter) из реестра.
func metricValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		require.Len(t, mf.GetMetric(), 1)
		m := mf.GetMetric()[0]
		if m.GetGauge() != nil {
			return m.GetGauge().GetValue()
		}
		return m.GetCounter().GetValue()
	}
	t.Fatalf("метрика %s не зарегистрирована", name)
	return 0
}

func TestKeyCache_MultipleActiveKeys(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(
		keyRow(wk1, testKey, appA, sqlc.AppStatusActive),
		keyRow(wk2, otherKey, appA, sqlc.AppStatusActive),
	)
	require.NoError(t, h.cache.Refresh(t.Context()))

	requireAccepted(t, h.post(t, testKey), 2)
	requireAccepted(t, h.post(t, otherKey), 2)

	calls := h.proc.calls()
	require.Len(t, calls, 2)
	require.Equal(t, ingest.KeyInfo{WriteKeyID: wk1, AppID: appA, AppStatus: sqlc.AppStatusActive, AppPlatform: sqlc.AppPlatformIos}, calls[0].Key)
	require.Equal(t, wk2, calls[1].Key.WriteKeyID, "каждый ключ пишет свой write_key_id")
	require.Equal(t, appA, calls[1].Key.AppID)

	require.InDelta(t, 2, metricValue(t, h.reg, "ingest_key_cache_size"), 0)
	require.InDelta(t, float64(receivedAt.Unix()), metricValue(t, h.reg, "ingest_key_cache_last_refresh_timestamp_seconds"), 0)
	require.NotContains(t, h.logs.String(), testKey, "ключ pk_ не логируется")
}

func TestKeyCache_RevokedKeyDisappearsAfterRefresh(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive), keyRow(wk2, otherKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))
	requireAccepted(t, h.post(t, testKey), 2)

	// Отзыв в PG: LoadWriteKeyCache больше не возвращает ключ. До цикла обновления кэш ещё старый (≤15 с).
	h.store.set(keyRow(wk2, otherKey, appA, sqlc.AppStatusActive))
	requireAccepted(t, h.post(t, testKey), 2)

	h.clock.Advance(15 * time.Second)
	require.NoError(t, h.cache.Refresh(t.Context()))

	requireError(t, h.post(t, testKey), http.StatusUnauthorized, "invalid_app_key")
	require.Len(t, h.proc.calls(), 2, "I-21: батч с отозванным ключом не доходит до записи — в CH 0 строк")
	requireAccepted(t, h.post(t, otherKey), 2)

	requireError(t, h.post(t, "pk_unknownunknownunknownunknownun"), http.StatusUnauthorized, "invalid_app_key")
	require.InDelta(t, 1, metricValue(t, h.reg, "ingest_key_cache_size"), 0)
}

func TestKeyCache_ArchivedAppRejected(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusArchived), keyRow(wk2, otherKey, appB, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))

	requireError(t, h.post(t, testKey), http.StatusForbidden, "app_archived")
	require.Empty(t, h.proc.calls(), "I-22: архивное приложение не пишет событий")
	requireAccepted(t, h.post(t, otherKey), 2)

	// Разархивирование возвращает приём после следующего обновления.
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))
	requireAccepted(t, h.post(t, testKey), 2)
}

func TestKeyCache_KeepsStaleOnError(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))
	loadedAt := float64(receivedAt.Unix())

	pgDown := errors.New("dial tcp: connection refused")
	h.store.fail(pgDown, nil)

	// 10 минут без Postgres: каждое обновление падает, ключи работают по старому снимку.
	for range 40 {
		h.clock.Advance(15 * time.Second)
		require.ErrorIs(t, h.cache.Refresh(t.Context()), pgDown)
	}
	requireAccepted(t, h.post(t, testKey), 2)
	require.InDelta(t, 40, metricValue(t, h.reg, "ingest_key_cache_refresh_errors_total"), 0)
	require.InDelta(t, loadedAt, metricValue(t, h.reg, "ingest_key_cache_last_refresh_timestamp_seconds"), 0, "время последней успешной загрузки не сдвигается")
	require.InDelta(t, 1, metricValue(t, h.reg, "ingest_key_cache_size"), 0)
	require.Contains(t, h.logs.String(), "key cache refresh failed")

	// Ровно 15 минут — снимок ещё допустим; старше — 503, отзыв ключей нельзя гарантировать.
	h.clock.Set(receivedAt.Add(15 * time.Minute))
	requireAccepted(t, h.post(t, testKey), 2)

	h.clock.Set(receivedAt.Add(16 * time.Minute))
	require.Error(t, h.cache.Refresh(t.Context()))
	rec := h.post(t, testKey)
	requireError(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
	require.Equal(t, "60", rec.Header().Get("Retry-After"))
	requireError(t, h.post(t, otherKey), http.StatusServiceUnavailable, "storage_unavailable")
	require.Len(t, h.proc.calls(), 2, "при устаревшем снимке ни одного события не пишется")

	// Postgres вернулся: следующий цикл обновления восстанавливает приём.
	h.store.fail(nil, nil)
	require.NoError(t, h.cache.Refresh(t.Context()))
	requireAccepted(t, h.post(t, testKey), 2)
	require.NotContains(t, h.logs.String(), testKey)
}

// Возраст снимка считается от начала чтения: ключ, отозванный во время долгого запроса, в снимке ещё есть, и
// такой снимок не должен жить дольше KeyCacheMaxAge после момента, когда он ещё был верен.
func TestKeyCache_SnapshotAgeCountsFromReadStart(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	h.store.onLoad = func() { h.clock.Advance(2 * time.Minute) }
	require.NoError(t, h.cache.Refresh(t.Context()))
	require.InDelta(t, float64(receivedAt.Unix()), metricValue(t, h.reg, "ingest_key_cache_last_refresh_timestamp_seconds"), 0,
		"метрика показывает начало чтения")

	h.store.onLoad = nil
	h.store.fail(errors.New("dial tcp: connection refused"), nil)
	h.clock.Set(receivedAt.Add(15 * time.Minute))
	requireAccepted(t, h.post(t, testKey), 2)

	h.clock.Set(receivedAt.Add(15*time.Minute + time.Second))
	requireError(t, h.post(t, testKey), http.StatusServiceUnavailable, "storage_unavailable")
	_, known := h.cache.Peek(testKey)
	require.False(t, known, "журнал отказов тоже не использует снимок старше KeyCacheMaxAge от начала чтения")
}

func TestKeyCache_NotLoadedYet(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	require.False(t, h.cache.Loaded())

	rec := h.post(t, testKey)
	requireError(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
	require.Empty(t, h.proc.calls())

	require.NoError(t, h.cache.Refresh(t.Context()))
	require.True(t, h.cache.Loaded())
}

func TestKeyCache_EmptySnapshotIsLoaded(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	require.NoError(t, h.cache.Refresh(t.Context()))
	require.True(t, h.cache.Loaded(), "пустая таблица write_keys — тоже успешная загрузка")
	requireError(t, h.post(t, testKey), http.StatusUnauthorized, "invalid_app_key")
}

func TestKeyCache_FlushLastUsed(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive), keyRow(wk2, otherKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))

	t0 := receivedAt
	requireAccepted(t, h.post(t, testKey), 2)
	h.clock.Advance(time.Minute)
	requireAccepted(t, h.post(t, testKey), 2)
	h.clock.Advance(time.Minute)
	requireAccepted(t, h.post(t, otherKey), 2)
	require.Empty(t, h.store.touchCalls(), "last_used_at копится в памяти, а не пишется на каждый запрос")

	require.NoError(t, h.cache.FlushLastUsed(t.Context()))
	touches := h.store.touchCalls()
	require.Len(t, touches, 1, "один UPDATE … FROM unnest на все ключи")
	require.Equal(t, []uuid.UUID{wk1, wk2}, touches[0].Ids)
	require.Equal(t, []time.Time{t0.Add(time.Minute), t0.Add(2 * time.Minute)}, touches[0].LastUsed, "по ключу — последнее использование")

	require.NoError(t, h.cache.FlushLastUsed(t.Context()))
	require.Len(t, h.store.touchCalls(), 1, "без новых запросов UPDATE не выполняется")

	// Отклонённые запросы не обновляют last_used_at.
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusArchived))
	require.NoError(t, h.cache.Refresh(t.Context()))
	requireError(t, h.post(t, testKey), http.StatusForbidden, "app_archived")
	require.NoError(t, h.cache.FlushLastUsed(t.Context()))
	require.Len(t, h.store.touchCalls(), 1)
}

func TestKeyCache_FlushLastUsedRetriesAfterError(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive), keyRow(wk2, otherKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))
	requireAccepted(t, h.post(t, testKey), 2)

	pgDown := errors.New("connection refused")
	h.store.fail(nil, pgDown)
	require.ErrorIs(t, h.cache.FlushLastUsed(t.Context()), pgDown)

	// Пока Postgres лежал, ключ использовался снова: в следующий сброс уходит более позднее время.
	h.clock.Advance(time.Minute)
	requireAccepted(t, h.post(t, testKey), 2)
	h.store.fail(nil, nil)
	require.NoError(t, h.cache.FlushLastUsed(t.Context()))

	touches := h.store.touchCalls()
	require.Len(t, touches, 1)
	require.Equal(t, []uuid.UUID{wk1}, touches[0].Ids)
	require.Equal(t, []time.Time{receivedAt.Add(time.Minute)}, touches[0].LastUsed)
}

func TestKeyCache_RunRefreshesAndFlushesOnStop(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{Refresh: 5 * time.Millisecond, FlushInterval: time.Hour})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		h.cache.Run(ctx)
		close(done)
	}()

	require.Eventually(t, h.cache.Loaded, 2*time.Second, time.Millisecond, "первая загрузка сразу при старте")
	requireAccepted(t, h.post(t, testKey), 2)

	h.store.set()
	require.Eventually(t, func() bool {
		return h.post(t, testKey).Code == http.StatusUnauthorized
	}, 2*time.Second, time.Millisecond, "отзыв подхватывается следующим циклом обновления")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
	}
	touches := h.store.touchCalls()
	require.Len(t, touches, 1, "при остановке накопленный last_used_at сбрасывается")
	require.Equal(t, []uuid.UUID{wk1}, touches[0].Ids)
}

func TestKeyCache_RunFlushesPeriodically(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{Refresh: time.Hour, FlushInterval: 5 * time.Millisecond})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go h.cache.Run(ctx)

	require.Eventually(t, h.cache.Loaded, 2*time.Second, time.Millisecond)
	requireAccepted(t, h.post(t, testKey), 2)
	require.Eventually(t, func() bool { return len(h.store.touchCalls()) == 1 }, 2*time.Second, time.Millisecond)
}

func TestKeyCache_DefaultIntervals(t *testing.T) {
	require.Equal(t, 15*time.Minute, ingest.KeyCacheMaxAge)
	require.Equal(t, 5*time.Minute, ingest.DefaultLastUsedFlushInterval)
	require.Equal(t, 15*time.Second, ingest.DefaultKeyCacheRefresh)
}

func TestKeyCache_ConcurrentRefreshAndLookup(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 200 {
				_, err := h.cache.Lookup(testKey)
				if err != nil {
					t.Errorf("lookup: %v", err)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for range 200 {
			if err := h.cache.Refresh(t.Context()); err != nil {
				t.Errorf("refresh: %v", err)
				return
			}
		}
	})
	wg.Go(func() {
		for range 50 {
			if err := h.cache.FlushLastUsed(t.Context()); err != nil {
				t.Errorf("flush: %v", err)
				return
			}
		}
	})
	wg.Wait()
}

func TestAuthenticate_PassesProcessorErrors(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	h.store.set(keyRow(wk1, testKey, appA, sqlc.AppStatusActive))
	require.NoError(t, h.cache.Refresh(t.Context()))
	h.proc.err = &ingest.Error{Status: http.StatusTooManyRequests, Code: "app_rate_limited", Message: "x", RetryAfter: 1}

	requireError(t, h.post(t, testKey), http.StatusTooManyRequests, "app_rate_limited")
}
