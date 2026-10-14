package merge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
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
	addErr   error
	purgeErr error
	tz       string // часовой пояс цели; "" — UTC

	calls  []string
	failed []string    // тексты FailAppMerge
	runIDs []uuid.UUID // запуски job, записавшие удалённый источник
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

func (s *fakeStore) appTimezone(context.Context, uuid.UUID) (string, error) {
	s.calls = append(s.calls, "timezone")
	if s.tz == "" {
		return "UTC", nil
	}
	return s.tz, nil
}

func (s *fakeStore) addCopiedPartition(_ context.Context, arg sqlc.AddCopiedPartitionParams) error {
	s.calls = append(s.calls, fmt.Sprintf("add_partition:%d", arg.Partition))
	if s.addErr != nil {
		return s.addErr
	}
	s.merge.CopiedPartitions = append(s.merge.CopiedPartitions, arg.Partition)
	s.merge.CopiedEvents += arg.Rows
	return nil
}

func (s *fakeStore) purgeSource(ctx context.Context, _ sqlc.AppMerge, runID uuid.UUID, now time.Time) error {
	s.calls = append(s.calls, "purge_source")
	if s.purgeErr != nil {
		return s.purgeErr
	}
	s.runIDs = append(s.runIDs, runID)
	s.merge.SourceAppID = nil
	return s.setStep(ctx, sqlc.SetAppMergeStepParams{Step: sqlc.AppMergeStepReclose, Now: now})
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

// fakeCopier — партиции источника в памяти; ошибки копирования задаются по партиции.
type fakeCopier struct {
	parts    []Partition
	partsErr error
	copyErr  map[uint32]error
	rejErr   error
	// onCopy вызывается после каждой успешной копии партиции (сдвиг часов в тестах).
	onCopy func(partition uint32)

	calls []string
}

func (c *fakeCopier) partitions(_ context.Context, _ uuid.UUID, tz string) ([]Partition, error) {
	c.calls = append(c.calls, "partitions:"+tz)
	return c.parts, c.partsErr
}

func (c *fakeCopier) copyPartition(_ context.Context, _, _ uuid.UUID, partition uint32) error {
	c.calls = append(c.calls, fmt.Sprintf("copy:%d", partition))
	if err := c.copyErr[partition]; err != nil {
		return err
	}
	if c.onCopy != nil {
		c.onCopy(partition)
	}
	return nil
}

func (c *fakeCopier) copyRejected(context.Context, uuid.UUID, uuid.UUID) error {
	c.calls = append(c.calls, "rejected")
	return c.rejErr
}

type fakePurger struct {
	err    error
	purged []uuid.UUID
}

func (p *fakePurger) PurgeApp(_ context.Context, appID uuid.UUID) error {
	if p.err != nil {
		return p.err
	}
	p.purged = append(p.purged, appID)
	return nil
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// sourceParts — три месячные партиции источника: 400 + 400 + 10 событий с 2026-08-10 по 2026-10-02.
func sourceParts() []Partition {
	return []Partition{
		{ID: 202608, Rows: 400, FirstDay: day(2026, 8, 10), LastDay: day(2026, 8, 26)},
		{ID: 202609, Rows: 400, FirstDay: day(2026, 9, 4), LastDay: day(2026, 9, 21)},
		{ID: 202610, Rows: 10, FirstDay: day(2026, 10, 1), LastDay: day(2026, 10, 2)},
	}
}

func runningAt(step sqlc.AppMergeStep) *sqlc.AppMerge {
	m := pendingMerge()
	m.Status, m.Step = sqlc.AppMergeStatusRunning, step
	return m
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
	return newTestExecutorWith(s, &fakeCopier{}, &fakePurger{}, c, failed, opts...)
}

func newTestExecutorWith(s *fakeStore, cp *fakeCopier, p *fakePurger, c clock.Clock, failed *[]failedCall, opts ...Option) *Executor {
	onFailed := func(_ context.Context, m sqlc.AppMerge, cause string) {
		*failed = append(*failed, failedCall{m.ID, m.Step, cause})
	}
	return newExecutor(s, cp, p, c, slog.New(slog.DiscardHandler), onFailed, opts...)
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

// Полный цикл на фейке: шаги идут подряд в одном запуске, пока ни один не ждёт.
func TestRunOnceRunsStepsToDone(t *testing.T) {
	s := &fakeStore{merge: pendingMerge()}
	cp := &fakeCopier{parts: sourceParts()}
	p := &fakePurger{}
	var failed []failedCall
	e := newTestExecutorWith(s, cp, p, clock.NewFixed(execNow), &failed, WithKeysWait(0))
	runID := uuid.New()
	source := *s.merge.SourceAppID
	res, err := e.RunOnce(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, Result{MergeID: s.merge.ID, Status: sqlc.AppMergeStatusDone, Step: sqlc.AppMergeStepDone}, res)
	require.Equal(t, []string{
		"claim", "set_step:move_keys", "move_keys", "set_step:wait_keys", "set_step:copy",
		"timezone", "add_partition:202608", "add_partition:202609", "add_partition:202610", "set_step:purge_source",
		"purge_source", "set_step:reclose", "reclose",
	}, s.calls)
	require.Equal(t, []string{"partitions:UTC", "copy:202608", "copy:202609", "copy:202610", "rejected"}, cp.calls)
	require.Equal(t, []uuid.UUID{source}, p.purged)
	require.Equal(t, []uuid.UUID{runID}, s.runIDs)
	require.Equal(t, execNow, *s.merge.FinishedAt)
	require.Equal(t, int64(810), s.merge.CopiedEvents)
	require.Equal(t, pgtype.Date{Time: day(2026, 8, 10), Valid: true}, s.merge.FirstSourceDay)
	require.Nil(t, s.merge.SourceAppID)

	// Завершённое слияние больше не берётся.
	s.calls = nil
	res, err = e.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Result{}, res)
	require.Equal(t, []string{"claim"}, s.calls)
	require.Empty(t, failed)
}

// copy пропускает партиции из copied_partitions; first_source_day — по всем партициям источника в часовом поясе
// цели, включая скопированные раньше.
func TestCopyStepSkipsCopiedPartitions(t *testing.T) {
	m := runningAt(sqlc.AppMergeStepCopy)
	m.CopiedPartitions, m.CopiedEvents = []int32{202608}, 400
	s := &fakeStore{merge: m, tz: "Europe/Moscow"}
	cp := &fakeCopier{parts: sourceParts()}
	var failed []failedCall
	res, err := newTestExecutorWith(s, cp, &fakePurger{}, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	require.Equal(t, []string{"partitions:Europe/Moscow", "copy:202609", "copy:202610", "rejected"}, cp.calls)
	require.Equal(t, []int32{202608, 202609, 202610}, s.merge.CopiedPartitions)
	require.Equal(t, int64(810), s.merge.CopiedEvents)
	require.Equal(t, pgtype.Date{Time: day(2026, 8, 10), Valid: true}, s.merge.FirstSourceDay)
	require.Empty(t, failed)
}

// Источник без событий: копировать нечего, first_source_day не задан — reclose ничего не переоткроет.
func TestCopyStepEmptySource(t *testing.T) {
	s := &fakeStore{merge: runningAt(sqlc.AppMergeStepCopy)}
	cp := &fakeCopier{}
	var failed []failedCall
	res, err := newTestExecutorWith(s, cp, &fakePurger{}, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	require.Equal(t, []string{"partitions:UTC", "rejected"}, cp.calls)
	require.NotContains(t, s.calls, "add_partition")
	require.False(t, s.merge.FirstSourceDay.Valid)
	require.Zero(t, s.merge.CopiedEvents)
	require.Empty(t, failed)
}

// Сбой на второй партиции: failed на шаге copy, первая партиция записана в copied_partitions, повтор продолжит
// со второй.
func TestCopyStepFailureKeepsProgress(t *testing.T) {
	s := &fakeStore{merge: runningAt(sqlc.AppMergeStepCopy)}
	cp := &fakeCopier{parts: sourceParts(), copyErr: map[uint32]error{202609: errors.New("clickhouse: connection closed")}}
	p := &fakePurger{}
	var failed []failedCall
	e := newTestExecutorWith(s, cp, p, clock.NewFixed(execNow), &failed)
	res, err := e.RunOnce(t.Context(), uuid.New())
	require.ErrorContains(t, err, "connection closed")
	require.Equal(t, Result{MergeID: s.merge.ID, Status: sqlc.AppMergeStatusFailed, Step: sqlc.AppMergeStepCopy}, res)
	require.Equal(t, []string{"partitions:UTC", "copy:202608", "copy:202609"}, cp.calls)
	require.Equal(t, []int32{202608}, s.merge.CopiedPartitions)
	require.Contains(t, s.failed[0], "202609")
	require.Len(t, failed, 1)
	require.Empty(t, p.purged, "источник не удаляется без полной копии")

	// «Повторить»: ClickHouse снова доступен, копируются только оставшиеся партиции.
	s.merge.Status = sqlc.AppMergeStatusPending
	cp.copyErr, cp.calls = nil, nil
	res, err = e.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	require.Equal(t, []string{"partitions:UTC", "copy:202609", "copy:202610", "rejected"}, cp.calls)
	require.Equal(t, []int32{202608, 202609, 202610}, s.merge.CopiedPartitions)
}

// Ошибки шагов copy и purge_source делают слияние failed; источник удаляется только после PurgeApp.
func TestCopyAndPurgeFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		step  sqlc.AppMergeStep
		setup func(m *sqlc.AppMerge, s *fakeStore, cp *fakeCopier, p *fakePurger)
		cause string
	}{
		{"source partitions", sqlc.AppMergeStepCopy, func(_ *sqlc.AppMerge, _ *fakeStore, cp *fakeCopier, _ *fakePurger) {
			cp.partsErr = errors.New("ch down")
		}, "ch down"},
		{"copied partition not recorded", sqlc.AppMergeStepCopy, func(_ *sqlc.AppMerge, s *fakeStore, cp *fakeCopier, _ *fakePurger) {
			cp.parts, s.addErr = sourceParts(), errors.New("pg down")
		}, "pg down"},
		{"rejected events", sqlc.AppMergeStepCopy, func(_ *sqlc.AppMerge, _ *fakeStore, cp *fakeCopier, _ *fakePurger) {
			cp.rejErr = errors.New("ch down")
		}, "ch down"},
		{"copy without source", sqlc.AppMergeStepCopy, func(m *sqlc.AppMerge, _ *fakeStore, _ *fakeCopier, _ *fakePurger) {
			m.SourceAppID = nil
		}, errSourceGone.Error()},
		{"purge clickhouse", sqlc.AppMergeStepPurgeSource, func(_ *sqlc.AppMerge, _ *fakeStore, _ *fakeCopier, p *fakePurger) {
			p.err = errors.New("mutation failed")
		}, "mutation failed"},
		{"purge postgres", sqlc.AppMergeStepPurgeSource, func(_ *sqlc.AppMerge, s *fakeStore, _ *fakeCopier, _ *fakePurger) {
			s.purgeErr = errors.New("pg down")
		}, "pg down"},
		{"purge without source", sqlc.AppMergeStepPurgeSource, func(m *sqlc.AppMerge, _ *fakeStore, _ *fakeCopier, _ *fakePurger) {
			m.SourceAppID = nil
		}, errSourceGone.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := runningAt(tc.step)
			s := &fakeStore{merge: m}
			cp, p := &fakeCopier{}, &fakePurger{}
			tc.setup(m, s, cp, p)
			var failed []failedCall
			res, err := newTestExecutorWith(s, cp, p, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), uuid.New())
			require.ErrorContains(t, err, tc.cause)
			require.Equal(t, sqlc.AppMergeStatusFailed, res.Status)
			require.Equal(t, tc.step, res.Step)
			require.Len(t, failed, 1)
			require.NotContains(t, s.calls, "set_step:reclose")
			if tc.step == sqlc.AppMergeStepCopy {
				require.NotContains(t, s.calls, "set_step:purge_source")
				require.Empty(t, p.purged)
			}
		})
	}
}

// purge_source: PurgeApp источника в ClickHouse, затем строка apps источника удаляется с записью в job_runs
// запуска; app_merges.source_app_id обнуляется.
func TestPurgeSourceStep(t *testing.T) {
	m := runningAt(sqlc.AppMergeStepPurgeSource)
	source := *m.SourceAppID
	s := &fakeStore{merge: m}
	p := &fakePurger{}
	var failed []failedCall
	runID := uuid.New()
	res, err := newTestExecutorWith(s, &fakeCopier{}, p, clock.NewFixed(execNow), &failed).RunOnce(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, sqlc.AppMergeStatusDone, res.Status)
	require.Equal(t, []uuid.UUID{source}, p.purged)
	require.Equal(t, []uuid.UUID{runID}, s.runIDs)
	require.Equal(t, []string{"claim", "purge_source", "set_step:reclose", "reclose"}, s.calls)
	require.Nil(t, s.merge.SourceAppID)
	require.Empty(t, failed)
}

// Метрики: merge_copied_events_total — скопированные события, merge_step_duration_seconds{step} — каждый шаг.
func TestExecutorMetrics(t *testing.T) {
	m := runningAt(sqlc.AppMergeStepCopy)
	m.CopiedPartitions = []int32{202608}
	s := &fakeStore{merge: m}
	var failed []failedCall
	e := newTestExecutorWith(s, &fakeCopier{parts: sourceParts()}, &fakePurger{}, clock.NewFixed(execNow), &failed)
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, e.RegisterMetrics(reg))
	require.Error(t, e.RegisterMetrics(reg), "повторная регистрация")

	_, err := e.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.InDelta(t, 410, promtest.ToFloat64(e.metrics.copiedEvents), 0, "партиция 202608 скопирована раньше")
	require.Equal(t, 3, promtest.CollectAndCount(e.metrics.stepDuration), "copy, purge_source, reclose")
	for _, step := range []string{"copy", "purge_source", "reclose"} {
		require.Equal(t, 1, promtest.CollectAndCount(e.metrics.stepDuration.MustCurryWith(prometheus.Labels{"step": step})), step)
	}
	require.NoError(t, promtest.GatherAndCompare(reg, strings.NewReader(`
# HELP merge_copied_events_total События, скопированные из источника в цель шагом copy слияния приложений.
# TYPE merge_copied_events_total counter
merge_copied_events_total 410
`), "merge_copied_events_total"))
}

// Шаг copy дольше 30 минут — предупреждение merge.slow в логе.
func TestSlowCopyStepLogsWarning(t *testing.T) {
	s := &fakeStore{merge: runningAt(sqlc.AppMergeStepCopy)}
	clk := clock.NewFixed(execNow)
	cp := &fakeCopier{parts: sourceParts(), onCopy: func(uint32) { clk.Advance(11 * time.Minute) }}
	var buf bytes.Buffer
	var failed []failedCall
	e := newTestExecutorWith(s, cp, &fakePurger{}, clk, &failed)
	e.logger = slog.New(slog.NewJSONHandler(&buf, nil))
	_, err := e.RunOnce(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"event":"merge.slow"`)
	require.Contains(t, buf.String(), `"level":"WARN"`)
	require.Equal(t, 1, strings.Count(buf.String(), "merge.slow"), "только шаг copy")
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
