// Package merge — слияние приложения-источника с целью (FR-8): предпросмотр, заявка в app_merges и повтор;
// шаги выполняет job worker app_merge.
package merge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// pgUniqueViolation — частичный уникальный индекс активных слияний: параллельная заявка успела раньше.
const pgUniqueViolation = "23505"

// Service — заявки на слияние приложений.
type Service struct {
	store  *pgstore.Store
	ch     driver.Conn
	clock  clock.Clock
	logger *slog.Logger
}

func NewService(store *pgstore.Store, ch driver.Conn, c clock.Clock, logger *slog.Logger) *Service {
	return &Service{store: store, ch: ch, clock: c, logger: logger}
}

// Partition — месячная партиция событий источника (toYYYYMM(event_date)).
type Partition struct {
	ID       uint32
	Rows     uint64
	FirstDay time.Time // первый день событий партиции в часовом поясе источника
	LastDay  time.Time
}

// Preview — объём переноса из источника в цель.
type Preview struct {
	Target     sqlc.App
	Source     sqlc.App
	Partitions []Partition
	Events     uint64
	// FirstDay и LastDay — первый и последний день событий источника в его часовом поясе; nil без событий.
	FirstDay  *time.Time
	LastDay   *time.Time
	WriteKeys int // активные ключи источника
}

// CreateInput — тело createAppMerge.
type CreateInput struct {
	SourceAppID uuid.UUID
	ConfirmName string
}

// Preview считает события источника по партициям в ClickHouse и его активные ключи в Postgres.
func (s *Service) Preview(ctx context.Context, targetID, sourceID uuid.UUID) (Preview, error) {
	if err := distinctApps(targetID, sourceID); err != nil {
		return Preview{}, err
	}
	q := s.store.Queries()
	var p Preview
	var err error
	if p.Target, err = getApp(ctx, q, targetID); err != nil {
		return Preview{}, err
	}
	if p.Source, err = getApp(ctx, q, sourceID); err != nil {
		return Preview{}, err
	}
	if p.Partitions, err = s.partitions(ctx, p.Source); err != nil {
		return Preview{}, err
	}
	for i, part := range p.Partitions {
		p.Events += part.Rows
		if i == 0 || part.FirstDay.Before(*p.FirstDay) {
			p.FirstDay = &part.FirstDay
		}
		if i == 0 || part.LastDay.After(*p.LastDay) {
			p.LastDay = &part.LastDay
		}
	}
	keys, err := q.ListWriteKeys(ctx, sourceID)
	if err != nil {
		return Preview{}, fmt.Errorf("merge: list write keys: %w", err)
	}
	for _, k := range keys {
		if k.RevokedAt == nil {
			p.WriteKeys++
		}
	}
	return p, nil
}

// Create ставит слияние источника в цель в очередь worker-а (status pending, step move_keys). Обе строки apps
// блокируются до конца транзакции, как в apps.Service: архивация, удаление и новые ключи ждут и затем видят
// активное слияние. Проверки: заявок на удаление нет, цель не в архиве, ни одно из приложений не участвует в
// другом слиянии pending/running/failed, confirmName совпадает с именем источника. Объём источника
// (total_events) считается в ClickHouse до транзакции: он справочный, и держать строки apps заблокированными
// на время запроса к ClickHouse незачем.
func (s *Service) Create(ctx context.Context, targetID uuid.UUID, in CreateInput) (sqlc.AppMerge, error) {
	// Strict-сервер обязательные поля тела не проверяет: без них приходят нулевые значения.
	switch {
	case in.SourceAppID == uuid.Nil:
		return sqlc.AppMerge{}, &ValidationError{Field: "source_app_id", Message: "Выберите приложение для объединения"}
	case in.ConfirmName == "":
		return sqlc.AppMerge{}, &ValidationError{Field: "confirm_name", Message: "Введите название приложения-источника"}
	}
	if err := distinctApps(targetID, in.SourceAppID); err != nil {
		return sqlc.AppMerge{}, err
	}
	source, err := getApp(ctx, s.store.Queries(), in.SourceAppID)
	if err != nil {
		return sqlc.AppMerge{}, err
	}
	parts, err := s.partitions(ctx, source)
	if err != nil {
		return sqlc.AppMerge{}, err
	}
	var total uint64
	for _, p := range parts {
		total += p.Rows
	}
	var m sqlc.AppMerge
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		target, source, err := lockPair(ctx, q, targetID, in.SourceAppID)
		if err != nil {
			return err
		}
		for _, id := range []uuid.UUID{targetID, in.SourceAppID} {
			pending, err := q.AppDeletionPending(ctx, id)
			if err != nil {
				return fmt.Errorf("merge: deletion pending: %w", err)
			}
			if pending {
				return ErrDeletionPending
			}
		}
		if target.Status == sqlc.AppStatusArchived {
			return ErrAppArchived
		}
		for _, id := range []uuid.UUID{targetID, in.SourceAppID} {
			if err := noActiveMerge(ctx, q, id); err != nil {
				return err
			}
		}
		if in.ConfirmName != source.Name {
			return ErrConfirmNameMismatch
		}
		m, err = q.CreateAppMerge(ctx, sqlc.CreateAppMergeParams{
			TargetAppID:     targetID,
			SourceAppID:     &source.ID,
			SourceName:      source.Name,
			SourcePlatforms: source.Platforms,
			SourceTimezone:  source.Timezone,
			TotalEvents:     int64(total), //nolint:gosec // число строк событий одного приложения далеко от 2^63
		})
		var pgErr *pgconn.PgError
		switch {
		case errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation:
			return ErrMergeInProgress
		case err != nil:
			return fmt.Errorf("merge: create: %w", err)
		}
		return nil
	})
	if err != nil {
		return sqlc.AppMerge{}, err
	}
	s.logger.InfoContext(ctx, "merge created",
		slog.String("merge_id", m.ID.String()), slog.String("target_app_id", targetID.String()),
		slog.String("source_app_id", in.SourceAppID.String()), slog.Int64("total_events", m.TotalEvents))
	return m, nil
}

// List — до 20 слияний, где приложение — цель, новые первыми; нет приложения — ErrAppNotFound.
func (s *Service) List(ctx context.Context, targetID uuid.UUID) ([]sqlc.AppMerge, error) {
	q := s.store.Queries()
	if _, err := getApp(ctx, q, targetID); err != nil {
		return nil, err
	}
	list, err := q.ListAppMerges(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("merge: list: %w", err)
	}
	return list, nil
}

// Retry возвращает упавшее слияние в очередь (status pending, error NULL, attempts+1); шаг и скопированные
// партиции сохраняются, worker продолжает с них. Слияние не в статусе failed — ErrMergeInProgress.
func (s *Service) Retry(ctx context.Context, targetID, mergeID uuid.UUID) (sqlc.AppMerge, error) {
	q := s.store.Queries()
	m, err := q.RetryAppMerge(ctx, sqlc.RetryAppMergeParams{ID: mergeID, TargetAppID: targetID})
	if err == nil {
		s.logger.InfoContext(ctx, "merge retried",
			slog.String("merge_id", m.ID.String()), slog.String("step", string(m.Step)), slog.Int("attempts", int(m.Attempts)))
		return m, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.AppMerge{}, fmt.Errorf("merge: retry: %w", err)
	}
	_, err = q.GetAppMerge(ctx, sqlc.GetAppMergeParams{ID: mergeID, TargetAppID: targetID})
	switch {
	case err == nil:
		return sqlc.AppMerge{}, ErrMergeInProgress
	case !errors.Is(err, pgx.ErrNoRows):
		return sqlc.AppMerge{}, fmt.Errorf("merge: get: %w", err)
	}
	if _, err := getApp(ctx, q, targetID); err != nil {
		return sqlc.AppMerge{}, err
	}
	return sqlc.AppMerge{}, ErrNotFound
}

// partitions — партиции событий источника по возрастанию; дни — в часовом поясе источника.
func (s *Service) partitions(ctx context.Context, source sqlc.App) ([]Partition, error) {
	return chCopier{conn: s.ch}.partitions(ctx, source.ID, source.Timezone)
}

func distinctApps(targetID, sourceID uuid.UUID) error {
	if targetID == sourceID {
		return &ValidationError{Field: "source_app_id", Message: "Выберите другое приложение: нельзя объединить приложение с самим собой"}
	}
	return nil
}

func getApp(ctx context.Context, q *sqlc.Queries, id uuid.UUID) (sqlc.App, error) {
	app, err := q.GetApp(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.App{}, ErrAppNotFound
	case err != nil:
		return sqlc.App{}, fmt.Errorf("merge: get app: %w", err)
	}
	return app, nil
}

// lockPair блокирует строки цели и источника в порядке id: встречные заявки T←S и S←T не взаимоблокируются.
func lockPair(ctx context.Context, q *sqlc.Queries, targetID, sourceID uuid.UUID) (target, source sqlc.App, err error) {
	ids := []uuid.UUID{targetID, sourceID}
	if bytes.Compare(sourceID[:], targetID[:]) < 0 {
		ids[0], ids[1] = sourceID, targetID
	}
	for _, id := range ids {
		app, err := q.GetAppForUpdate(ctx, id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return sqlc.App{}, sqlc.App{}, ErrAppNotFound
		case err != nil:
			return sqlc.App{}, sqlc.App{}, fmt.Errorf("merge: lock app: %w", err)
		}
		if id == targetID {
			target = app
		} else {
			source = app
		}
	}
	return target, source, nil
}

// noActiveMerge — ErrMergeInProgress, если приложение — цель или источник слияния pending/running/failed.
func noActiveMerge(ctx context.Context, q *sqlc.Queries, id uuid.UUID) error {
	_, err := q.ActiveMergeForApp(ctx, id)
	switch {
	case err == nil:
		return ErrMergeInProgress
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	default:
		return fmt.Errorf("merge: active merge: %w", err)
	}
}
