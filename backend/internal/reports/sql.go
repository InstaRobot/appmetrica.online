// Package reports — канонические запросы отчётов ClickHouse (05-data-model.md §10.4) и их параметры.
//
// Параметры передаются server-side binding'ом clickhouse-go (clickhouse.Parameters) в синтаксисе {name:Type};
// в текст SQL подставляются только константы из whitelist, пользовательский ввод — никогда. Люди и сессии
// считаются только uniqExact: до слияния частей ReplacingMergeTree дубли событий видны (sql_lint_test.go).
// Таблицы без имени базы: соединение открыто с базой CLICKHOUSE_DATABASE (appmetrica в production).
package reports

import (
	"context"
	"maps"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/identity"
)

const (
	EnvProduction  = "production"
	EnvDevelopment = "development"
)

// eventsFilter — условия по events AS e: приложение, отсечение партиций по UTC-датам, период в UTC,
// environment и фильтры (OR внутри параметра — IN по массиву, AND между параметрами, пустой массив — без фильтра).
const eventsFilter = `e.app_id = {app_id:UUID}
    AND e.event_date BETWEEN {d_lo:Date} AND {d_hi:Date}
    AND e.event_time >= {start_utc:DateTime64(3)}
    AND e.event_time < {end_utc:DateTime64(3)}
    AND e.environment = {env:String}
    AND (empty({f_platform:Array(String)}) OR e.platform IN {f_platform:Array(String)})
    AND (empty({f_app_version:Array(String)}) OR e.app_version IN {f_app_version:Array(String)})
    AND (empty({f_os_name:Array(String)}) OR e.os_name IN {f_os_name:Array(String)})
    AND (empty({f_country:Array(String)}) OR e.country IN {f_country:Array(String)})`

// evCTE — события периода с person_key. Требует identity.LinksCTE.
const evCTE = `ev AS (
    SELECT ` + identity.PersonKeyExpr + ` AS person_key,
           e.name AS name, e.session_id AS session_id, e.duration_ms AS duration_ms, e.event_time AS event_time
    FROM events AS e
    LEFT JOIN links AS l ON l.install_id = e.install_id
    WHERE ` + eventsFilter + `
)`

const (
	paramDateTime = "2006-01-02 15:04:05.000"
	day           = 24 * time.Hour
)

// Period — календарные даты from..to включительно в TZ приложения. Используется только дата значений
// (год, месяц, день), время и пояс игнорируются.
type Period struct {
	From, To time.Time
}

// Days — число дней периода включительно (from = to — один день).
func (p Period) Days() int {
	return int(civil(p.To).Sub(civil(p.From))/day) + 1
}

// Bounds — полуинтервал [from 00:00, to+1 00:00) в TZ loc, переведённый в UTC. Сутки перехода DST длятся
// 23 или 25 часов.
func (p Period) Bounds(loc *time.Location) (start, end time.Time) {
	y, m, d := p.From.Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
	y, m, d = p.To.Date()
	end = time.Date(y, m, d+1, 0, 0, 0, 0, loc).UTC()
	return start, end
}

// civil — дата t в полночь UTC.
func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Params — общие параметры отчёта приложения.
type Params struct {
	AppID    uuid.UUID
	Location *time.Location // TZ приложения (apps.timezone); nil — UTC
	Env      string         // EnvProduction | EnvDevelopment
	Unit     identity.Unit
	Period   Period
	Filters  Filters
}

func (p Params) location() *time.Location { return orUTC(p.Location) }

// Parameters — значения {app_id}, {env}, {unit}, {tz}, {start_utc}, {end_utc}, {d_lo}, {d_hi} и {f_*}.
func (p Params) Parameters() clickhouse.Parameters {
	loc := p.location()
	start, end := p.Period.Bounds(loc)
	out := clickhouse.Parameters{
		"app_id":    p.AppID.String(),
		"env":       p.Env,
		"unit":      string(p.Unit),
		"tz":        loc.String(),
		"start_utc": start.Format(paramDateTime),
		"end_utc":   end.Format(paramDateTime),
		"d_lo":      start.Format(time.DateOnly),
		"d_hi":      end.Format(time.DateOnly),
	}
	p.Filters.bind(out)
	return out
}

// Bind — контекст запроса с параметрами отчёта. session_timezone=UTC: значения {*_utc:DateTime64(3)}
// без пояса читаются как UTC независимо от пояса сервера.
func (p Params) Bind(ctx context.Context) context.Context {
	return p.bindWith(ctx, nil)
}

// bindWith — Bind с дополнительными параметрами запроса (например, {date:Date} портфеля).
func (p Params) bindWith(ctx context.Context, extra clickhouse.Parameters) context.Context {
	params := p.Parameters()
	maps.Copy(params, extra)
	return clickhouse.Context(ctx,
		clickhouse.WithParameters(params),
		clickhouse.WithSettings(clickhouse.Settings{"session_timezone": "UTC"}),
	)
}
