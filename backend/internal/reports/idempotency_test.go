//go:build integration

package reports

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// AC-029.4: батч, загруженный трижды (ретраи клиента с теми же insert_id), до слияния частей
// ReplacingMergeTree не меняет ни одной метрики аудитории.
func TestAudience_DuplicateBatchesDoNotInflate(t *testing.T) {
	report := func(t *testing.T, loads int) map[string]audienceSnapshot {
		t.Helper()
		env := testutil.Start(t)
		// Слияния остановлены: дубли остаются видимыми, как в окне между вставкой и merge.
		require.NoError(t, env.CH.Conn().Exec(t.Context(), "SYSTEM STOP MERGES events"))
		rows := env.LoadNDJSON(t, testutil.ScenarioPath())
		// Повторы приходят в другом порядке и другими батчами.
		reversed := slices.Clone(rows)
		slices.Reverse(reversed)
		for range loads - 1 {
			for chunk := range slices.Chunk(reversed, 100) {
				require.NoError(t, env.CH.InsertEvents(t.Context(), chunk))
			}
		}
		var stored uint64
		require.NoError(t, env.CH.Conn().QueryRow(t.Context(), "SELECT count() FROM events").Scan(&stored))
		require.Equal(t, uint64(loads*len(rows)), stored, "дубли не схлопнуты до отчёта")

		svc := newService(env)
		out := map[string]audienceSnapshot{}
		for _, app := range testutil.ScenarioApps {
			out[app.Code] = audienceReport(t, svc, scenarioParams(t, app, "2026-10-01", "2026-10-12"))
		}
		return out
	}

	var once, thrice map[string]audienceSnapshot
	t.Run("once", func(t *testing.T) { once = report(t, 1) })
	t.Run("thrice", func(t *testing.T) { thrice = report(t, 3) })
	require.NotEmpty(t, once)
	require.Equal(t, once, thrice)
}
