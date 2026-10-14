package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/identity"
)

// qCloseDayInsert — строка daily_stats дня {day:Date} для {env} и {unit} (05-data-model.md §10.4, QCloseDayInsert).
// Активные и новые — как в qAudiencePeople, сессии и длительности — как в qAudienceSessions, поэтому закрытый
// день совпадает с getAudienceSummary за этот день. Длительность сессии — max(duration_ms) её session_end.
var qCloseDayInsert = `INSERT INTO daily_stats
    (app_id, day, environment, unit, active, new, sessions, sessions_with_duration, duration_ms_sum, closed_at)
WITH ` + identity.LinksCTE + `, ` + peopleFirstCTE(flag("1", "act")) + `
SELECT {app_id:UUID}, {day:Date}, {env:String}, {unit:String},
       p.active, p.new, s.sessions, d.sessions_with_duration, d.duration_ms_sum, {now_utc:DateTime64(3)}
FROM (
    SELECT uniqExactIf(person_key, act) AS active,
           uniqExactIf(person_key, act
               AND first_seen >= {start_utc:DateTime64(3)} AND first_seen < {end_utc:DateTime64(3)}) AS new
    FROM people
) AS p
CROSS JOIN (
    SELECT uniqExact(e.session_id) AS sessions
    FROM events AS e
    WHERE ` + eventsFilter + ` AND e.name = 'session_start'
) AS s
CROSS JOIN (
    SELECT uniqExact(session_id) AS sessions_with_duration, toUInt64(sum(duration_ms)) AS duration_ms_sum
    FROM (
        SELECT e.session_id AS session_id, max(e.duration_ms) AS duration_ms
        FROM events AS e
        WHERE ` + eventsFilter + ` AND e.name = 'session_end'
        GROUP BY session_id
    )
) AS d`

// CloseDay — job close_day (FR-022): 4 строки daily_stats дня day (календарная дата в TZ loc) —
// environment production/development × unit persons/installs, без фильтров, closed_at — сейчас. Повторное
// закрытие перезаписывает строки (ReplacingMergeTree(closed_at)).
func (s *Service) CloseDay(ctx context.Context, appID uuid.UUID, loc *time.Location, day time.Time) error {
	date := day.Format(time.DateOnly)
	extra := clickhouse.Parameters{"day": date, "now_utc": s.clock.Now().UTC().Format(paramDateTime)}
	for _, env := range []string{EnvProduction, EnvDevelopment} {
		for _, unit := range []identity.Unit{identity.Persons, identity.Installs} {
			p := Params{AppID: appID, Location: loc, Env: env, Unit: unit, Period: Period{From: day, To: day}}
			if err := s.conn.Exec(p.bindWith(ctx, extra), qCloseDayInsert); err != nil {
				return fmt.Errorf("reports: close day %s %s %s: %w", date, env, unit, err)
			}
		}
	}
	return nil
}
