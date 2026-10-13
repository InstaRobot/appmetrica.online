package reports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/identity"
)

// Пределы профилей (FR-016, 06-api-contracts.md §2.4.7).
const (
	MaxProfileResults    = 20
	DefaultSessionsLimit = 100
	MaxSessionsLimit     = 500
)

// ErrProfileNotFound — у person_key нет ни одного события в приложении (404 profile_not_found).
var ErrProfileNotFound = errors.New("reports: профиль не найден")

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

var errProfilesNotImplemented = errors.New("reports: профили не реализованы")

// SearchProfiles ищет профили приложения appID по точному install_id или user_id q.
func (s *Service) SearchProfiles(_ context.Context, _ uuid.UUID, _ string) ([]ProfileSummary, error) {
	return nil, errProfilesNotImplemented
}

// Profile — карточка профиля приложения appID.
func (s *Service) Profile(_ context.Context, _ uuid.UUID, _ ProfileQuery) (Profile, error) {
	return Profile{}, errProfilesNotImplemented
}
