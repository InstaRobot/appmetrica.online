//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	gen "appmetrica.online/backend/internal/gen/api"
)

const systemPath = "/admin/v1/system/status"

// backupAPI — httptest вместо REST clickhouse-backup: GET /backup/list/remote в формате JSONEachRow.
func backupAPI(t *testing.T, status int, body string, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backup/list/remote" {
			http.NotFound(w, r)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(delay):
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const backupList = `{"name":"full-2026-10-06T02-00-00","created":"2026-10-06 02:00:12","size":1024,"location":"remote","required":"","desc":"tar, regular"}
{"name":"increment-2026-10-13T02-00-00","created":"2026-10-13 02:00:41","size":10,"location":"remote","required":"full-2026-10-06T02-00-00","desc":"tar, regular"}
{"name":"increment-2026-10-12T02-00-00","created":"2026-10-12 02:00:33","size":10,"location":"remote","required":"full-2026-10-06T02-00-00","desc":"tar, regular"}
`

func (e *portfolioEnv) systemStatus(t *testing.T) gen.SystemStatus {
	t.Helper()
	cookie := ownerCookie(t, e)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, systemPath, nil)
	req.AddCookie(cookie)
	rec := serveRequest(t, e.handler, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	validateResponse(t, loadAPIDoc(t), req, rec)
	var st gen.SystemStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &st))
	return st
}

// seedOps — состояния алертов и запуски jobs: по два запуска system_check, по одному digest и deletion.
func (e *portfolioEnv) seedOps(t *testing.T) {
	t.Helper()
	pool := e.env.PG.Pool()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO alert_states (key, firing, first_fired_at, last_notified_at, last_value) VALUES
			('disk_free', false, NULL, NULL, '81.4'),
			('backup_stale', true, '2026-10-13 01:00:00+00', '2026-10-13 01:00:05+00', '27h')`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `
		INSERT INTO job_runs (job, started_at, finished_at, status, details) VALUES
			('system_check', '2026-10-13 04:50:00+00', '2026-10-13 04:50:01+00', 'ok', '{"checks": 5}'),
			('system_check', '2026-10-13 04:55:00+00', NULL, 'running', '{}'),
			('digest', '2026-10-13 06:00:00+00', '2026-10-13 06:00:02+00', 'failed', '{"error": "chat not found"}'),
			('deletion', '2026-10-13 04:59:00+00', '2026-10-13 04:59:00.5+00', 'ok', '{"processed": 0}')`)
	require.NoError(t, err)
}

func TestSystemStatus(t *testing.T) {
	e := newPortfolioEnv(t)
	e.seedOps(t)
	e.backup = chstore.NewBackupClient(backupAPI(t, http.StatusOK, backupList, 0).URL, nil)
	e.rebuild(t)

	st := e.systemStatus(t)

	require.True(t, st.Clickhouse.Ok)
	pct, err := st.Clickhouse.DiskFreePct.Get()
	require.NoError(t, err)
	require.Greater(t, pct, float32(0))
	require.LessOrEqual(t, pct, float32(100))
	rows, err := st.Clickhouse.EventsRows.Get()
	require.NoError(t, err)
	var parts uint64
	require.NoError(t, e.env.CH.Conn().QueryRow(t.Context(),
		`SELECT sum(rows) FROM system.parts WHERE active AND database = currentDatabase() AND table = 'events'`).Scan(&parts))
	require.Positive(t, rows)
	require.Equal(t, int64(parts), rows) //nolint:gosec // счётчик строк в пределах int64

	require.True(t, st.Postgres.Ok)

	at, err := st.Backup.LastBackupAt.Get()
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 13, 2, 0, 41, 0, time.UTC), at.UTC())
	name, err := st.Backup.LastBackupName.Get()
	require.NoError(t, err)
	require.Equal(t, "increment-2026-10-13T02-00-00", name)

	require.Len(t, st.Alerts, 2)
	require.Equal(t, gen.AlertKey("backup_stale"), st.Alerts[0].Key)
	require.True(t, st.Alerts[0].Firing)
	first, err := st.Alerts[0].FirstFiredAt.Get()
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC), first.UTC())
	require.Equal(t, "27h", st.Alerts[0].LastValue)
	require.Equal(t, gen.AlertKey("disk_free"), st.Alerts[1].Key)
	require.False(t, st.Alerts[1].Firing)
	require.True(t, st.Alerts[1].FirstFiredAt.IsNull())
	require.True(t, st.Alerts[1].LastNotifiedAt.IsNull())

	// Последний запуск каждого job, свежие сверху; не больше 20.
	require.LessOrEqual(t, len(st.Jobs), 20)
	type run struct {
		job      gen.JobName
		status   gen.JobStatus
		finished bool
	}
	got := make([]run, 0, len(st.Jobs))
	for _, j := range st.Jobs {
		got = append(got, run{j.Job, j.Status, !j.FinishedAt.IsNull()})
	}
	require.Equal(t, []run{
		{"digest", "failed", true},
		{"deletion", "ok", true},
		{"system_check", "running", false},
	}, got)
	require.Equal(t, map[string]any{"error": "chat not found"}, st.Jobs[0].Details)
}

// Без BACKUP_API_URL и без запусков: backup и списки пустые, остальное заполнено.
func TestSystemStatus_Empty(t *testing.T) {
	e := newPortfolioEnv(t)
	st := e.systemStatus(t)
	require.True(t, st.Clickhouse.Ok)
	require.True(t, st.Postgres.Ok)
	require.True(t, st.Backup.LastBackupAt.IsNull())
	require.True(t, st.Backup.LastBackupName.IsNull())
	require.NotNil(t, st.Alerts)
	require.Empty(t, st.Alerts)
	require.NotNil(t, st.Jobs)
	require.Empty(t, st.Jobs)
}

func TestSystemStatus_ClickHouseDown(t *testing.T) {
	e := newPortfolioEnv(t)
	down, err := chstore.Open(t.Context(), e.env.ClickHouse)
	require.NoError(t, err)
	require.NoError(t, down.Close())
	e.handler = e.newHandler(t, e.reports, down, prometheus.NewRegistry())

	st := e.systemStatus(t)
	require.False(t, st.Clickhouse.Ok)
	require.True(t, st.Clickhouse.DiskFreePct.IsNull())
	require.True(t, st.Clickhouse.EventsRows.IsNull())
	require.True(t, st.Postgres.Ok)
}

func TestSystemStatus_PostgresDown(t *testing.T) {
	e := newPortfolioEnv(t)
	e.seedOps(t)
	e.pgPing = func(context.Context) error { return errors.New("connection refused") }
	e.rebuild(t)

	st := e.systemStatus(t)
	require.False(t, st.Postgres.Ok)
	require.Empty(t, st.Alerts)
	require.Empty(t, st.Jobs)
	require.True(t, st.Clickhouse.Ok)
}

// clickhouse-backup недоступен или отвечает ошибкой: backup пустой, статус остальных компонентов показывается.
func TestSystemStatus_BackupUnavailable(t *testing.T) {
	for name, srv := range map[string]func(t *testing.T) string{
		"server_error": func(t *testing.T) string {
			return backupAPI(t, http.StatusInternalServerError, `{"status":"error"}`, 0).URL
		},
		"unreachable": func(t *testing.T) string {
			s := backupAPI(t, http.StatusOK, backupList, 0)
			s.Close()
			return s.URL
		},
		"no_backups": func(t *testing.T) string { return backupAPI(t, http.StatusOK, "", 0).URL },
	} {
		t.Run(name, func(t *testing.T) {
			e := newPortfolioEnv(t)
			e.backup = chstore.NewBackupClient(srv(t), nil)
			e.rebuild(t)
			st := e.systemStatus(t)
			require.True(t, st.Backup.LastBackupAt.IsNull())
			require.True(t, st.Backup.LastBackupName.IsNull())
			require.True(t, st.Clickhouse.Ok)
			require.True(t, st.Postgres.Ok)
		})
	}
}

// Проверки идут параллельно с таймаутом 1,5 с: зависший clickhouse-backup не задерживает ответ дольше 2 с.
func TestSystemStatus_SlowBackupTimesOut(t *testing.T) {
	e := newPortfolioEnv(t)
	e.backup = chstore.NewBackupClient(backupAPI(t, http.StatusOK, backupList, 5*time.Second).URL, nil)
	e.rebuild(t)

	start := time.Now()
	st := e.systemStatus(t)
	require.Less(t, time.Since(start), 2*time.Second)
	require.True(t, st.Backup.LastBackupAt.IsNull())
	require.True(t, st.Clickhouse.Ok)
}

func TestSystemStatus_RequiresOwner(t *testing.T) {
	e := newPortfolioEnv(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, systemPath, nil)
	requireEnvelope(t, serveRequest(t, e.handler, req), http.StatusUnauthorized, "unauthenticated")

	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, systemPath, nil)
	req.Header.Set("Authorization", "Bearer "+e.all)
	requireEnvelope(t, serveRequest(t, e.handler, req), http.StatusUnauthorized, "unauthenticated")
}
