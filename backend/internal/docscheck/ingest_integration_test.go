//go:build integration

package docscheck_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

// newIngest — ingest поверх настоящих Postgres и ClickHouse (testcontainers): приложение платформы server
// с write-ключом DEV_WRITE_KEY, как после `make seed`.
func newIngest(t *testing.T) (http.Handler, func(t *testing.T, accepted int)) {
	t.Helper()
	ctx := t.Context()
	env := testutil.Start(t)
	q := env.PG.Queries()
	app, err := q.CreateApp(ctx, sqlc.CreateAppParams{Name: "Docs", Platforms: []sqlc.AppPlatform{sqlc.AppPlatformServer}, Timezone: "Europe/Moscow"})
	require.NoError(t, err)
	_, err = q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: devWriteKey, Label: "DEV_WRITE_KEY"})
	require.NoError(t, err)

	rejected := chstore.NewRejectedLog(chstore.RejectedLogConfig{Inserter: env.CH})
	h := ingestHandler(t, q, env.CH, rejected)
	return h, func(t *testing.T, accepted int) {
		t.Helper()
		var n uint64
		require.NoError(t, env.CH.Conn().QueryRow(t.Context(),
			"SELECT count() FROM events WHERE app_id = ?", app.ID).Scan(&n))
		require.Equal(t, uint64(accepted), n, "в ClickHouse столько событий, сколько принято")
	}
}
