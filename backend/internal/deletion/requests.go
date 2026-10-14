// Package deletion — запросы на удаление данных (FR-019): приём и журнал в Postgres; исполнение в ClickHouse — job deletion.
package deletion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

var (
	// ErrAppNotFound — приложения из запроса нет.
	ErrAppNotFound = errors.New("deletion: app not found")
	// ErrNotFound — запроса с таким id у приложения нет.
	ErrNotFound = errors.New("deletion: request not found")

	errNotImplemented = errors.New("deletion: not implemented")
)

// ValidationError — некорректное поле тела createDeletionRequest.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "deletion: invalid " + e.Field + ": " + e.Message }

// Service — запросы на удаление.
type Service struct {
	store  *pgstore.Store
	clock  clock.Clock
	logger *slog.Logger
}

func NewService(store *pgstore.Store, c clock.Clock, logger *slog.Logger) *Service {
	return &Service{store: store, clock: c, logger: logger}
}

// CreateInput — тело createDeletionRequest и его источник.
type CreateInput struct {
	AppID        uuid.UUID
	SubjectType  string
	SubjectValue string
	// APIKeyID — Bearer-ключ запроса (source api); nil — дашборд (source dashboard).
	APIKeyID *uuid.UUID
}

func (s *Service) Create(_ context.Context, _ CreateInput) (sqlc.DeletionRequest, error) {
	return sqlc.DeletionRequest{}, errNotImplemented
}

func (s *Service) Get(_ context.Context, _, _ uuid.UUID) (sqlc.DeletionRequest, error) {
	return sqlc.DeletionRequest{}, errNotImplemented
}

func (s *Service) List(_ context.Context, _ *uuid.UUID) ([]sqlc.DeletionRequest, error) {
	return nil, errNotImplemented
}
