package main

import (
	"context"
	"flag"
	"io"

	"appmetrica.online/backend/internal/config"
)

// runMigrate — `appmetrica migrate up|status` для Postgres и ClickHouse.
func runMigrate(_ context.Context, args []string, _, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "up" && args[0] != "status") {
		say(stderr, "Использование: appmetrica migrate up|status\n")
		return 2
	}
	action := args[0]
	if !parseFlags(flag.NewFlagSet("migrate "+action, flag.ContinueOnError), args[1:], stderr) {
		return 2
	}
	if _, err := config.LoadMigrate(); err != nil {
		say(stderr, "migrate: конфигурация: %v\n", err)
		return 1
	}
	// Миграции и goose-провайдеры появляются в T-004 (Postgres) и T-005 (ClickHouse).
	say(stderr, "migrate %s: миграций пока нет (T-004, T-005)\n", action)
	return 1
}
