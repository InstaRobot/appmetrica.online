package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/pressly/goose/v3"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
)

// runMigrate — `appmetrica migrate up|status` для Postgres и ClickHouse.
func runMigrate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "up" && args[0] != "status") {
		say(stderr, "Использование: appmetrica migrate up|status\n")
		return 2
	}
	action := args[0]
	if !parseFlags(flag.NewFlagSet("migrate "+action, flag.ContinueOnError), args[1:], stderr) {
		return 2
	}
	cfg, err := config.LoadMigrate()
	if err != nil {
		say(stderr, "migrate: конфигурация: %v\n", err)
		return 1
	}
	if action == "up" {
		err = migrateUp(ctx, cfg.PostgresURL, cfg.ClickHouse, stdout)
	} else {
		err = migrateStatus(ctx, cfg.PostgresURL, cfg.ClickHouse, stdout)
	}
	if err != nil {
		say(stderr, "migrate %s: %v\n", action, err)
		return 1
	}
	return 0
}

// migrationTarget — хранилище со своей последовательностью goose-версий.
type migrationTarget struct {
	name string
	// open возвращает провайдер и функцию, освобождающую соединения.
	open func(ctx context.Context) (*goose.Provider, func(), error)
}

// migrationTargets: сначала Postgres, затем ClickHouse. DDL в ClickHouse выполняет пользователь
// CLICKHOUSE_MIGRATE_USER (appmetrica_admin): у appmetrica_rw/ro прав на DDL нет.
func migrationTargets(pgURL string, ch config.ClickHouse) []migrationTarget {
	return []migrationTarget{
		{"postgres", func(ctx context.Context) (*goose.Provider, func(), error) {
			store, err := pgstore.Open(ctx, pgURL)
			if err != nil {
				return nil, nil, err
			}
			p, err := pgstore.NewMigrationProvider(store.Pool())
			if err != nil {
				store.Close()
				return nil, nil, err
			}
			return p, func() { _ = p.Close(); store.Close() }, nil
		}},
		{"clickhouse", func(ctx context.Context) (*goose.Provider, func(), error) {
			opts := chstore.Options{Addr: ch.Addr, Database: ch.Database, User: ch.User, Password: ch.Password}
			// Ping до goose: ошибка подключения понятнее, чем ошибка создания таблицы версий.
			store, err := chstore.Open(ctx, opts)
			if err != nil {
				return nil, nil, err
			}
			_ = store.Close()
			p, err := chstore.NewMigrationProvider(opts)
			if err != nil {
				return nil, nil, err
			}
			return p, func() { _ = p.Close() }, nil
		}},
	}
}

// migrateUp применяет все новые миграции; повторный запуск — no-op.
// Используется подкомандой migrate up и api при MIGRATE_ON_START=true.
func migrateUp(ctx context.Context, pgURL string, ch config.ClickHouse, w io.Writer) error {
	for _, t := range migrationTargets(pgURL, ch) {
		if err := withProvider(ctx, t, func(p *goose.Provider) error {
			res, err := p.Up(ctx)
			if err != nil {
				return err
			}
			v, err := p.GetDBVersion(ctx)
			if err != nil {
				return err
			}
			say(w, "%s: применено миграций: %d, версия %05d\n", t.name, len(res), v)
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// migrateStatus печатает текущую версию и состояние каждой встроенной миграции.
func migrateStatus(ctx context.Context, pgURL string, ch config.ClickHouse, w io.Writer) error {
	for _, t := range migrationTargets(pgURL, ch) {
		if err := withProvider(ctx, t, func(p *goose.Provider) error {
			v, err := p.GetDBVersion(ctx)
			if err != nil {
				return err
			}
			statuses, err := p.Status(ctx)
			if err != nil {
				return err
			}
			say(w, "%s: версия %05d\n", t.name, v)
			for _, s := range statuses {
				state := "ожидает"
				if s.State == goose.StateApplied {
					state = "применена " + s.AppliedAt.UTC().Format(time.RFC3339)
				}
				say(w, "  %-28s %s\n", path.Base(s.Source.Path), state)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func withProvider(ctx context.Context, t migrationTarget, fn func(p *goose.Provider) error) error {
	p, release, err := t.open(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", t.name, err)
	}
	defer release()
	if err := fn(p); err != nil {
		var partial *goose.PartialError
		if errors.As(err, &partial) && partial.Failed != nil {
			return fmt.Errorf("%s: миграция %s: %w", t.name, path.Base(partial.Failed.Source.Path), partial.Err)
		}
		return fmt.Errorf("%s: %w", t.name, err)
	}
	return nil
}
