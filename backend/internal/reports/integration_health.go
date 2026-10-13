package reports

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Окно экрана «Интеграция» (FR-017): hours 1..168, по умолчанию 24 — rejected_events хранятся 7 дней.
const (
	DefaultIntegrationHours = 24
	MaxIntegrationHours     = 168
	IntegrationVersionsMax  = 50
)

// Запросы экрана «Интеграция» (05-data-model.md §10.4, QIntegration*). Окно — [{since_utc}, {now_utc}] по received_at,
// {since_utc} — начало первого часа ряда в TZ приложения. Отличия от канонического текста (OQ-048):
//   - first_event_at — minOrNull: у приложения без событий null, а не 1970-01-01;
//   - accepted — uniqExact(insert_id) в часе последнего приёма (argMax по received_at): до слияния частей ретрай
//     считается одним событием, как после слияния ReplacingMergeTree(received_at); count() по events запрещён
//     линтером отчётных SQL;
//   - hourly и versions считаются одним запросом на таблицу (час × версия), суммы и слияние — в Go;
//   - строки rejected_events с nil app_id (нераспознанный ключ) отсекает условие app_id.
const (
	qIntegrationFirstEvent = `SELECT minOrNull(first_seen) AS first_event_at
FROM install_first_seen
WHERE app_id = {app_id:UUID}`

	qIntegrationAccepted = `WITH acc AS (
    SELECT e.insert_id AS insert_id, max(e.received_at) AS last_received,
           argMax(e.app_version, e.received_at) AS app_version
    FROM events AS e
    WHERE e.app_id = {app_id:UUID} AND e.event_date >= {d_lo:Date}
      AND e.received_at >= {since_utc:DateTime64(3)}
      AND e.received_at <= {now_utc:DateTime64(3)}
    GROUP BY e.insert_id
)
SELECT toStartOfHour(last_received, {tz:String}) AS hour, app_version, uniqExact(insert_id) AS n,
       max(last_received) AS last_seen
FROM acc
GROUP BY hour, app_version`

	qIntegrationRejected = `SELECT toStartOfHour(received_at, {tz:String}) AS hour, app_version, count() AS n,
       max(received_at) AS last_seen
FROM rejected_events
WHERE app_id = {app_id:UUID}
  AND received_at >= {since_utc:DateTime64(3)}
  AND received_at <= {now_utc:DateTime64(3)}
GROUP BY hour, app_version`

	qIntegrationReasons = `SELECT code, count() AS count, max(received_at) AS last_seen,
       argMax(payload_sample, received_at) AS example_payload, argMax(message, received_at) AS example_message
FROM rejected_events
WHERE app_id = {app_id:UUID}
  AND received_at >= {since_utc:DateTime64(3)}
  AND received_at <= {now_utc:DateTime64(3)}
GROUP BY code
ORDER BY count DESC, code`
)

// IntegrationHealth — здоровье интеграции приложения за последние Hours часов (S-10).
type IntegrationHealth struct {
	Hours         int
	FirstEventAt  *time.Time // nil — приложение ещё не прислало ни одного события
	AcceptedTotal uint64
	RejectedTotal uint64
	Hourly        []IntegrationHour
	Reasons       []IntegrationReason
	Versions      []IntegrationVersion
}

// IntegrationHour — принятые и отклонённые события часа Hour (начало часа в TZ приложения).
type IntegrationHour struct {
	Hour     time.Time
	Accepted uint64
	Rejected uint64
}

// IntegrationReason — причина отказа с последним примером.
type IntegrationReason struct {
	Code           string    `ch:"code"`
	Count          uint64    `ch:"count"`
	LastSeen       time.Time `ch:"last_seen"`
	ExamplePayload string    `ch:"example_payload"`
	ExampleMessage string    `ch:"example_message"`
}

// IntegrationVersion — принятые и отклонённые события версии приложения.
type IntegrationVersion struct {
	AppVersion string
	Accepted   uint64
	Rejected   uint64
	LastSeen   time.Time
}

// hourVersionRow — события часа и версии приложения из qIntegrationAccepted / qIntegrationRejected.
type hourVersionRow struct {
	Hour       time.Time `ch:"hour"`
	AppVersion string    `ch:"app_version"`
	N          uint64    `ch:"n"`
	LastSeen   time.Time `ch:"last_seen"`
}

// IntegrationHealth — здоровье интеграции приложения p.AppID за hours часов (1..MaxIntegrationHours, иначе
// ErrUnsupported) на текущий момент часов сервиса: события всех environment, часы — в TZ p.Location.
// Период, environment, unit и фильтры p не используются.
func (s *Service) IntegrationHealth(ctx context.Context, p Params, hours int) (IntegrationHealth, error) {
	if hours < 1 || hours > MaxIntegrationHours {
		return IntegrationHealth{}, fmt.Errorf("%w: hours", ErrUnsupported)
	}
	now := s.clock.Now().UTC()
	loc := p.location()
	since := integrationWindowStart(now, loc, hours)
	ctx = clickhouse.Context(ctx,
		clickhouse.WithParameters(clickhouse.Parameters{
			"app_id":    p.AppID.String(),
			"tz":        loc.String(),
			"since_utc": since.UTC().Format(paramDateTime),
			"now_utc":   now.Format(paramDateTime),
			"d_lo":      since.Add(-realtimeLateWindow).UTC().Format(time.DateOnly), // поздняя доставка, как в realtime
		}),
		clickhouse.WithSettings(clickhouse.Settings{"session_timezone": "UTC"}),
	)

	out := IntegrationHealth{Hours: hours, Reasons: []IntegrationReason{}}
	if err := s.conn.QueryRow(ctx, qIntegrationFirstEvent).Scan(&out.FirstEventAt); err != nil {
		return IntegrationHealth{}, fmt.Errorf("reports: integration first_event_at: %w", err)
	}
	if out.FirstEventAt != nil {
		first := out.FirstEventAt.UTC()
		out.FirstEventAt = &first
	}
	var accepted, rejected []hourVersionRow
	if err := s.conn.Select(ctx, &accepted, qIntegrationAccepted); err != nil {
		return IntegrationHealth{}, fmt.Errorf("reports: integration accepted: %w", err)
	}
	if err := s.conn.Select(ctx, &rejected, qIntegrationRejected); err != nil {
		return IntegrationHealth{}, fmt.Errorf("reports: integration rejected: %w", err)
	}
	if err := s.conn.Select(ctx, &out.Reasons, qIntegrationReasons); err != nil {
		return IntegrationHealth{}, fmt.Errorf("reports: integration reasons: %w", err)
	}
	for i := range out.Reasons {
		out.Reasons[i].LastSeen = out.Reasons[i].LastSeen.UTC()
	}

	acceptedByHour, rejectedByHour := byHour(accepted), byHour(rejected)
	out.Hourly = integrationHours(now, loc, hours, acceptedByHour, rejectedByHour)
	for _, h := range out.Hourly {
		out.AcceptedTotal += h.Accepted
		out.RejectedTotal += h.Rejected
	}
	out.Versions = integrationVersions(accepted, rejected)
	return out, nil
}

// byHour — число событий по началу часа (Unix).
func byHour(rows []hourVersionRow) map[int64]uint64 {
	out := make(map[int64]uint64, len(rows))
	for _, r := range rows {
		out[r.Hour.Unix()] += r.N
	}
	return out
}

// integrationVersions сливает принятые и отклонённые события по app_version: по убыванию last_seen, при равенстве —
// по версии, не больше IntegrationVersionsMax строк.
func integrationVersions(accepted, rejected []hourVersionRow) []IntegrationVersion {
	byVersion := map[string]*IntegrationVersion{}
	add := func(r hourVersionRow, isAccepted bool) {
		v, ok := byVersion[r.AppVersion]
		if !ok {
			v = &IntegrationVersion{AppVersion: r.AppVersion}
			byVersion[r.AppVersion] = v
		}
		if isAccepted {
			v.Accepted += r.N
		} else {
			v.Rejected += r.N
		}
		if last := r.LastSeen.UTC(); last.After(v.LastSeen) {
			v.LastSeen = last
		}
	}
	for _, r := range accepted {
		add(r, true)
	}
	for _, r := range rejected {
		add(r, false)
	}
	out := make([]IntegrationVersion, 0, len(byVersion))
	for _, v := range byVersion {
		out = append(out, *v)
	}
	slices.SortFunc(out, func(a, b IntegrationVersion) int {
		if c := b.LastSeen.Compare(a.LastSeen); c != 0 {
			return c
		}
		return cmp.Compare(a.AppVersion, b.AppVersion)
	})
	return out[:min(len(out), IntegrationVersionsMax)]
}

// integrationWindowStart — начало первого из hours часов окна: часы отсчитываются назад от начала часа now в loc
// (nil — UTC), у TZ со смещением +05:30 час начинается в :30 UTC.
func integrationWindowStart(now time.Time, loc *time.Location, hours int) time.Time {
	local := now.In(orUTC(loc))
	sinceHour := time.Duration(local.Minute())*time.Minute + time.Duration(local.Second())*time.Second +
		time.Duration(local.Nanosecond())
	return local.Add(-sinceHour - time.Duration(hours-1)*time.Hour)
}

// integrationHours — hours точек почасового ряда до часа now включительно (текущий неполный час — последняя
// точка), начала часов — в loc (nil — UTC); accepted и rejected — событий по началу часа (Unix), отсутствующие
// часы — 0.
func integrationHours(now time.Time, loc *time.Location, hours int, accepted, rejected map[int64]uint64) []IntegrationHour {
	first := integrationWindowStart(now, loc, hours)
	out := make([]IntegrationHour, 0, hours)
	for i := range hours {
		h := first.Add(time.Duration(i) * time.Hour)
		out = append(out, IntegrationHour{Hour: h, Accepted: accepted[h.Unix()], Rejected: rejected[h.Unix()]})
	}
	return out
}
