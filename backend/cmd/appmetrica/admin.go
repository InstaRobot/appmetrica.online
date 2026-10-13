package main

import (
	"context"
	"flag"
	"io"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
)

// runAdmin — `appmetrica admin passkey-link`: одноразовая ссылка регистрации passkey.
// Ссылка печатается в stdout и больше нигде не сохраняется; в лог (stderr) попадает только срок действия.
func runAdmin(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "passkey-link" {
		say(stderr, "Использование: appmetrica admin passkey-link\n")
		return 2
	}
	if !parseFlags(flag.NewFlagSet("admin passkey-link", flag.ContinueOnError), args[1:], stderr) {
		return 2
	}
	cfg, err := config.LoadAPI()
	if err != nil {
		say(stderr, "admin: конфигурация: %v\n", err)
		return 1
	}
	logger := newLogger(stderr, cfg.Common, "admin")

	pg, err := pgstore.Open(ctx, cfg.PostgresURL)
	if err != nil {
		say(stderr, "admin: postgres: %v\n", err)
		return 1
	}
	defer pg.Close()
	svc, err := auth.NewService(pg, clock.System{}, logger, cfg)
	if err != nil {
		say(stderr, "admin: %v\n", err)
		return 1
	}
	link, err := svc.CreatePasskeyLink(ctx)
	if err != nil {
		say(stderr, "admin: %v\n", err)
		return 1
	}
	say(stdout, "%s\n", link)
	say(stderr, "Ссылка действует %s и подходит для одной регистрации passkey.\n", auth.SetupTokenTTL)
	return 0
}
