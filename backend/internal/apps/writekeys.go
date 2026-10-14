package apps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const writeKeyLabelMaxLen = 64

// ErrWriteKeyNotFound — у приложения нет write-ключа с таким id.
var ErrWriteKeyNotFound = errors.New("apps: write key not found")

// ListWriteKeys — ключи приложения, включая отозванные, новые первыми; нет приложения — ErrNotFound.
func (s *Service) ListWriteKeys(ctx context.Context, appID uuid.UUID) ([]sqlc.WriteKey, error) {
	if _, err := s.Get(ctx, appID); err != nil {
		return nil, err
	}
	keys, err := s.store.Queries().ListWriteKeys(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("apps: list write keys: %w", err)
	}
	return keys, nil
}

// CreateWriteKey выпускает ещё один write-ключ: активных ключей у приложения может быть несколько,
// чтобы ротировать ключ без поломки старых версий клиентов. Label обрезается по краям, 0..64 символа.
// Приложению с ожидающей заявкой на удаление ключ не выпускается — ErrDeletionPending.
func (s *Service) CreateWriteKey(ctx context.Context, appID uuid.UUID, label string) (sqlc.WriteKey, error) {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > writeKeyLabelMaxLen || !utf8.ValidString(label) {
		return sqlc.WriteKey{}, &ValidationError{Field: "label", Message: "Название ключа — не длиннее 64 символов"}
	}
	key, err := newWriteKey()
	if err != nil {
		return sqlc.WriteKey{}, err
	}
	var wk sqlc.WriteKey
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if err := s.lockNotDeleting(ctx, q, appID); err != nil {
			return err
		}
		wk, err = q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{AppID: appID, Key: key, Label: label})
		if err != nil {
			return fmt.Errorf("apps: create write key: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlc.WriteKey{}, err
	}
	s.logger.InfoContext(ctx, "write key created",
		slog.String("app_id", appID.String()), slog.String("write_key_id", wk.ID.String()))
	return wk, nil
}

// RevokeWriteKey отзывает ключ; ingest перестаёт его принимать со следующим обновлением кэша. Отзыв
// последнего активного ключа разрешён. Повторный вызов не меняет revoked_at. Нет приложения — ErrNotFound,
// нет такого ключа у приложения — ErrWriteKeyNotFound.
func (s *Service) RevokeWriteKey(ctx context.Context, appID, keyID uuid.UUID) error {
	q := s.store.Queries()
	now := s.clock.Now()
	_, err := q.RevokeWriteKey(ctx, sqlc.RevokeWriteKeyParams{ID: keyID, AppID: appID, Now: &now})
	if err == nil {
		s.logger.InfoContext(ctx, "write key revoked",
			slog.String("app_id", appID.String()), slog.String("write_key_id", keyID.String()))
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("apps: revoke write key: %w", err)
	}
	_, err = q.GetWriteKey(ctx, sqlc.GetWriteKeyParams{ID: keyID, AppID: appID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := s.Get(ctx, appID); err != nil {
			return err
		}
		return ErrWriteKeyNotFound
	case err != nil:
		return fmt.Errorf("apps: get write key: %w", err)
	}
	return nil
}
