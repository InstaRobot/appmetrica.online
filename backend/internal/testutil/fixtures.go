package testutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

// fixtureEvent — строка NDJSON-фикстуры (22-test-strategy.md §6). Обязательны app_id, event_time, install_id,
// session_id, name; client_time, sent_at и received_at по умолчанию равны event_time, environment — production.
type fixtureEvent struct {
	AppID       uuid.UUID `json:"app_id"`
	EventTime   time.Time `json:"event_time"`
	ClientTime  time.Time `json:"client_time"`
	SentAt      time.Time `json:"sent_at"`
	ReceivedAt  time.Time `json:"received_at"`
	InsertID    uuid.UUID `json:"insert_id"`
	Name        string    `json:"name"`
	InstallID   uuid.UUID `json:"install_id"`
	UserID      string    `json:"user_id"`
	SessionID   uuid.UUID `json:"session_id"`
	DurationMs  uint32    `json:"duration_ms"`
	Environment string    `json:"environment"`
	Platform    string    `json:"platform"`
	OSName      string    `json:"os_name"`
	OSVersion   string    `json:"os_version"`
	AppVersion  string    `json:"app_version"`
	AppBuild    string    `json:"app_build"`
	DeviceModel string    `json:"device_model"`
	DeviceType  string    `json:"device_type"`
	Locale      string    `json:"locale"`
	Country     string    `json:"country"`
	Region      string    `json:"region"`
	City        string    `json:"city"`
	WriteKeyID  uuid.UUID `json:"write_key_id"`
}

// fixtureNamespace — пространство имён детерминированных insert_id (UUID v5 от "<source>:<строка>").
var fixtureNamespace = uuid.MustParse("5f0c8f4e-2b7a-4d4e-9a51-6d1e0c3b7a10")

// ParseNDJSON разбирает фикстуру: пустые строки и строки с '#' пропускаются, неизвестные поля — ошибка.
// source участвует в сообщениях об ошибках и в детерминированном insert_id.
func ParseNDJSON(r io.Reader, source string) ([]chstore.EventRow, error) {
	var rows []chstore.EventRow
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		row, err := parseEvent([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", source, line, err)
		}
		if row.InsertID == uuid.Nil {
			row.InsertID = uuid.NewSHA1(fixtureNamespace, fmt.Appendf(nil, "%s:%d", source, line))
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: нет событий", source)
	}
	return rows, nil
}

func parseEvent(b []byte) (chstore.EventRow, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var e fixtureEvent
	if err := dec.Decode(&e); err != nil {
		return chstore.EventRow{}, err
	}
	switch {
	case e.AppID == uuid.Nil:
		return chstore.EventRow{}, errors.New("нет app_id")
	case e.EventTime.IsZero():
		return chstore.EventRow{}, errors.New("нет event_time")
	case e.InstallID == uuid.Nil:
		return chstore.EventRow{}, errors.New("нет install_id")
	case e.SessionID == uuid.Nil:
		return chstore.EventRow{}, errors.New("нет session_id")
	case e.Name == "":
		return chstore.EventRow{}, errors.New("нет name")
	}
	at := e.EventTime.UTC()
	orDefault := func(v time.Time) time.Time {
		if v.IsZero() {
			return at
		}
		return v.UTC()
	}
	if e.Environment == "" {
		e.Environment = "production"
	}
	return chstore.EventRow{
		AppID:       e.AppID,
		EventDate:   time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
		EventTime:   at,
		ClientTime:  orDefault(e.ClientTime),
		SentAt:      orDefault(e.SentAt),
		ReceivedAt:  orDefault(e.ReceivedAt),
		InsertID:    e.InsertID,
		Name:        e.Name,
		InstallID:   e.InstallID,
		UserID:      e.UserID,
		SessionID:   e.SessionID,
		DurationMs:  e.DurationMs,
		Environment: e.Environment,
		Platform:    e.Platform,
		OSName:      e.OSName,
		OSVersion:   e.OSVersion,
		AppVersion:  e.AppVersion,
		AppBuild:    e.AppBuild,
		DeviceModel: e.DeviceModel,
		DeviceType:  e.DeviceType,
		Locale:      e.Locale,
		Country:     e.Country,
		Region:      e.Region,
		City:        e.City,
		WriteKeyID:  e.WriteKeyID,
	}, nil
}

// LoadNDJSON читает фикстуру (путь относительно backend/ или абсолютный), вставляет события в events
// тестовой базы ClickHouse с ожиданием ack и возвращает вставленные строки.
func (e *Env) LoadNDJSON(t testing.TB, path string) []chstore.EventRow {
	t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(BackendDir(), path)
	}
	f, err := os.Open(path) //nolint:gosec // путь к фикстуре задаёт тест
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	rows, err := ParseNDJSON(f, filepath.Base(path))
	require.NoError(t, err)
	require.NoError(t, e.CH.InsertEvents(t.Context(), rows))
	return rows
}
