// Команда seed заливает в локальные Postgres и ClickHouse демо-данные (`make seed`).
package main

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
)

const seedTimezone = "Europe/Moscow"

type options struct {
	Apps          int
	Days          int
	DailyInstalls int
	WriteKey      string
	Seed          uint64
}

type seedApp struct {
	ID         uuid.UUID
	Platform   string
	WriteKeyID uuid.UUID
}

type genConfig struct {
	Days          int
	DailyInstalls int
	Seed          uint64
	Today         time.Time
	Location      *time.Location
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, clock.System{}))
}

func run(_ context.Context, _ []string, _, _ io.Writer, _ clock.Clock) int { return 1 }

func parseOptions(_ []string, _ io.Writer) (options, bool) { return options{}, false }

func generateApp(_ genConfig, _ int, _ seedApp) []chstore.EventRow { return nil }
