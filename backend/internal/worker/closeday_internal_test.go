package worker

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func tz(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func TestDaysToClose(t *testing.T) {
	msk, nyc := tz(t, "Europe/Moscow"), tz(t, "America/New_York")
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		require.NoError(t, err)
		return v
	}
	for _, tc := range []struct {
		name       string
		now        string
		loc        *time.Location
		created    string
		lastClosed time.Time
		maxDays    int
		from, to   string // "" — закрывать нечего
	}{
		{"after 03:30 closes today-8", "2026-10-21T00:31:00Z", msk, "2026-10-12T21:00:00Z", time.Time{}, 400, "2026-10-13", "2026-10-13"},
		{"before 03:30 waits", "2026-10-21T00:29:00Z", msk, "2026-10-12T21:00:00Z", time.Time{}, 400, "", ""},
		{"exactly 03:30 closes", "2026-10-21T00:30:00Z", msk, "2026-10-12T21:00:00Z", time.Time{}, 400, "2026-10-13", "2026-10-13"},
		{"created day in app timezone", "2026-10-21T00:31:00Z", msk, "2026-10-13T21:00:00Z", time.Time{}, 400, "", ""},
		{"app timezone behind UTC", "2026-10-21T00:31:00Z", nyc, "2026-10-01T12:00:00Z", time.Time{}, 400, "2026-10-01", "2026-10-12"},
		{"continues after last closed", "2026-10-21T00:31:00Z", msk, "2026-09-01T00:00:00Z", day("2026-10-12"), 400, "2026-10-13", "2026-10-13"},
		{"already closed", "2026-10-21T00:31:00Z", msk, "2026-09-01T00:00:00Z", day("2026-10-13"), 400, "", ""},
		{"no closed days — from created", "2026-10-21T00:31:00Z", msk, "2026-10-05T09:00:00Z", day("1970-01-01"), 400, "2026-10-05", "2026-10-13"},
		{"limited per run", "2026-10-21T00:31:00Z", msk, "2026-09-01T09:00:00Z", time.Time{}, 3, "2026-09-01", "2026-09-03"},
		// 2026-11-01 в Нью-Йорке: 02:00 EDT → 01:00 EST, 03:30 EST = 08:30Z.
		{"DST end before 03:30", "2026-11-01T08:29:00Z", nyc, "2026-10-01T12:00:00Z", day("2026-10-23"), 400, "", ""},
		{"DST end after 03:30", "2026-11-01T08:30:00Z", nyc, "2026-10-01T12:00:00Z", day("2026-10-23"), 400, "2026-10-24", "2026-10-24"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, to, ok := daysToClose(at(tc.now), tc.loc, at(tc.created), tc.lastClosed, tc.maxDays)
			if tc.from == "" {
				require.False(t, ok, "from %s to %s", from, to)
				return
			}
			require.True(t, ok)
			require.Equal(t, tc.from, from.Format(time.DateOnly))
			require.Equal(t, tc.to, to.Format(time.DateOnly))
		})
	}
}

// fakeCloseDayStore — apps и closed_days в памяти.
type fakeCloseDayStore struct {
	mu      sync.Mutex
	apps    []sqlc.ListAppsForJobsRow
	closed  map[uuid.UUID][]time.Time
	listErr error
	markErr error
}

func (s *fakeCloseDayStore) ListAppsForJobs(context.Context) ([]sqlc.ListAppsForJobsRow, error) {
	return s.apps, s.listErr
}

func (s *fakeCloseDayStore) GetLastClosedDay(_ context.Context, appID uuid.UUID) (pgtype.Date, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last := day("1970-01-01")
	for _, d := range s.closed[appID] {
		if d.After(last) {
			last = d
		}
	}
	return pgtype.Date{Time: last, Valid: true}, nil
}

func (s *fakeCloseDayStore) MarkDayClosed(_ context.Context, arg sqlc.MarkDayClosedParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markErr != nil {
		return s.markErr
	}
	if s.closed == nil {
		s.closed = map[uuid.UUID][]time.Time{}
	}
	s.closed[arg.AppID] = append(s.closed[arg.AppID], arg.Day.Time)
	return nil
}

func (s *fakeCloseDayStore) days(appID uuid.UUID) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, d := range s.closed[appID] {
		out = append(out, d.Format(time.DateOnly))
	}
	return out
}

type closeCall struct {
	app uuid.UUID
	day string
	loc string
}

// fakeDayCloser — записывает вызовы; fail возвращает ошибку для дня приложения.
type fakeDayCloser struct {
	mu    sync.Mutex
	calls []closeCall
	fail  func(app uuid.UUID, day string) error
}

func (c *fakeDayCloser) CloseDay(_ context.Context, appID uuid.UUID, loc *time.Location, d time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := closeCall{appID, d.Format(time.DateOnly), loc.String()}
	c.calls = append(c.calls, call)
	if c.fail != nil {
		return c.fail(appID, call.day)
	}
	return nil
}

type fakeLocker struct {
	ok       bool
	err      error
	keys     []string
	released int
}

func (l *fakeLocker) TryAdvisoryLock(_ context.Context, key string) (func(), bool, error) {
	l.keys = append(l.keys, key)
	if l.err != nil || !l.ok {
		return nil, false, l.err
	}
	return func() { l.released++ }, true, nil
}

var (
	appMSK = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	appNYC = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	// closeNow — 03:31 в Москве 2026-10-21, 20:31 2026-10-20 в Нью-Йорке.
	closeNow = time.Date(2026, 10, 21, 0, 31, 0, 0, time.UTC)
)

func jobApp(id uuid.UUID, tzName string, created time.Time, status sqlc.AppStatus) sqlc.ListAppsForJobsRow {
	return sqlc.ListAppsForJobsRow{ID: id, Name: id.String(), Timezone: tzName, Status: status, CreatedAt: created}
}

func runCloseDay(t *testing.T, store *fakeCloseDayStore, closer *fakeDayCloser, lock *fakeLocker, maxDays int) (Details, error) {
	t.Helper()
	job := CloseDayJob(store, closer, lock, clock.NewFixed(closeNow), maxDays)
	return job.Run(t.Context(), uuid.New())
}

func TestCloseDayJob_Schedule(t *testing.T) {
	job := CloseDayJob(&fakeCloseDayStore{}, &fakeDayCloser{}, &fakeLocker{ok: true}, clock.NewFixed(closeNow), DefaultCloseMaxDays)
	require.Equal(t, "close_day", job.Name)
	require.Equal(t, 15*time.Minute, job.Every)
	require.Positive(t, job.Timeout)
	require.Less(t, job.Timeout, job.Every+time.Minute)
}

// Закрываются все приложения, включая архивные, каждое — в своей TZ; итог — в details.
func TestCloseDayJob_ClosesAppsInTheirTimezones(t *testing.T) {
	store := &fakeCloseDayStore{apps: []sqlc.ListAppsForJobsRow{
		jobApp(appMSK, "Europe/Moscow", time.Date(2026, 10, 12, 21, 0, 0, 0, time.UTC), sqlc.AppStatusArchived),
		jobApp(appNYC, "America/New_York", time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
	}}
	closer := &fakeDayCloser{}
	lock := &fakeLocker{ok: true}

	details, err := runCloseDay(t, store, closer, lock, DefaultCloseMaxDays)
	require.NoError(t, err)
	require.Equal(t, []closeCall{
		{appMSK, "2026-10-13", "Europe/Moscow"},
		{appNYC, "2026-10-11", "America/New_York"},
		{appNYC, "2026-10-12", "America/New_York"},
	}, closer.calls)
	require.Equal(t, []string{"2026-10-13"}, store.days(appMSK))
	require.Equal(t, []string{"2026-10-11", "2026-10-12"}, store.days(appNYC))
	require.Equal(t, Details{
		"closed_days": 3,
		"apps": []closedRange{
			{AppID: appMSK, From: "2026-10-13", To: "2026-10-13"},
			{AppID: appNYC, From: "2026-10-11", To: "2026-10-12"},
		},
	}, details)
	require.Equal(t, []string{"close_day"}, lock.keys)
	require.Equal(t, 1, lock.released)
}

func TestCloseDayJob_MaxDaysPerRun(t *testing.T) {
	store := &fakeCloseDayStore{apps: []sqlc.ListAppsForJobsRow{
		jobApp(appMSK, "Europe/Moscow", time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
	}}
	closer := &fakeDayCloser{}
	_, err := runCloseDay(t, store, closer, &fakeLocker{ok: true}, 5)
	require.NoError(t, err)
	require.Equal(t, []string{"2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05"}, store.days(appMSK))

	_, err = runCloseDay(t, store, closer, &fakeLocker{ok: true}, 5)
	require.NoError(t, err)
	require.Len(t, store.days(appMSK), 10)
	require.Equal(t, "2026-10-10", store.days(appMSK)[9])
}

// Второй экземпляр worker-а при занятом lock ничего не делает и не считается ошибкой.
func TestCloseDayJob_SkipsWhenLocked(t *testing.T) {
	store := &fakeCloseDayStore{apps: []sqlc.ListAppsForJobsRow{
		jobApp(appMSK, "Europe/Moscow", time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
	}}
	closer := &fakeDayCloser{}
	details, err := runCloseDay(t, store, closer, &fakeLocker{ok: false}, DefaultCloseMaxDays)
	require.NoError(t, err)
	require.Equal(t, Details{"skipped": "locked"}, details)
	require.Empty(t, closer.calls)
}

func TestCloseDayJob_LockError(t *testing.T) {
	closer := &fakeDayCloser{}
	_, err := runCloseDay(t, &fakeCloseDayStore{}, closer, &fakeLocker{err: errors.New("pg down")}, DefaultCloseMaxDays)
	require.ErrorContains(t, err, "pg down")
	require.Empty(t, closer.calls)
}

func TestCloseDayJob_ListAppsError(t *testing.T) {
	lock := &fakeLocker{ok: true}
	_, err := runCloseDay(t, &fakeCloseDayStore{listErr: errors.New("pg down")}, &fakeDayCloser{}, lock, DefaultCloseMaxDays)
	require.ErrorContains(t, err, "pg down")
	require.Equal(t, 1, lock.released)
}

// Ошибка ClickHouse на дне приложения: следующие дни этого приложения не закрываются (closed_days без дыр),
// остальные приложения закрываются, запуск — failed.
func TestCloseDayJob_AppErrorDoesNotStopOthers(t *testing.T) {
	store := &fakeCloseDayStore{apps: []sqlc.ListAppsForJobsRow{
		jobApp(appMSK, "Europe/Moscow", time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
		jobApp(appNYC, "America/New_York", time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
	}}
	closer := &fakeDayCloser{fail: func(app uuid.UUID, d string) error {
		if app == appMSK && d == "2026-10-11" {
			return errors.New("clickhouse: connection refused")
		}
		return nil
	}}
	details, err := runCloseDay(t, store, closer, &fakeLocker{ok: true}, DefaultCloseMaxDays)
	require.ErrorContains(t, err, "clickhouse: connection refused")
	require.ErrorContains(t, err, appMSK.String())
	require.ErrorContains(t, err, "2026-10-11")
	require.Equal(t, []string{"2026-10-10"}, store.days(appMSK))
	require.Equal(t, []string{"2026-10-12"}, store.days(appNYC))
	require.False(t, slices.Contains(closer.calls, closeCall{appMSK, "2026-10-12", "Europe/Moscow"}))
	require.Equal(t, 2, details["closed_days"])
}

func TestCloseDayJob_InvalidTimezone(t *testing.T) {
	store := &fakeCloseDayStore{apps: []sqlc.ListAppsForJobsRow{
		jobApp(appMSK, "Mars/Olympus", time.Date(2026, 10, 12, 21, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
		jobApp(appNYC, "America/New_York", time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), sqlc.AppStatusActive),
	}}
	_, err := runCloseDay(t, store, &fakeDayCloser{}, &fakeLocker{ok: true}, DefaultCloseMaxDays)
	require.ErrorContains(t, err, "Mars/Olympus")
	require.Empty(t, store.days(appMSK))
	require.Equal(t, []string{"2026-10-12"}, store.days(appNYC))
}

func TestCloseDayJob_MarkError(t *testing.T) {
	store := &fakeCloseDayStore{
		apps:    []sqlc.ListAppsForJobsRow{jobApp(appMSK, "Europe/Moscow", time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC), sqlc.AppStatusActive)},
		markErr: errors.New("pg down"),
	}
	closer := &fakeDayCloser{}
	_, err := runCloseDay(t, store, closer, &fakeLocker{ok: true}, DefaultCloseMaxDays)
	require.ErrorContains(t, err, "pg down")
	require.Len(t, closer.calls, 1, "после ошибки closed_days следующий день не закрывается")
}
