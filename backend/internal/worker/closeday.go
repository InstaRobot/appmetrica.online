package worker

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// CloseDayStore — приложения и closed_days (pgstore/sqlc.Queries).
type CloseDayStore interface {
	ListAppsForJobs(ctx context.Context) ([]sqlc.ListAppsForJobsRow, error)
	GetLastClosedDay(ctx context.Context, appID uuid.UUID) (pgtype.Date, error)
	MarkDayClosed(ctx context.Context, arg sqlc.MarkDayClosedParams) error
}

// DayCloser — reports.Service: daily_stats одного дня приложения.
type DayCloser interface {
	CloseDay(ctx context.Context, appID uuid.UUID, loc *time.Location, day time.Time) error
}

// Locker — pgstore.Store: advisory lock на время запуска.
type Locker interface {
	TryAdvisoryLock(ctx context.Context, key string) (release func(), ok bool, err error)
}

// CloseDayOption настраивает CloseDayJob.
type CloseDayOption func(*closeDay)

// WithMaxDaysPerRun — сколько дней одного приложения закрывается за запуск.
func WithMaxDaysPerRun(n int) CloseDayOption {
	return func(c *closeDay) { c.maxDays = n }
}

// closedRange — дни from … to приложения, закрытые запуском (job_runs.details.apps).
type closedRange struct {
	AppID uuid.UUID `json:"app_id"`
	From  string    `json:"from"`
	To    string    `json:"to"`
}

type closeDay struct {
	maxDays int
}

// CloseDayJob — job close_day раз в 15 минут (FR-022).
func CloseDayJob(store CloseDayStore, stats DayCloser, lock Locker, c clock.Clock, opts ...CloseDayOption) Job {
	return Job{
		Name: "close_day", Every: 15 * time.Minute,
		Run: func(context.Context, uuid.UUID) (Details, error) {
			return nil, errors.New("close_day: not implemented")
		},
	}
}

// daysToClose — дни from … to (даты в TZ приложения, полночь UTC), которые нужно закрыть; ok = false — нечего.
func daysToClose(now time.Time, loc *time.Location, created, lastClosed time.Time, maxDays int) (from, to time.Time, ok bool) {
	return time.Time{}, time.Time{}, false
}
