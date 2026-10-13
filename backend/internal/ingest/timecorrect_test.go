package ingest_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	require.NoError(t, err)
	return ts
}

func TestCorrectTime(t *testing.T) {
	cfg := ingest.DefaultTimeConfig
	t0 := receivedAt // 2026-10-13T09:15:02Z

	tests := []struct {
		name        string
		sentAt, ts  string
		wantTime    time.Time
		wantClamped bool
		wantTooOld  bool
	}{
		{
			// I-02: часы устройства спешат на 3 ч, событие за минуту до отправки.
			name: "device_clock_ahead", sentAt: "2026-10-13T12:15:02.000Z", ts: "2026-10-13T12:14:02.000Z",
			wantTime: mustTime(t, "2026-10-13T09:14:02.000Z"),
		},
		{
			// AC-1.2.1: спешат на 3 ч, событие за 10 с до отправки; offset не в UTC.
			name: "device_clock_ahead_with_offset", sentAt: "2026-10-13T15:15:02+03:00", ts: "2026-10-13T15:14:52+03:00",
			wantTime: t0.Add(-10 * time.Second),
		},
		{
			name: "device_clock_1970", sentAt: "1970-01-01T00:10:00Z", ts: "1970-01-01T00:09:00Z",
			wantTime: t0.Add(-time.Minute),
		},
		{
			// I-06: ровно 168 ч назад — граница включительно.
			name: "exactly_7_days", sentAt: "2026-10-13T09:15:02.000Z", ts: "2026-10-06T09:15:02.000Z",
			wantTime: t0.Add(-168 * time.Hour),
		},
		{
			// I-03: +25 мин после коррекции → clamp к received_at.
			name: "future_clamped", sentAt: "2026-10-13T09:15:02.000Z", ts: "2026-10-13T09:40:02.000Z",
			wantTime: t0, wantClamped: true,
		},
		{
			// I-04: +9 мин 59 с — в пределах допуска, без clamp.
			name: "future_within_tolerance", sentAt: "2026-10-13T09:15:02.000Z", ts: "2026-10-13T09:25:01.000Z",
			wantTime: t0.Add(9*time.Minute + 59*time.Second),
		},
		{
			name: "future_exactly_tolerance", sentAt: "2026-10-13T09:15:02.000Z", ts: "2026-10-13T09:25:02.000Z",
			wantTime: t0.Add(10 * time.Minute),
		},
		{
			name: "future_tolerance_plus_1ms", sentAt: "2026-10-13T09:15:02.000Z", ts: "2026-10-13T09:25:02.001Z",
			wantTime: t0, wantClamped: true,
		},
		{
			// I-05: 168 ч + 1 мс → event_too_old.
			name: "late_window_boundary", sentAt: "2026-10-13T09:15:02.000Z", ts: "2026-10-06T09:15:01.999Z",
			wantTime: mustTime(t, "2026-10-06T09:15:01.999Z"), wantTooOld: true,
		},
		{
			// AC-1.2.3: досыл очереди 6 дн 23 ч принимается.
			name: "late_queue_167h", sentAt: "2026-10-13T09:15:02Z", ts: "2026-10-06T10:15:02Z",
			wantTime: t0.Add(-167 * time.Hour),
		},
		{
			name: "far_past_saturates", sentAt: "9999-12-31T23:59:59Z", ts: "0001-01-01T00:00:00Z",
			wantTime: t0.Add(minDuration), wantTooOld: true,
		},
		{
			name: "far_future_saturates", sentAt: "0001-01-01T00:00:00Z", ts: "9999-12-31T23:59:59Z",
			wantTime: t0, wantClamped: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, clamped, tooOld := ingest.CorrectTime(t0, mustTime(t, tc.sentAt), mustTime(t, tc.ts), cfg)
			require.True(t, tc.wantTime.Equal(got), "event_time: want %s, got %s", tc.wantTime, got)
			require.Equal(t, tc.wantClamped, clamped, "clamped")
			require.Equal(t, tc.wantTooOld, tooOld, "tooOld")
		})
	}
}

// minDuration — нижняя граница time.Duration: time.Time.Sub насыщается на ней.
const minDuration = time.Duration(-1 << 63)

func TestCorrectTime_CustomConfig(t *testing.T) {
	cfg := ingest.TimeConfig{FutureTolerance: time.Minute, LateWindow: time.Hour}
	sent := receivedAt

	_, clamped, _ := ingest.CorrectTime(receivedAt, sent, sent.Add(2*time.Minute), cfg)
	require.True(t, clamped)

	_, _, tooOld := ingest.CorrectTime(receivedAt, sent, sent.Add(-time.Hour-time.Millisecond), cfg)
	require.True(t, tooOld)

	_, _, tooOld = ingest.CorrectTime(receivedAt, sent, sent.Add(-time.Hour), cfg)
	require.False(t, tooOld)
}

func TestCorrectEventTime(t *testing.T) {
	cfg := ingest.DefaultTimeConfig
	sent := mustTime(t, "2026-10-13T09:15:02.000Z")
	insertID := "0199d6c1-0000-7000-8000-000000000004"

	validated := func(t *testing.T, index int, ts string) *ingest.Event {
		t.Helper()
		ev, rej := ingest.ValidateEvent(index, eventJSON(t, map[string]any{"insert_id": insertID, "timestamp": ts}))
		require.Nil(t, rej)
		return ev
	}

	t.Run("accepted", func(t *testing.T) {
		ev := validated(t, 0, "2026-10-13T09:14:02.000Z")
		warn, rej := ingest.CorrectEventTime(ev, receivedAt, sent, cfg)
		require.Nil(t, warn)
		require.Nil(t, rej)
		require.True(t, receivedAt.Add(-time.Minute).Equal(ev.EventTime))
	})

	t.Run("future_clamped", func(t *testing.T) {
		ev := validated(t, 2, "2026-10-13T09:40:02.000Z")
		warn, rej := ingest.CorrectEventTime(ev, receivedAt, sent, cfg)
		require.Nil(t, rej)
		require.NotNil(t, warn)
		require.Equal(t, ingestapi.WarningCodeTimestampInFutureClamped, warn.Code)
		require.Equal(t, 2, warn.Index)
		require.NotEmpty(t, warn.Message)
		require.True(t, receivedAt.Equal(ev.EventTime))
	})

	t.Run("late_window_boundary", func(t *testing.T) {
		ev := validated(t, 4, "2026-10-06T09:15:01.999Z")
		warn, rej := ingest.CorrectEventTime(ev, receivedAt, sent, cfg)
		require.Nil(t, warn)
		require.NotNil(t, rej)
		require.Equal(t, ingestapi.RejectionCodeEventTooOld, rej.Code)
		require.Equal(t, "events[4].timestamp", rej.Field)
		require.Equal(t, 4, rej.Index)
		require.False(t, rej.Retryable)
		require.Equal(t, insertID, rej.InsertId.MustGet())
		require.NotEmpty(t, rej.Message)
	})
}

func BenchmarkCorrectTime(b *testing.B) {
	sent := receivedAt.Add(3 * time.Hour)
	ts := sent.Add(-10 * time.Second)
	for b.Loop() {
		ingest.CorrectTime(receivedAt, sent, ts, ingest.DefaultTimeConfig)
	}
}
