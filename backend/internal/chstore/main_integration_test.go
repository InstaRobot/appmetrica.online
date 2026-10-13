//go:build integration

package chstore_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"appmetrica.online/backend/internal/chstore"
)

const (
	clickhouseImage = "clickhouse/clickhouse-server:26.8.25.14"
	database        = "appmetrica"
	adminUser       = "appmetrica_admin"
	adminPassword   = "test_admin_password"
	rwUser          = "appmetrica_rw"
	rwPassword      = "test_rw_password"
	roUser          = "appmetrica_ro"
	roPassword      = "test_ro_password"
)

// chAddr — host:port нативного протокола контейнера; миграции применены в TestMain.
var chAddr string

func TestMain(m *testing.M) {
	os.Exit(runWithClickHouse(m))
}

// runWithClickHouse поднимает ClickHouse с боевыми users.d/config.d из deploy/ и применяет миграции
// пользователем appmetrica_admin, как это делает `appmetrica migrate up`.
func runWithClickHouse(m *testing.M) int {
	ctx := context.Background()
	ctr, err := tcclickhouse.Run(ctx, clickhouseImage,
		tcclickhouse.WithDatabase(database),
		tcclickhouse.WithUsername(adminUser),
		tcclickhouse.WithPassword(adminPassword),
		testcontainers.WithEnv(map[string]string{
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1",
			"CLICKHOUSE_RW_PASSWORD":               rwPassword,
			"CLICKHOUSE_RO_PASSWORD":               roPassword,
		}),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				HostFilePath:      "../../../deploy/clickhouse/users.d/appmetrica.xml",
				ContainerFilePath: "/etc/clickhouse-server/users.d/appmetrica.xml",
				FileMode:          0o644,
			},
			testcontainers.ContainerFile{
				HostFilePath:      "../../../deploy/clickhouse/config.d/appmetrica.xml",
				ContainerFilePath: "/etc/clickhouse-server/config.d/appmetrica.xml",
				FileMode:          0o644,
			},
		),
	)
	if ctr != nil {
		defer func() { _ = ctr.Terminate(ctx) }()
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "start clickhouse: %v\n", err)
		return 1
	}
	chAddr, err = ctr.ConnectionHost(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "clickhouse addr: %v\n", err)
		return 1
	}

	p, err := chstore.NewMigrationProvider(options(adminUser, adminPassword))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "clickhouse goose provider: %v\n", err)
		return 1
	}
	_, err = p.Up(ctx)
	_ = p.Close()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "clickhouse migrate up: %v\n", err)
		return 1
	}
	return m.Run()
}

func options(user, password string) chstore.Options {
	return chstore.Options{Addr: chAddr, Database: database, User: user, Password: password}
}

// openStore подключается к ClickHouse пользователем user и закрывает соединение по окончании теста.
func openStore(t *testing.T, user, password string) *chstore.Store {
	t.Helper()
	store, err := chstore.Open(t.Context(), options(user, password))
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
