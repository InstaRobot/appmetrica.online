package merge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// maxErrorLen — байт текста ошибки в app_merges.error (его показывают API и баннер слияния).
const maxErrorLen = 1000

// slowStep — шаг дольше таймаута запуска job app_merge: предупреждение merge.slow в логе.
const slowStep = 30 * time.Minute

// CloseDayLockKey — advisory lock запуска job close_day (worker): шаг reclose берёт его на время переоткрытия
// дней цели, а close_day под ним читает последний закрытый день и закрывает следующие.
const CloseDayLockKey = "close_day"

// errSourceGone — строки источника нет: переносить ключи некуда и неоткуда.
var errSourceGone = errors.New("merge: source app is gone")

// errCloseDayBusy — идёт запуск close_day: шаг reclose повторится в следующем запуске job.
var errCloseDayBusy = errors.New("merge: close_day is running")

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
	// appTimezone — часовой пояс приложения (first_source_day считается в поясе цели).
	appTimezone(ctx context.Context, id uuid.UUID) (string, error)
	// setTotalEvents — объём источника, пересчитанный шагом copy (прогресс «перенесено N из M» в баннере).
	setTotalEvents(ctx context.Context, id uuid.UUID, total int64) error
	addCopiedPartition(ctx context.Context, arg sqlc.AddCopiedPartitionParams) error
	// purgeSource — одной транзакцией: ключи Read API и заявки на удаление человека или установки источника
	// переходят в цель, копия слияния пишется в job_runs.details запуска runID, строка apps источника
	// удаляется, шаг — reclose.
	purgeSource(ctx context.Context, m sqlc.AppMerge, runID uuid.UUID, now time.Time) error
	// reclose — шаг reclose одной транзакцией под advisory lock close_day: переоткрытие closed_days цели и
	// status done; errCloseDayBusy — lock держит запуск close_day.
	reclose(ctx context.Context, m sqlc.AppMerge, now time.Time) error
	fail(ctx context.Context, id uuid.UUID, msg string) error
}

// appPurger — удаление данных приложения из ClickHouse (deletion.Purger).
type appPurger interface {
	PurgeApp(ctx context.Context, appID uuid.UUID) error
}

// Executor выполняет шаги слияния (FR-8): job app_merge worker-а.
type Executor struct {
	store    store
	copier   copier
	purger   appPurger
	clock    clock.Clock
	logger   *slog.Logger
	onFailed OnFailed
	keysWait time.Duration
	metrics  *metrics
}

// NewExecutor — исполнитель поверх Postgres и соединения ClickHouse рабочего пользователя (копия событий
// и удаление источника через deletion.Purger).
func NewExecutor(store *pgstore.Store, ch driver.Conn, c clock.Clock, logger *slog.Logger, onFailed OnFailed, opts ...Option) *Executor {
	return newExecutor(pgStore{store: store}, chCopier{conn: ch}, deletion.NewClickHousePurger(ch), c, logger, onFailed, opts...)
}

func newExecutor(s store, cp copier, p appPurger, c clock.Clock, logger *slog.Logger, onFailed OnFailed, opts ...Option) *Executor {
	e := &Executor{
		store: s, copier: cp, purger: p, clock: c, logger: logger, onFailed: onFailed,
		keysWait: deletion.AppPurgeDelay, metrics: newMetrics(),
	}
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
func (e *Executor) RunOnce(ctx context.Context, runID uuid.UUID) (Result, error) {
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
		next, waiting, err := e.step(ctx, m, runID, started)
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
// runID — запуск job, в job_runs.details которого purge_source записывает удалённый источник.
func (e *Executor) step(ctx context.Context, m sqlc.AppMerge, runID uuid.UUID, now time.Time) (next sqlc.AppMerge, waiting bool, err error) {
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
		m, err := e.copy(ctx, m, now)
		return m, false, err
	case sqlc.AppMergeStepPurgeSource:
		m, err := e.purgeSource(ctx, m, runID, now)
		return m, false, err
	case sqlc.AppMergeStepReclose:
		err := e.store.reclose(ctx, m, now)
		switch {
		case errors.Is(err, errCloseDayBusy):
			return m, true, nil
		case err != nil:
			return m, false, err
		}
		m.Status, m.Step, m.FinishedAt = sqlc.AppMergeStatusDone, sqlc.AppMergeStepDone, &now
		return m, false, nil
	default:
		return m, false, fmt.Errorf("merge: unexpected step %q", m.Step)
	}
}

// copy сбрасывает очередь async insert ClickHouse, копирует партиции источника, которых нет в copied_partitions,
// отмечая каждую после копии, затем журнал отказов, и переходит к purge_source с first_source_day — первым днём
// событий источника в часовом поясе цели. Сбой оставляет скопированные партиции в copied_partitions: повтор
// продолжит со следующей.
func (e *Executor) copy(ctx context.Context, m sqlc.AppMerge, now time.Time) (sqlc.AppMerge, error) {
	if m.SourceAppID == nil {
		return m, errSourceGone
	}
	source := *m.SourceAppID
	// Блок, принятый сервером в очередь до конца wait_keys, но ещё не записанный, иначе не попал бы в партиции:
	// его записал бы сброс в PurgeApp — и тут же удалил вместе с источником.
	if err := e.copier.flush(ctx); err != nil {
		return m, fmt.Errorf("merge: flush async inserts: %w", err)
	}
	tz, err := e.store.appTimezone(ctx, m.TargetAppID)
	if err != nil {
		return m, fmt.Errorf("merge: target timezone: %w", err)
	}
	parts, err := e.copier.partitions(ctx, source, tz)
	if err != nil {
		return m, err
	}
	// total_events снят при заявке, а за wait_keys источник ещё принимал события: без пересчёта прогресс
	// показал бы «перенесено больше, чем всего».
	var total int64
	for _, p := range parts {
		total += int64(p.Rows) //nolint:gosec // строк в партиции одного приложения далеко до 2^63
	}
	if total != m.TotalEvents {
		if err := e.store.setTotalEvents(ctx, m.ID, total); err != nil {
			return m, fmt.Errorf("merge: refresh total events: %w", err)
		}
		m.TotalEvents = total
	}
	for _, p := range parts {
		id := int32(p.ID) //nolint:gosec // YYYYMM
		if slices.Contains(m.CopiedPartitions, id) {
			continue
		}
		if err := e.copier.copyPartition(ctx, source, m.TargetAppID, p.ID); err != nil {
			return m, fmt.Errorf("merge: copy partition %d: %w", p.ID, err)
		}
		rows := int64(p.Rows) //nolint:gosec // строк в партиции одного приложения далеко до 2^63
		if err := e.store.addCopiedPartition(ctx, sqlc.AddCopiedPartitionParams{Partition: id, Rows: rows, ID: m.ID}); err != nil {
			return m, fmt.Errorf("merge: record partition %d: %w", p.ID, err)
		}
		m.CopiedPartitions = append(m.CopiedPartitions, id)
		m.CopiedEvents += rows
		e.metrics.copiedEvents.Add(float64(p.Rows))
	}
	if err := e.copier.copyRejected(ctx, source, m.TargetAppID); err != nil {
		return m, err
	}
	var first pgtype.Date
	for _, p := range parts {
		if !first.Valid || p.FirstDay.Before(first.Time) {
			first = pgtype.Date{Time: p.FirstDay, Valid: true}
		}
	}
	if err := e.store.setStep(ctx, sqlc.SetAppMergeStepParams{
		Step: sqlc.AppMergeStepPurgeSource, Now: now, FirstSourceDay: first, ID: m.ID,
	}); err != nil {
		return m, fmt.Errorf("merge: set step %s: %w", sqlc.AppMergeStepPurgeSource, err)
	}
	m.Step = sqlc.AppMergeStepPurgeSource
	if first.Valid {
		m.FirstSourceDay = first
	}
	return m, nil
}

// purgeSource удаляет данные источника из ClickHouse (повтор безопасен: ALTER … DELETE по app_id), затем строку
// apps источника; app_merges.source_app_id обнуляется внешним ключом.
func (e *Executor) purgeSource(ctx context.Context, m sqlc.AppMerge, runID uuid.UUID, now time.Time) (sqlc.AppMerge, error) {
	if m.SourceAppID == nil {
		return m, errSourceGone
	}
	if err := e.purger.PurgeApp(ctx, *m.SourceAppID); err != nil {
		return m, fmt.Errorf("merge: purge source: %w", err)
	}
	if err := e.store.purgeSource(ctx, m, runID, now); err != nil {
		return m, err
	}
	m.SourceAppID, m.Step = nil, sqlc.AppMergeStepReclose
	return m, nil
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

// logStep — событие merge.step.
func (e *Executor) logStep(ctx context.Context, m sqlc.AppMerge, step sqlc.AppMergeStep, started time.Time, out outcome, attrs ...slog.Attr) {
	level := slog.LevelInfo
	if out == outcomeFailed {
		level = slog.LevelWarn
	}
	source := ""
	if m.SourceAppID != nil {
		source = m.SourceAppID.String()
	}
	duration := e.clock.Now().Sub(started)
	if out != outcomeWaiting {
		e.metrics.stepDuration.WithLabelValues(string(step)).Observe(duration.Seconds())
	}
	ids := []slog.Attr{
		slog.String("merge_id", m.ID.String()),
		slog.String("target_app_id", m.TargetAppID.String()),
		slog.String("source_app_id", source),
		slog.String("step", string(step)),
		slog.Int64("duration_ms", duration.Milliseconds()),
	}
	e.logger.LogAttrs(ctx, level, "merge step", slices.Concat(
		[]slog.Attr{slog.String("event", "merge.step")}, ids, []slog.Attr{slog.String("outcome", string(out))}, attrs)...)
	if step == sqlc.AppMergeStepCopy && duration > slowStep {
		e.logger.LogAttrs(ctx, slog.LevelWarn, "merge step slow", append([]slog.Attr{slog.String("event", "merge.slow")}, ids...)...)
	}
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

// addsPlatforms — есть ли у источника платформа, которой нет у цели (AbsorbAppForMerge расширит набор цели).
func addsPlatforms(target, source []sqlc.AppPlatform) bool {
	for _, p := range source {
		if !slices.Contains(target, p) {
			return true
		}
	}
	return false
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
// источника, а не из снимка заявки: после заявки их могли изменить. Ключи цели без платформы до расширения
// набора получают её единственную платформу: ingest ставил её их событиям как единственную платформу
// приложения, а с набором из нескольких писал бы other с platform_missing (диалог слияния обещает, что ключи
// работают как раньше).
func (s pgStore) moveKeys(ctx context.Context, m sqlc.AppMerge, now time.Time) error {
	if m.SourceAppID == nil {
		return errSourceGone
	}
	return s.store.InTx(ctx, func(q *sqlc.Queries) error {
		target, source, err := lockPair(ctx, q, m.TargetAppID, *m.SourceAppID)
		switch {
		case errors.Is(err, ErrAppNotFound):
			return errSourceGone
		case err != nil:
			return err
		}
		if p := defaultKeyPlatform(target.Platforms); p != nil && addsPlatforms(target.Platforms, source.Platforms) {
			if _, err := q.DefaultWriteKeyPlatform(ctx, sqlc.DefaultWriteKeyPlatformParams{Platform: *p, AppID: target.ID}); err != nil {
				return fmt.Errorf("merge: default target key platform: %w", err)
			}
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
// и завершает слияние. Без событий источника first_source_day пуст и переоткрывать нечего. Транзакция идёт под
// advisory lock close_day: его запуск читает последний закрытый день и затем записывает следующие — переоткрытие
// посреди запуска оставило бы дни с first_source_day до прочитанного незакрытыми навсегда (следующий запуск начал
// бы после записанных). Идёт запуск — errCloseDayBusy, шаг повторится через минуту; запуск close_day, пришедший
// во время шага, пропускает тик.
func (s pgStore) reclose(ctx context.Context, m sqlc.AppMerge, now time.Time) error {
	release, ok, err := s.store.TryAdvisoryLock(ctx, CloseDayLockKey)
	if err != nil {
		return fmt.Errorf("merge: lock close_day: %w", err)
	}
	if !ok {
		return errCloseDayBusy
	}
	defer release()
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

func (s pgStore) appTimezone(ctx context.Context, id uuid.UUID) (string, error) {
	app, err := s.store.Queries().GetApp(ctx, id)
	if err != nil {
		return "", err
	}
	return app.Timezone, nil
}

func (s pgStore) setTotalEvents(ctx context.Context, id uuid.UUID, total int64) error {
	return s.store.Queries().SetAppMergeTotalEvents(ctx, sqlc.SetAppMergeTotalEventsParams{TotalEvents: total, ID: id})
}

func (s pgStore) addCopiedPartition(ctx context.Context, arg sqlc.AddCopiedPartitionParams) error {
	return s.store.Queries().AddCopiedPartition(ctx, arg)
}

// mergedApp — копия слияния в job_runs.details.merged_apps: строка источника удаляется, а журнал запусков
// хранит, какое приложение и куда было объединено.
type mergedApp struct {
	MergeID         uuid.UUID          `json:"merge_id"`
	TargetAppID     uuid.UUID          `json:"target_app_id"`
	SourceAppID     uuid.UUID          `json:"source_app_id"`
	SourceName      string             `json:"source_name"`
	SourcePlatforms []sqlc.AppPlatform `json:"source_platforms"`
	SourceTimezone  string             `json:"source_timezone"`
	CopiedEvents    int64              `json:"copied_events"`
	TotalEvents     int64              `json:"total_events"`
	CreatedAt       time.Time          `json:"created_at"`
	DeletedAt       time.Time          `json:"deleted_at"`
}

func (s pgStore) purgeSource(ctx context.Context, m sqlc.AppMerge, runID uuid.UUID, now time.Time) error {
	copyJSON, err := json.Marshal(mergedApp{
		MergeID: m.ID, TargetAppID: m.TargetAppID, SourceAppID: *m.SourceAppID, SourceName: m.SourceName,
		SourcePlatforms: m.SourcePlatforms, SourceTimezone: m.SourceTimezone, CopiedEvents: m.CopiedEvents,
		TotalEvents: m.TotalEvents, CreatedAt: m.CreatedAt, DeletedAt: now,
	})
	if err != nil {
		return fmt.Errorf("merge: encode merge copy: %w", err)
	}
	return s.store.InTx(ctx, func(q *sqlc.Queries) error {
		// Строка источника под блокировкой до конца транзакции: ключ Read API или заявка на удаление, которые
		// вставляют в источник параллельно (INSERT ждёт KEY SHARE строки apps), либо успевают под перенос ниже,
		// либо после удаления строки получают app not found — а не исчезают каскадом, уже ответив 201/202.
		if _, err := q.GetAppForUpdate(ctx, *m.SourceAppID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errSourceGone
			}
			return fmt.Errorf("merge: lock source app: %w", err)
		}
		if err := q.AppendMergedAppToJobRun(ctx, sqlc.AppendMergedAppToJobRunParams{Merge: copyJSON, ID: runID}); err != nil {
			return fmt.Errorf("merge: copy merge to job_runs: %w", err)
		}
		// Удаление строки apps каскадом снесло бы ключи Read API источника и заявки на удаление его людей:
		// ключи должны работать дальше (диалог слияния это обещает), заявки — выполниться в цели.
		if _, err := q.MoveAPIKeysForMerge(ctx, sqlc.MoveAPIKeysForMergeParams{TargetID: m.TargetAppID, SourceID: *m.SourceAppID}); err != nil {
			return fmt.Errorf("merge: move api keys: %w", err)
		}
		if _, err := q.MoveDeletionRequestsForMerge(ctx, sqlc.MoveDeletionRequestsForMergeParams{TargetID: m.TargetAppID, SourceID: *m.SourceAppID}); err != nil {
			return fmt.Errorf("merge: move deletion requests: %w", err)
		}
		if _, err := q.DeleteApp(ctx, *m.SourceAppID); err != nil {
			return fmt.Errorf("merge: delete source app: %w", err)
		}
		if err := q.SetAppMergeStep(ctx, sqlc.SetAppMergeStepParams{Step: sqlc.AppMergeStepReclose, Now: now, ID: m.ID}); err != nil {
			return fmt.Errorf("merge: set step reclose: %w", err)
		}
		return nil
	})
}

func (s pgStore) fail(ctx context.Context, id uuid.UUID, msg string) error {
	return s.store.Queries().FailAppMerge(ctx, sqlc.FailAppMergeParams{Error: msg, ID: id})
}
