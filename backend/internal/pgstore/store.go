// Package pgstore — доступ к Postgres: пул pgx, запросы sqlc, транзакции и goose-миграции.
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/migrations"
)

// Store владеет пулом соединений Postgres.
type Store struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// Open создаёт пул по POSTGRES_URL и проверяет соединение.
// Тексты ошибок не содержат URL: в нём пароль.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgx может процитировать строку подключения целиком — исходную ошибку не оборачиваем.
		return nil, errors.New("pgstore: invalid POSTGRES_URL")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgstore: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	return &Store{pool: pool, queries: sqlc.New(pool)}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Queries выполняет запросы вне транзакции.
func (s *Store) Queries() *sqlc.Queries { return s.queries }

// InTx выполняет fn в транзакции: commit, если fn вернула nil, иначе rollback.
// Паника внутри fn откатывает транзакцию и пробрасывается дальше.
func (s *Store) InTx(ctx context.Context, fn func(q *sqlc.Queries) error) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin: %w", err)
	}
	defer func() {
		// Откат по контексту без отмены: исходный ctx может быть уже отменён.
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(s.queries.WithTx(tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgstore: commit: %w", err)
	}
	return nil
}

// TryAdvisoryLock берёт сессионный advisory lock key на отдельном соединении пула. ok = false — lock держит
// другой процесс. release снимает lock и возвращает соединение в пул.
func (s *Store) TryAdvisoryLock(ctx context.Context, key string) (release func(), ok bool, err error) {
	return nil, false, errors.New("pgstore: advisory lock: not implemented")
}

// NewMigrationProvider возвращает goose-провайдер встроенных миграций Postgres поверх пула.
// Provider.Close закрывает только обёртку database/sql, пул остаётся открытым.
func NewMigrationProvider(pool *pgxpool.Pool) (*goose.Provider, error) {
	db := stdlib.OpenDBFromPool(pool)
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Postgres())
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pgstore: goose provider: %w", err)
	}
	return p, nil
}
