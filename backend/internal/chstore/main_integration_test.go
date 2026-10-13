//go:build integration

package chstore_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/testutil"
)

const (
	adminUser     = testutil.ClickHouseAdminUser
	adminPassword = testutil.ClickHouseAdminPassword
	rwUser        = testutil.ClickHouseRWUser
	rwPassword    = testutil.ClickHouseRWPassword
	roUser        = testutil.ClickHouseROUser
	roPassword    = testutil.ClickHouseROPassword
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

var (
	migrateOnce sync.Once
	migrateErr  error
)

// options — подключение к базе appmetrica общего сервера testutil (боевые users.d/config.d, гранты rw/ro).
// При первом вызове база мигрируется пользователем appmetrica_admin, как это делает `appmetrica migrate up`.
func options(t *testing.T, user, password string) chstore.Options {
	t.Helper()
	srv := testutil.ClickHouseServer(t)
	migrateOnce.Do(func() {
		p, err := chstore.NewMigrationProvider(srv.Options(adminUser, adminPassword))
		if err == nil {
			_, err = p.Up(context.Background())
			err = errors.Join(err, p.Close())
		}
		migrateErr = err
	})
	require.NoError(t, migrateErr, "clickhouse migrate up")
	return srv.Options(user, password)
}

// openStore подключается к ClickHouse пользователем user и закрывает соединение по окончании теста.
func openStore(t *testing.T, user, password string) *chstore.Store {
	t.Helper()
	store, err := chstore.Open(t.Context(), options(t, user, password))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

// adminConn — соединение appmetrica_admin для служебных запросов (OPTIMIZE, system.*).
func adminConn(t *testing.T) driver.Conn {
	t.Helper()
	return openStore(t, adminUser, adminPassword).Conn()
}

// base — фиксированный момент в пределах TTL; миллисекунды проверяют точность DateTime64(3).
var base = time.Date(2026, 10, 12, 9, 30, 15, 123_000_000, time.UTC)

// eventRow — валидная строка session_start; каждый тест берёт свой app_id, чтобы не мешать соседям.
func eventRow(appID, installID uuid.UUID, at time.Time) chstore.EventRow {
	at = at.UTC()
	return chstore.EventRow{
		AppID:       appID,
		EventDate:   time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
		EventTime:   at,
		ClientTime:  at.Add(-3 * time.Hour),
		SentAt:      at.Add(time.Second),
		ReceivedAt:  at.Add(2 * time.Second),
		InsertID:    uuid.New(),
		Name:        "session_start",
		InstallID:   installID,
		SessionID:   uuid.New(),
		Environment: "production",
		Platform:    "ios",
		OSName:      "iOS",
		OSVersion:   "26.1",
		AppVersion:  "1.0.0",
		AppBuild:    "100",
		DeviceModel: "iPhone17,1",
		DeviceType:  "phone",
		Locale:      "ru-RU",
		Country:     "RU",
		Region:      "Moscow",
		City:        "Moscow",
		WriteKeyID:  uuid.New(),
	}
}

func countEvents(t *testing.T, conn driver.Conn, appID uuid.UUID) uint64 {
	t.Helper()
	var n uint64
	require.NoError(t, conn.QueryRow(t.Context(),
		"SELECT count() FROM appmetrica.events WHERE app_id = ?", appID).Scan(&n))
	return n
}
