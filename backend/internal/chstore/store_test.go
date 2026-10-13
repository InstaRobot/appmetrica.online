package chstore_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

// Пустой батч не обращается к ClickHouse: Store без соединения не паникует.
func TestInsert_EmptyBatchSkipsClickHouse(t *testing.T) {
	var s chstore.Store
	require.NoError(t, s.InsertEvents(t.Context(), nil))
	require.NoError(t, s.InsertRejected(t.Context(), []chstore.RejectedRow{}))
}
