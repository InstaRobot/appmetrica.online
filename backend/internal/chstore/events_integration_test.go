//go:build integration

package chstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

func TestEvents_ReplacingMergeTreeDedup(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)
	admin := adminConn(t)

	appID := uuid.New()
	first := eventRow(appID, uuid.New(), base)
	retry := first
	retry.ReceivedAt = first.ReceivedAt.Add(time.Minute)
	retry.Country = "DE"

	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{first}))
	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{retry}))
	// До merge дубль виден: async_insert_deduplicate=0, схлопывает только ReplacingMergeTree.
	require.Equal(t, uint64(2), countEvents(t, admin, appID))

	require.NoError(t, admin.Exec(ctx, "OPTIMIZE TABLE appmetrica.events FINAL"))
	require.Equal(t, uint64(1), countEvents(t, admin, appID))

	// Версия строки — received_at: остаётся последняя вставка.
	var country string
	require.NoError(t, admin.QueryRow(ctx,
		"SELECT country FROM appmetrica.events WHERE app_id = ?", appID).Scan(&country))
	require.Equal(t, "DE", country)
}

func TestEvents_DifferentInsertIDsAreNotMerged(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)
	admin := adminConn(t)

	appID, installID := uuid.New(), uuid.New()
	start := eventRow(appID, installID, base)
	end := eventRow(appID, installID, base.Add(time.Minute))
	end.Name, end.SessionID, end.DurationMs = "session_end", start.SessionID, 60_000

	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{start, end}))
	require.NoError(t, admin.Exec(ctx, "OPTIMIZE TABLE appmetrica.events FINAL"))
	require.Equal(t, uint64(2), countEvents(t, admin, appID))
}

func TestInsertEvents_AckMeansVisible(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)
	ro := openStore(t, roUser, roPassword)

	appID, installID := uuid.New(), uuid.New()
	start := eventRow(appID, installID, base)
	start.UserID = "user_8f3a9c"
	end := eventRow(appID, installID, base.Add(42*time.Second))
	end.Name, end.SessionID, end.DurationMs = "session_end", start.SessionID, 42_000

	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{start, end}))
	// Без ожидания и без FINAL: nil от InsertEvents означает, что данные уже в таблице.
	require.Equal(t, uint64(2), countEvents(t, ro.Conn(), appID))

	var got []chstore.EventRow
	require.NoError(t, ro.Conn().Select(ctx, &got,
		"SELECT * FROM appmetrica.events WHERE app_id = ? ORDER BY event_time", appID))
	require.Len(t, got, 2)
	for i, want := range []chstore.EventRow{start, end} {
		g := got[i]
		require.Equal(t, want.InsertID, g.InsertID)
		require.Equal(t, want.Name, g.Name)
		require.Equal(t, want.UserID, g.UserID)
		require.Equal(t, want.DurationMs, g.DurationMs)
		require.Equal(t, want.WriteKeyID, g.WriteKeyID)
		require.Equal(t, want.City, g.City)
		require.True(t, want.EventTime.Equal(g.EventTime), "event_time с миллисекундами: %v != %v", want.EventTime, g.EventTime)
		require.True(t, want.ClientTime.Equal(g.ClientTime))
		require.True(t, want.EventDate.Equal(g.EventDate.UTC()), "event_date: %v != %v", want.EventDate, g.EventDate)
	}
}

func TestInsertEvents_EmptyBatchIsNoop(t *testing.T) {
	rw := openStore(t, rwUser, rwPassword)
	require.NoError(t, rw.InsertEvents(t.Context(), nil))
	require.NoError(t, rw.InsertRejected(t.Context(), []chstore.RejectedRow{}))
}

func TestInsertEvents_CanceledContext(t *testing.T) {
	rw := openStore(t, rwUser, rwPassword)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	appID := uuid.New()
	err := rw.InsertEvents(ctx, []chstore.EventRow{eventRow(appID, uuid.New(), base)})
	require.Error(t, err)
	require.Equal(t, uint64(0), countEvents(t, adminConn(t), appID))
}

func TestInsertEvents_ReadOnlyUserFails(t *testing.T) {
	ro := openStore(t, roUser, roPassword)
	appID := uuid.New()
	err := ro.InsertEvents(t.Context(), []chstore.EventRow{eventRow(appID, uuid.New(), base)})
	requireCode(t, err, codeAccessDenied)
	require.Equal(t, uint64(0), countEvents(t, adminConn(t), appID))
}

func TestInsertRejected_RoundTrip(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)

	appID := uuid.New()
	rows := []chstore.RejectedRow{
		{
			ReceivedAt: base, AppID: appID, KeyPrefix: "pk_live_abc", Code: "user_id_looks_like_pii",
			Field: "events[1].user_id", Message: "user_id похож на email или телефон", EventName: "session_start",
			AppVersion: "1.0.0", Platform: "ios", PayloadSample: `{"name":"session_start"}`,
		},
		{ReceivedAt: base.Add(time.Second), AppID: appID, Code: "invalid_json", Message: "тело не является JSON"},
	}
	require.NoError(t, rw.InsertRejected(ctx, rows))

	var got []chstore.RejectedRow
	require.NoError(t, adminConn(t).Select(ctx, &got,
		"SELECT * FROM appmetrica.rejected_events WHERE app_id = ? ORDER BY received_at", appID))
	require.Len(t, got, 2)
	require.Equal(t, rows[0].Code, got[0].Code)
	require.Equal(t, rows[0].Field, got[0].Field)
	require.Equal(t, rows[0].PayloadSample, got[0].PayloadSample)
	require.True(t, rows[0].ReceivedAt.Equal(got[0].ReceivedAt))
	require.Equal(t, "invalid_json", got[1].Code)
	require.Empty(t, got[1].EventName)
}

func TestOpen_WrongPasswordDoesNotLeakIt(t *testing.T) {
	const secret = "wrong-secret-password-4711"
	_, err := chstore.Open(t.Context(), options(rwUser, secret))
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
}

func TestOpen_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	const secret = "unreachable-secret-password"
	_, err := chstore.Open(ctx, chstore.Options{Addr: "127.0.0.1:1", Database: database, User: rwUser, Password: secret})
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), secret))
}

func TestStore_Ping(t *testing.T) {
	require.NoError(t, openStore(t, roUser, roPassword).Ping(t.Context()))
}
