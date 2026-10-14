package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

type fakeMergeRunner struct {
	runIDs []uuid.UUID
	res    merge.Result
	err    error
}

func (f *fakeMergeRunner) RunOnce(_ context.Context, runID uuid.UUID) (merge.Result, error) {
	f.runIDs = append(f.runIDs, runID)
	return f.res, f.err
}

func TestMergeJob(t *testing.T) {
	mergeID := uuid.New()
	runner := &fakeMergeRunner{res: merge.Result{MergeID: mergeID, Status: sqlc.AppMergeStatusRunning, Step: sqlc.AppMergeStepWaitKeys}}
	lock := &fakeLocker{ok: true}
	job := MergeJob(runner, lock)
	require.Equal(t, "app_merge", job.Name)
	require.Equal(t, time.Minute, job.Every)
	require.Equal(t, 30*time.Minute, job.Timeout, "копирование крупного источника длиннее тика")

	runID := uuid.New()
	details, err := job.Run(t.Context(), runID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{runID}, runner.runIDs)
	require.Equal(t, Details{"merge_id": mergeID.String(), "status": "running", "step": "wait_keys"}, details)
	require.Equal(t, []string{"app_merge"}, lock.keys)
	require.Equal(t, 1, lock.released)
}

func TestMergeJobIdle(t *testing.T) {
	job := MergeJob(&fakeMergeRunner{}, &fakeLocker{ok: true})
	details, err := job.Run(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Details{}, details)
}

func TestMergeJobFailure(t *testing.T) {
	mergeID := uuid.New()
	runner := &fakeMergeRunner{
		res: merge.Result{MergeID: mergeID, Status: sqlc.AppMergeStatusFailed, Step: sqlc.AppMergeStepMoveKeys},
		err: errors.New("pg down"),
	}
	lock := &fakeLocker{ok: true}
	details, err := MergeJob(runner, lock).Run(t.Context(), uuid.New())
	require.ErrorContains(t, err, "pg down")
	require.Equal(t, Details{"merge_id": mergeID.String(), "status": "failed", "step": "move_keys"}, details)
	require.Equal(t, 1, lock.released)
}

// Второй worker пропускает запуск, пока первый держит advisory lock app_merge.
func TestMergeJobLocked(t *testing.T) {
	runner := &fakeMergeRunner{}
	details, err := MergeJob(runner, &fakeLocker{ok: false}).Run(t.Context(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, Details{"skipped": "locked"}, details)
	require.Empty(t, runner.runIDs)

	_, err = MergeJob(runner, &fakeLocker{err: errors.New("pool closed")}).Run(t.Context(), uuid.New())
	require.ErrorContains(t, err, "pool closed")
	require.Empty(t, runner.runIDs)
}

func TestMergeFailedNotifier(t *testing.T) {
	chat := "-1001234"
	source := uuid.New()
	m := sqlc.AppMerge{
		ID: uuid.New(), TargetAppID: uuid.New(), SourceAppID: &source, SourceName: "Notes <Android> & co",
		Status: sqlc.AppMergeStatusFailed, Step: sqlc.AppMergeStepCopy, Attempts: 2,
	}
	cases := []struct {
		name     string
		settings fakeSettings
		nilTG    bool
		tgErr    error
		wantSent bool
	}{
		{name: "sent", settings: fakeSettings{row: sqlc.GetSettingsRow{TelegramChatID: &chat, AlertsEnabled: true}}, wantSent: true},
		{name: "telegram_error_is_logged", settings: fakeSettings{row: sqlc.GetSettingsRow{TelegramChatID: &chat, AlertsEnabled: true}}, tgErr: errors.New("chat not found"), wantSent: true},
		{name: "alerts_disabled", settings: fakeSettings{row: sqlc.GetSettingsRow{TelegramChatID: &chat}}},
		{name: "no_chat", settings: fakeSettings{row: sqlc.GetSettingsRow{AlertsEnabled: true}}},
		{name: "no_token", settings: fakeSettings{row: sqlc.GetSettingsRow{TelegramChatID: &chat, AlertsEnabled: true}}, nilTG: true},
		{name: "settings_error", settings: fakeSettings{err: errors.New("pg down")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tg := &fakeTelegram{err: tc.tgErr}
			var sender TelegramSender = tg
			if tc.nilTG {
				sender = nil
			}
			notify := MergeFailedNotifier(tc.settings, sender, discardLogger())
			notify(t.Context(), m, `clickhouse: <timeout> & "retry"`)
			if !tc.wantSent {
				require.Empty(t, tg.sent)
				return
			}
			require.Len(t, tg.sent, 1)
			msg := tg.sent[0]
			require.Equal(t, chat, msg.chatID)
			require.Contains(t, msg.text, "Слияние приложений не выполнено")
			require.Contains(t, msg.text, m.ID.String())
			require.Contains(t, msg.text, m.TargetAppID.String())
			require.Contains(t, msg.text, "Notes &lt;Android&gt; &amp; co", "имя источника экранируется")
			require.Contains(t, msg.text, "copy")
			require.Contains(t, msg.text, "clickhouse: &lt;timeout&gt; &amp; &#34;retry&#34;", "HTML parse mode — причина экранируется")
		})
	}
}
