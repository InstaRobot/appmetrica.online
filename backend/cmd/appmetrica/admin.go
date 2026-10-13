package main

import (
	"context"
	"flag"
	"io"
)

// runAdmin — `appmetrica admin passkey-link`: одноразовая ссылка регистрации passkey.
func runAdmin(_ context.Context, args []string, _, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "passkey-link" {
		say(stderr, "Использование: appmetrica admin passkey-link\n")
		return 2
	}
	if !parseFlags(flag.NewFlagSet("admin passkey-link", flag.ContinueOnError), args[1:], stderr) {
		return 2
	}
	// Setup-токены passkey появляются вместе с auth (Postgres-таблица passkey_setup_tokens).
	say(stderr, "admin passkey-link: ещё не реализовано\n")
	return 1
}
