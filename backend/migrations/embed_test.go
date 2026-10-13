package migrations_test

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/migrations"
)

var fileName = regexp.MustCompile(`^\d{5}_[a-z0-9_]+\.sql$`)

// readNumbered проверяет имена NNNNN_snake_case.sql с номерами подряд и наличие секций Up/Down.
func readNumbered(t *testing.T, fsys fs.FS) map[string]string {
	t.Helper()
	entries, err := fs.ReadDir(fsys, ".")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	require.Equal(t, "00001_init.sql", entries[0].Name())

	bodies := make(map[string]string, len(entries))
	for i, e := range entries {
		require.Regexp(t, fileName, e.Name())
		require.True(t, strings.HasPrefix(e.Name(), fmt.Sprintf("%05d_", i+1)), "номера миграций идут подряд: %s", e.Name())

		body, err := fs.ReadFile(fsys, e.Name())
		require.NoError(t, err)
		up := strings.Index(string(body), "-- +goose Up")
		down := strings.Index(string(body), "-- +goose Down")
		require.GreaterOrEqual(t, up, 0, "%s: нет секции Up", e.Name())
		require.Greater(t, down, up, "%s: секция Down обязательна и идёт после Up", e.Name())
		bodies[e.Name()] = string(body)
	}
	return bodies
}

func TestPostgres_EmbedsNumberedGooseFiles(t *testing.T) {
	readNumbered(t, migrations.Postgres())
}

func TestClickHouse_EmbedsNumberedGooseFiles(t *testing.T) {
	bodies := readNumbered(t, migrations.ClickHouse())
	require.Len(t, bodies, 3)

	for name, body := range bodies {
		require.True(t, strings.HasPrefix(body, "-- +goose NO TRANSACTION\n"), "%s: ClickHouse не поддерживает транзакции DDL", name)

		// Каждый DDL — отдельный блок StatementBegin/End: ClickHouse не принимает несколько запросов за раз.
		inBlock := false
		for n, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "-- +goose StatementBegin":
				require.False(t, inBlock, "%s:%d: вложенный StatementBegin", name, n+1)
				inBlock = true
			case line == "-- +goose StatementEnd":
				require.True(t, inBlock, "%s:%d: StatementEnd без StatementBegin", name, n+1)
				inBlock = false
			case line == "" || strings.HasPrefix(line, "--"):
			default:
				require.True(t, inBlock, "%s:%d: SQL вне блока StatementBegin/End: %q", name, n+1, line)
			}
		}
		require.False(t, inBlock, "%s: незакрытый StatementBegin", name)
		require.Equal(t, strings.Count(body, "StatementBegin"), strings.Count(body, ";"), "%s: один запрос на блок", name)
	}
}
