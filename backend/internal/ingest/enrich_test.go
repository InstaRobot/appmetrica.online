package ingest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

const londonIP = "81.2.69.142"

var (
	enrichAppID = uuid.MustParse("0199d690-0000-7000-8000-00000000a001")
	enrichKeyID = uuid.MustParse("0199d690-0000-7000-8000-00000000b001")
	london      = geo.Location{Country: "GB", Region: "England", City: "London"}
)

func openGeo(t *testing.T) *geo.Reader {
	t.Helper()
	r, err := geo.Open(testutil.GeoDBPath())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close()) })
	return r
}

// enrichPipeline — Processor для тестов обогащения: валидация context и событий, коррекция времени,
// обогащение и строки ClickHouse так, как их соберёт конвейер ingest. Принятые строки и payload_sample
// отклонённых событий запоминаются; fail — ошибка вместо обработки (чтобы хендлер записал лог).
type enrichPipeline struct {
	enricher ingest.Enricher

	mu      sync.Mutex
	rows    []chstore.EventRow
	samples []string
	fail    error
}

func (p *enrichPipeline) Process(_ context.Context, b *ingest.Batch) (ingestapi.IngestResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail != nil {
		return ingestapi.IngestResult{}, fmt.Errorf("pipeline: %w", p.fail)
	}
	b.Key = ingest.KeyInfo{WriteKeyID: enrichKeyID, AppID: enrichAppID, AppStatus: sqlc.AppStatusActive, AppPlatform: sqlc.AppPlatformIos}
	res := ingestapi.IngestResult{ServerTime: b.ReceivedAt}
	batchCtx, rejected := ingest.ValidateBatchContext(b)
	if len(rejected) > 0 {
		res.Rejected = rejected
		return res, nil
	}
	for i, raw := range b.Events {
		ev, rej := ingest.ValidateEvent(i, raw)
		if rej == nil {
			_, rej = ingest.CorrectEventTime(ev, b.ReceivedAt, b.SentAt, ingest.DefaultTimeConfig)
		}
		if rej != nil {
			res.Rejected = append(res.Rejected, *rej)
			p.samples = append(p.samples, ingest.PayloadSample(raw))
			continue
		}
		warning, write := p.enricher.Enrich(b, batchCtx, ev)
		if warning != nil {
			res.Warnings = append(res.Warnings, *warning)
		}
		res.Accepted++
		if write {
			p.rows = append(p.rows, ingest.NewEventRow(b, ev))
		}
	}
	return res, nil
}

func newEnrichHandler(t *testing.T, p *enrichPipeline, logs *bytes.Buffer) http.Handler {
	t.Helper()
	metrics, err := ingest.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	c := clock.NewFixed(receivedAt)
	return httpx.Chain(ingest.NewHandler(ingest.HandlerConfig{
		Processor: p,
		Clock:     c,
		Logger:    slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Metrics:   metrics,
	}), httpx.RequestID(c))
}

func postEvents(t *testing.T, h http.Handler, body, remoteAddr, forwardedFor string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Key", testKey)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// batchI01WithRejected — I-01 плюс третье событие с неизвестным именем (уходит в rejected и payload_sample).
func batchI01WithRejected() string {
	bad := `{"insert_id":"0199d6a4-0000-7000-8000-000000000003","name":"purchase","timestamp":"2026-10-13T09:15:00Z",` +
		`"install_id":"6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21","session_id":"0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f"}`
	return strings.Replace(batchI01, "\n  ]\n}", ",\n    "+bad+"\n  ]\n}", 1)
}

func TestEnrich_GeoFromForwardedFor_NoIPPersisted(t *testing.T) {
	logs := &bytes.Buffer{}
	p := &enrichPipeline{enricher: ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true}}
	h := newEnrichHandler(t, p, logs)

	rec := postEvents(t, h, batchI01WithRejected(), "192.0.2.10:52311", "10.0.0.1, "+londonIP)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var res ingestapi.IngestResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Equal(t, 2, res.Accepted)
	require.Len(t, res.Rejected, 1)
	require.Empty(t, res.Warnings)

	require.Len(t, p.rows, 2)
	for _, row := range p.rows {
		require.Equal(t, london, geo.Location{Country: row.Country, Region: row.Region, City: row.City})
	}
	require.Len(t, p.samples, 1)

	// Ошибка конвейера — чтобы в логе хендлера были строки по запросу с этим IP.
	p.fail = errors.New("clickhouse down")
	rec2 := postEvents(t, h, batchI01, "192.0.2.10:52311", "10.0.0.1, "+londonIP)
	require.Equal(t, http.StatusInternalServerError, rec2.Code)
	require.NotEmpty(t, logs.String())

	rows, err := json.Marshal(p.rows)
	require.NoError(t, err)
	for name, s := range map[string]string{
		"events (json)":  string(rows),
		"events (%+v)":   fmt.Sprintf("%+v", p.rows),
		"payload_sample": strings.Join(p.samples, "\n"),
		"slog":           logs.String(),
		"response":       rec.Body.String() + rec2.Body.String(),
	} {
		require.NotContains(t, s, londonIP, name)
	}
}

func TestEnrich_ClientIPFromRemoteAddr(t *testing.T) {
	p := &enrichPipeline{enricher: ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true}}
	h := newEnrichHandler(t, p, &bytes.Buffer{})

	require.Equal(t, http.StatusOK, postEvents(t, h, batchI01, londonIP+":52311", "").Code)
	require.Equal(t, "GB", p.rows[0].Country)

	// Невалидный X-Forwarded-For: адрес берётся из RemoteAddr.
	require.Equal(t, http.StatusOK, postEvents(t, h, batchI01, "[2a02:6b8::1]:443", "garbage").Code)
	require.Equal(t, "RU", p.rows[2].Country)
}

// enrichEvent валидирует batchCtx и событие с evCtx (nil — без context) и обогащает его.
func enrichEvent(t *testing.T, e ingest.Enricher, clientIP string, batchCtx, evCtx map[string]any) (
	*ingest.Event, *ingestapi.IngestWarning, bool,
) {
	t.Helper()
	overrides := map[string]any{}
	if evCtx != nil {
		overrides["context"] = evCtx
	}
	raw := eventJSON(t, overrides)
	b := batchWithContext(t, batchCtx, raw)
	b.Key = ingest.KeyInfo{WriteKeyID: enrichKeyID, AppID: enrichAppID, AppPlatform: sqlc.AppPlatformIos}
	if clientIP != "" {
		b.ClientIP = netip.MustParseAddr(clientIP)
	}
	fields, rejected := ingest.ValidateBatchContext(b)
	require.Empty(t, rejected)
	ev, rej := ingest.ValidateEvent(0, raw)
	require.Nil(t, rej)
	warning, write := e.Enrich(b, fields, ev)
	return ev, warning, write
}

func location(ev *ingest.Event) geo.Location {
	return geo.Location{Country: ev.Resolved.Country, Region: ev.Resolved.Region, City: ev.Resolved.City}
}

func TestEnrich_ContextIPOverride(t *testing.T) {
	e := ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true}

	t.Run("context_ip_wins_over_client", func(t *testing.T) {
		ev, warning, write := enrichEvent(t, e, "2a02:6b8::1", nil, map[string]any{"ip": londonIP})
		require.Nil(t, warning)
		require.True(t, write)
		require.Equal(t, london, location(ev))
	})

	t.Run("none_disables_geo", func(t *testing.T) {
		ev, warning, _ := enrichEvent(t, e, londonIP, nil, map[string]any{"ip": "none"})
		require.Nil(t, warning)
		require.Equal(t, geo.Location{}, location(ev))
	})

	t.Run("invalid_ip_warns", func(t *testing.T) {
		for _, ip := range []string{"999.1.1.1", "", "localhost", " 81.2.69.142"} {
			ev, warning, write := enrichEvent(t, e, londonIP, nil, map[string]any{"ip": ip})
			require.True(t, write, "событие принято: %q", ip)
			require.Equal(t, geo.Location{}, location(ev), ip)
			require.NotNil(t, warning, ip)
			require.Equal(t, ingestapi.IngestWarning{
				Index: 0, Code: ingestapi.WarningCodeGeoIpInvalid,
				Message: "context.ip не является IP-адресом; гео не определено.",
			}, *warning)
		}
	})

	t.Run("ipv6_context_ip", func(t *testing.T) {
		ev, warning, _ := enrichEvent(t, e, "", nil, map[string]any{"ip": "2a02:6b8::1"})
		require.Nil(t, warning)
		require.Equal(t, "RU", ev.Resolved.Country)
	})

	t.Run("private_context_ip_empty_without_warning", func(t *testing.T) {
		ev, warning, _ := enrichEvent(t, e, londonIP, nil, map[string]any{"ip": "10.0.0.1"})
		require.Nil(t, warning)
		require.Equal(t, geo.Location{}, location(ev))
	})

	t.Run("event_ip_overrides_batch_none", func(t *testing.T) {
		ev, _, _ := enrichEvent(t, e, "", map[string]any{"ip": "none"}, map[string]any{"ip": londonIP})
		require.Equal(t, london, location(ev))
	})

	t.Run("batch_ip_applies_to_events", func(t *testing.T) {
		ev, _, _ := enrichEvent(t, e, "2a02:6b8::1", map[string]any{"ip": londonIP}, nil)
		require.Equal(t, london, location(ev))
	})

	t.Run("no_ip_anywhere", func(t *testing.T) {
		ev, warning, _ := enrichEvent(t, e, "", nil, nil)
		require.Nil(t, warning)
		require.Equal(t, geo.Location{}, location(ev))
	})

	t.Run("payload_sample_drops_ip", func(t *testing.T) {
		raw := eventJSON(t, map[string]any{"name": "purchase", "context": map[string]any{"ip": londonIP, "app_version": "1.0"}})
		sample := ingest.PayloadSample(raw)
		require.NotContains(t, sample, londonIP)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(sample), &got))
		require.Equal(t, map[string]any{"app_version": "1.0"}, got["context"])
		require.Equal(t, "purchase", got["name"])
	})
}

func TestEnrich_ContextMerge(t *testing.T) {
	e := ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true}
	batchCtx := map[string]any{"app_version": "3.4.0", "os_name": "iOS", "os_version": "26.0.1", "locale": "ru-RU"}

	t.Run("event_overrides_batch_field_by_field", func(t *testing.T) {
		ev, _, write := enrichEvent(t, e, "", batchCtx, map[string]any{"app_version": "3.5.0"})
		require.True(t, write)
		require.Equal(t, ingest.ResolvedContext{
			Platform: "ios", OSName: "iOS", OSVersion: "26.0.1", AppVersion: "3.5.0", Locale: "ru-RU",
			Environment: "production",
		}, ev.Resolved)
	})

	t.Run("development_from_event_and_defaults", func(t *testing.T) {
		ev, _, _ := enrichEvent(t, e, "", map[string]any{"app_version": "2.4.0"},
			map[string]any{"app_version": "2.5.0", "environment": "development"})
		require.Equal(t, "2.5.0", ev.Resolved.AppVersion)
		require.Equal(t, "development", ev.Resolved.Environment)
		require.Equal(t, "ios", ev.Resolved.Platform)

		ev, _, _ = enrichEvent(t, e, "", map[string]any{"app_version": "2.4.0"}, nil)
		require.Equal(t, "2.4.0", ev.Resolved.AppVersion)
		require.Equal(t, "production", ev.Resolved.Environment)
		require.Equal(t, "ios", ev.Resolved.Platform)
	})

	t.Run("platform_precedence", func(t *testing.T) {
		ev, _, _ := enrichEvent(t, e, "", map[string]any{"platform": "web"}, nil)
		require.Equal(t, "web", ev.Resolved.Platform, "батч перекрывает apps.platform")
		ev, _, _ = enrichEvent(t, e, "", map[string]any{"platform": "web"}, map[string]any{"platform": "android"})
		require.Equal(t, "android", ev.Resolved.Platform, "событие перекрывает батч")
	})

	t.Run("all_fields", func(t *testing.T) {
		ev, _, _ := enrichEvent(t, e, londonIP, map[string]any{
			"platform": "macos", "os_name": "macOS", "os_version": "26.1", "app_version": "1.2.3", "app_build": "77",
			"device_model": "Mac16,1", "device_type": "desktop", "locale": "en-GB", "environment": "development",
		}, map[string]any{"device_type": "tablet", "app_build": ""})
		require.Equal(t, ingest.ResolvedContext{
			Platform: "macos", OSName: "macOS", OSVersion: "26.1", AppVersion: "1.2.3", AppBuild: "",
			DeviceModel: "Mac16,1", DeviceType: "tablet", Locale: "en-GB", Environment: "development",
			Country: "GB", Region: "England", City: "London",
		}, ev.Resolved, "пустая строка события тоже перекрывает батч")
	})
}

func TestMergeContext(t *testing.T) {
	batch := ingest.ContextFields{"app_version": "3.4.0", "platform": "ios"}
	event := ingest.ContextFields{"app_version": "3.5.0", "locale": "ru-RU"}
	require.Equal(t, ingest.ContextFields{"app_version": "3.5.0", "platform": "ios", "locale": "ru-RU"},
		ingest.MergeContext(batch, event))
	require.Equal(t, ingest.ContextFields{"app_version": "3.4.0", "platform": "ios"}, batch, "батч не меняется")
	require.Empty(t, ingest.MergeContext(nil, nil))
	require.Equal(t, event, ingest.MergeContext(nil, event))
}

func TestEnrich_DevelopmentKillSwitch(t *testing.T) {
	g := openGeo(t)
	dev := map[string]any{"environment": "development"}

	_, warning, write := enrichEvent(t, ingest.Enricher{Geo: g, AcceptDevelopment: false}, "", nil, dev)
	require.Nil(t, warning)
	require.False(t, write, "development при INGEST_ACCEPT_DEVELOPMENT=false не пишется")

	_, _, write = enrichEvent(t, ingest.Enricher{Geo: g, AcceptDevelopment: false}, "", nil, nil)
	require.True(t, write, "production не затрагивается")

	_, _, write = enrichEvent(t, ingest.Enricher{Geo: g, AcceptDevelopment: true}, "", nil, dev)
	require.True(t, write)
}

func TestNewEventRow(t *testing.T) {
	e := ingest.Enricher{Geo: openGeo(t), AcceptDevelopment: true}
	raw := eventJSON(t, map[string]any{
		"name": "session_end", "duration_ms": 183986, "timestamp": "2026-10-13T02:30:00+03:00",
		"context": map[string]any{"app_version": "3.5.0"},
	})
	b := batchWithContext(t, map[string]any{"os_name": "iOS"}, raw)
	b.Key = ingest.KeyInfo{WriteKeyID: enrichKeyID, AppID: enrichAppID, AppPlatform: sqlc.AppPlatformIos}
	b.ClientIP = netip.MustParseAddr(londonIP)
	b.SentAt = time.Date(2026, 10, 12, 23, 30, 1, 0, time.UTC)
	b.ReceivedAt = time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC)
	fields, rejected := ingest.ValidateBatchContext(b)
	require.Empty(t, rejected)
	ev, rej := ingest.ValidateEvent(0, raw)
	require.Nil(t, rej)
	_, rej = ingest.CorrectEventTime(ev, b.ReceivedAt, b.SentAt, ingest.DefaultTimeConfig)
	require.Nil(t, rej)
	_, write := e.Enrich(b, fields, ev)
	require.True(t, write)

	row := ingest.NewEventRow(b, ev)
	require.Equal(t, chstore.EventRow{
		AppID:       enrichAppID,
		EventDate:   time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		EventTime:   time.Date(2026, 10, 12, 23, 29, 59, 0, time.UTC),
		ClientTime:  ev.Timestamp,
		SentAt:      b.SentAt,
		ReceivedAt:  b.ReceivedAt,
		InsertID:    uuid.MustParse(validInsertID),
		Name:        "session_end",
		InstallID:   uuid.MustParse(validInstallID),
		UserID:      "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
		SessionID:   uuid.MustParse(validSessionID),
		DurationMs:  183986,
		Environment: "production",
		Platform:    "ios",
		OSName:      "iOS",
		AppVersion:  "3.5.0",
		Country:     "GB",
		Region:      "England",
		City:        "London",
		WriteKeyID:  enrichKeyID,
	}, row)
}

func TestPayloadSample(t *testing.T) {
	t.Run("batch_drops_every_context_ip", func(t *testing.T) {
		body := `{"sent_at":"2026-10-13T09:15:02Z","context":{"ip":"` + londonIP + `","platform":"server"},` +
			`"events":[{"name":"x","context":{"ip":"2a02:6b8::1","locale":"ru"}},{"name":"y"},"z"]}`
		sample := ingest.PayloadSample([]byte(body))
		require.JSONEq(t, `{"sent_at":"2026-10-13T09:15:02Z","context":{"platform":"server"},`+
			`"events":[{"name":"x","context":{"locale":"ru"}},{"name":"y"},"z"]}`, sample)
	})

	t.Run("event_without_context_unchanged", func(t *testing.T) {
		raw := eventJSON(t, nil)
		require.JSONEq(t, string(raw), ingest.PayloadSample(raw))
	})

	t.Run("non_object_json_kept", func(t *testing.T) {
		require.Equal(t, `[1,2]`, ingest.PayloadSample([]byte(`[1,2]`)))
		require.JSONEq(t, `{"context":"ios"}`, ingest.PayloadSample([]byte(`{"context":"ios"}`)))
	})

	t.Run("truncated_to_4096_bytes_on_rune_boundary", func(t *testing.T) {
		raw := eventJSON(t, map[string]any{"user_id": strings.Repeat("я", 5000)})
		sample := ingest.PayloadSample(raw)
		require.LessOrEqual(t, len(sample), ingest.MaxPayloadSampleBytes)
		require.Greater(t, len(sample), ingest.MaxPayloadSampleBytes-4)
		require.True(t, utf8.ValidString(sample))
	})

	t.Run("invalid_json_ip_scrubbed", func(t *testing.T) {
		for _, body := range []string{
			`{"context":{"ip": "` + londonIP + `", "platform":"server"}, "events": [`,
			`{"events":[{"context":{"ip":"` + londonIP,
			`{"context":{"ip":"81.2.69.\"142"`,
		} {
			sample := ingest.PayloadSample([]byte(body))
			require.NotContains(t, sample, "81.2.69", body)
			require.True(t, strings.HasPrefix(sample, `{"`), sample)
		}
	})
}
