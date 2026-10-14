// Package deletion — запросы на удаление данных (FR-019): приём и журнал в Postgres; исполнение в ClickHouse — job deletion.
package deletion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	// maxUserIDLen — как у user_id в ingest (символов после TrimSpace).
	maxUserIDLen = 128
	// pgForeignKeyViolation — приложение удалено между проверкой и вставкой.
	pgForeignKeyViolation = "23503"
)

var (
	// ErrAppNotFound — приложения из запроса нет.
	ErrAppNotFound = errors.New("deletion: app not found")
	// ErrNotFound — запроса с таким id у приложения нет.
	ErrNotFound = errors.New("deletion: request not found")
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

// Create проверяет субъект и ставит запрос в очередь со status=pending. Пока запрос на тот же
// (app_id, subject_type, subject_value) pending или running, возвращается он же — с его source и api_key_id.
// Субъект app через этот вызов не принимается: приложение удаляет apps.Service.Delete.
func (s *Service) Create(ctx context.Context, in CreateInput) (sqlc.DeletionRequest, error) {
	subject, value, err := validSubject(in.SubjectType, in.SubjectValue)
	if err != nil {
		return sqlc.DeletionRequest{}, err
	}
	source := sqlc.DeletionSourceDashboard
	if in.APIKeyID != nil {
		source = sqlc.DeletionSourceApi
	}
	var dr sqlc.DeletionRequest
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if _, err := q.GetApp(ctx, in.AppID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAppNotFound
			}
			return fmt.Errorf("deletion: get app: %w", err)
		}
		dr, err = q.CreateDeletionRequest(ctx, sqlc.CreateDeletionRequestParams{
			AppID:        in.AppID,
			SubjectType:  subject,
			SubjectValue: value,
			Source:       source,
			ApiKeyID:     in.APIKeyID,
			Now:          s.clock.Now(),
		})
		var pgErr *pgconn.PgError
		switch {
		case errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation:
			return ErrAppNotFound
		case err != nil:
			return fmt.Errorf("deletion: create request: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlc.DeletionRequest{}, err
	}
	// subject_value не логируется: это user_id или install_id.
	s.logger.InfoContext(ctx, "deletion requested",
		slog.String("app_id", in.AppID.String()), slog.String("deletion_request_id", dr.ID.String()),
		slog.String("subject_type", string(dr.SubjectType)), slog.String("status", string(dr.Status)))
	return dr, nil
}

// validSubject: install — UUID в каноничной форме 8-4-4-4-12 (регистр любой), не nil, хранится в нижнем
// регистре; user — 1..128 символов после TrimSpace без U+0000, как user_id в ingest (Postgres text NUL не хранит).
func validSubject(subjectType, value string) (sqlc.DeletionSubject, string, error) {
	switch sqlc.DeletionSubject(subjectType) {
	case sqlc.DeletionSubjectInstall:
		id, err := uuid.Parse(value)
		if err != nil || len(value) != 36 || id == uuid.Nil {
			return "", "", &ValidationError{Field: "subject_value", Message: "Для install нужен install_id в формате UUID."}
		}
		return sqlc.DeletionSubjectInstall, id.String(), nil
	case sqlc.DeletionSubjectUser:
		v := strings.TrimSpace(value)
		if n := utf8.RuneCountInString(v); n == 0 || n > maxUserIDLen || strings.ContainsRune(v, 0) {
			return "", "", &ValidationError{Field: "subject_value", Message: "Для user нужен user_id длиной от 1 до 128 символов без символа U+0000."}
		}
		return sqlc.DeletionSubjectUser, v, nil
	default:
		return "", "", &ValidationError{Field: "subject_type", Message: "subject_type: допустимы install или user."}
	}
}

// Get — запрос id приложения appID. Нет приложения — ErrAppNotFound, нет запроса у этого приложения — ErrNotFound.
func (s *Service) Get(ctx context.Context, appID, id uuid.UUID) (sqlc.DeletionRequest, error) {
	q := s.store.Queries()
	dr, err := q.GetDeletionRequest(ctx, sqlc.GetDeletionRequestParams{ID: id, AppID: appID})
	if err == nil {
		return dr, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.DeletionRequest{}, fmt.Errorf("deletion: get request: %w", err)
	}
	switch _, err := q.GetApp(ctx, appID); {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.DeletionRequest{}, ErrAppNotFound
	case err != nil:
		return sqlc.DeletionRequest{}, fmt.Errorf("deletion: get app: %w", err)
	}
	return sqlc.DeletionRequest{}, ErrNotFound
}

// List — последние 200 запросов по requested_at DESC; appID nil — по всем приложениям.
func (s *Service) List(ctx context.Context, appID *uuid.UUID) ([]sqlc.DeletionRequest, error) {
	list, err := s.store.Queries().ListDeletionRequests(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("deletion: list requests: %w", err)
	}
	return list, nil
}
