package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/identity"
)

// Окно realtime (FR-015): активные — сессии, начатые за последние 30 минут; поминутные старты — 30 полных
// минут; лента — до 50 событий, принятых за последние 30 минут.
const (
	RealtimeWindow    = 30 * time.Minute
	RealtimeMinutes   = 30
	RecentEventsLimit = 50
)

// Запросы realtime (05-data-model.md §10.4, QRealtimeActiveNow / QRealtimePerMinute / QRealtimeRecentEvents).
// unit всегда persons, фильтры отчётов не применяются. Отличия от канонического текста (OQ-046):
//   - active_now: признаки начала и конца сессии — max(name = …), а не countIf по events (OQ-042);
//   - per_minute: 30 полных минут [{m_from_utc}, {m_to_utc}), текущая неполная минута не входит
//     (06-api-contracts.md §2.4.6);
//   - recent_events: LIMIT 1 BY insert_id после ORDER BY received_at DESC — до слияния частей ретрай виден
//     одной строкой с последним received_at, как после слияния ReplacingMergeTree(received_at); принятые
//     позже {now_utc} не показываются.
const (
	qRealtimeActiveNow = `WITH ` + identity.LinksCTE + `,
s AS (
    SELECT e.session_id AS session_id, any(e.install_id) AS install_id, max(e.user_id) AS user_id,
           max(e.name = 'session_start') AS has_start, max(e.name = 'session_end') AS has_end
    FROM events AS e
    WHERE e.app_id = {app_id:UUID} AND e.environment = {env:String}
      AND e.event_date BETWEEN {d_lo:Date} AND {d_hi:Date}
      AND e.event_time >= {from_utc:DateTime64(3)}
      AND e.event_time <= {future_utc:DateTime64(3)}
    GROUP BY e.session_id
)
SELECT uniqExact(` + identity.PersonKeyExpr + `) AS active_now
FROM s AS e LEFT JOIN links AS l ON l.install_id = e.install_id
WHERE e.has_start = 1 AND e.has_end = 0`

	qRealtimePerMinute = `SELECT toStartOfMinute(e.event_time) AS minute, uniqExact(e.session_id) AS sessions_started
FROM events AS e
WHERE e.app_id = {app_id:UUID} AND e.environment = {env:String} AND e.name = 'session_start'
  AND e.event_date BETWEEN {d_lo:Date} AND {d_hi:Date}
  AND e.event_time >= {m_from_utc:DateTime64(3)}
  AND e.event_time < {m_to_utc:DateTime64(3)}
GROUP BY minute
ORDER BY minute`

	qRealtimeRecentEvents = `WITH ` + identity.LinksCTE + `
SELECT e.insert_id AS insert_id, e.event_time AS event_time, e.received_at AS received_at, e.name AS name,
       e.environment AS environment, e.platform AS platform, e.app_version AS app_version, e.os_name AS os_name,
       e.os_version AS os_version, e.device_model AS device_model, e.country AS country, e.city AS city,
       e.install_id AS install_id, ` + identity.PersonKeyExpr + ` AS person_key
FROM events AS e
LEFT JOIN links AS l ON l.install_id = e.install_id
WHERE e.app_id = {app_id:UUID} AND e.environment = {env:String}
  AND e.event_date >= {late_d_lo:Date}
  AND e.received_at >= {from_utc:DateTime64(3)}
  AND e.received_at <= {now_utc:DateTime64(3)}
ORDER BY e.received_at DESC, e.event_time DESC, e.insert_id
LIMIT 1 BY insert_id
LIMIT 50` // RecentEventsLimit
)

// Допуски realtime: события чуть впереди часов сервера (ingest прижимает timestamp дальше 10 минут в будущем)
// и поздняя доставка в ленту (ingest принимает события до 7 суток назад, плюс сутки на границу UTC-даты).
const (
	realtimeFutureTolerance = 10 * time.Minute
	realtimeLateWindow      = 8 * 24 * time.Hour
)

// Realtime — срез «прямо сейчас» приложения (S-07).
type Realtime struct {
	ActiveNow    uint64
	PerMinute    []RealtimeMinute
	RecentEvents []RecentEvent
	GeneratedAt  time.Time
}

// RealtimeMinute — число сессий, начатых в минуту Minute (начало минуты в TZ приложения).
type RealtimeMinute struct {
	Minute          time.Time
	SessionsStarted uint64
}

// RecentEvent — событие ленты realtime.
type RecentEvent struct {
	InsertID    uuid.UUID `ch:"insert_id"`
	EventTime   time.Time `ch:"event_time"`
	ReceivedAt  time.Time `ch:"received_at"`
	Name        string    `ch:"name"`
	Environment string    `ch:"environment"`
	Platform    string    `ch:"platform"`
	AppVersion  string    `ch:"app_version"`
	OSName      string    `ch:"os_name"`
	OSVersion   string    `ch:"os_version"`
	DeviceModel string    `ch:"device_model"`
	Country     string    `ch:"country"`
	City        string    `ch:"city"`
	InstallID   uuid.UUID `ch:"install_id"`
	PersonKey   string    `ch:"person_key"`
}

// Realtime — срез приложения p.AppID в environment p.Env ("" — production) на текущий момент часов сервиса.
// Период, unit и фильтры p не используются: realtime всегда считает людей приложения без фильтров.
func (s *Service) Realtime(ctx context.Context, p Params) (Realtime, error) {
	now := s.clock.Now().UTC()
	env := p.Env
	if env == "" {
		env = EnvProduction
	}
	from := now.Add(-RealtimeWindow)
	mTo := now.Truncate(time.Minute)
	mFrom := mTo.Add(-RealtimeMinutes * time.Minute)
	future := now.Add(realtimeFutureTolerance)
	ctx = clickhouse.Context(ctx,
		clickhouse.WithParameters(clickhouse.Parameters{
			"app_id":     p.AppID.String(),
			"env":        env,
			"unit":       string(identity.Persons),
			"now_utc":    now.Format(paramDateTime),
			"from_utc":   from.Format(paramDateTime),
			"future_utc": future.Format(paramDateTime),
			"m_from_utc": mFrom.Format(paramDateTime),
			"m_to_utc":   mTo.Format(paramDateTime),
			"d_lo":       mFrom.Format(time.DateOnly), // mFrom ≤ from
			"d_hi":       future.Format(time.DateOnly),
			"late_d_lo":  now.Add(-realtimeLateWindow).Format(time.DateOnly),
		}),
		clickhouse.WithSettings(clickhouse.Settings{"session_timezone": "UTC"}),
	)

	out := Realtime{GeneratedAt: now, RecentEvents: []RecentEvent{}}
	if err := s.conn.QueryRow(ctx, qRealtimeActiveNow).Scan(&out.ActiveNow); err != nil {
		return Realtime{}, fmt.Errorf("reports: realtime active_now: %w", err)
	}
	counts, err := s.realtimeStarts(ctx)
	if err != nil {
		return Realtime{}, err
	}
	out.PerMinute = perMinute(now, p.location(), counts)
	if err := s.conn.Select(ctx, &out.RecentEvents, qRealtimeRecentEvents); err != nil {
		return Realtime{}, fmt.Errorf("reports: realtime recent_events: %w", err)
	}
	return out, nil
}

// realtimeStarts — число начатых сессий по началу минуты (Unix).
func (s *Service) realtimeStarts(ctx context.Context) (map[int64]uint64, error) {
	rows, err := s.conn.Query(ctx, qRealtimePerMinute)
	if err != nil {
		return nil, fmt.Errorf("reports: realtime per_minute: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := map[int64]uint64{}
	for rows.Next() {
		var (
			minute time.Time
			n      uint64
		)
		if err := rows.Scan(&minute, &n); err != nil {
			return nil, fmt.Errorf("reports: realtime per_minute: %w", err)
		}
		counts[minute.Unix()] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reports: realtime per_minute: %w", err)
	}
	return counts, nil
}

// perMinute — RealtimeMinutes точек: 30 полных минут до минуты now (текущая неполная не входит), начала минут —
// в loc (nil — UTC); counts — стартов по началу минуты (Unix), отсутствующие минуты — 0.
func perMinute(now time.Time, loc *time.Location, counts map[int64]uint64) []RealtimeMinute {
	loc = orUTC(loc)
	first := now.Truncate(time.Minute).Add(-RealtimeMinutes * time.Minute)
	out := make([]RealtimeMinute, 0, RealtimeMinutes)
	for i := range RealtimeMinutes {
		m := first.Add(time.Duration(i) * time.Minute)
		out = append(out, RealtimeMinute{Minute: m.In(loc), SessionsStarted: counts[m.Unix()]})
	}
	return out
}
