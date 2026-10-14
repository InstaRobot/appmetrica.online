package merge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// maxErrorLen — байт текста ошибки в app_merges.error (его показывают API и баннер слияния).
const maxErrorLen = 1000

// errSourceGone — строки источника нет: переносить ключи некуда и неоткуда.
var errSourceGone = errors.New("merge: source app is gone")

// Result — итог запуска job app_merge (job_runs.details); нулевой — слияний в очереди нет.
type Result struct {
	MergeID uuid.UUID
	Status  sqlc.AppMergeStatus
	Step    sqlc.AppMergeStep
}

// OnFailed вызывается, когда шаг слияния упал и оно стало failed (алерт в Telegram); nil — без уведомления.
type OnFailed func(ctx context.Context, m sqlc.AppMerge, cause string)

// Option настраивает Executor.
type Option func(*Executor)

// WithKeysWait задаёт ожидание шага wait_keys вместо deletion.AppPurgeDelay (e2e-стенд и тесты).
func WithKeysWait(d time.Duration) Option {
	return func(e *Executor) { e.keysWait = d }
}

// store — Postgres-часть слияния; в unit-тестах подменяется.
type store interface {
	// claim — старейшее слияние pending или running; ok=false — очередь пуста.
	claim(ctx context.Context) (m sqlc.AppMerge, ok bool, err error)
	setStep(ctx context.Context, arg sqlc.SetAppMergeStepParams) error
	// moveKeys — шаг move_keys одной транзакцией, включая переход к wait_keys с keys_moved_at = now.
	moveKeys(ctx context.Context, m sqlc.AppMerge, now time.Time) error
	// reclose — шаг reclose одной транзакцией: переоткрытие closed_days цели и status done.
	reclose(ctx context.Context, m sqlc.AppMerge, now time.Time) error
	fail(ctx context.Context, id uuid.UUID, msg string) error
}

// Executor выполняет шаги слияния (FR-8): job app_merge worker-а.
type Executor struct {
	store    store
	clock    clock.Clock
	logger   *slog.Logger
	onFailed OnFailed
	keysWait time.Duration
	// stubSteps — шаги copy и purge_source переходят к следующему без работы (unit-тесты до T-MP-009);
	// без него они ждут: источник нельзя удалять, пока события не скопированы.
	stubSteps bool
}

func NewExecutor(store *pgstore.Store, c clock.Clock, logger *slog.Logger, onFailed OnFailed, opts ...Option) *Executor {
	return newExecutor(pgStore{store: store}, c, logger, onFailed, opts...)
}

func newExecutor(s store, c clock.Clock, logger *slog.Logger, onFailed OnFailed, opts ...Option) *Executor {
	e := &Executor{store: s, clock: c, logger: logger, onFailed: onFailed, keysWait: deletion.AppPurgeDelay}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// outcome — итог шага для лога: шаг выполнен, ждёт (повтор в следующем запуске) или упал.
type outcome string

const (
	outcomeOK      outcome = "ok"
	outcomeWaiting outcome = "waiting"
	outcomeFailed  outcome = "failed"
)

// RunOnce — один запуск job app_merge: берёт старейшее слияние pending или running и выполняет его шаги
// подряд, пока очередной шаг не ждёт или слияние не завершено. Слияния идут по одному в порядке создания:
// следующее начнётся, когда текущее завершится или упадёт. Ошибка шага делает слияние failed (onFailed);
// остановка worker-а (отмена ctx) — нет: слияние остаётся running и продолжится со своего шага.
func (e *Executor) RunOnce(ctx context.Context, _ uuid.UUID) (Result, error) {
	m, ok, err := e.store.claim(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("merge: claim: %w", err)
	}
	if !ok {
		return Result{}, nil
	}
	if m.Status == sqlc.AppMergeStatusPending {
		// Новое или повторённое слияние: running с сохранённого шага, started_at — при первом запуске.
		if err := e.store.setStep(ctx, sqlc.SetAppMergeStepParams{Step: m.Step, Now: e.clock.Now(), ID: m.ID}); err != nil {
			return result(m), fmt.Errorf("merge: start: %w", err)
		}
		m.Status = sqlc.AppMergeStatusRunning
	}
	for m.Status == sqlc.AppMergeStatusRunning {
		started := e.clock.Now()
		step := m.Step
		next, waiting, err := e.step(ctx, m, started)
		if err != nil {
			e.logStep(ctx, m, step, started, outcomeFailed, slog.String("error", err.Error()))
			return e.failed(ctx, m, err)
		}
		if waiting {
			e.logStep(ctx, m, step, started, outcomeWaiting)
			break
		}
		e.logStep(ctx, m, step, started, outcomeOK)
		m = next
	}
	return result(m), nil
}

// step выполняет текущий шаг m и возвращает слияние после него; waiting — шаг повторится в следующем запуске.
func (e *Executor) step(ctx context.Context, m sqlc.AppMerge, now time.Time) (next sqlc.AppMerge, waiting bool, err error) {
	switch m.Step {
	case sqlc.AppMergeStepMoveKeys:
		if err := e.store.moveKeys(ctx, m, now); err != nil {
			return m, false, err
		}
		m.Step, m.KeysMovedAt = sqlc.AppMergeStepWaitKeys, &now
		return m, false, nil
	case sqlc.AppMergeStepWaitKeys:
		// Ingest принимает события по снимку ключей до deletion.AppPurgeDelay после переноса: копировать раньше —
		// значит оставить в источнике события, принятые по старому снимку уже после копии.
		if m.KeysMovedAt == nil {
			return m, false, errors.New("merge: keys_moved_at is not set")
		}
		if now.Sub(*m.KeysMovedAt) < e.keysWait {
			return m, true, nil
		}
		return e.advance(ctx, m, sqlc.AppMergeStepCopy, now)
	case sqlc.AppMergeStepCopy:
		if !e.stubSteps {
			return m, true, nil
		}
		return e.advance(ctx, m, sqlc.AppMergeStepPurgeSource, now)
	case sqlc.AppMergeStepPurgeSource:
		if !e.stubSteps {
			return m, true, nil
		}
		return e.advance(ctx, m, sqlc.AppMergeStepReclose, now)
	case sqlc.AppMergeStepReclose:
		if err := e.store.reclose(ctx, m, now); err != nil {
			return m, false, err
		}
		m.Status, m.Step, m.FinishedAt = sqlc.AppMergeStatusDone, sqlc.AppMergeStepDone, &now
		return m, false, nil
	default:
		return m, false, fmt.Errorf("merge: unexpected step %q", m.Step)
	}
}

func (e *Executor) advance(ctx context.Context, m sqlc.AppMerge, step sqlc.AppMergeStep, now time.Time) (sqlc.AppMerge, bool, error) {
	if err := e.store.setStep(ctx, sqlc.SetAppMergeStepParams{Step: step, Now: now, ID: m.ID}); err != nil {
		return m, false, fmt.Errorf("merge: set step %s: %w", step, err)
	}
	m.Step = step
	return m, false, nil
}

// failed записывает ошибку шага: status failed с текстом и onFailed. Отмена ctx остановкой worker-а слияние
// не роняет: оно остаётся running и продолжится в следующем запуске.
func (e *Executor) failed(ctx context.Context, m sqlc.AppMerge, cause error) (Result, error) {
	if errors.Is(ctx.Err(), context.Canceled) {
		return result(m), cause
	}
	msg := truncate(cause.Error(), maxErrorLen)
	// Таймаут запуска уже отменил ctx: запись итога от него не зависит.
	if err := e.store.fail(context.WithoutCancel(ctx), m.ID, msg); err != nil {
		return result(m), errors.Join(cause, fmt.Errorf("merge: record failure: %w", err))
	}
	m.Status, m.Error = sqlc.AppMergeStatusFailed, &msg
	if e.onFailed != nil {
		e.onFailed(context.WithoutCancel(ctx), m, msg)
	}
	return result(m), cause
}

// logStep — событие merge.step (05-impact-and-regression.md §7.2).
func (e *Executor) logStep(ctx context.Context, m sqlc.AppMerge, step sqlc.AppMergeStep, started time.Time, out outcome, attrs ...slog.Attr) {
	level := slog.LevelInfo
	if out == outcomeFailed {
		level = slog.LevelWarn
	}
	source := ""
	if m.SourceAppID != nil {
		source = m.SourceAppID.String()
	}
	e.logger.LogAttrs(ctx, level, "merge step", append([]slog.Attr{
		slog.String("event", "merge.step"),
		slog.String("merge_id", m.ID.String()),
		slog.String("target_app_id", m.TargetAppID.String()),
		slog.String("source_app_id", source),
		slog.String("step", string(step)),
		slog.Int64("duration_ms", e.clock.Now().Sub(started).Milliseconds()),
		slog.String("outcome", string(out)),
	}, attrs...)...)
}

func result(m sqlc.AppMerge) Result {
	return Result{MergeID: m.ID, Status: m.Status, Step: m.Step}
}

// defaultKeyPlatform — платформа для ключей источника без платформы: единственная платформа источника,
// иначе nil (ключ остаётся «любая платформа», ingest берёт платформу из context).
func defaultKeyPlatform(platforms []sqlc.AppPlatform) *sqlc.AppPlatform {
	if len(platforms) != 1 {
		return nil
	}
	return &platforms[0]
}

// truncate обрезает s до max байт по границе символа, отмечая обрезку «…».
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	const ellipsis = "…"
	s = s[:maxBytes-len(ellipsis)]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + ellipsis
}

// pgStore — store поверх sqlc-запросов merges.sql, apps.sql, keys.sql и ops.sql.
type pgStore struct{ store *pgstore.Store }

func (s pgStore) claim(ctx context.Context) (sqlc.AppMerge, bool, error) {
	m, err := s.store.Queries().ClaimNextAppMerge(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.AppMerge{}, false, nil
	case err != nil:
		return sqlc.AppMerge{}, false, err
	}
	return m, true, nil
}

func (s pgStore) setStep(ctx context.Context, arg sqlc.SetAppMergeStepParams) error {
	return s.store.Queries().SetAppMergeStep(ctx, arg)
}

// moveKeys блокирует обе строки apps (как Create, в порядке id), переносит ключи источника в цель, архивирует
// источник и расширяет цель его платформами и created_at. Платформы и created_at берутся из текущей строки
// источника, а не из снимка заявки: после заявки их могли изменить.
func (s pgStore) moveKeys(ctx context.Context, m sqlc.AppMerge, now time.Time) error {
	if m.SourceAppID == nil {
		return errSourceGone
	}
	return s.store.InTx(ctx, func(q *sqlc.Queries) error {
		_, source, err := lockPair(ctx, q, m.TargetAppID, *m.SourceAppID)
		switch {
		case errors.Is(err, ErrAppNotFound):
			return errSourceGone
		case err != nil:
			return err
		}
		if _, err := q.MoveWriteKeys(ctx, sqlc.MoveWriteKeysParams{
			TargetID: m.TargetAppID, SourceID: source.ID, DefaultPlatform: defaultKeyPlatform(source.Platforms),
		}); err != nil {
			return fmt.Errorf("merge: move write keys: %w", err)
		}
		// Источник мог быть в архиве ещё до заявки: тогда строка не меняется.
		if _, err := q.ArchiveApp(ctx, sqlc.ArchiveAppParams{Now: &now, ID: source.ID}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("merge: archive source: %w", err)
		}
		if _, err := q.AbsorbAppForMerge(ctx, sqlc.AbsorbAppForMergeParams{
			SourceCreatedAt: source.CreatedAt, Now: now, TargetID: m.TargetAppID, SourcePlatforms: source.Platforms,
		}); err != nil {
			return fmt.Errorf("merge: absorb source into target: %w", err)
		}
		if err := q.SetAppMergeStep(ctx, sqlc.SetAppMergeStepParams{
			Step: sqlc.AppMergeStepWaitKeys, Now: now, KeysMovedAt: &now, ID: m.ID,
		}); err != nil {
			return fmt.Errorf("merge: set step wait_keys: %w", err)
		}
		return nil
	})
}

// reclose удаляет closed_days цели с first_source_day (job close_day закроет их заново с событиями источника)
// и завершает слияние. Без событий источника first_source_day пуст и переоткрывать нечего.
func (s pgStore) reclose(ctx context.Context, m sqlc.AppMerge, now time.Time) error {
	return s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if m.FirstSourceDay.Valid {
			if _, err := q.ReopenClosedDays(ctx, sqlc.ReopenClosedDaysParams{AppID: m.TargetAppID, FromDay: m.FirstSourceDay}); err != nil {
				return fmt.Errorf("merge: reopen closed days: %w", err)
			}
		}
		if err := q.FinishAppMerge(ctx, sqlc.FinishAppMergeParams{Now: now, ID: m.ID}); err != nil {
			return fmt.Errorf("merge: finish: %w", err)
		}
		return nil
	})
}

func (s pgStore) fail(ctx context.Context, id uuid.UUID, msg string) error {
	return s.store.Queries().FailAppMerge(ctx, sqlc.FailAppMergeParams{Error: msg, ID: id})
}
