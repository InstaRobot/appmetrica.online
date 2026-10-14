//go:build integration

package chstore_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Статус системы читает api пользователем appmetrica_ro: гранты на system.disks и system.parts из users.d.
func TestSystemStats_ReadOnlyUser(t *testing.T) {
	ro := openStore(t, roUser, roPassword)
	rw := openStore(t, rwUser, rwPassword)

	pct, err := ro.DiskFreePct(t.Context())
	require.NoError(t, err)
	require.Greater(t, pct, 0.0)
	require.LessOrEqual(t, pct, 100.0)

	before, err := ro.EventsRows(t.Context())
	require.NoError(t, err)
	require.GreaterOrEqual(t, before, int64(0))

	for range 3 {
		require.NoError(t, rw.Conn().Exec(t.Context(), insertMinimalEvent, uuid.New()))
	}
	after, err := ro.EventsRows(t.Context())
	require.NoError(t, err)
	require.Equal(t, before+3, after)
}
