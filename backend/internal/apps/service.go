// Package apps — приложения портфеля и их write-ключи в Postgres.
package apps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	nameMaxLen     = 64
	timezoneMaxLen = 64
	nameUniqueIdx  = "apps_name_lower_uq"
)

var (
	// ErrNotFound — приложения с таким id нет.
	ErrNotFound = errors.New("apps: not found")
	// ErrNameTaken — имя уже занято другим приложением (без учёта регистра).
	ErrNameTaken = errors.New("apps: name taken")
	// ErrConfirmNameMismatch — confirm_name при удалении не совпал с именем приложения.
	ErrConfirmNameMismatch = errors.New("apps: confirm name mismatch")
)

// ValidationError — некорректное поле запроса; Field пуст, если ошибка не относится к одному полю.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "apps: invalid " + e.Field + ": " + e.Message }

// Service — операции над приложениями.
type Service struct {
	store  *pgstore.Store
	clock  clock.Clock
	logger *slog.Logger
}

func NewService(store *pgstore.Store, c clock.Clock, logger *slog.Logger) *Service {
	return &Service{store: store, clock: c, logger: logger}
}

// CreateInput — тело createApp.
type CreateInput struct {
	Name     string
	Platform string
	Timezone string
}

// UpdateInput — тело updateApp: nil — поле не меняется.
type UpdateInput struct {
	Name     *string
	Timezone *string
}

// List — приложения по имени без учёта регистра; архивные — только при includeArchived.
func (s *Service) List(ctx context.Context, includeArchived bool) ([]sqlc.App, error) {
	list, err := s.store.Queries().ListApps(ctx, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("apps: list: %w", err)
	}
	return list, nil
}

// Get — приложение по id; нет такого — ErrNotFound.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (sqlc.App, error) {
	app, err := s.store.Queries().GetApp(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.App{}, ErrNotFound
	case err != nil:
		return sqlc.App{}, fmt.Errorf("apps: get: %w", err)
	}
	return app, nil
}

// Create создаёт приложение и его первый write-ключ в одной транзакции.
func (s *Service) Create(ctx context.Context, in CreateInput) (sqlc.App, sqlc.WriteKey, error) {
	name, err := validateName(in.Name)
	if err != nil {
		return sqlc.App{}, sqlc.WriteKey{}, err
	}
	platform := sqlc.AppPlatform(in.Platform)
	if !platform.Valid() {
		return sqlc.App{}, sqlc.WriteKey{}, &ValidationError{Field: "platform", Message: "Неизвестная платформа"}
	}
	tz, err := validateTimezone(in.Timezone)
	if err != nil {
		return sqlc.App{}, sqlc.WriteKey{}, err
	}
	key, err := newWriteKey()
	if err != nil {
		return sqlc.App{}, sqlc.WriteKey{}, err
	}
	var (
		app sqlc.App
		wk  sqlc.WriteKey
	)
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		app, err = q.CreateApp(ctx, sqlc.CreateAppParams{Name: name, Platform: platform, Timezone: tz})
		switch {
		case isNameTaken(err):
			return ErrNameTaken
		case err != nil:
			return fmt.Errorf("apps: create: %w", err)
		}
		wk, err = q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: app.ID, Key: key, Label: firstKeyLabel})
		if err != nil {
			return fmt.Errorf("apps: create write key: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlc.App{}, sqlc.WriteKey{}, err
	}
	s.logger.InfoContext(ctx, "app created", slog.String("app_id", app.ID.String()))
	return app, wk, nil
}

// Update меняет имя и/или часовой пояс. Платформа не меняется: от неё зависят уже собранные события.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (sqlc.App, error) {
	if in.Name == nil && in.Timezone == nil {
		return sqlc.App{}, &ValidationError{Message: "Укажите название или часовой пояс"}
	}
	params := sqlc.UpdateAppParams{ID: id, Now: s.clock.Now()}
	if in.Name != nil {
		name, err := validateName(*in.Name)
		if err != nil {
			return sqlc.App{}, err
		}
		params.Name = &name
	}
	if in.Timezone != nil {
		tz, err := validateTimezone(*in.Timezone)
		if err != nil {
			return sqlc.App{}, err
		}
		params.Timezone = &tz
	}
	app, err := s.store.Queries().UpdateApp(ctx, params)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.App{}, ErrNotFound
	case isNameTaken(err):
		return sqlc.App{}, ErrNameTaken
	case err != nil:
		return sqlc.App{}, fmt.Errorf("apps: update: %w", err)
	}
	return app, nil
}

// Archive переводит приложение в архив: ingest перестаёт принимать его ключи со следующим обновлением кэша.
// Повторный вызов ничего не меняет (archived_at остаётся прежним).
func (s *Service) Archive(ctx context.Context, id uuid.UUID) error {
	now := s.clock.Now()
	_, err := s.store.Queries().ArchiveApp(ctx, sqlc.ArchiveAppParams{ID: id, Now: &now})
	return s.idempotent(ctx, id, err, "archive")
}

// Unarchive возвращает приложение из архива; для активного ничего не меняет.
func (s *Service) Unarchive(ctx context.Context, id uuid.UUID) error {
	_, err := s.store.Queries().UnarchiveApp(ctx, sqlc.UnarchiveAppParams{ID: id, Now: s.clock.Now()})
	return s.idempotent(ctx, id, err, "unarchive")
}

// Delete сверяет confirmName с именем (точно, с учётом регистра), архивирует приложение и создаёт заявку
// на удаление subject_type=app. Строку apps удаляет worker после очистки ClickHouse.
// Повторный вызов возвращает ту же заявку, пока она pending или running.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, confirmName string) (sqlc.DeletionRequest, error) {
	now := s.clock.Now()
	var dr sqlc.DeletionRequest
	err := s.store.InTx(ctx, func(q *sqlc.Queries) error {
		app, err := q.GetApp(ctx, id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNotFound
		case err != nil:
			return fmt.Errorf("apps: get: %w", err)
		}
		if confirmName != app.Name {
			return ErrConfirmNameMismatch
		}
		if _, err := q.ArchiveApp(ctx, sqlc.ArchiveAppParams{ID: id, Now: &now}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("apps: archive: %w", err)
		}
		dr, err = q.CreateDeletionRequest(ctx, sqlc.CreateDeletionRequestParams{
			AppID:        id,
			SubjectType:  sqlc.DeletionSubjectApp,
			SubjectValue: id.String(),
			Source:       sqlc.DeletionSourceDashboard,
			Now:          now,
		})
		if err != nil {
			return fmt.Errorf("apps: create deletion request: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlc.DeletionRequest{}, err
	}
	s.logger.InfoContext(ctx, "app deletion requested",
		slog.String("app_id", id.String()), slog.String("deletion_request_id", dr.ID.String()))
	return dr, nil
}

// idempotent: UPDATE … WHERE status = … не нашёл строку — либо приложение уже в нужном статусе (nil),
// либо его нет (ErrNotFound).
func (s *Service) idempotent(ctx context.Context, id uuid.UUID, err error, op string) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return fmt.Errorf("apps: %s: %w", op, err)
		}
		return nil
	}
	_, err = s.Get(ctx, id)
	return err
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > nameMaxLen || !utf8.ValidString(name) {
		return "", &ValidationError{Field: "name", Message: "Название — от 1 до 64 символов"}
	}
	return name, nil
}

// validateTimezone принимает только имена IANA: пустая строка и "Local" для time.LoadLocation
// означают UTC и пояс сервера, поэтому отклоняются.
func validateTimezone(tz string) (string, error) {
	if tz == "" || tz == "Local" || len(tz) > timezoneMaxLen {
		return "", &ValidationError{Field: "timezone", Message: "Неизвестный часовой пояс"}
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return "", &ValidationError{Field: "timezone", Message: "Неизвестный часовой пояс"}
	}
	return tz, nil
}

// isNameTaken — нарушение уникального индекса lower(name).
func isNameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == nameUniqueIdx
}
