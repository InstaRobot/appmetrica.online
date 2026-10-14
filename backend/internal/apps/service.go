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
	// DeleteTimeout — предел транзакции Delete: заявка на удаление становится видна (commit) не позже чем через
	// этот срок после requested_at, который берётся уже под блокировкой строки apps. На него опирается
	// deletion.AppPurgeDelay: снимок ключей ingest, начатый до commit, ещё содержит ключи приложения.
	DeleteTimeout = 10 * time.Second
)

var (
	// ErrNotFound — приложения с таким id нет.
	ErrNotFound = errors.New("apps: not found")
	// ErrNameTaken — имя уже занято другим приложением (без учёта регистра).
	ErrNameTaken = errors.New("apps: name taken")
	// ErrConfirmNameMismatch — confirm_name при удалении не совпал с именем приложения.
	ErrConfirmNameMismatch = errors.New("apps: confirm name mismatch")
	// ErrDeletionPending — у приложения есть невыполненная заявка на удаление: новые ключи и возврат из архива
	// запрещены, иначе ingest снова писал бы строки приложения, в том числе после purge.
	ErrDeletionPending = errors.New("apps: deletion pending")
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
	Name      string
	Platforms []string
	Timezone  string
}

// UpdateInput — тело updateApp: nil — поле не меняется.
type UpdateInput struct {
	Name      *string
	Timezone  *string
	Platforms *[]string
}

// CreateResult — новое приложение и его write-ключи, по одному на платформу в порядке app.Platforms.
type CreateResult struct {
	App       sqlc.App
	WriteKeys []sqlc.WriteKey
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

// Create создаёт приложение и по write-ключу на каждую его платформу в одной транзакции: ключ с платформой
// задаёт платформу событий без context.platform. Метка ключа — название платформы.
func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	name, err := validateName(in.Name)
	if err != nil {
		return CreateResult{}, err
	}
	platforms, err := normalizePlatforms(in.Platforms)
	if err != nil {
		return CreateResult{}, err
	}
	tz, err := validateTimezone(in.Timezone)
	if err != nil {
		return CreateResult{}, err
	}
	keys := make([]string, len(platforms))
	for i := range keys {
		if keys[i], err = newWriteKey(); err != nil {
			return CreateResult{}, err
		}
	}
	var res CreateResult
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		res.App, err = q.CreateApp(ctx, sqlc.CreateAppParams{Name: name, Platforms: platforms, Timezone: tz})
		switch {
		case isNameTaken(err):
			return ErrNameTaken
		case err != nil:
			return fmt.Errorf("apps: create: %w", err)
		}
		res.WriteKeys = make([]sqlc.WriteKey, 0, len(platforms))
		for i, p := range platforms {
			wk, err := q.CreateWriteKey(ctx, sqlc.CreateWriteKeyParams{
				AppID: res.App.ID, Key: keys[i], Label: platformLabels[p], Platform: &p,
			})
			if err != nil {
				return fmt.Errorf("apps: create write key: %w", err)
			}
			res.WriteKeys = append(res.WriteKeys, wk)
		}
		return nil
	})
	if err != nil {
		return CreateResult{}, err
	}
	s.logger.InfoContext(ctx, "app created", slog.String("app_id", res.App.ID.String()))
	return res, nil
}

// Update меняет имя, часовой пояс и/или набор платформ. Ключи и собранные события смена платформ не
// затрагивает: ключи снятых платформ остаются активными, их отзывают отдельно.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (sqlc.App, error) {
	if in.Name == nil && in.Timezone == nil && in.Platforms == nil {
		return sqlc.App{}, &ValidationError{Message: "Укажите название, часовой пояс или платформы"}
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
	if in.Platforms != nil {
		platforms, err := normalizePlatforms(*in.Platforms)
		if err != nil {
			return sqlc.App{}, err
		}
		params.Platforms = platforms
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

// Unarchive возвращает приложение из архива; для активного ничего не меняет. Приложение с ожидающей заявкой
// на удаление (Delete) не возвращается — ErrDeletionPending.
func (s *Service) Unarchive(ctx context.Context, id uuid.UUID) error {
	now := s.clock.Now()
	return s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if err := s.lockNotDeleting(ctx, q, id); err != nil {
			return err
		}
		_, err := q.UnarchiveApp(ctx, sqlc.UnarchiveAppParams{ID: id, Now: now})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("apps: unarchive: %w", err)
		}
		return nil
	})
}

// lockNotDeleting блокирует строку приложения до конца транзакции (ErrNotFound, если его нет) и проверяет, что
// заявки на его удаление нет (ErrDeletionPending). Delete берёт ту же блокировку, поэтому ключ или возврат из
// архива не проскочат между архивацией и отзывом ключей.
func (s *Service) lockNotDeleting(ctx context.Context, q *sqlc.Queries, id uuid.UUID) error {
	if _, err := q.GetAppForUpdate(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("apps: lock: %w", err)
	}
	pending, err := q.AppDeletionPending(ctx, id)
	if err != nil {
		return fmt.Errorf("apps: deletion pending: %w", err)
	}
	if pending {
		return ErrDeletionPending
	}
	return nil
}

// Delete сверяет confirmName с именем (точно, с учётом регистра), архивирует приложение, отзывает все его
// write-ключи и создаёт заявку на удаление subject_type=app. Отзыв нужен, чтобы ingest после перезагрузки
// снимка не писал для приложения ничего: архивный ключ ещё даёт 403 app_archived в rejected_events, а
// отозванный — 401 invalid_app_key без строки в журнале, иначе записи появлялись бы после purge.
// Строку apps удаляет worker после очистки ClickHouse (deletion.AppPurgeDelay).
// Повторный вызов возвращает ту же заявку, пока она pending или running.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, confirmName string) (sqlc.DeletionRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, DeleteTimeout)
	defer cancel()
	var dr sqlc.DeletionRequest
	err := s.store.InTx(ctx, func(q *sqlc.Queries) error {
		// Блокировка строки: параллельные CreateWriteKey и Unarchive ждут и затем видят заявку.
		app, err := q.GetAppForUpdate(ctx, id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNotFound
		case err != nil:
			return fmt.Errorf("apps: get: %w", err)
		}
		// requested_at — после захвата блокировки: ожидание не должно отодвигать commit от requested_at дальше
		// DeleteTimeout, иначе снимок ключей, начатый во время ожидания, пережил бы deletion.AppPurgeDelay.
		now := s.clock.Now()
		if confirmName != app.Name {
			return ErrConfirmNameMismatch
		}
		if _, err := q.ArchiveApp(ctx, sqlc.ArchiveAppParams{ID: id, Now: &now}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("apps: archive: %w", err)
		}
		if err := q.RevokeAppWriteKeys(ctx, sqlc.RevokeAppWriteKeysParams{Now: &now, AppID: id}); err != nil {
			return fmt.Errorf("apps: revoke write keys: %w", err)
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

// normalizePlatforms убирает дубли и упорядочивает платформы как в enum app_platform; пустой набор и
// неизвестное значение — ValidationError по полю platforms.
func normalizePlatforms(in []string) ([]sqlc.AppPlatform, error) {
	if len(in) == 0 {
		return nil, &ValidationError{Field: "platforms", Message: "Выберите хотя бы одну платформу"}
	}
	seen := make(map[sqlc.AppPlatform]bool, len(in))
	for _, v := range in {
		p := sqlc.AppPlatform(v)
		if !p.Valid() {
			return nil, &ValidationError{Field: "platforms", Message: "Неизвестная платформа"}
		}
		seen[p] = true
	}
	out := make([]sqlc.AppPlatform, 0, len(seen))
	for _, p := range sqlc.AllAppPlatformValues() {
		if seen[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > nameMaxLen || !utf8.ValidString(name) {
		return "", &ValidationError{Field: "name", Message: "Название — от 1 до 64 символов"}
	}
	return name, nil
}

// ValidTimezone принимает только имена IANA: пустая строка и "Local" для time.LoadLocation
// означают UTC и пояс сервера, поэтому отклоняются.
func ValidTimezone(tz string) bool {
	if tz == "" || tz == "Local" || len(tz) > timezoneMaxLen {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

func validateTimezone(tz string) (string, error) {
	if !ValidTimezone(tz) {
		return "", &ValidationError{Field: "timezone", Message: "Неизвестный часовой пояс"}
	}
	return tz, nil
}

// isNameTaken — нарушение уникального индекса lower(name).
func isNameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == nameUniqueIdx
}
