package merge

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

var execNow = time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)

// fakeStore — одно слияние в памяти; ошибки шагов задаются полями *Err.
type fakeStore struct {
	merge    *sqlc.AppMerge // nil — очередь пуста
	claimErr error
	moveErr  error
	reclErr  error
	failErr  error

	calls  []string
	failed []string // тексты FailAppMerge
}

func (s *fakeStore) claim(context.Context) (sqlc.AppMerge, bool, error) {
	s.calls = append(s.calls, "claim")
	if s.claimErr != nil {
		return sqlc.AppMerge{}, false, s.claimErr
	}
	if s.merge == nil || s.merge.Status == sqlc.AppMergeStatusDone || s.merge.Status == sqlc.AppMergeStatusFailed {
		return sqlc.AppMerge{}, false, nil
	}
	return *s.merge, true, nil
}

func (s *fakeStore) setStep(_ context.Context, arg sqlc.SetAppMergeStepParams) error {
	s.calls = append(s.calls, "set_step:"+string(arg.Step))
	s.merge.Status = sqlc.AppMergeStatusRunning
	s.merge.Step = arg.Step
	if s.merge.StartedAt == nil {
		s.merge.StartedAt = &arg.Now
	}
	if arg.KeysMovedAt != nil {
		s.merge.KeysMovedAt = arg.KeysMovedAt
	}
	if arg.FirstSourceDay.Valid {
		s.merge.FirstSourceDay = arg.FirstSourceDay
	}
	return nil
}

func (s *fakeStore) moveKeys(ctx context.Context, _ sqlc.AppMerge, now time.Time) error {
	s.calls = append(s.calls, "move_keys")
	if s.moveErr != nil {
		return s.moveErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.setStep(ctx, sqlc.SetAppMergeStepParams{Step: sqlc.AppMergeStepWaitKeys, Now: now, KeysMovedAt: &now})
}

func (s *fakeStore) reclose(_ context.Context, _ sqlc.AppMerge, now time.Time) error {
	s.calls = append(s.calls, "reclose")
	if s.reclErr != nil {
		return s.reclErr
	}
	s.merge.Status, s.merge.Step, s.merge.FinishedAt = sqlc.AppMergeStatusDone, sqlc.AppMergeStepDone, &now
	return nil
}

func (s *fakeStore) fail(_ context.Context, _ uuid.UUID, msg string) error {
	s.calls = append(s.calls, "fail")
	if s.failErr != nil {
		return s.failErr
	}
	s.failed = append(s.failed, msg)
	s.merge.Status, s.merge.Error = sqlc.AppMergeStatusFailed, &msg
	return nil
}

type failedCall struct {
	mergeID uuid.UUID
	step    sqlc.AppMergeStep
	cause   string
}

func pendingMerge() *sqlc.AppMerge {
	source := uuid.New()
	return &sqlc.AppMerge{
		ID: uuid.New(), TargetAppID: uuid.New(), SourceAppID: &source, SourceName: "Notes Android",
		SourcePlatforms: []sqlc.AppPlatform{sqlc.AppPlatformAndroid}, SourceTimezone: "UTC",
		Status: sqlc.AppMergeStatusPending, Step: sqlc.AppMergeStepMoveKeys, CreatedAt: execNow.Add(-time.Minute),
	}
}

func newTestExecutor(s *fakeStore, c clock.Clock, failed *[]failedCall, opts ...Option) *Executor {
	onFailed := func(_ context.Context, m sqlc.AppMerge, cause string) {
		*failed = append(*failed, failedCall{m.ID, m.Step, cause})
	}
	return newExecutor(s, c, slog.New(slog.DiscardHandler), onFailed, opts...)
}

func TestRunOnceIdle(t *testing.T) {
	s := &fakeStore{}
	var failed []failedCall
	res, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{}, res)
	require.Equal(t, []string{"claim"}, s.calls)
	require.Empty(t, failed)
}

// AC-008.1 на фейке: move_keys переводит слияние в wait_keys с keys_moved_at; ожидание кэша ещё не прошло.
func TestMoveKeysThenWaits(t *testing.T) {
	s := &fakeStore{merge: pendingMerge()}
	var failed []failedCall
	res, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, []string{"claim", "set_step:move_keys", "move_keys", "set_step:wait_keys"}, s.calls)
	require.Equal(t, Result{MergeID: s.merge.ID, Status: sqlc.AppMergeStatusRunning, Step: sqlc.AppMergeStepWaitKeys}, res)
	require.Equal(t, execNow, *s.merge.KeysMovedAt)
	require.Equal(t, execNow, *s.merge.StartedAt)
	require.Empty(t, failed)
}

// AC-008.2: wait_keys ждёт deletion.AppPurgeDelay от keys_moved_at, затем переходит к copy.
func TestWaitKeysStep(t *testing.T) {
	for _, tc := range []struct {
		name  string
		moved time.Duration // keys_moved_at = now − moved
		step  sqlc.AppMergeStep
		calls []string
	}{
		{"waits 5 minutes after the move", 5 * time.Minute, sqlc.AppMergeStepWaitKeys, []string{"claim"}},
		{"waits a second before the delay", deletion.AppPurgeDelay - time.Second, sqlc.AppMergeStepWaitKeys, []string{"claim"}},
		{"copies at the delay", deletion.AppPurgeDelay, sqlc.AppMergeStepCopy, []string{"claim", "set_step:copy"}},
		{"copies 18 minutes after the move", 18 * time.Minute, sqlc.AppMergeStepCopy, []string{"claim", "set_step:copy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := pendingMerge()
			moved := execNow.Add(-tc.moved)
			m.Status, m.Step, m.KeysMovedAt, m.StartedAt = sqlc.AppMergeStatusRunning, sqlc.AppMergeStepWaitKeys, &moved, &moved
			s := &fakeStore{merge: m}
			var failed []failedCall
			res, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
			require.NoError(t, err)
			require.Equal(t, tc.step, s.merge.Step)
			require.Equal(t, tc.step, res.Step)
			require.Equal(t, sqlc.AppMergeStatusRunning, res.Status)
			require.Equal(t, tc.calls, s.calls)
			require.Empty(t, failed)
		})
	}
}

// Задержку ожидания задаёт WithKeysWait (e2e-стенд и интеграционные тесты — 0).
func TestWithKeysWait(t *testing.T) {
	s := &fakeStore{merge: pendingMerge()}
	var failed []failedCall
	_, err := newTestExecutor(s, clock.NewFixed(execNow), &failed, WithKeysWait(0)).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStepCopy, s.merge.Step, "без ожидания — сразу к копированию")
}

// Шаги copy и purge_source до их реализации стоят на месте: источник не удаляется без копии событий.
func TestCopyStepNotImplementedWaits(t *testing.T) {
	for _, step := range []sqlc.AppMergeStep{sqlc.AppMergeStepCopy, sqlc.AppMergeStepPurgeSource} {
		t.Run(string(step), func(t *testing.T) {
			m := pendingMerge()
			m.Status, m.Step = sqlc.AppMergeStatusRunning, step
			s := &fakeStore{merge: m}
			var failed []failedCall
			res, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
			require.NoError(t, err)
			require.Equal(t, step, res.Step)
			require.Equal(t, []string{"claim"}, s.calls)
		})
	}
}

// Полный цикл на фейке: шаги идут подряд в одном запуске, пока ни один не ждёт.
func TestRunOnceRunsStepsToDone(t *testing.T) {
	s := &fakeStore{merge: pendingMerge()}
	var failed []failedCall
	e := newTestExecutor(s, clock.NewFixed(execNow), &failed, WithKeysWait(0))
	e.stubSteps = true
	res, err := e.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{MergeID: s.merge.ID, Status: sqlc.AppMergeStatusDone, Step: sqlc.AppMergeStepDone}, res)
	require.Equal(t, []string{
		"claim", "set_step:move_keys", "move_keys", "set_step:wait_keys",
		"set_step:copy", "set_step:purge_source", "set_step:reclose", "reclose",
	}, s.calls)
	require.Equal(t, execNow, *s.merge.FinishedAt)

	// Завершённое слияние больше не берётся.
	s.calls = nil
	res, err = e.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{}, res)
	require.Equal(t, []string{"claim"}, s.calls)
	require.Empty(t, failed)
}

// После «Повторить» слияние pending продолжает с сохранённого шага и снова становится running.
func TestRunOnceResumesRetriedStep(t *testing.T) {
	m := pendingMerge()
	m.Step = sqlc.AppMergeStepReclose
	m.FirstSourceDay = pgtype.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	s := &fakeStore{merge: m}
	var failed []failedCall
	res, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, []string{"claim", "set_step:reclose", "reclose"}, s.calls)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
}

// Ошибка шага: FailAppMerge с текстом, onFailed ровно один раз, запуск job — с ошибкой.
func TestStepFailureMarksMergeFailed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		step  sqlc.AppMergeStep
		setup func(s *fakeStore)
	}{
		{"move_keys", sqlc.AppMergeStepMoveKeys, func(s *fakeStore) { s.moveErr = errors.New("pg down") }},
		{"reclose", sqlc.AppMergeStepReclose, func(s *fakeStore) { s.reclErr = errors.New("pg down") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := pendingMerge()
			m.Status, m.Step = sqlc.AppMergeStatusRunning, tc.step
			s := &fakeStore{merge: m}
			tc.setup(s)
			var failed []failedCall
			res, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
			require.ErrorContains(t, err, "pg down")
			require.Equal(t, sqlc.AppMergeStatusFailed, res.Status)
			require.Equal(t, tc.step, res.Step)
			require.Len(t, s.failed, 1)
			require.Contains(t, s.failed[0], "pg down")
			require.Equal(t, []failedCall{{m.ID, tc.step, s.failed[0]}}, failed)
		})
	}
}

// Длинный текст ошибки обрезается до maxErrorLen байт по границе символа.
func TestStepFailureTruncatesError(t *testing.T) {
	m := pendingMerge()
	m.Status = sqlc.AppMergeStatusRunning
	s := &fakeStore{merge: m, moveErr: errors.New(strings.Repeat("ошибка ", 300))}
	var failed []failedCall
	_, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.Error(t, err)
	require.LessOrEqual(t, len(s.failed[0]), maxErrorLen)
	require.True(t, strings.HasSuffix(s.failed[0], "…"))
}

// Не удалось записать failed — обе ошибки в результате, алерта нет: слияние останется running и продолжится.
func TestStepFailureNotRecorded(t *testing.T) {
	m := pendingMerge()
	m.Status = sqlc.AppMergeStatusRunning
	s := &fakeStore{merge: m, moveErr: errors.New("tx aborted"), failErr: errors.New("pg gone")}
	var failed []failedCall
	_, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.ErrorContains(t, err, "tx aborted")
	require.ErrorContains(t, err, "pg gone")
	require.Empty(t, failed)
}

// Остановка worker-а посреди шага не делает слияние failed: оно остаётся running и продолжается при следующем
// запуске, а не ждёт «Повторить».
func TestCanceledRunKeepsMergeRunning(t *testing.T) {
	m := pendingMerge()
	m.Status = sqlc.AppMergeStatusRunning
	s := &fakeStore{merge: m}
	var failed []failedCall
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(ctx, uuid.New())
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, s.calls, "fail")
	require.Equal(t, sqlc.AppMergeStatusRunning, s.merge.Status)
	require.Empty(t, failed)
}

// Таймаут запуска — сбой шага: слияние failed, владелец получает алерт.
func TestDeadlineMarksMergeFailed(t *testing.T) {
	m := pendingMerge()
	m.Status = sqlc.AppMergeStatusRunning
	s := &fakeStore{merge: m, moveErr: context.DeadlineExceeded}
	var failed []failedCall
	_, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, sqlc.AppMergeStatusFailed, s.merge.Status)
	require.Len(t, failed, 1)
}

func TestClaimError(t *testing.T) {
	s := &fakeStore{claimErr: errors.New("pg down")}
	var failed []failedCall
	_, err := newTestExecutor(s, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.ErrorContains(t, err, "pg down")
	require.Empty(t, failed)
}

// Ключ без платформы получает платформу источника, только если она у источника одна.
func TestDefaultKeyPlatform(t *testing.T) {
	android := sqlc.AppPlatformAndroid
	require.Equal(t, &android, defaultKeyPlatform([]sqlc.AppPlatform{android}))
	require.Nil(t, defaultKeyPlatform([]sqlc.AppPlatform{sqlc.AppPlatformIos, android}))
	require.Nil(t, defaultKeyPlatform(nil))
}
