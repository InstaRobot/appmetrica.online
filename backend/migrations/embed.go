// Package migrations встраивает goose-миграции в бинарник appmetrica.
// Postgres и ClickHouse ведут независимые последовательности версий в своих каталогах.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed postgres/*.sql clickhouse/*.sql
var files embed.FS

// Postgres возвращает миграции Postgres с корнем в каталоге postgres/ (формат, который ждёт goose Provider).
func Postgres() fs.FS {
	return sub("postgres")
}

// ClickHouse возвращает миграции ClickHouse с корнем в каталоге clickhouse/.
func ClickHouse() fs.FS {
	return sub("clickhouse")
}

func sub(dir string) fs.FS {
	fsys, err := fs.Sub(files, dir)
	if err != nil {
		// fs.Sub отказывает только на невалидном пути, а dir — константа пакета.
		panic("migrations: " + err.Error())
	}
	return fsys
}
