package reports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Окно realtime (FR-015): активные и поминутные старты — за последние 30 минут, лента — до 50 событий,
// принятых за последние 30 минут.
const (
	RealtimeWindow    = 30 * time.Minute
	RealtimeMinutes   = 30
	RecentEventsLimit = 50
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

// Realtime — срез приложения p.AppID в environment p.Env на текущий момент.
func (s *Service) Realtime(ctx context.Context, p Params) (Realtime, error) {
	return Realtime{}, errors.New("reports: realtime: not implemented")
}

// perMinute — RealtimeMinutes точек: 30 полных минут до минуты now (текущая неполная не входит), начала минут —
// в loc; counts — стартов по началу минуты (Unix), отсутствующие минуты — 0.
func perMinute(now time.Time, loc *time.Location, counts map[int64]uint64) []RealtimeMinute {
	return nil
}
