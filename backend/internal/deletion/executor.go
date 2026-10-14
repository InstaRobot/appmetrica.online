package deletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	// maxAttempts — попыток на запрос; совпадает с FailDeletionRequest и CHECK deletion_requests_attempts_chk.
	maxAttempts = 5
	// batchSize — запросов за цикл job deletion (02-tech-spec.md FR-019: LIMIT 10).
	batchSize = 10
	// staleRunning — запрос running дольше этого считается брошенным упавшим worker-ом.
	// Мутации ClickHouse с mutations_sync=2 укладываются в минуты, job ограничен таймаутом меньше часа.
	staleRunning = time.Hour
	// maxErrorLen — байт текста ошибки в deletion_requests.error (его показывают API и S-12).
	maxErrorLen = 1000
	// AppPurgeDelay — запрос subject_type=app берётся не раньше этого срока после requested_at. Вместе с
	// заявкой приложение архивируется и его write-ключи отзываются, но ingest работает по снимку ключей:
	// обычно он перезагружается раз в KEY_CACHE_REFRESH (15 с), а при недоступном Postgres живёт до
	// ingest.KeyCacheMaxAge (15 мин) от начала его загрузки, после чего ingest отвечает 503 и ничего не пишет.
	// Заявка становится видна ingest (commit) не позже apps.DeleteTimeout после requested_at, который берётся
	// под блокировкой строки apps; снимок, начатый до commit, ещё принимает события приложения (вставка
	// синхронная, не дольше ingest.DefaultInsertTimeout) и ставит отказы в буфер rejected_events, где строка
	// живёт не дольше chstore.DefaultRejectedLogMaxAge и пишется не дольше chstore.DefaultRejectedLogInsertTimeout;
	// purge раньше последней такой записи оставил бы в ClickHouse строки удалённого приложения без второй
	// попытки очистки. Поэтому задержка — KeyCacheMaxAge плюс эти пределы плюс запас на расхождение часов
	// worker и ingest (executor_test проверяет неравенство; константа не общая, чтобы worker не зависел от
	// ingest). Блоки, которые сервер ClickHouse принял в очередь async insert, но ещё не записал (клиент
	// отвалился по таймауту раньше ack), в задержку не входят: purge сбрасывает очередь сам (chPurger).
	// Ключи, выпущенные после заявки, и возврат из архива в снимок не попадают (LoadWriteKeyCache, 409
	// deletion_pending). Запросы user и install не ждут: их данные могут приходить и дальше по определению.
	AppPurgeDelay = 17 * time.Minute
)

// Purger — ClickHouse-часть удаления (05-data-model.md, job deletion). Реализация — NewClickHousePurger.
type Purger interface {
	Ping(ctx context.Context) error
	PurgeInstall(ctx context.Context, appID, installID uuid.UUID) error
	PurgeUser(ctx context.Context, appID uuid.UUID, userID string) error
	PurgeApp(ctx context.Context, appID uuid.UUID) error
}

// Result — итог цикла job deletion (job_runs.details).
type Result struct {
	Requeued int64 // зависших running возвращено в очередь
	Done     int   // выполнено
	Retried  int   // ошибка, запрос вернулся в pending
	Failed   int   // ошибка последней попытки, запрос failed
}

// queue — deletion_requests в Postgres; в unit-тестах подменяется.
type queue interface {
	requeueStuck(ctx context.Context, staleBefore time.Time) (int64, error)
	// claim берёт старейший pending (FOR UPDATE SKIP LOCKED) и переводит в running; запросы app —
	// только с requested_at ≤ appRequestedBefore; запросы приложений в незавершённом слиянии (app_merges
	// pending/running/failed) пропускаются до его конца; ok=false — брать нечего.
	claim(ctx context.Context, now, appRequestedBefore time.Time) (dr sqlc.DeletionRequest, ok bool, err error)
	complete(ctx context.Context, id uuid.UUID, now time.Time) error
	fail(ctx context.Context, id uuid.UUID, msg string) error
	// deleteApp в одной транзакции копирует запрос (completed_at уже выставлен) в job_runs.details цикла
	// jobRunID и удаляет строку apps.
	deleteApp(ctx context.Context, dr sqlc.DeletionRequest, jobRunID uuid.UUID) error
}

// Executor исполняет запросы на удаление (FR-019): job deletion worker-а.
type Executor struct {
	queue    queue
	purger   Purger
	clock    clock.Clock
	logger   *slog.Logger
	onFailed OnFailed
}

// OnFailed вызывается, когда запрос исчерпал попытки и стал failed (алерт в Telegram); nil — без уведомления.
type OnFailed func(ctx context.Context, dr sqlc.DeletionRequest, cause string)

func NewExecutor(store *pgstore.Store, purger Purger, c clock.Clock, logger *slog.Logger, onFailed OnFailed) *Executor {
	return newExecutor(pgQueue{store: store}, purger, c, logger, onFailed)
}

func newExecutor(q queue, purger Purger, c clock.Clock, logger *slog.Logger, onFailed OnFailed) *Executor {
	return &Executor{queue: q, purger: purger, clock: c, logger: logger, onFailed: onFailed}
}

// RunOnce — один цикл job deletion: при доступном ClickHouse возвращает в очередь зависшие запросы
// и выполняет до batchSize pending по requested_at. Первая ошибка прерывает цикл, иначе тот же запрос
// снова оказался бы первым в очереди и потратил все попытки за минуту. jobRunID — строка job_runs цикла.
func (e *Executor) RunOnce(ctx context.Context, jobRunID uuid.UUID) (Result, error) {
	var res Result
	// ClickHouse недоступен — запросы не берутся, попытки не тратятся (03-architecture.md, сбои).
	if err := e.purger.Ping(ctx); err != nil {
		return res, fmt.Errorf("deletion: clickhouse unavailable: %w", err)
	}
	n, err := e.queue.requeueStuck(ctx, e.clock.Now().Add(-staleRunning))
	if err != nil {
		return res, fmt.Errorf("deletion: requeue stuck requests: %w", err)
	}
	res.Requeued = n
	for range batchSize {
		now := e.clock.Now()
		dr, ok, err := e.queue.claim(ctx, now, now.Add(-AppPurgeDelay))
		if err != nil {
			return res, fmt.Errorf("deletion: claim request: %w", err)
		}
		if !ok {
			return res, nil
		}
		if err := e.execute(ctx, dr, jobRunID); err != nil {
			return res, e.failed(ctx, dr, err, &res)
		}
		res.Done++
		e.log(ctx, dr, "deletion done")
	}
	return res, nil
}

// execute удаляет данные субъекта: сначала ClickHouse, затем отметка в Postgres. Для приложения запрос
// не помечается done — строку apps и с ней запрос снимает deleteApp.
func (e *Executor) execute(ctx context.Context, dr sqlc.DeletionRequest, jobRunID uuid.UUID) error {
	switch dr.SubjectType {
	case sqlc.DeletionSubjectInstall:
		id, err := uuid.Parse(dr.SubjectValue)
		if err != nil {
			return fmt.Errorf("deletion: install_id: %w", err)
		}
		if err := e.purger.PurgeInstall(ctx, dr.AppID, id); err != nil {
			return err
		}
	case sqlc.DeletionSubjectUser:
		if err := e.purger.PurgeUser(ctx, dr.AppID, dr.SubjectValue); err != nil {
			return err
		}
	case sqlc.DeletionSubjectApp:
		if err := e.purger.PurgeApp(ctx, dr.AppID); err != nil {
			return err
		}
		now := e.clock.Now()
		dr.CompletedAt = &now
		return e.queue.deleteApp(ctx, dr, jobRunID)
	default:
		return fmt.Errorf("deletion: unknown subject_type %q", dr.SubjectType)
	}
	return e.queue.complete(ctx, dr.ID, e.clock.Now())
}

// failed записывает ошибку попытки: pending до maxAttempts, затем failed и onFailed.
func (e *Executor) failed(ctx context.Context, dr sqlc.DeletionRequest, cause error, res *Result) error {
	msg := truncate(cause.Error(), maxErrorLen)
	// Остановка worker-а посреди удаления: попытка всё равно записывается, запрос не ждёт staleRunning.
	if err := e.queue.fail(context.WithoutCancel(ctx), dr.ID, msg); err != nil {
		return errors.Join(cause, fmt.Errorf("deletion: record failure: %w", err))
	}
	if dr.Attempts < maxAttempts {
		res.Retried++
		e.log(ctx, dr, "deletion attempt failed", slog.String("error", msg))
		return cause
	}
	res.Failed++
	e.log(ctx, dr, "deletion failed", slog.String("error", msg))
	if e.onFailed != nil {
		e.onFailed(ctx, dr, msg)
	}
	return cause
}

// log — subject_value (user_id, install_id) не пишется.
func (e *Executor) log(ctx context.Context, dr sqlc.DeletionRequest, msg string, attrs ...slog.Attr) {
	level := slog.LevelInfo
	if len(attrs) > 0 {
		level = slog.LevelWarn
	}
	e.logger.LogAttrs(ctx, level, msg, append([]slog.Attr{
		slog.String("app_id", dr.AppID.String()), slog.String("deletion_request_id", dr.ID.String()),
		slog.String("subject_type", string(dr.SubjectType)), slog.Int("attempts", int(dr.Attempts)),
	}, attrs...)...)
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

// pgQueue — queue поверх sqlc-запросов deletion_requests.sql и ops.sql.
type pgQueue struct{ store *pgstore.Store }

func (q pgQueue) requeueStuck(ctx context.Context, staleBefore time.Time) (int64, error) {
	return q.store.Queries().RequeueStuckDeletionRequests(ctx, staleBefore)
}

func (q pgQueue) claim(ctx context.Context, now, appRequestedBefore time.Time) (sqlc.DeletionRequest, bool, error) {
	dr, err := q.store.Queries().ClaimNextDeletionRequest(ctx, sqlc.ClaimNextDeletionRequestParams{
		Now: &now, AppRequestedBefore: appRequestedBefore,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.DeletionRequest{}, false, nil
	case err != nil:
		return sqlc.DeletionRequest{}, false, err
	}
	return dr, true, nil
}

func (q pgQueue) complete(ctx context.Context, id uuid.UUID, now time.Time) error {
	return q.store.Queries().CompleteDeletionRequest(ctx, sqlc.CompleteDeletionRequestParams{Now: &now, ID: id})
}

func (q pgQueue) fail(ctx context.Context, id uuid.UUID, msg string) error {
	return q.store.Queries().FailDeletionRequest(ctx, sqlc.FailDeletionRequestParams{Error: &msg, ID: id})
}

// deletedApp — копия запроса subject_type app в job_runs.details.deleted_apps: строка запроса исчезает
// вместе с приложением, а журнал удалений хранится бессрочно (05-data-model.md §2.4).
type deletedApp struct {
	ID           uuid.UUID  `json:"id"`
	AppID        uuid.UUID  `json:"app_id"`
	SubjectType  string     `json:"subject_type"`
	SubjectValue string     `json:"subject_value"`
	Source       string     `json:"source"`
	Status       string     `json:"status"`
	APIKeyID     *uuid.UUID `json:"api_key_id"`
	RequestedAt  time.Time  `json:"requested_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	Attempts     int32      `json:"attempts"`
}

func (q pgQueue) deleteApp(ctx context.Context, dr sqlc.DeletionRequest, jobRunID uuid.UUID) error {
	copyJSON, err := json.Marshal(deletedApp{
		ID: dr.ID, AppID: dr.AppID, SubjectType: string(dr.SubjectType), SubjectValue: dr.SubjectValue,
		Source: string(dr.Source), Status: string(sqlc.DeletionStatusDone), APIKeyID: dr.ApiKeyID,
		RequestedAt: dr.RequestedAt, StartedAt: dr.StartedAt, CompletedAt: dr.CompletedAt, Attempts: dr.Attempts,
	})
	if err != nil {
		return fmt.Errorf("deletion: encode request copy: %w", err)
	}
	return q.store.InTx(ctx, func(tx *sqlc.Queries) error {
		if err := tx.AppendDeletedAppToJobRun(ctx, sqlc.AppendDeletedAppToJobRunParams{Request: copyJSON, ID: jobRunID}); err != nil {
			return fmt.Errorf("deletion: copy request to job_runs: %w", err)
		}
		if _, err := tx.DeleteApp(ctx, dr.AppID); err != nil {
			return fmt.Errorf("deletion: delete app: %w", err)
		}
		return nil
	})
}
