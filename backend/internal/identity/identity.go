// Package identity — склейка установок и пользователей в людей (FR-007, ADR-003 «первый владелец»):
// SQL-фрагменты CTE links и persons_first, выражение person_key и ключи u:<user_id> / i:<install_id>.
package identity

import (
	"errors"

	"github.com/google/uuid"
)

// Unit — единица подсчёта отчётов.
type Unit string

const (
	Persons  Unit = "persons"
	Installs Unit = "installs"
)

var (
	ErrInvalidUnit      = errors.New("identity: unit — persons или installs")
	ErrInvalidPersonKey = errors.New("identity: person_key — u:<user_id> или i:<install_id>")
)

// ParseUnit — TODO(T-027).
func ParseUnit(string) (Unit, error) { return "", ErrInvalidUnit }

// TODO(T-027): SQL-фрагменты.
const (
	LinksCTE        = ``
	PersonKeyExpr   = ``
	PersonsFirstCTE = ``
)

// PersonKey — человек отчётов.
type PersonKey struct {
	UserID    string
	InstallID uuid.UUID
}

func UserKey(string) PersonKey       { return PersonKey{} }
func InstallKey(uuid.UUID) PersonKey { return PersonKey{} }

func (k PersonKey) IsUser() bool   { return false }
func (k PersonKey) String() string { return "" }

// ParsePersonKey — TODO(T-027).
func ParsePersonKey(string) (PersonKey, error) { return PersonKey{}, ErrInvalidPersonKey }
