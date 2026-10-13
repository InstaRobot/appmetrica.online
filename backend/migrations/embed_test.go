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

func TestPostgres_EmbedsNumberedGooseFiles(t *testing.T) {
	fsys := migrations.Postgres()
	entries, err := fs.ReadDir(fsys, ".")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	require.Equal(t, "00001_init.sql", entries[0].Name())

	for i, e := range entries {
		require.Regexp(t, fileName, e.Name())
		require.True(t, strings.HasPrefix(e.Name(), fmt.Sprintf("%05d_", i+1)), "номера миграций идут подряд: %s", e.Name())

		body, err := fs.ReadFile(fsys, e.Name())
		require.NoError(t, err)
		up := strings.Index(string(body), "-- +goose Up")
		down := strings.Index(string(body), "-- +goose Down")
		require.GreaterOrEqual(t, up, 0, "%s: нет секции Up", e.Name())
		require.Greater(t, down, up, "%s: секция Down обязательна и идёт после Up", e.Name())
	}
}
