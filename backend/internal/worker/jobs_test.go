package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/deletion"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

type fakeDeletionRunner struct {
	runIDs []uuid.UUID
	res    deletion.Result
	err    error
}

func (f *fakeDeletionRunner) RunOnce(_ context.Context, jobRunID uuid.UUID) (deletion.Result, error) {
	f.runIDs = append(f.runIDs, jobRunID)
	return f.res, f.err
}

func TestDeletionJob(t *testing.T) {
	runner := &fakeDeletionRunner{res: deletion.Result{Requeued: 1, Done: 3, Retried: 1}, err: errors.New("boom")}
	job := DeletionJob(runner)
	require.Equal(t, "deletion", job.Name)
	require.Equal(t, time.Minute, job.Every)
	require.Positive(t, job.Timeout)

	runID := uuid.New()
	details, err := job.Run(t.Context(), runID)
	require.ErrorContains(t, err, "boom")
	require.Equal(t, []uuid.UUID{runID}, runner.runIDs)
	require.Equal(t, Details{"requeued": int64(1), "done": 3, "retried": 1, "failed": 0}, details)
}

type fakeSettings struct {
	row sqlc.GetSettingsRow
	err error
}

func (f fakeSettings) GetSettings(context.Context) (sqlc.GetSettingsRow, error) { return f.row, f.err }

type sentMessage struct{ chatID, text string }

type fakeTelegram struct {
	sent []sentMessage
	err  error
}

func (f *fakeTelegram) Send(_ context.Context, chatID, text string) (int64, error) {
	f.sent = append(f.sent, sentMessage{chatID: chatID, text: text})
	return 1, f.err
}

func TestDeletionFailedNotifier(t *testing.T) {
	chat := "-1001234"
	dr := sqlc.DeletionRequest{
		ID: uuid.New(), AppID: uuid.New(), SubjectType: sqlc.DeletionSubjectUser, SubjectValue: "secret-user-42",
		Attempts: 5,
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
			notify := DeletionFailedNotifier(tc.settings, sender, discardLogger())
			notify(t.Context(), dr, `clickhouse: <timeout> & "retry"`)
			if !tc.wantSent {
				require.Empty(t, tg.sent)
				return
			}
			require.Len(t, tg.sent, 1)
			msg := tg.sent[0]
			require.Equal(t, chat, msg.chatID)
			require.Contains(t, msg.text, "Удаление данных не выполнено")
			require.Contains(t, msg.text, dr.ID.String())
			require.Contains(t, msg.text, dr.AppID.String())
			require.Contains(t, msg.text, "user")
			require.Contains(t, msg.text, "5")
			require.Contains(t, msg.text, "clickhouse: &lt;timeout&gt; &amp; &#34;retry&#34;", "HTML parse mode — причина экранируется")
			require.NotContains(t, msg.text, dr.SubjectValue, "user_id не уходит за пределы системы")
		})
	}
}
