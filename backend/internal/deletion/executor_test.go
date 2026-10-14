package deletion

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

var (
	testNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	// afterAppPurgeDelay — все запросы fakeQueue.add (requested_at = testNow + i с) уже доступны для claim.
	afterAppPurgeDelay = testNow.Add(AppPurgeDelay + time.Minute)
)

// fakeQueue — очередь deletion_requests в памяти с семантикой запросов deletion_requests.sql.
type fakeQueue struct {
	mu        sync.Mutex
	rows      []sqlc.DeletionRequest
	requeued  []time.Time // staleBefore каждого requeueStuck
	deleted   []uuid.UUID // приложения, удалённые deleteApp
	deleteRun []uuid.UUID // jobRunID каждого deleteApp
	claimErr  error
}

func (q *fakeQueue) add(subject sqlc.DeletionSubject, value string, appID uuid.UUID) uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	id := uuid.New()
	q.rows = append(q.rows, sqlc.DeletionRequest{
		ID: id, AppID: appID, SubjectType: subject, SubjectValue: value,
		Status: sqlc.DeletionStatusPending, RequestedAt: testNow.Add(time.Duration(len(q.rows)) * time.Second),
	})
	return id
}

func (q *fakeQueue) get(id uuid.UUID) sqlc.DeletionRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, r := range q.rows {
		if r.ID == id {
			return r
		}
	}
	return sqlc.DeletionRequest{}
}

func (q *fakeQueue) requeueStuck(_ context.Context, staleBefore time.Time) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.requeued = append(q.requeued, staleBefore)
	var n int64
	for i := range q.rows {
		r := &q.rows[i]
		if r.Status == sqlc.DeletionStatusRunning && r.StartedAt != nil && r.StartedAt.Before(staleBefore) {
			r.Status = sqlc.DeletionStatusPending
			if r.Attempts >= maxAttempts {
				r.Status = sqlc.DeletionStatusFailed
			}
			n++
		}
	}
	return n, nil
}

func (q *fakeQueue) claim(_ context.Context, now, appRequestedBefore time.Time) (sqlc.DeletionRequest, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.claimErr != nil {
		return sqlc.DeletionRequest{}, false, q.claimErr
	}
	for i := range q.rows {
		r := &q.rows[i]
		if r.SubjectType == sqlc.DeletionSubjectApp && r.RequestedAt.After(appRequestedBefore) {
			continue
		}
		if r.Status == sqlc.DeletionStatusPending {
			r.Status = sqlc.DeletionStatusRunning
			r.StartedAt = &now
			r.Attempts++
			return *r, true, nil
		}
	}
	return sqlc.DeletionRequest{}, false, nil
}

func (q *fakeQueue) complete(_ context.Context, id uuid.UUID, now time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.rows {
		if q.rows[i].ID == id {
			q.rows[i].Status = sqlc.DeletionStatusDone
			q.rows[i].CompletedAt = &now
			q.rows[i].Error = nil
		}
	}
	return nil
}

func (q *fakeQueue) fail(_ context.Context, id uuid.UUID, msg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.rows {
		r := &q.rows[i]
		if r.ID == id {
			r.Status = sqlc.DeletionStatusPending
			if r.Attempts >= maxAttempts {
				r.Status = sqlc.DeletionStatusFailed
			}
			r.Error = &msg
		}
	}
	return nil
}

func (q *fakeQueue) deleteApp(_ context.Context, dr sqlc.DeletionRequest, jobRunID uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.deleted = append(q.deleted, dr.AppID)
	q.deleteRun = append(q.deleteRun, jobRunID)
	q.rows = slices.DeleteFunc(q.rows, func(r sqlc.DeletionRequest) bool { return r.AppID == dr.AppID })
	return nil
}

// fakePurger записывает вызовы ClickHouse-части и возвращает заданные ошибки.
type fakePurger struct {
	mu       sync.Mutex
	calls    []string
	pingErr  error
	purgeErr error
}

func (p *fakePurger) record(call string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, call)
	return p.purgeErr
}

func (p *fakePurger) Ping(context.Context) error { return p.pingErr }

func (p *fakePurger) PurgeInstall(_ context.Context, appID, installID uuid.UUID) error {
	return p.record("install " + appID.String() + " " + installID.String())
}

func (p *fakePurger) PurgeUser(_ context.Context, appID uuid.UUID, userID string) error {
	return p.record("user " + appID.String() + " " + userID)
}

func (p *fakePurger) PurgeApp(_ context.Context, appID uuid.UUID) error {
	return p.record("app " + appID.String())
}

type failedCall struct {
	id    uuid.UUID
	cause string
}

func newTestExecutor(q *fakeQueue, p *fakePurger, c clock.Clock, failed *[]failedCall) *Executor {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return newExecutor(q, p, c, logger, func(_ context.Context, dr sqlc.DeletionRequest, cause string) {
		*failed = append(*failed, failedCall{id: dr.ID, cause: cause})
	})
}

// AC-046.3 / AC-6.2.3: ClickHouse отвечает ошибкой 5 циклов подряд — запрос failed, attempts = 5,
// error заполнен, уведомление о сбое — одно.
func TestDeletion_FailsAfterFiveAttempts(t *testing.T) {
	q := &fakeQueue{}
	id := q.add(sqlc.DeletionSubjectUser, "u-42", uuid.New())
	p := &fakePurger{purgeErr: errors.New("code: 241, memory limit exceeded")}
	c := clock.NewFixed(testNow)
	var failed []failedCall
	ex := newTestExecutor(q, p, c, &failed)

	for cycle := 1; cycle <= maxAttempts; cycle++ {
		res, err := ex.RunOnce(t.Context(), uuid.New())
		require.Error(t, err, "цикл %d", cycle)
		dr := q.get(id)
		require.Equal(t, int32(cycle), dr.Attempts)
		if cycle < maxAttempts {
			require.Equal(t, sqlc.DeletionStatusPending, dr.Status, "цикл %d", cycle)
			require.Equal(t, Result{Retried: 1}, res)
			require.Empty(t, failed)
		} else {
			require.Equal(t, Result{Failed: 1}, res)
		}
		c.Advance(time.Minute)
	}

	dr := q.get(id)
	require.Equal(t, sqlc.DeletionStatusFailed, dr.Status)
	require.Equal(t, int32(maxAttempts), dr.Attempts)
	require.NotNil(t, dr.Error)
	require.Contains(t, *dr.Error, "memory limit exceeded")
	require.Len(t, failed, 1)
	require.Equal(t, id, failed[0].id)
	require.Contains(t, failed[0].cause, "memory limit exceeded")

	// Шестой цикл: брать нечего, purger не вызывается.
	before := len(p.calls)
	res, err := ex.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{}, res)
	require.Len(t, p.calls, before)
}

func TestDeletion_ExecutesSubjects(t *testing.T) {
	appID := uuid.New()
	install := uuid.New()
	q := &fakeQueue{}
	installReq := q.add(sqlc.DeletionSubjectInstall, install.String(), appID)
	userReq := q.add(sqlc.DeletionSubjectUser, "a/b c", appID)
	c := clock.NewFixed(testNow)
	p := &fakePurger{}
	var failed []failedCall
	runID := uuid.New()

	res, err := newTestExecutor(q, p, c, &failed).RunOnce(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, Result{Done: 2}, res)
	require.Equal(t, []string{
		"install " + appID.String() + " " + install.String(),
		"user " + appID.String() + " a/b c",
	}, p.calls)
	for _, id := range []uuid.UUID{installReq, userReq} {
		dr := q.get(id)
		require.Equal(t, sqlc.DeletionStatusDone, dr.Status)
		require.Equal(t, testNow, *dr.CompletedAt)
		require.Equal(t, int32(1), dr.Attempts)
	}
	require.Empty(t, failed)
	require.Empty(t, q.deleted)
}

// subject_type=app: сначала ClickHouse, затем строка apps вместе с копией запроса в job_runs текущего цикла.
func TestDeletion_AppDeletesRowAfterClickHouse(t *testing.T) {
	appID := uuid.New()
	q := &fakeQueue{}
	q.add(sqlc.DeletionSubjectApp, appID.String(), appID)
	q.add(sqlc.DeletionSubjectUser, "u1", appID) // снимается каскадом вместе с приложением
	other := uuid.New()
	otherReq := q.add(sqlc.DeletionSubjectUser, "u2", other)
	p := &fakePurger{}
	var failed []failedCall
	runID := uuid.New()

	res, err := newTestExecutor(q, p, clock.NewFixed(afterAppPurgeDelay), &failed).RunOnce(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, Result{Done: 2}, res)
	require.Equal(t, []string{"app " + appID.String(), "user " + other.String() + " u2"}, p.calls)
	require.Equal(t, []uuid.UUID{appID}, q.deleted)
	require.Equal(t, []uuid.UUID{runID}, q.deleteRun)
	require.Equal(t, sqlc.DeletionStatusDone, q.get(otherReq).Status)
}

func TestDeletion_AppNotDeletedWhenClickHouseFails(t *testing.T) {
	appID := uuid.New()
	q := &fakeQueue{}
	id := q.add(sqlc.DeletionSubjectApp, appID.String(), appID)
	p := &fakePurger{purgeErr: errors.New("timeout")}
	var failed []failedCall

	_, err := newTestExecutor(q, p, clock.NewFixed(afterAppPurgeDelay), &failed).RunOnce(t.Context(), uuid.New())
	require.Error(t, err)
	require.Empty(t, q.deleted)
	require.Equal(t, sqlc.DeletionStatusPending, q.get(id).Status)
}

// Ревью: ingest держит снимок ключей KEY_CACHE_REFRESH и после архивации ещё принимает события
// приложения; purge раньше их вставки оставил бы строки без приложения. Запрос app берётся не раньше
// AppPurgeDelay после requested_at; запросы user/install той же очереди не ждут.
func TestDeletion_AppWaitsForPurgeDelay(t *testing.T) {
	appID := uuid.New()
	q := &fakeQueue{}
	appReq := q.add(sqlc.DeletionSubjectApp, appID.String(), appID)
	userReq := q.add(sqlc.DeletionSubjectUser, "u1", uuid.New())
	p := &fakePurger{}
	var failed []failedCall

	tooEarly := q.get(appReq).RequestedAt.Add(AppPurgeDelay - time.Second)
	res, err := newTestExecutor(q, p, clock.NewFixed(tooEarly), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{Done: 1}, res)
	require.Equal(t, sqlc.DeletionStatusPending, q.get(appReq).Status)
	require.Equal(t, sqlc.DeletionStatusDone, q.get(userReq).Status)
	require.Empty(t, q.deleted)

	ready := q.get(appReq).RequestedAt.Add(AppPurgeDelay)
	res, err = newTestExecutor(q, p, clock.NewFixed(ready), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{Done: 1}, res)
	require.Equal(t, []uuid.UUID{appID}, q.deleted)
}

// AppPurgeDelay не короче жизни снимка ключей ingest плюс незавершённые записи: ingest со снимком, загруженным
// до заявки (ключи ещё активны, Postgres недоступен для refresh), отвечает 503 после KeyCacheMaxAge от начала
// чтения снимка; заявка становится видна не позже apps.DeleteTimeout после requested_at (снимок, начатый в это
// окно, ключи ещё содержит), уже начатая вставка events длится не дольше InsertTimeout, строка rejected_events
// ждёт в буфере не дольше MaxAge и пишется не дольше InsertTimeout журнала; остаток — расхождение часов worker
// и ingest. Очередь async insert сервера purge сбрасывает сам (chPurger).
// Иначе purge обгонял бы последние записи, и строки удалённого приложения оставались бы в ClickHouse.
func TestDeletion_AppPurgeDelayCoversKeyCacheLifetime(t *testing.T) {
	const clockSkew = 30 * time.Second
	inFlight := apps.DeleteTimeout + ingest.DefaultInsertTimeout + chstore.DefaultRejectedLogMaxAge + chstore.DefaultRejectedLogInsertTimeout
	require.GreaterOrEqual(t, AppPurgeDelay, ingest.KeyCacheMaxAge+inFlight+clockSkew)
}

// Ошибка прерывает цикл: иначе тот же запрос снова встал бы первым в очереди и потратил все попытки за минуту.
func TestDeletion_StopsCycleOnError(t *testing.T) {
	q := &fakeQueue{}
	first := q.add(sqlc.DeletionSubjectUser, "u1", uuid.New())
	second := q.add(sqlc.DeletionSubjectUser, "u2", uuid.New())
	p := &fakePurger{purgeErr: errors.New("boom")}
	var failed []failedCall

	res, err := newTestExecutor(q, p, clock.NewFixed(testNow), &failed).RunOnce(t.Context(), uuid.New())
	require.ErrorContains(t, err, "boom")
	require.Equal(t, Result{Retried: 1}, res)
	require.Len(t, p.calls, 1)
	require.Equal(t, int32(1), q.get(first).Attempts)
	require.Equal(t, int32(0), q.get(second).Attempts)
	require.Equal(t, sqlc.DeletionStatusPending, q.get(second).Status)
}

// ClickHouse недоступен — запросы не берутся и попытки не тратятся.
func TestDeletion_SkipsWhenClickHouseUnavailable(t *testing.T) {
	q := &fakeQueue{}
	id := q.add(sqlc.DeletionSubjectUser, "u1", uuid.New())
	p := &fakePurger{pingErr: errors.New("connection refused")}
	var failed []failedCall

	res, err := newTestExecutor(q, p, clock.NewFixed(testNow), &failed).RunOnce(t.Context(), uuid.New())
	require.ErrorContains(t, err, "connection refused")
	require.Equal(t, Result{}, res)
	dr := q.get(id)
	require.Equal(t, sqlc.DeletionStatusPending, dr.Status)
	require.Equal(t, int32(0), dr.Attempts)
	require.Empty(t, p.calls)
}

func TestDeletion_PostgresClaimError(t *testing.T) {
	q := &fakeQueue{claimErr: errors.New("pg down")}
	q.add(sqlc.DeletionSubjectUser, "u1", uuid.New())
	var failed []failedCall

	_, err := newTestExecutor(q, &fakePurger{}, clock.NewFixed(testNow), &failed).RunOnce(t.Context(), uuid.New())
	require.ErrorContains(t, err, "pg down")
}

// Запрос, зависший в running дольше staleRunning (worker упал посреди удаления), возвращается в очередь.
func TestDeletion_RequeuesStuckRequests(t *testing.T) {
	q := &fakeQueue{}
	id := q.add(sqlc.DeletionSubjectUser, "u1", uuid.New())
	c := clock.NewFixed(testNow)
	var failed []failedCall
	ex := newTestExecutor(q, &fakePurger{}, c, &failed)

	// Первый claim «зависает»: имитируем падение worker-а сразу после взятия запроса.
	_, ok, err := q.claim(t.Context(), c.Now(), c.Now())
	require.NoError(t, err)
	require.True(t, ok)

	res, err := ex.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{}, res, "запрос running моложе staleRunning не трогается")
	require.Equal(t, []time.Time{testNow.Add(-staleRunning)}, q.requeued)

	c.Advance(staleRunning + time.Minute)
	res, err = ex.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{Requeued: 1, Done: 1}, res)
	require.Equal(t, sqlc.DeletionStatusDone, q.get(id).Status)
	require.Equal(t, int32(2), q.get(id).Attempts)
}

// AC-6.2.5: 50 запросов разбираются за минуты, за цикл — не больше batchSize.
func TestDeletion_DrainsQueue(t *testing.T) {
	q := &fakeQueue{}
	for i := range 50 {
		q.add(sqlc.DeletionSubjectUser, "u"+strconv.Itoa(i), uuid.New())
	}
	c := clock.NewFixed(testNow)
	p := &fakePurger{}
	var failed []failedCall
	ex := newTestExecutor(q, p, c, &failed)

	for range 60 {
		res, err := ex.RunOnce(t.Context(), uuid.New())
		require.NoError(t, err)
		require.LessOrEqual(t, res.Done, batchSize)
		c.Advance(time.Minute)
	}
	for _, r := range q.rows {
		require.Equal(t, sqlc.DeletionStatusDone, r.Status)
		require.Less(t, r.CompletedAt.Sub(r.RequestedAt), 24*time.Hour)
	}
	require.Len(t, p.calls, 50)
}

// Текст ошибки ClickHouse обрезается до maxErrorLen байт по границе символа.
func TestDeletion_TruncatesError(t *testing.T) {
	q := &fakeQueue{}
	id := q.add(sqlc.DeletionSubjectUser, "u1", uuid.New())
	p := &fakePurger{purgeErr: errors.New(strings.Repeat("ошибка ", 500))}
	var failed []failedCall

	_, err := newTestExecutor(q, p, clock.NewFixed(testNow), &failed).RunOnce(t.Context(), uuid.New())
	require.Error(t, err)
	msg := *q.get(id).Error
	require.LessOrEqual(t, len(msg), maxErrorLen)
	require.True(t, utf8.ValidString(msg))
	require.True(t, strings.HasSuffix(msg, "…"))
}

func TestRejectedNeedles(t *testing.T) {
	require.Equal(t, []string{`"u-42"`}, userNeedles("u-42"))
	require.Equal(t, []string{`"a\"b\\c"`}, userNeedles(`a"b\c`))
	require.Equal(t, []string{`"Юзер"`}, userNeedles("Юзер"))
	// JSONEncoder в Swift экранирует слеш: ищем обе записи; HTML-символы не экранируются.
	require.Equal(t, []string{`"a/b <x>"`, `"a\/b <x>"`}, userNeedles("a/b <x>"))
	// Ревью: сэмпл хранится в каноничном виде ingest.PayloadSample (encoding/json без HTML-экранирования),
	// поэтому первый needle совпадает и с user_id, который клиент прислал экранированным ("\u007531");
	// проверка самого сэмпла — ingest TestPayloadSample/canonical_escaping_for_deletion_needles.
	require.Equal(t, `"u31"`, userNeedles("u31")[0])

	ids := make([]uuid.UUID, 450)
	for i := range ids {
		ids[i] = uuid.New()
	}
	chunks := needleChunks(ids)
	require.Len(t, chunks, 3)
	total := 0
	for _, c := range chunks {
		require.LessOrEqual(t, len(c), maxNeedles)
		total += len(c)
	}
	require.Equal(t, 450, total)
	require.Equal(t, ids[0].String(), chunks[0][0])
	require.Empty(t, needleChunks(nil))
}
