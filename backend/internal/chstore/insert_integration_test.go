//go:build integration

package chstore_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

// AC-002.2: свойства события пишутся в events как есть и читаются обратно.
func TestInsertEvents_Properties(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)
	admin := adminConn(t)

	appID, installID := uuid.New(), uuid.New()
	withProps := eventRow(appID, installID, base)
	withProps.Name = "purchase"
	withProps.Properties = map[string]string{"plan": "12", "paid": "true", "note": "кофе"}
	without := eventRow(appID, installID, base)

	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{withProps, without}))

	got := map[uuid.UUID]map[string]string{}
	rows, err := admin.Query(ctx, "SELECT insert_id, properties FROM appmetrica.events WHERE app_id = ?", appID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id uuid.UUID
		var props map[string]string
		require.NoError(t, rows.Scan(&id, &props))
		got[id] = props
	}
	require.NoError(t, rows.Err())
	require.Equal(t, map[string]string{"plan": "12", "paid": "true", "note": "кофе"}, got[withProps.InsertID])
	require.Empty(t, got[without.InsertID], "nil Properties пишется как {}")
}
