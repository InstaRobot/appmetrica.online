package reports

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/identity"
)

// Пределы профилей (FR-016, 06-api-contracts.md §2.4.7).
const (
	MaxProfileResults    = 20
	DefaultSessionsLimit = 100
	MaxSessionsLimit     = 500

	DefaultProfileEventsLimit = 50
	MaxProfileEventsLimit     = 200
)

// ErrProfileNotFound — у person_key нет ни одного события в приложении (404 profile_not_found).
var ErrProfileNotFound = errors.New("reports: профиль не найден")

// ErrInvalidCursor — курсор before событий профиля не разобран (400 invalid_request, details.field=before).
var ErrInvalidCursor = errors.New("reports: неверный курсор")

// MatchedBy — по какому идентификатору найден профиль.
type MatchedBy string

const (
	MatchedByInstallID MatchedBy = "install_id"
	MatchedByUserID    MatchedBy = "user_id"
)

// ProfileSummary — найденный профиль (S-08).
type ProfileSummary struct {
	PersonKey     identity.PersonKey
	MatchedBy     MatchedBy
	InstallsCount int
	FirstSeen     time.Time
	LastSeen      time.Time
}

// ProfileQuery — запрос карточки профиля: PersonKey — u:<user_id> или i:<install_id>, SessionsLimit 0 —
// DefaultSessionsLimit, Before — курсор (исключающая граница started_at), нулевой — первая страница.
type ProfileQuery struct {
	PersonKey     string
	SessionsLimit int
	Before        time.Time
}

// Profile — карточка профиля (S-09).
type Profile struct {
	PersonKey  identity.PersonKey
	FirstSeen  time.Time
	LastSeen   time.Time
	Installs   []ProfileInstall
	Sessions   []ProfileSession
	NextBefore *time.Time // nil — страниц больше нет
}

// ProfileInstall — установка профиля с контекстом устройства из её последнего события.
type ProfileInstall struct {
	InstallID   uuid.UUID `ch:"install_id"`
	Platform    string    `ch:"platform"`
	OSName      string    `ch:"os_name"`
	OSVersion   string    `ch:"os_version"`
	AppVersion  string    `ch:"app_version"`
	DeviceModel string    `ch:"device_model"`
	DeviceType  string    `ch:"device_type"`
	Locale      string    `ch:"locale"`
	Country     string    `ch:"country"`
	City        string    `ch:"city"`
	FirstSeen   time.Time `ch:"first_seen"`
	LastSeen    time.Time `ch:"last_seen"`
}

// ProfileSession — сессия профиля; EndedAt и DurationMs — nil у сессии без session_end.
type ProfileSession struct {
	SessionID   uuid.UUID  `ch:"session_id"`
	InstallID   uuid.UUID  `ch:"install_id"`
	StartedAt   time.Time  `ch:"started_at"`
	EndedAt     *time.Time `ch:"ended_at"`
	DurationMs  *uint32    `ch:"duration_ms"`
	AppVersion  string     `ch:"app_version"`
	Platform    string     `ch:"platform"`
	Environment string     `ch:"environment"`
	Country     string     `ch:"country"`
	City        string     `ch:"city"`
}

// Запросы профилей (05-data-model.md §10.4, QProfileSearchByInstall / QProfileUserInstalls / QProfileInstalls /
// QProfileSessions). Профиль ищется по всем environment. Отличия от канонического текста (OQ-047):
//   - человек профиля — события установок {install_ids} с person_key = {person_key} (склейка «первый владелец»):
//     first/last seen и наличие профиля считаются по ним, а не по install_first_seen / user_first_seen;
//   - признаки начала и конца сессии — max(name = …), а не countIf по events (OQ-042); ended_at — event_time
//     session_end, атрибуты сессии — из её первого события; установки — с locale;
//   - курсор — {has_before:UInt8} и {before:DateTime64(3)} вместо Nullable-параметра.
const (
	qProfileInstallOwner = `SELECT argMinMerge(first_user) AS linked_user
FROM identity_links
WHERE app_id = {app_id:UUID} AND install_id = {install_id:UUID}`

	qProfileUserInstalls = `SELECT install_id FROM (
    SELECT install_id FROM identity_links
    WHERE app_id = {app_id:UUID}
    GROUP BY install_id
    HAVING argMinMerge(first_user) = {user_id:String}
    UNION DISTINCT
    SELECT DISTINCT install_id FROM events
    WHERE app_id = {app_id:UUID} AND user_id = {user_id:String}
)`

	profileLinksCTE = `links AS (
    SELECT install_id, argMinMerge(first_user) AS linked_user
    FROM identity_links
    WHERE app_id = {app_id:UUID} AND install_id IN {install_ids:Array(UUID)}
    GROUP BY install_id
)`

	profileEventsFrom = `
FROM events AS e
LEFT JOIN links AS l ON l.install_id = e.install_id
WHERE e.app_id = {app_id:UUID} AND e.install_id IN {install_ids:Array(UUID)}
  AND ` + identity.PersonKeyExpr + ` = {person_key:String}`

	qProfilePerson = `WITH ` + profileLinksCTE + `
SELECT minOrNull(e.event_time) AS first_seen, maxOrNull(e.event_time) AS last_seen` + profileEventsFrom

	qProfileInstalls = `SELECT e.install_id AS install_id,
       argMax(e.platform, e.event_time) AS platform, argMax(e.os_name, e.event_time) AS os_name,
       argMax(e.os_version, e.event_time) AS os_version, argMax(e.app_version, e.event_time) AS app_version,
       argMax(e.device_model, e.event_time) AS device_model, argMax(e.device_type, e.event_time) AS device_type,
       argMax(e.locale, e.event_time) AS locale, argMax(e.country, e.event_time) AS country,
       argMax(e.city, e.event_time) AS city,
       min(e.event_time) AS first_seen, max(e.event_time) AS last_seen
FROM events AS e
WHERE e.app_id = {app_id:UUID} AND e.install_id IN {install_ids:Array(UUID)}
GROUP BY e.install_id
ORDER BY last_seen DESC, install_id`

	qProfileSessions = `WITH ` + profileLinksCTE + `,
pe AS (
    SELECT e.session_id AS session_id, e.install_id AS install_id, e.name AS name, e.event_time AS event_time,
           e.duration_ms AS duration_ms, e.app_version AS app_version, e.platform AS platform,
           e.environment AS environment, e.country AS country, e.city AS city` + profileEventsFrom + `
)
SELECT p.session_id AS session_id, any(p.install_id) AS install_id,
       if(max(p.name = 'session_start') = 1, minIf(p.event_time, p.name = 'session_start'), min(p.event_time)) AS started_at,
       if(max(p.name = 'session_end') = 1, toNullable(maxIf(p.event_time, p.name = 'session_end')), NULL) AS ended_at,
       if(max(p.name = 'session_end') = 1, toNullable(maxIf(p.duration_ms, p.name = 'session_end')), NULL) AS duration_ms,
       argMin(p.app_version, p.event_time) AS app_version, argMin(p.platform, p.event_time) AS platform,
       argMin(p.environment, p.event_time) AS environment, argMin(p.country, p.event_time) AS country,
       argMin(p.city, p.event_time) AS city
FROM pe AS p
GROUP BY p.session_id
HAVING {has_before:UInt8} = 0 OR started_at < {before:DateTime64(3)}
ORDER BY started_at DESC, session_id DESC
LIMIT {limit:UInt32}`
)

// qProfileEvents — свои события человека (системные имена не входят), новые первыми по (event_time, insert_id):
// LIMIT 1 BY insert_id оставляет одну строку повтора батча до слияния частей ReplacingMergeTree; курсор —
// исключающая граница {before_time}/{before_id} по тому же кортежу порядка. {limit} — на одно больше страницы.
var qProfileEvents = `WITH ` + profileLinksCTE + `
SELECT e.event_time AS event_time, e.insert_id AS insert_id, e.name AS name, e.session_id AS session_id,
       e.install_id AS install_id, e.environment AS environment, e.platform AS platform, e.app_version AS app_version,
       e.properties AS properties` +
	profileEventsFrom + `
  AND ({env:String} = '' OR e.environment = {env:String})` + customEventsWhere + `
  AND ({has_before:UInt8} = 0 OR (e.event_time, e.insert_id) < ({before_time:DateTime64(3)}, {before_id:UUID}))
ORDER BY e.event_time DESC, e.insert_id DESC
LIMIT 1 BY e.insert_id
LIMIT {limit:UInt32}`

// SearchProfiles ищет профили приложения appID по точному q: если q — UUID, установка q находится как профиль
// её человека (matched_by=install_id); q как user_id — профиль u:<q> (matched_by=user_id). Без совпадений —
// пустой список.
func (s *Service) SearchProfiles(ctx context.Context, appID uuid.UUID, q string) ([]ProfileSummary, error) {
	defer s.track(ReportProfileSearch)()
	out := []ProfileSummary{}
	if q == "" || utf8.RuneCountInString(q) > identity.MaxUserIDLength {
		return out, nil
	}
	type candidate struct {
		key identity.PersonKey
		by  MatchedBy
	}
	var candidates []candidate
	if id, err := uuid.Parse(q); err == nil && id != uuid.Nil {
		key, err := s.installPerson(ctx, appID, id)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate{key, MatchedByInstallID})
	}
	candidates = append(candidates, candidate{identity.UserKey(q), MatchedByUserID})

	for _, c := range candidates {
		if slices.ContainsFunc(out, func(p ProfileSummary) bool { return p.PersonKey == c.key }) {
			continue
		}
		p, ok, err := s.person(ctx, appID, c.key)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, ProfileSummary{
			PersonKey: c.key, MatchedBy: c.by, InstallsCount: len(p.installs), FirstSeen: p.firstSeen, LastSeen: p.lastSeen,
		})
	}
	return out, nil
}

// Profile — карточка профиля приложения appID: ошибки — identity.ErrInvalidPersonKey (формат person_key),
// ErrUnsupported (sessions_limit вне 1..MaxSessionsLimit), ErrProfileNotFound (нет событий человека).
func (s *Service) Profile(ctx context.Context, appID uuid.UUID, q ProfileQuery) (Profile, error) {
	defer s.track(ReportProfile)()
	key, err := identity.ParsePersonKey(q.PersonKey)
	if err != nil {
		return Profile{}, fmt.Errorf("reports: profile: %w", err)
	}
	limit := q.SessionsLimit
	if limit == 0 {
		limit = DefaultSessionsLimit
	}
	if limit < 1 || limit > MaxSessionsLimit {
		return Profile{}, fmt.Errorf("reports: profile sessions_limit %d: %w", limit, ErrUnsupported)
	}
	p, ok, err := s.person(ctx, appID, key)
	if err != nil {
		return Profile{}, err
	}
	if !ok {
		return Profile{}, ErrProfileNotFound
	}

	out := Profile{PersonKey: key, FirstSeen: p.firstSeen, LastSeen: p.lastSeen, Installs: []ProfileInstall{}, Sessions: []ProfileSession{}}
	params := personParameters(appID, key, p.installs)
	if err := s.conn.Select(profileContext(ctx, params), &out.Installs, qProfileInstalls); err != nil {
		return Profile{}, fmt.Errorf("reports: profile installs: %w", err)
	}
	params["has_before"], params["before"] = "0", time.Unix(0, 0).UTC().Format(paramDateTime)
	if !q.Before.IsZero() {
		params["has_before"], params["before"] = "1", ceilMillis(q.Before).Format(paramDateTime)
	}
	params["limit"] = strconv.Itoa(limit + 1) // лишняя строка — признак следующей страницы
	if err := s.conn.Select(profileContext(ctx, params), &out.Sessions, qProfileSessions); err != nil {
		return Profile{}, fmt.Errorf("reports: profile sessions: %w", err)
	}
	if len(out.Sessions) > limit {
		out.Sessions = out.Sessions[:limit]
		next := out.Sessions[limit-1].StartedAt
		out.NextBefore = &next
	}
	return out, nil
}

// person — установки и first/last seen человека key; ok = false, если у него нет событий.
type person struct {
	installs            []uuid.UUID
	firstSeen, lastSeen time.Time
}

func (s *Service) person(ctx context.Context, appID uuid.UUID, key identity.PersonKey) (person, bool, error) {
	installs := []uuid.UUID{key.InstallID}
	if key.IsUser() {
		var err error
		if installs, err = s.userInstalls(ctx, appID, key.UserID); err != nil {
			return person{}, false, err
		}
		if len(installs) == 0 {
			return person{}, false, nil
		}
	}
	var first, last *time.Time
	ctx = profileContext(ctx, personParameters(appID, key, installs))
	if err := s.conn.QueryRow(ctx, qProfilePerson).Scan(&first, &last); err != nil {
		return person{}, false, fmt.Errorf("reports: profile person: %w", err)
	}
	if first == nil || last == nil {
		return person{}, false, nil
	}
	return person{installs: installs, firstSeen: *first, lastSeen: *last}, true, nil
}

// installPerson — человек установки id: её первый владелец u:<user> или анонимная установка i:<id>.
func (s *Service) installPerson(ctx context.Context, appID, id uuid.UUID) (identity.PersonKey, error) {
	var linked string
	ctx = profileContext(ctx, clickhouse.Parameters{"app_id": appID.String(), "install_id": id.String()})
	if err := s.conn.QueryRow(ctx, qProfileInstallOwner).Scan(&linked); err != nil {
		return identity.PersonKey{}, fmt.Errorf("reports: profile install owner: %w", err)
	}
	if linked != "" {
		return identity.UserKey(linked), nil
	}
	return identity.InstallKey(id), nil
}

// userInstalls — установки пользователя: первый владелец которых user, и те, где были события с его user_id.
func (s *Service) userInstalls(ctx context.Context, appID uuid.UUID, user string) ([]uuid.UUID, error) {
	var rows []struct {
		InstallID uuid.UUID `ch:"install_id"`
	}
	ctx = profileContext(ctx, clickhouse.Parameters{"app_id": appID.String(), "user_id": chstore.StringParam(user)})
	if err := s.conn.Select(ctx, &rows, qProfileUserInstalls); err != nil {
		return nil, fmt.Errorf("reports: profile user installs: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.InstallID)
	}
	return out, nil
}

// personParameters — {app_id}, {unit}, {person_key} и {install_ids} запросов человека.
func personParameters(appID uuid.UUID, key identity.PersonKey, installs []uuid.UUID) clickhouse.Parameters {
	return clickhouse.Parameters{
		"app_id":      appID.String(),
		"unit":        string(identity.Persons),
		"person_key":  chstore.StringParam(key.String()),
		"install_ids": chstore.UUIDArrayParam(installs),
	}
}

func profileContext(ctx context.Context, params clickhouse.Parameters) context.Context {
	return clickhouse.Context(ctx,
		clickhouse.WithParameters(params),
		clickhouse.WithSettings(clickhouse.Settings{"session_timezone": "UTC"}),
	)
}

// ceilMillis — t, округлённое вверх до миллисекунды: для времени с точностью до миллисекунды (DateTime64(3))
// started_at < t ⇔ started_at < ceilMillis(t).
func ceilMillis(t time.Time) time.Time {
	t = t.UTC()
	c := t.Truncate(time.Millisecond)
	if c.Before(t) {
		c = c.Add(time.Millisecond)
	}
	return c
}

// ProfileEventsQuery — страница своих событий человека: Environment "" — все окружения (как у Profile), Limit 0 —
// DefaultProfileEventsLimit, Before — непрозрачный курсор NextBefore предыдущей страницы ("" — первая).
type ProfileEventsQuery struct {
	PersonKey   string
	Environment string
	Limit       int
	Before      string
}

// ProfileEvent — своё событие человека; Properties — всегда объект (пустой у события без свойств).
type ProfileEvent struct {
	EventTime   time.Time         `ch:"event_time"`
	InsertID    uuid.UUID         `ch:"insert_id"`
	Name        string            `ch:"name"`
	SessionID   uuid.UUID         `ch:"session_id"`
	InstallID   uuid.UUID         `ch:"install_id"`
	Environment string            `ch:"environment"`
	Platform    string            `ch:"platform"`
	AppVersion  string            `ch:"app_version"`
	Properties  map[string]string `ch:"properties"`
}

// ProfileEventPage — страница событий; NextBefore пуст на последней.
type ProfileEventPage struct {
	Events     []ProfileEvent
	NextBefore string
}

// ProfileEvents — свои события человека всех его установок, новые первыми. Ошибки: identity.ErrInvalidPersonKey,
// ErrUnsupported (limit вне 1..MaxProfileEventsLimit), ErrInvalidCursor, ErrProfileNotFound (нет событий человека
// в любом environment — как у Profile).
func (s *Service) ProfileEvents(ctx context.Context, appID uuid.UUID, q ProfileEventsQuery) (ProfileEventPage, error) {
	key, err := identity.ParsePersonKey(q.PersonKey)
	if err != nil {
		return ProfileEventPage{}, fmt.Errorf("reports: profile events: %w", err)
	}
	limit := q.Limit
	if limit == 0 {
		limit = DefaultProfileEventsLimit
	}
	if limit < 1 || limit > MaxProfileEventsLimit {
		return ProfileEventPage{}, fmt.Errorf("reports: profile events limit %d: %w", limit, ErrUnsupported)
	}
	env := q.Environment // "" — все окружения
	var beforeTime time.Time
	var beforeID uuid.UUID
	if q.Before != "" {
		if beforeTime, beforeID, err = decodeProfileEventsCursor(q.Before); err != nil {
			return ProfileEventPage{}, err
		}
	}
	p, ok, err := s.person(ctx, appID, key)
	if err != nil {
		return ProfileEventPage{}, err
	}
	if !ok {
		return ProfileEventPage{}, ErrProfileNotFound
	}

	params := personParameters(appID, key, p.installs)
	params["env"] = env
	params["has_before"], params["before_time"], params["before_id"] = "0", time.Unix(0, 0).UTC().Format(paramDateTime), uuid.Nil.String()
	if q.Before != "" {
		params["has_before"], params["before_time"], params["before_id"] = "1", beforeTime.Format(paramDateTime), beforeID.String()
	}
	params["limit"] = strconv.Itoa(limit + 1) // лишняя строка — признак следующей страницы
	out := ProfileEventPage{Events: []ProfileEvent{}}
	if err := s.conn.Select(profileContext(ctx, params), &out.Events, qProfileEvents); err != nil {
		return ProfileEventPage{}, fmt.Errorf("reports: profile events: %w", err)
	}
	if len(out.Events) > limit {
		out.Events = out.Events[:limit]
		last := out.Events[limit-1]
		out.NextBefore = encodeProfileEventsCursor(last.EventTime, last.InsertID)
	}
	for i := range out.Events {
		if out.Events[i].Properties == nil {
			out.Events[i].Properties = map[string]string{}
		}
	}
	return out, nil
}

// Границы времени курсора — диапазон DateTime64(3) ClickHouse (1900-01-01…2299-12-31): время вне него не
// форматируется в параметр запроса, и такой курсор — 400, а не ошибка хранилища.
var (
	minCursorMillis = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	maxCursorMillis = time.Date(2299, 12, 31, 23, 59, 59, 999_000_000, time.UTC).UnixMilli()
)

// Курсор событий профиля — base64url «<unix_ms>.<insert_id>»: время события с точностью до миллисекунды и
// insert_id последнего события страницы.
func encodeProfileEventsCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMilli(), 10) + "." + id.String()))
}

func decodeProfileEventsCursor(cursor string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	ms, id, ok := strings.Cut(string(raw), ".")
	if !ok {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	millis, err := strconv.ParseInt(ms, 10, 64)
	if err != nil || millis < minCursorMillis || millis > maxCursorMillis {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	return time.UnixMilli(millis).UTC(), parsed, nil
}
