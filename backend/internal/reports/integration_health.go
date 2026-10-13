package reports

import (
	"context"
	"errors"
	"time"
)

// Окно экрана «Интеграция» (FR-017): hours 1..168, по умолчанию 24 — rejected_events хранятся 7 дней.
const (
	DefaultIntegrationHours = 24
	MaxIntegrationHours     = 168
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

var errIntegrationNotImplemented = errors.New("reports: здоровье интеграции не реализовано")

// IntegrationHealth — здоровье интеграции приложения p.AppID за hours часов.
func (s *Service) IntegrationHealth(_ context.Context, _ Params, _ int) (IntegrationHealth, error) {
	return IntegrationHealth{}, errIntegrationNotImplemented
}

// integrationWindowStart — начало первого часа окна.
func integrationWindowStart(now time.Time, _ *time.Location, _ int) time.Time {
	return now
}

// integrationHours — hours точек почасового ряда, последняя — час now в loc.
func integrationHours(_ time.Time, _ *time.Location, _ int, _, _ map[int64]uint64) []IntegrationHour {
	return nil
}
