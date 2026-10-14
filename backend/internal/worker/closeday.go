package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/merge"
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

// closedRange — дни from … to приложения, закрытые запуском (job_runs.details.apps).
type closedRange struct {
	AppID uuid.UUID `json:"app_id"`
	From  string    `json:"from"`
	To    string    `json:"to"`
}

type closeDay struct {
	store   CloseDayStore
	stats   DayCloser
	lock    Locker
	clock   clock.Clock
	maxDays int
}

// CloseDayJob — job close_day раз в 15 минут (FR-022): для каждого приложения, включая архивные, закрывает
// дни до D = сегодня − 8 в его TZ, как только там наступило 03:30. Первый запуск закрывает дни с apps.created_at,
// дальше — с дня после последнего закрытого, не больше maxDays дней приложения за запуск (DefaultCloseMaxDays). День
// закрывается записью daily_stats, затем closed_days: сбой между ними безопасен, повтор перезапишет строки.
// Ошибка приложения останавливает только его дни; остальные приложения закрываются, запуск — failed.
func CloseDayJob(store CloseDayStore, stats DayCloser, lock Locker, c clock.Clock, maxDays int) Job {
	cd := &closeDay{store: store, stats: stats, lock: lock, clock: c, maxDays: maxDays}
	return Job{Name: "close_day", Every: closeDayEvery, Timeout: closeDayTimeout, Run: cd.run}
}

// DefaultCloseMaxDays — дней одного приложения за запуск close_day в production (02-tech-spec.md FR-022).
const DefaultCloseMaxDays = 400

const (
	closeDayEvery = 15 * time.Minute
	// closeDayTimeout — запуск с догоняющим закрытием длиннее тика; следующий ждёт его окончания.
	closeDayTimeout = 14 * time.Minute
	// closeDayLockKey — его же берёт шаг reclose слияния, переоткрывая дни цели (merge.CloseDayLockKey).
	closeDayLockKey = merge.CloseDayLockKey
	// closeAfterMinutes — 03:30: к этому времени по TZ приложения день D − 8 закрывается.
	closeAfterMinutes = 3*60 + 30
)

func (cd *closeDay) run(ctx context.Context, _ uuid.UUID) (Details, error) {
	release, ok, err := cd.lock.TryAdvisoryLock(ctx, closeDayLockKey)
	if err != nil {
		return nil, fmt.Errorf("close_day: %w", err)
	}
	if !ok {
		return Details{"skipped": "locked"}, nil
	}
	defer release()

	apps, err := cd.store.ListAppsForJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("close_day: list apps: %w", err)
	}
	now := cd.clock.Now()
	closed := []closedRange{}
	total := 0
	var errs []error
	for _, app := range apps {
		r, n, err := cd.closeApp(ctx, app, now)
		total += n
		if n > 0 {
			closed = append(closed, r)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("app %s: %w", app.ID, err))
		}
	}
	details := Details{"closed_days": total, "apps": closed}
	if err := errors.Join(errs...); err != nil {
		return details, fmt.Errorf("close_day: %w", err)
	}
	return details, nil
}

// closeApp закрывает дни приложения по порядку до первой ошибки: closed_days остаётся без дыр. r — закрытые
// дни, n — их число.
func (cd *closeDay) closeApp(ctx context.Context, app sqlc.ListAppsForJobsRow, now time.Time) (r closedRange, n int, err error) {
	loc, err := time.LoadLocation(app.Timezone)
	if err != nil {
		return r, 0, fmt.Errorf("timezone %q: %w", app.Timezone, err)
	}
	last, err := cd.store.GetLastClosedDay(ctx, app.ID)
	if err != nil {
		return r, 0, fmt.Errorf("last closed day: %w", err)
	}
	from, to, ok := daysToClose(now, loc, app.CreatedAt, last.Time, cd.maxDays)
	if !ok {
		return r, 0, nil
	}
	r = closedRange{AppID: app.ID, From: from.Format(time.DateOnly)}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if err := cd.stats.CloseDay(ctx, app.ID, loc, d); err != nil {
			return r, n, fmt.Errorf("close %s: %w", d.Format(time.DateOnly), err)
		}
		err := cd.store.MarkDayClosed(ctx, sqlc.MarkDayClosedParams{
			AppID: app.ID, Day: pgtype.Date{Time: d, Valid: true}, Now: cd.clock.Now(),
		})
		if err != nil {
			return r, n, fmt.Errorf("mark %s closed: %w", d.Format(time.DateOnly), err)
		}
		r.To = d.Format(time.DateOnly)
		n++
	}
	return r, n, nil
}

// daysToClose — дни from … to (даты в TZ приложения, полночь UTC), которые нужно закрыть; ok = false — нечего.
// Последний — сегодня − 8, если по TZ приложения уже 03:30, иначе сегодня − 9; первый — день после lastClosed,
// но не раньше дня создания приложения; дней — не больше maxDays.
func daysToClose(now time.Time, loc *time.Location, created, lastClosed time.Time, maxDays int) (from, to time.Time, ok bool) {
	local := now.In(loc)
	back := 8
	if local.Hour()*60+local.Minute() < closeAfterMinutes {
		back = 9
	}
	to = civilDate(local).AddDate(0, 0, -back)
	from = civilDate(created.In(loc))
	if next := civilDate(lastClosed).AddDate(0, 0, 1); !lastClosed.IsZero() && next.After(from) {
		from = next
	}
	if from.After(to) {
		return from, to, false
	}
	if limit := from.AddDate(0, 0, maxDays-1); maxDays > 0 && limit.Before(to) {
		to = limit
	}
	return from, to, true
}

// civilDate — календарная дата t (в её поясе) в полночь UTC.
func civilDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
