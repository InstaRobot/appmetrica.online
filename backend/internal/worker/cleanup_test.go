//go:build integration

package worker_test

import (
	"bytes"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/testutil"
	"appmetrica.online/backend/internal/worker"
)

var cleanupNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// hash32 — 32-байтный token_hash, различимый по первому байту.
func hash32(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

// AC-052.5: истёкшие, отозванные и простаивающие сессии, истёкшие церемонии, использованные и истёкшие
// setup-токены, job_runs старше 90 дней удаляются; актуальные строки остаются.
func TestSessionCleanup_RemovesExpired(t *testing.T) {
	env := testutil.Start(t)
	ctx := t.Context()
	exec := func(stmt string, args ...any) {
		t.Helper()
		_, err := env.PG.Pool().Exec(ctx, stmt, args...)
		require.NoError(t, err)
	}
	day := 24 * time.Hour

	var ownerID string
	require.NoError(t, env.PG.Pool().QueryRow(ctx,
		`INSERT INTO owner (webauthn_user_handle) VALUES ($1) RETURNING id::text`, bytes.Repeat([]byte{7}, 64)).Scan(&ownerID))
	session := func(b byte, lastSeen, expires time.Time, revoked *time.Time) {
		exec(`INSERT INTO auth_sessions (token_hash, owner_id, created_at, last_seen_at, expires_at, idle_timeout, revoked_at)
			VALUES ($1, $2, $3, $4, $5, interval '7 days', $6)`, hash32(b), ownerID, lastSeen.Add(-time.Hour), lastSeen, expires, revoked)
	}
	revokedAt := cleanupNow.Add(-time.Hour)
	session(1, cleanupNow.Add(-time.Hour), cleanupNow.Add(10*day), nil)        // актуальная
	session(2, cleanupNow.Add(-time.Hour), cleanupNow.Add(-time.Minute), nil)  // истёк срок
	session(3, cleanupNow.Add(-8*day), cleanupNow.Add(10*day), nil)            // простой дольше idle_timeout
	session(4, cleanupNow.Add(-time.Hour), cleanupNow.Add(10*day), &revokedAt) // отозвана
	session(5, cleanupNow.Add(-6*day), cleanupNow.Add(time.Minute), nil)       // ещё жива

	token := func(b byte, created, expires time.Time, used *time.Time) {
		exec(`INSERT INTO passkey_setup_tokens (token_hash, created_at, expires_at, used_at) VALUES ($1, $2, $3, $4)`,
			hash32(b), created, expires, used)
	}
	usedAt := cleanupNow.Add(-10 * time.Minute)
	token(1, cleanupNow.Add(-time.Hour), cleanupNow.Add(23*time.Hour), nil)     // актуальный
	token(2, cleanupNow.Add(-25*time.Hour), cleanupNow.Add(-time.Hour), nil)    // истёк
	token(3, cleanupNow.Add(-time.Hour), cleanupNow.Add(23*time.Hour), &usedAt) // использован

	ceremony := func(expires time.Time, setupToken []byte) {
		exec(`INSERT INTO webauthn_ceremonies (kind, session_data, setup_token_hash, created_at, expires_at)
			VALUES ('registration', '{}', $1, $2, $3)`, setupToken, expires.Add(-5*time.Minute), expires)
	}
	ceremony(cleanupNow.Add(4*time.Minute), nil)       // актуальная
	ceremony(cleanupNow.Add(-time.Minute), nil)        // истекла
	ceremony(cleanupNow.Add(4*time.Minute), hash32(1)) // актуальная, токен жив
	ceremony(cleanupNow.Add(4*time.Minute), hash32(3)) // токен использован — уходит каскадом

	exec(`INSERT INTO job_runs (job, started_at, finished_at, status) VALUES
		('digest', $1, $1, 'ok'), ('close_day', $2, $2, 'ok'), ('deletion', $3, $3, 'failed')`,
		cleanupNow.Add(-91*day), cleanupNow.Add(-89*day), cleanupNow.Add(-90*day-time.Minute))

	c := clock.NewFixed(cleanupNow)
	sched := worker.NewScheduler(env.PG.Queries(), c, slog.New(slog.NewTextHandler(io.Discard, nil)),
		[]string{"session_cleanup"}, worker.SessionCleanupJob(env.PG.Queries(), c))
	require.NoError(t, sched.RunOnce(ctx, "session_cleanup"))

	hashes := func(table string) []int {
		t.Helper()
		rows, err := env.PG.Pool().Query(ctx, `SELECT get_byte(token_hash, 0) FROM `+table+` ORDER BY 1`)
		require.NoError(t, err)
		out, err := pgx.CollectRows(rows, pgx.RowTo[int])
		require.NoError(t, err)
		return out
	}
	require.Equal(t, []int{1, 5}, hashes("auth_sessions"))
	require.Equal(t, []int{1}, hashes("passkey_setup_tokens"))

	var ceremonies int
	require.NoError(t, env.PG.Pool().QueryRow(ctx, `SELECT count(*) FROM webauthn_ceremonies`).Scan(&ceremonies))
	require.Equal(t, 2, ceremonies)

	rows, err := env.PG.Pool().Query(ctx, `SELECT job::text FROM job_runs WHERE job <> 'session_cleanup' ORDER BY job`)
	require.NoError(t, err)
	jobs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.Equal(t, []string{"close_day"}, jobs)

	var (
		status string
		raw    map[string]any
	)
	require.NoError(t, env.PG.Pool().QueryRow(ctx,
		`SELECT status::text, details FROM job_runs WHERE job = 'session_cleanup'`).Scan(&status, &raw))
	require.Equal(t, "ok", status)
	require.Equal(t, map[string]any{
		"sessions": float64(3), "ceremonies": float64(1), "setup_tokens": float64(2), "job_runs": float64(2),
	}, raw)

	require.NoError(t, sched.RunOnce(ctx, "session_cleanup"), "повторный запуск — нечего удалять")
	require.Equal(t, []int{1, 5}, hashes("auth_sessions"))
}
