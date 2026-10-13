package chstore

import (
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pressly/goose/v3"

	"appmetrica.online/backend/migrations"
)

// NewMigrationProvider возвращает goose-провайдер встроенных миграций ClickHouse.
// opts — пользователь с правами DDL (CLICKHOUSE_MIGRATE_USER, appmetrica_admin).
// Таблица версий goose_db_version создаётся goose на MergeTree в базе opts.Database.
// Provider.Close закрывает соединение database/sql, открытое здесь.
func NewMigrationProvider(opts Options) (*goose.Provider, error) {
	db := clickhouse.OpenDB(opts.clickhouse())
	p, err := goose.NewProvider(goose.DialectClickHouse, db, migrations.ClickHouse())
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("chstore: goose provider: %w", err)
	}
	return p, nil
}
