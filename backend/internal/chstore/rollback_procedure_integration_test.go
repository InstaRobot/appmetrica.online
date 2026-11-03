//go:build integration

package chstore_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/testutil"
	"appmetrica.online/backend/migrations"
)

// Ручной откат релиза FR-102 (21-ops-runbooks.md §5.7): у `appmetrica migrate` нет down, поэтому оператор
// выполняет два оператора SQL от имени администратора ClickHouse. Тест проверяет их на настоящем ClickHouse:
// после них образ предыдущего релиза (встроенные миграции 00001–00003) стартует, а без второго оператора новый
// релиз при повторной выкатке не вернул бы колонку (версия 4 числилась бы применённой).
func TestManualRollbackProcedure_PreviousImageStartsAtVersion3(t *testing.T) {
	ctx := t.Context()
	env := testutil.StartEmpty(t)
	conn := env.CH.Conn()

	cur, err := chstore.NewMigrationProvider(env.ClickHouse)
	require.NoError(t, err)
	defer func() { require.NoError(t, cur.Close()) }()
	_, err = cur.Up(ctx)
	require.NoError(t, err)

	// Провайдер «предыдущего образа»: те же файлы без 00004.
	old := fstest.MapFS{}
	require.NoError(t, fs.WalkDir(migrations.ClickHouse(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path >= "00004" {
			return err
		}
		b, err := fs.ReadFile(migrations.ClickHouse(), path)
		old[path] = &fstest.MapFile{Data: b}
		return err
	}))
	require.Len(t, old, 3)
	oldProvider := func() *goose.Provider {
		p, err := goose.NewProvider(goose.DialectClickHouse, clickhouse.OpenDB(&clickhouse.Options{
			Addr: []string{env.ClickHouse.Addr},
			Auth: clickhouse.Auth{Database: env.ClickHouse.Database, Username: env.ClickHouse.User, Password: env.ClickHouse.Password},
		}), old)
		require.NoError(t, err)
		return p
	}

	hasProperties := func() bool {
		var n uint64
		require.NoError(t, conn.QueryRow(ctx, `SELECT count() FROM system.columns
			WHERE database = currentDatabase() AND table = 'events' AND name = 'properties'`).Scan(&n))
		return n == 1
	}

	// Процедура, шаг 1: колонка снята. Строку версии 4 не трогаем: goose считает 00004 применённой.
	require.NoError(t, conn.Exec(ctx, "ALTER TABLE events DROP COLUMN IF EXISTS properties"))
	require.False(t, hasProperties())
	res, err := cur.Up(ctx)
	require.NoError(t, err)
	require.Empty(t, res, "без шага 2 новый релиз не вернёт колонку: версия 4 числится применённой")
	require.False(t, hasProperties())

	// Шаг 2: строка версии 4 удалена из таблицы версий goose.
	require.NoError(t, conn.Exec(ctx, "ALTER TABLE goose_db_version DELETE WHERE version_id = 4 SETTINGS mutations_sync = 2"))

	var maxVersion int64
	require.NoError(t, conn.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied = 1").Scan(&maxVersion))
	require.Equal(t, int64(3), maxVersion)

	p := oldProvider()
	res, err = p.Up(ctx)
	require.NoError(t, err, "предыдущий образ стартует на версии 3")
	require.Empty(t, res)
	v, err := p.GetDBVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), v)
	require.NoError(t, p.Close())

	// Повторная выкатка нового релиза применяет 00004 заново.
	res, err = cur.Up(ctx)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.True(t, hasProperties())
}
