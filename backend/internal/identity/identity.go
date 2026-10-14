// Package identity — склейка установок и пользователей в людей (FR-007, ADR-003 «первый владелец»):
// SQL-фрагменты CTE links и persons_first, выражение person_key и ключи u:<user_id> / i:<install_id>.
//
// Склейка вычисляется при чтении, сырые события не переписываются. Первый владелец установки —
// user_id самого раннего по event_time её события с непустым user_id (argMinMerge по identity_links);
// анонимные события установки принадлежат ему, событие с явным user_id — этому user_id.
package identity

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Unit — единица подсчёта отчётов: люди (склейка) или установки (сверка с AppMetrica).
type Unit string

const (
	Persons  Unit = "persons"
	Installs Unit = "installs"
)

// MaxUserIDLength — предел user_id в символах (ingest отклоняет более длинные).
const MaxUserIDLength = 128

var (
	ErrInvalidUnit      = errors.New("identity: unit — persons или installs")
	ErrInvalidPersonKey = errors.New("identity: person_key — u:<user_id> или i:<install_id>")
)

// ParseUnit разбирает параметр unit; пустая строка — значение по умолчанию persons.
func ParseUnit(s string) (Unit, error) {
	switch u := Unit(s); u {
	case "":
		return Persons, nil
	case Persons, Installs:
		return u, nil
	default:
		return "", ErrInvalidUnit
	}
}

// SQL-фрагменты (05-data-model.md §10.4). Параметры: {app_id:UUID}, {env:String}, {unit:String}.
// Таблицы без имени базы: соединение открыто с базой CLICKHOUSE_DATABASE.
const (
	// LinksCTE — первый владелец каждой установки приложения.
	LinksCTE = `links AS (
    SELECT install_id, argMinMerge(first_user) AS linked_user
    FROM identity_links
    WHERE app_id = {app_id:UUID}
    GROUP BY install_id
)`

	// PersonKeyExpr — person_key события events AS e, присоединённого к links AS l (LEFT JOIN по install_id).
	// unit=installs — всегда установка.
	PersonKeyExpr = `if({unit:String} = 'installs',
        concat('i:', toString(e.install_id)),
        if(e.user_id != '',
            concat('u:', e.user_id),
            if(l.linked_user != '', concat('u:', l.linked_user), concat('i:', toString(e.install_id)))))`

	// Склейка после предагрегации — то же, что PersonKeyExpr, но links присоединяется не к каждому событию,
	// а к уже свёрнутым анонимным установкам. UserPersonKeyExpr — человек события events AS e с user_id
	// (unit=persons). AnonPersonKeyExpr — человек установки a.install_id, присоединённой AnonLinksJoin: первый
	// владелец или сама установка; при unit=installs связка не присоединяется и ключ — всегда установка.
	UserPersonKeyExpr = `concat('u:', e.user_id)`
	AnonLinksJoin     = `LEFT JOIN links AS l ON l.install_id = a.install_id AND {unit:String} = 'persons'`
	AnonPersonKeyExpr = `if(l.linked_user != '', concat('u:', l.linked_user), concat('i:', toString(a.install_id)))`

	// InstallFirstSeenSQL — first_seen установок приложения в environment: строки (install_id, first_seen).
	InstallFirstSeenSQL = `SELECT install_id, min(first_seen) AS first_seen
            FROM install_first_seen
            WHERE app_id = {app_id:UUID} AND environment = {env:String}
            GROUP BY install_id`

	// UserFirstSeenSQL — first_seen людей u:<user_id> по их явным событиям: строки (person_key, first_seen);
	// при unit=installs строк нет.
	UserFirstSeenSQL = `SELECT concat('u:', user_id) AS person_key, min(first_seen) AS first_seen
        FROM user_first_seen
        WHERE app_id = {app_id:UUID} AND environment = {env:String} AND {unit:String} = 'persons'
        GROUP BY user_id`

	// PersonsFirstCTE — first_seen человека в environment: для u:X минимум из user_first_seen[X] и
	// install_first_seen установок с первым владельцем X, для i:I — install_first_seen[I]. Требует LinksCTE.
	PersonsFirstCTE = `persons_first AS (
    SELECT person_key, min(first_seen) AS first_seen
    FROM (
        SELECT if({unit:String} = 'installs' OR l.linked_user = '',
                  concat('i:', toString(f.install_id)),
                  concat('u:', l.linked_user)) AS person_key,
               f.first_seen AS first_seen
        FROM (
            ` + InstallFirstSeenSQL + `
        ) AS f
        LEFT JOIN links AS l ON l.install_id = f.install_id
        UNION ALL
        ` + UserFirstSeenSQL + `
    )
    GROUP BY person_key
)`
)

const (
	userPrefix    = "u:"
	installPrefix = "i:"
)

// PersonKey — человек отчётов: пользователь (UserID != "") или анонимная установка.
type PersonKey struct {
	UserID    string
	InstallID uuid.UUID
}

func UserKey(userID string) PersonKey          { return PersonKey{UserID: userID} }
func InstallKey(installID uuid.UUID) PersonKey { return PersonKey{InstallID: installID} }

func (k PersonKey) IsUser() bool { return k.UserID != "" }

// String — u:<user_id> или i:<install_id>, как person_key в SQL.
func (k PersonKey) String() string {
	if k.IsUser() {
		return userPrefix + k.UserID
	}
	return installPrefix + k.InstallID.String()
}

// ParsePersonKey разбирает u:<user_id> (1..128 символов) или i:<install_id> (канонический UUID, не nil).
func ParsePersonKey(s string) (PersonKey, error) {
	if user, ok := strings.CutPrefix(s, userPrefix); ok {
		if user == "" || utf8.RuneCountInString(user) > MaxUserIDLength {
			return PersonKey{}, ErrInvalidPersonKey
		}
		return UserKey(user), nil
	}
	if raw, ok := strings.CutPrefix(s, installPrefix); ok {
		id, err := uuid.Parse(raw)
		// uuid.Parse принимает и {…}, urn:uuid:, запись без дефисов и заглавные буквы: ключ один на установку.
		if err != nil || id == uuid.Nil || id.String() != raw {
			return PersonKey{}, ErrInvalidPersonKey
		}
		return InstallKey(id), nil
	}
	return PersonKey{}, ErrInvalidPersonKey
}
