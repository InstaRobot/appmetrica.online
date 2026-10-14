package ingest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// histogramCount — число наблюдений гистограммы name без меток.
func histogramCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() == name {
			require.Len(t, mf.GetMetric(), 1)
			return mf.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	return 0
}

func histogramSum(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() == name {
			return mf.GetMetric()[0].GetHistogram().GetSampleSum()
		}
	}
	return 0
}

func TestPipeline_Metrics(t *testing.T) {
	h := newPipelineHarness(t)
	body := batchBody(t, "2026-10-13T09:15:02.000Z", nil,
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0), "timestamp": "2026-10-13T09:40:02Z"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "user_id": "ivan@example.com"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(2), "name": "purchase"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(3), "name": "pageview"}),
	)
	res := decodeResult(t, h.postBody(t, body))
	require.Equal(t, 1, res.Accepted)
	requireAccepted(t, h.postBody(t, []byte(batchI01)), 2)
	h.do(t, request{contentType: "application/json", body: []byte(batchI01)})

	require.Equal(t, map[string]float64{appA.String(): 3}, labelCounts(t, h.reg, "ingest_events_accepted_total", "app_id"))
	require.Equal(t, map[string]float64{"user_id_looks_like_pii": 1, "unknown_event_name": 2},
		labelCounts(t, h.reg, "ingest_events_rejected_total", "code"))
	require.Equal(t, map[string]float64{"timestamp_in_future_clamped": 1, "duplicate_insert_id_in_batch": 1},
		labelCounts(t, h.reg, "ingest_warnings_total", "code"))
	require.Equal(t, uint64(2), histogramCount(t, h.reg, "ingest_batch_events"), "батчи, дошедшие до конвейера")
	require.InDelta(t, 7.0, histogramSum(t, h.reg, "ingest_batch_events"), 0)
	require.Equal(t, uint64(3), histogramCount(t, h.reg, "ingest_batch_duration_seconds"), "каждый запрос POST /v1/events")
	require.Equal(t, uint64(2), histogramCount(t, h.reg, "ingest_clickhouse_insert_duration_seconds"))
	require.Zero(t, metricValue(t, h.reg, "ingest_clickhouse_insert_errors_total"))
	require.Zero(t, metricValue(t, h.reg, "ingest_rejected_log_dropped_total"))
}

func TestPipeline_RejectedLogDropped(t *testing.T) {
	h := newPipelineHarness(t)
	h.sink.capacity = 1
	body := batchBody(t, "2026-10-13T09:15:02.000Z", nil,
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0), "name": "purchase"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "name": "purchase"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(2), "name": "purchase"}),
	)
	res := decodeResult(t, h.postBody(t, body))
	require.Len(t, res.Rejected, 3, "переполнение буфера не влияет на ответ")
	require.Len(t, h.sink.logged(), 1)
	require.InDelta(t, 2.0, metricValue(t, h.reg, "ingest_rejected_log_dropped_total"), 0)
}

func TestPipeline_RejectedRows(t *testing.T) {
	t.Run("event fields are best effort", func(t *testing.T) {
		h := newPipelineHarness(t)
		body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"app_version": "3.4.0", "ip": londonIP},
			eventJSON(t, map[string]any{"name": "purchase", "context": map[string]any{"platform": "android", "ip": londonIP}}),
			[]byte(`"not an object"`),
			eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "name": strings.Repeat("x", 200)}),
		)
		res := decodeResult(t, h.postBody(t, body))
		require.Len(t, res.Rejected, 3)
		logged := h.sink.logged()
		require.Len(t, logged, 3)

		require.Equal(t, "purchase", logged[0].EventName)
		require.Equal(t, "3.4.0", logged[0].AppVersion)
		require.Equal(t, "android", logged[0].Platform)
		require.Equal(t, "events[0].name", logged[0].Field)
		require.NotContains(t, logged[0].PayloadSample, londonIP)
		require.Contains(t, logged[0].PayloadSample, `"name":"purchase"`)

		require.Empty(t, logged[1].EventName)
		require.Equal(t, "ios", logged[1].Platform, "платформа по умолчанию — единственная платформа приложения")
		require.Equal(t, "3.4.0", logged[1].AppVersion)
		require.Equal(t, `"not an object"`, logged[1].PayloadSample)

		require.Len(t, logged[2].EventName, 64, "имя события обрезается до 64 символов")
	})

	t.Run("invalid batch context rejects every event", func(t *testing.T) {
		h := newPipelineHarness(t)
		body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"platform": "tvos", "app_version": "3.4.0"},
			eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
			eventJSON(t, map[string]any{"insert_id": numberedInsertID(1)}))
		res := decodeResult(t, h.postBody(t, body))
		require.Zero(t, res.Accepted)
		require.Len(t, res.Rejected, 2)
		require.Equal(t, "context.platform", res.Rejected[0].Field)
		require.Zero(t, h.writer.insertCalls())
		logged := h.sink.logged()
		require.Len(t, logged, 2)
		require.Equal(t, "ios", logged[0].Platform, "невалидный context батча не используется")
		require.Empty(t, logged[0].AppVersion)
		require.Equal(t, "session_start", logged[0].EventName)
	})

	t.Run("install rate limited is logged", func(t *testing.T) {
		h := newPipelineHarness(t)
		ok, _ := h.limits.Installs.Allow(installKey(appA, validInstallID), 1000)
		require.True(t, ok)
		res := decodeResult(t, h.postBody(t, oneEvent(t, nil)))
		require.Len(t, res.Rejected, 1)
		require.Equal(t, "install_rate_limited", h.sink.logged()[0].Code)
	})
}

func TestPipeline_DevelopmentKillSwitch(t *testing.T) {
	h := newPipelineHarness(t, func(c *ingest.PipelineConfig) { c.Enricher.AcceptDevelopment = false })
	body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"environment": "development"},
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "context": map[string]any{"environment": "production"}}))
	res := decodeResult(t, h.postBody(t, body))
	require.Equal(t, 2, res.Accepted, "development считается принятым")
	rows := h.writer.written()
	require.Len(t, rows, 1, "но не пишется")
	require.Equal(t, "production", rows[0].Environment)
}

// TestIngestHandler_BatchErrorsLogged — batch-ошибки известных приложений попадают в rejected_events
// (06-api-contracts.md §2.4.1 шаг 9, 05-data-model.md §2.9); неизвестные ключи не пишутся.
func TestIngestHandler_BatchErrorsLogged(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, h *pipelineHarness)
		req     request
		status  int
		code    string
		app     uuid.UUID // uuid.Nil — строки нет
		field   string
		maxSize bool
	}{
		{
			name: "app_rate_limited",
			setup: func(t *testing.T, h *pipelineHarness) {
				ok, _ := h.limits.Apps.Allow(appA, 4000)
				require.True(t, ok)
			},
			req:    request{contentType: "application/json", header: testKey, body: []byte(batchI01)},
			status: http.StatusTooManyRequests, code: "app_rate_limited", app: appA,
		},
		{
			name:   "app_archived",
			req:    request{contentType: "application/json", header: archivedKey, body: []byte(batchI01)},
			status: http.StatusForbidden, code: "app_archived", app: appB,
		},
		{
			name:   "invalid_batch with key in header",
			req:    request{contentType: "application/json", header: testKey, body: []byte(`{"events":[` + event + `]}`)},
			status: http.StatusBadRequest, code: "invalid_batch", app: appA, field: "sent_at",
		},
		{
			name:   "invalid_batch with key in body",
			req:    request{contentType: "text/plain", body: []byte(`{"app_key":"` + testKey + `","sent_at":"2026-10-13T09:15:02Z","events":[]}`)},
			status: http.StatusBadRequest, code: "invalid_batch", app: appA, field: "events",
		},
		{
			name: "app_rate_limited with key in body",
			setup: func(t *testing.T, h *pipelineHarness) {
				ok, _ := h.limits.Apps.Allow(appA, 4000)
				require.True(t, ok)
			},
			req:    request{contentType: "text/plain", body: []byte(`{"app_key":"` + testKey + `","sent_at":"2026-10-13T09:15:02Z","events":[` + event + `]}`)},
			status: http.StatusTooManyRequests, code: "app_rate_limited", app: appA,
		},
		{
			name:   "app_archived with key in body",
			req:    request{contentType: "text/plain", body: []byte(`{"app_key":"` + archivedKey + `","sent_at":"2026-10-13T09:15:02Z","events":[` + event + `]}`)},
			status: http.StatusForbidden, code: "app_archived", app: appB,
		},
		{
			name:   "invalid_json",
			req:    request{contentType: "application/json", header: testKey, body: []byte(`{"sent_at":`)},
			status: http.StatusBadRequest, code: "invalid_json", app: appA,
		},
		{
			name:   "payload_too_large",
			req:    request{contentType: "application/json", header: testKey},
			status: http.StatusRequestEntityTooLarge, code: "payload_too_large", app: appA, maxSize: true,
		},
		{
			name:   "invalid_app_key is not logged",
			req:    request{contentType: "application/json", header: "pk_unknownunknownunknownunknownun", body: []byte(batchI01)},
			status: http.StatusUnauthorized, code: "invalid_app_key",
		},
		{
			name:   "missing_app_key is not logged",
			req:    request{contentType: "application/json", body: []byte(batchI01)},
			status: http.StatusUnauthorized, code: "missing_app_key",
		},
		{
			name:   "unsupported_media_type is not logged",
			req:    request{contentType: "text/html", header: testKey, body: []byte(batchI01)},
			status: http.StatusUnsupportedMediaType, code: "unsupported_media_type",
		},
		{
			name:   "unsupported_content_encoding is not logged",
			req:    request{contentType: "application/json", encoding: "br", header: testKey, body: []byte(batchI01)},
			status: http.StatusBadRequest, code: "unsupported_content_encoding",
		},
		{
			name:   "storage_unavailable is not logged",
			setup:  func(_ *testing.T, h *pipelineHarness) { h.writer.fail(errStorageDown) },
			req:    request{contentType: "application/json", header: testKey, body: []byte(batchI01)},
			status: http.StatusServiceUnavailable, code: "storage_unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newPipelineHarness(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			if tt.maxSize {
				tt.req.body = paddedBatch(t, ingest.MaxBodyBytes+1)
			}
			rec := h.do(t, tt.req)
			env := requireError(t, rec, tt.status, tt.code)
			logged := h.sink.logged()
			if tt.app == uuid.Nil {
				require.Empty(t, logged)
				return
			}
			require.Len(t, logged, 1)
			r := logged[0]
			require.Equal(t, tt.app, r.AppID)
			require.Equal(t, tt.code, r.Code)
			require.Equal(t, env.Error.Message, r.Message)
			require.Equal(t, tt.field, r.Field)
			require.Empty(t, r.EventName)
			require.Equal(t, receivedAt, r.ReceivedAt)
			require.Equal(t, "pk_", r.KeyPrefix[:3])
			require.Len(t, r.KeyPrefix, 11)
			require.LessOrEqual(t, len(r.PayloadSample), ingest.MaxPayloadSampleBytes)
			require.NotEmpty(t, r.PayloadSample)
			// Ревью MVP: полный ключ из поля тела app_key не хранится — Read API отдаёт сэмпл как
			// example_payload, и ключ с read-scope мог бы получить ключ записи.
			require.NotContains(t, r.PayloadSample, testKey[11:], "полный app_key в сэмпле")
			require.NotContains(t, r.PayloadSample, archivedKey[11:], "полный app_key в сэмпле")
		})
	}
}

// TestRejectedLogger_BatchSampleWithoutIP — сэмпл тела батча при batch-ошибке без context.ip.
func TestRejectedLogger_BatchSampleWithoutIP(t *testing.T) {
	h := newPipelineHarness(t)
	ok, _ := h.limits.Apps.Allow(appA, 4000)
	require.True(t, ok)
	body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"ip": londonIP, "app_version": "3.4.0", "platform": "web"},
		eventJSON(t, map[string]any{"context": map[string]any{"ip": londonIP}}))
	requireError(t, h.postBody(t, body), http.StatusTooManyRequests, "app_rate_limited")
	logged := h.sink.logged()
	require.Len(t, logged, 1)
	require.NotContains(t, logged[0].PayloadSample, londonIP)
	require.Equal(t, "3.4.0", logged[0].AppVersion)
	require.Equal(t, "web", logged[0].Platform)
}

func TestKeyCache_Peek(t *testing.T) {
	h := newKeyHarness(t, ingest.KeyCacheConfig{})
	_, ok := h.cache.Peek(testKey)
	require.False(t, ok, "снимок не загружен")

	h.store.set(keyRow(wk1, testKey, appA, "active"), keyRow(wk2, archivedKey, appB, "archived"))
	require.NoError(t, h.cache.Refresh(t.Context()))
	info, ok := h.cache.Peek(archivedKey)
	require.True(t, ok, "Peek находит ключ архивного приложения")
	require.Equal(t, appB, info.AppID)
	_, ok = h.cache.Peek("pk_unknown")
	require.False(t, ok)

	require.Empty(t, h.store.touchCalls())
	require.NoError(t, h.cache.FlushLastUsed(t.Context()))
	require.Empty(t, h.store.touchCalls(), "Peek не обновляет last_used_at")

	// Снимок старше KeyCacheMaxAge не годится и для журнала: ключи могли быть отозваны, приложение — удалено,
	// и строка rejected_events с его app_id пережила бы purge. Peek молчит, пока снимок не перезагрузится.
	h.clock.Advance(ingest.KeyCacheMaxAge + 1)
	_, ok = h.cache.Peek(testKey)
	require.False(t, ok, "устаревший снимок не используется журналом отказов")
	h.store.set(keyRow(wk1, testKey, appA, "active"))
	require.NoError(t, h.cache.Refresh(t.Context()))
	_, ok = h.cache.Peek(testKey)
	require.True(t, ok)
	_, ok = h.cache.Peek(archivedKey)
	require.False(t, ok, "ключ, пропавший из снимка (отозван), журналу неизвестен")
}

// TestRejectedLogger_BatchSampleRedactsPII — user_id, похожий на email или телефон, не попадает в сэмпл
// тела батча при batch-ошибке (AC-1.5.2 действует и до проверки событий); обычный user_id сохраняется.
func TestRejectedLogger_BatchSampleRedactsPII(t *testing.T) {
	t.Run("json body", func(t *testing.T) {
		h := newPipelineHarness(t)
		ok, _ := h.limits.Apps.Allow(appA, 4000)
		require.True(t, ok)
		body := batchBody(t, "2026-10-13T09:15:02.000Z", nil,
			eventJSON(t, map[string]any{"user_id": "ivan@example.com"}),
			eventJSON(t, map[string]any{"user_id": "+7 (916) 123-45-67"}),
			eventJSON(t, map[string]any{"user_id": "user_42"}))
		requireError(t, h.postBody(t, body), http.StatusTooManyRequests, "app_rate_limited")
		logged := h.sink.logged()
		require.Len(t, logged, 1)
		sample := logged[0].PayloadSample
		require.NotContains(t, sample, "ivan@example.com")
		require.NotContains(t, sample, "916")
		require.Equal(t, 2, strings.Count(sample, `"user_id":"[redacted]"`))
		require.Contains(t, sample, `"user_id":"user_42"`)
	})

	t.Run("invalid_batch", func(t *testing.T) {
		h := newPipelineHarness(t)
		body := []byte(`{"events":[{"name":"session_start","user_id":"ivan@example.com"}]}`)
		rec := h.do(t, request{contentType: "application/json", header: testKey, body: body})
		requireError(t, rec, http.StatusBadRequest, "invalid_batch")
		logged := h.sink.logged()
		require.Len(t, logged, 1)
		require.NotContains(t, logged[0].PayloadSample, "ivan@example.com")
		require.Contains(t, logged[0].PayloadSample, `"user_id":"[redacted]"`)
	})

	t.Run("invalid_json", func(t *testing.T) {
		h := newPipelineHarness(t)
		body := []byte(`{"sent_at":"2026-10-13T09:15:02Z","events":[{"user_id":"ivan@example.com"},{"User_Id": "+79161234567"},{"user_id":"user_42"},{"user_id":"anna@exam`)
		rec := h.do(t, request{contentType: "application/json", header: testKey, body: body})
		requireError(t, rec, http.StatusBadRequest, "invalid_json")
		logged := h.sink.logged()
		require.Len(t, logged, 1)
		sample := logged[0].PayloadSample
		require.NotContains(t, sample, "ivan@example.com")
		require.NotContains(t, sample, "79161234567")
		require.NotContains(t, sample, "anna@exam")
		require.Contains(t, sample, `"user_id":"user_42"`)
	})
}

// TestPipeline_PlatformMissing — AC-004.2 на фейковом хранилище: ключ без платформы у приложения с двумя
// платформами пишет события как other с предупреждением на каждое и ingest_platform_missing_total; журнал
// отказов берёт платформу по той же цепочке.
func TestPipeline_PlatformMissing(t *testing.T) {
	h := newPlatformHarness(t)

	body := batchBody(t, "2026-10-13T09:15:02.000Z", map[string]any{"app_version": "3.4.0"},
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(0)}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(1), "name": "purchase"}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(2)}),
		eventJSON(t, map[string]any{"insert_id": numberedInsertID(3), "context": map[string]any{"platform": "web"}}),
	)
	res := decodeResult(t, h.postBody(t, body))
	require.Equal(t, 3, res.Accepted)
	message := "Платформа не определена: нет context.platform и платформы ключа; событие записано как other"
	require.Equal(t, []ingestapi.IngestWarning{
		{Index: 0, Code: ingestapi.WarningCodePlatformMissing, Message: message},
		{Index: 2, Code: ingestapi.WarningCodePlatformMissing, Message: message},
	}, res.Warnings)
	rows := h.writer.written()
	require.Len(t, rows, 3)
	require.Equal(t, []string{"other", "other", "web"}, []string{rows[0].Platform, rows[1].Platform, rows[2].Platform})
	require.InDelta(t, 2.0, metricValue(t, h.reg, "ingest_platform_missing_total"), 0)
	require.Equal(t, map[string]float64{"platform_missing": 2}, labelCounts(t, h.reg, "ingest_warnings_total", "code"))
	logged := h.sink.logged()
	require.Len(t, logged, 1)
	require.Equal(t, "other", logged[0].Platform, "отказ без платформы журналируется как other")

	res = decodeResult(t, h.do(t, request{contentType: "application/json", header: otherKey, body: oneEvent(t, nil)}))
	require.Equal(t, 1, res.Accepted)
	require.Empty(t, res.Warnings, "платформа ключа — не fallback")
	require.Equal(t, "android", h.writer.written()[3].Platform)
	require.InDelta(t, 2.0, metricValue(t, h.reg, "ingest_platform_missing_total"), 0)
}

func TestRejectedLogger_BatchErrorPlatform(t *testing.T) {
	h := newPlatformHarness(t)
	ok, _ := h.limits.Apps.Allow(appA, 4000)
	require.True(t, ok)
	requireError(t, h.do(t, request{contentType: "application/json", header: otherKey, body: oneEvent(t, nil)}),
		http.StatusTooManyRequests, "app_rate_limited")
	requireError(t, h.postBody(t, oneEvent(t, nil)), http.StatusTooManyRequests, "app_rate_limited")
	logged := h.sink.logged()
	require.Len(t, logged, 2)
	require.Equal(t, "android", logged[0].Platform, "batch-ошибка берёт платформу ключа")
	require.Equal(t, "other", logged[1].Platform, "ключ без платформы у приложения с двумя платформами")
	require.Zero(t, metricValue(t, h.reg, "ingest_platform_missing_total"), "журнал отказов не считает предупреждения")
}

// newPlatformHarness — конвейер с приложением appA (ios+android): testKey без платформы, otherKey — android.
func newPlatformHarness(t *testing.T) *pipelineHarness {
	t.Helper()
	h := newPipelineHarness(t)
	android := sqlc.AppPlatformAndroid
	both := []sqlc.AppPlatform{sqlc.AppPlatformIos, sqlc.AppPlatformAndroid}
	h.store.set(
		sqlc.LoadWriteKeyCacheRow{WriteKeyID: wk1, Key: testKey, AppID: appA, AppStatus: sqlc.AppStatusActive, AppPlatforms: both},
		sqlc.LoadWriteKeyCacheRow{
			WriteKeyID: wk2, Key: otherKey, AppID: appA, AppStatus: sqlc.AppStatusActive,
			KeyPlatform: &android, AppPlatforms: both,
		},
	)
	require.NoError(t, h.cache.Refresh(t.Context()))
	return h
}
