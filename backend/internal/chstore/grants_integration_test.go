//go:build integration

package chstore_test

import (
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// codeAccessDenied — ClickHouse ACCESS_DENIED: у пользователя нет гранта на операцию.
const codeAccessDenied = 497

func requireCode(t *testing.T, err error, code int32) {
	t.Helper()
	require.Error(t, err)
	var ex *clickhouse.Exception
	require.ErrorAs(t, err, &ex)
	require.Equal(t, code, ex.Code, ex.Message)
}

const insertMinimalEvent = "INSERT INTO appmetrica.events (app_id, event_date, name) VALUES (?, today(), 'session_start')"

func TestClickHouseGrants(t *testing.T) {
	ro := openStore(t, roUser, roPassword).Conn()
	rw := openStore(t, rwUser, rwPassword).Conn()
	admin := adminConn(t)

	t.Run("ro cannot insert", func(t *testing.T) {
		appID := uuid.New()
		requireCode(t, ro.Exec(t.Context(), insertMinimalEvent, appID), codeAccessDenied)
		require.Equal(t, uint64(0), countEvents(t, admin, appID))
	})

	t.Run("ro cannot delete", func(t *testing.T) {
		appID := uuid.New()
		require.NoError(t, rw.Exec(t.Context(), insertMinimalEvent, appID))
		requireCode(t, ro.Exec(t.Context(), "DELETE FROM appmetrica.events WHERE app_id = ?", appID), codeAccessDenied)
		require.Equal(t, uint64(1), countEvents(t, admin, appID))
	})

	t.Run("ro can select", func(t *testing.T) {
		for _, table := range []string{"events", "install_first_seen", "user_first_seen", "identity_links", "rejected_events", "daily_stats"} {
			var n uint64
			require.NoError(t, ro.QueryRow(t.Context(), "SELECT count() FROM appmetrica."+table).Scan(&n), table)
		}
	})

	t.Run("rw inserts and deletes", func(t *testing.T) {
		appID := uuid.New()
		require.NoError(t, rw.Exec(t.Context(), insertMinimalEvent, appID))
		require.Equal(t, uint64(1), countEvents(t, admin, appID))

		// lightweight delete синхронный (lightweight_deletes_sync=2 в профиле appmetrica_rw).
		require.NoError(t, rw.Exec(t.Context(), "DELETE FROM appmetrica.events WHERE app_id = ?", appID))
		require.Equal(t, uint64(0), countEvents(t, admin, appID))
	})

	// job deletion перед удалением сбрасывает очередь async insert сервера (deletion.chPurger).
	t.Run("rw flushes async insert queue", func(t *testing.T) {
		require.NoError(t, rw.Exec(t.Context(), "SYSTEM FLUSH ASYNC INSERT QUEUE"))
		require.Error(t, ro.Exec(t.Context(), "SYSTEM FLUSH ASYNC INSERT QUEUE"))
	})

	t.Run("rw and ro have no DDL", func(t *testing.T) {
		ctx := t.Context()
		requireCode(t, rw.Exec(ctx, "CREATE TABLE appmetrica.rw_probe (x UInt8) ENGINE = Memory"), codeAccessDenied)
		requireCode(t, rw.Exec(ctx, "DROP TABLE appmetrica.daily_stats"), codeAccessDenied)
		requireCode(t, ro.Exec(ctx, "TRUNCATE TABLE appmetrica.events"), codeAccessDenied)
		var exists uint8
		require.NoError(t, admin.QueryRow(t.Context(), "EXISTS TABLE appmetrica.daily_stats").Scan(&exists))
		require.Equal(t, uint8(1), exists)
	})
}
