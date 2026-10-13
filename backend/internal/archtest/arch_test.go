package archtest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/archtest"
)

const p = "appmetrica.online/backend/"

func TestImportRules(t *testing.T) {
	t.Run("repository has no forbidden imports", func(t *testing.T) {
		graph, err := archtest.LoadGraph(t.Context())
		require.NoError(t, err)
		require.Contains(t, graph, p+"internal/chstore", "граф построен по модулю backend")
		require.Contains(t, graph, p+"cmd/appmetrica")
		require.Empty(t, archtest.Check(graph))
	})

	cases := []struct {
		name  string
		graph archtest.Graph
		want  []string
	}{
		{
			name:  "fake graph ingest imports api",
			graph: archtest.Graph{p + "internal/ingest": {"net/http", p + "internal/api"}},
			want:  []string{"internal/ingest → internal/api"},
		},
		{
			name:  "leaf package imports internal",
			graph: archtest.Graph{p + "internal/clock": {"time", p + "internal/config"}},
			want:  []string{"internal/clock → internal/config"},
		},
		{
			name:  "reports must not touch postgres",
			graph: archtest.Graph{p + "internal/reports": {p + "internal/chstore", p + "internal/pgstore"}},
			want:  []string{"internal/reports → internal/pgstore"},
		},
		{
			name:  "storage must not import domain",
			graph: archtest.Graph{p + "internal/chstore": {p + "internal/clock", p + "migrations", p + "internal/identity"}},
			want:  []string{"internal/chstore → internal/identity"},
		},
		{
			name: "several violations are sorted",
			graph: archtest.Graph{
				p + "internal/worker": {p + "internal/ingest", p + "internal/api"},
				p + "internal/auth":   {p + "internal/chstore"},
			},
			want: []string{
				"internal/auth → internal/chstore",
				"internal/worker → internal/api",
				"internal/worker → internal/ingest",
			},
		},
		{
			name:  "unknown internal package needs a rule",
			graph: archtest.Graph{p + "internal/billing": {"fmt"}},
			want:  []string{"internal/billing: нет правила в archtest (07-project-structure.md §5)"},
		},
		{
			name: "allowed imports pass",
			graph: archtest.Graph{
				p + "internal/ingest":   {p + "internal/gen/ingestapi", p + "internal/geo", p + "internal/chstore", p + "internal/clock"},
				p + "internal/api":      {p + "internal/gen/api", p + "internal/reports", p + "internal/auth"},
				p + "internal/pgstore":  {p + "internal/pgstore/sqlc", p + "internal/clock", p + "migrations"},
				p + "cmd/appmetrica":    {p + "internal/ingest", p + "internal/api", p + "internal/worker"},
				p + "internal/testutil": {p + "internal/chstore", p + "internal/pgstore"},
			},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, archtest.Check(tc.graph))
		})
	}
}
