// Package apps — приложения портфеля и их write-ключи в Postgres.
package apps

import (
	"log/slog"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
)

// Service — операции над приложениями.
type Service struct {
	store  *pgstore.Store
	clock  clock.Clock
	logger *slog.Logger
}

func NewService(store *pgstore.Store, c clock.Clock, logger *slog.Logger) *Service {
	return &Service{store: store, clock: c, logger: logger}
}
