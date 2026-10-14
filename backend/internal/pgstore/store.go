// Package pgstore — доступ к Postgres: пул pgx, запросы sqlc, транзакции и goose-миграции.
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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
	cfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		return true, registerTypes(ctx, conn)
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

// customTypes — enum-ы, которые pgx должен знать по имени: без регистрации массив app_platform[] не кодируется
// и не сканируется в []sqlc.AppPlatform («unknown type»). Массив идёт после элемента.
var customTypes = []string{"app_platform", "_app_platform"}

// registerTypes регистрирует customTypes на соединении один раз. Пул может открыться до миграций
// (migrate up, MIGRATE_ON_START): пока типов в базе нет, регистрация откладывается до следующего Acquire.
func registerTypes(ctx context.Context, conn *pgx.Conn) error {
	if _, ok := conn.TypeMap().TypeForName(customTypes[len(customTypes)-1]); ok {
		return nil
	}
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT to_regtype($1) IS NOT NULL`, customTypes[0]).Scan(&exists); err != nil {
		return fmt.Errorf("pgstore: check types: %w", err)
	}
	if !exists {
		return nil
	}
	types, err := conn.LoadTypes(ctx, customTypes)
	if err != nil {
		return fmt.Errorf("pgstore: load types: %w", err)
	}
	conn.TypeMap().RegisterTypes(types)
	return nil
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
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("pgstore: advisory lock %s: acquire: %w", key, err)
	}
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("pgstore: advisory lock %s: %w", key, err)
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		// Lock живёт, пока жива сессия: если снять его не удалось, соединение закрывается, а не возвращается в пул.
		unlockCtx := context.WithoutCancel(ctx)
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, key); err != nil {
			_ = conn.Conn().Close(unlockCtx)
		}
		conn.Release()
	}, true, nil
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
