package testutil

import (
	"context"
	"crypto/sha1" //nolint:gosec // sha1 только для короткого имени тестовой базы
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/pgstore"
)

// Образы совпадают с docker-compose.dev.yml и продом (04-tech-stack.md).
const (
	ClickHouseImage = "clickhouse/clickhouse-server:26.8.25.14"
	PostgresImage   = "postgres:18.6-trixie"
)

// Пользователи ClickHouse из deploy/clickhouse/users.d: appmetrica_admin создаёт образ, rw/ro — users.d с
// грантами на базу appmetrica.
const (
	ClickHouseDatabase      = "appmetrica"
	ClickHouseAdminUser     = "appmetrica_admin"
	ClickHouseAdminPassword = "test_admin_password"
	ClickHouseRWUser        = "appmetrica_rw"
	ClickHouseRWPassword    = "test_rw_password"
	ClickHouseROUser        = "appmetrica_ro"
	ClickHouseROPassword    = "test_ro_password"

	PostgresPassword = "test_pg_password"

	postgresUser     = "appmetrica"
	postgresTemplate = "appmetrica_template"
)

// Контейнеры поднимаются лениво при первом обращении и живут до конца пакета (Run в TestMain).
var (
	chOnce, pgOnce sync.Once
	chShared       *clickhouseShared
	pgShared       *postgresShared
	chErr, pgErr   error
	dbSeq          atomic.Int64
)

type clickhouseShared struct {
	ctr   testcontainers.Container
	addr  string
	admin *chstore.Store // база appmetrica, пользователь appmetrica_admin: CREATE/DROP тестовых баз
}

type postgresShared struct {
	ctr      testcontainers.Container
	adminURL string // служебная база appmetrica
}

// Run запускает тесты пакета и останавливает поднятые контейнеры: `os.Exit(testutil.Run(m))` в TestMain.
func Run(m *testing.M) int {
	code := m.Run()
	ctx := context.Background()
	if chShared != nil {
		_ = chShared.admin.Close()
		_ = chShared.ctr.Terminate(ctx)
	}
	if pgShared != nil {
		_ = pgShared.ctr.Terminate(ctx)
	}
	return code
}

// ClickHouseServerInfo — общий сервер ClickHouse пакета с боевыми users.d/config.d.
type ClickHouseServerInfo struct {
	Addr     string // host:port нативного протокола
	Database string // база appmetrica: на неё выданы гранты appmetrica_rw / appmetrica_ro
}

// Options — параметры подключения к базе appmetrica от имени user.
func (s ClickHouseServerInfo) Options(user, password string) chstore.Options {
	return chstore.Options{Addr: s.Addr, Database: s.Database, User: user, Password: password}
}

// ClickHouseServer возвращает общий сервер ClickHouse; для тестов грантов и миграций базы appmetrica.
func ClickHouseServer(t testing.TB) ClickHouseServerInfo {
	t.Helper()
	ch := startClickHouse(t)
	return ClickHouseServerInfo{Addr: ch.addr, Database: ClickHouseDatabase}
}

// Env — изолированные базы одного теста в общих контейнерах пакета.
type Env struct {
	ClickHouse  chstore.Options // тестовая база t_<hash>, пользователь appmetrica_admin
	PostgresURL string          // тестовая база t_<hash>
	CH          *chstore.Store
	PG          *pgstore.Store

	ch *clickhouseShared
	pg *postgresShared
}

func (e *Env) ClickHouseContainer() testcontainers.Container { return e.ch.ctr }
func (e *Env) PostgresContainer() testcontainers.Container   { return e.pg.ctr }

// Start создаёт для теста свежие базы ClickHouse и Postgres с применёнными миграциями.
// ClickHouse — CREATE DATABASE + goose up; Postgres — копия шаблонной базы с миграциями.
func Start(t testing.TB) *Env {
	t.Helper()
	return start(t, true)
}

// StartEmpty создаёт для теста пустые базы без миграций (тесты `appmetrica migrate`).
func StartEmpty(t testing.TB) *Env {
	t.Helper()
	return start(t, false)
}

func start(t testing.TB, migrated bool) *Env {
	t.Helper()
	ctx := t.Context()
	env := &Env{ch: startClickHouse(t), pg: startPostgres(t)}
	name := databaseName(t.Name())
	env.PostgresURL = createPostgresDB(t, env.pg.adminURL, name, migrated)
	env.ClickHouse = createClickHouseDB(t, env.ch, name, migrated)

	var err error
	env.CH, err = chstore.Open(ctx, env.ClickHouse)
	require.NoError(t, err)
	t.Cleanup(func() { _ = env.CH.Close() })
	env.PG, err = pgstore.Open(ctx, env.PostgresURL)
	require.NoError(t, err)
	t.Cleanup(env.PG.Close)
	return env
}

// NewPostgresDB создаёт для теста только базу Postgres (без ClickHouse) и возвращает её DSN;
// migrated=true — копия шаблонной базы с миграциями.
func NewPostgresDB(t testing.TB, migrated bool) string {
	t.Helper()
	return createPostgresDB(t, startPostgres(t).adminURL, databaseName(t.Name()), migrated)
}

// NewPostgresAt создаёт для теста базу Postgres на версии миграций version и возвращает пул к ней
// (тесты миграций: данные до миграции → MigrateTo → проверка).
func NewPostgresAt(t testing.TB, version int64) *pgxpool.Pool {
	t.Helper()
	store, err := pgstore.Open(t.Context(), NewPostgresDB(t, false))
	require.NoError(t, err)
	t.Cleanup(store.Close)
	require.NoError(t, MigrateTo(t.Context(), store.Pool(), version))
	return store.Pool()
}

// MigrateTo поднимает или откатывает схему базы пула до версии version.
func MigrateTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	p, err := pgstore.NewMigrationProvider(pool)
	if err != nil {
		return err
	}
	current, err := p.GetDBVersion(ctx)
	if err == nil {
		if version < current {
			_, err = p.DownTo(ctx, version)
		} else {
			_, err = p.UpTo(ctx, version)
		}
	}
	return errors.Join(err, p.Close())
}

// databaseName — t_<sha1(имя теста)[:12]>_<n>: читаемо в логах и уникально при повторных вызовах.
func databaseName(test string) string {
	sum := sha1.Sum([]byte(test)) //nolint:gosec // не криптография
	return fmt.Sprintf("t_%s_%d", hex.EncodeToString(sum[:])[:12], dbSeq.Add(1))
}

func createClickHouseDB(t testing.TB, ch *clickhouseShared, name string, migrated bool) chstore.Options {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, ch.admin.Conn().Exec(ctx, "CREATE DATABASE "+name))
	t.Cleanup(func() { _ = ch.admin.Conn().Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" SYNC") })

	opts := chstore.Options{Addr: ch.addr, Database: name, User: ClickHouseAdminUser, Password: ClickHouseAdminPassword}
	if migrated {
		p, err := chstore.NewMigrationProvider(opts)
		require.NoError(t, err)
		_, err = p.Up(ctx)
		require.NoError(t, errors.Join(err, p.Close()))
	}
	return opts
}

func createPostgresDB(t testing.TB, adminURL, name string, migrated bool) string {
	t.Helper()
	ctx := t.Context()
	stmt := "CREATE DATABASE " + name
	if migrated {
		stmt += " TEMPLATE " + postgresTemplate
	}
	require.NoError(t, pgExec(ctx, adminURL, stmt))
	t.Cleanup(func() {
		_ = pgExec(context.Background(), adminURL, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return withDatabase(adminURL, name)
}

func startClickHouse(t testing.TB) *clickhouseShared {
	t.Helper()
	chOnce.Do(func() { chShared, chErr = runClickHouse(context.Background()) })
	require.NoError(t, chErr, "testutil: ClickHouse не поднят (нужен Docker)")
	return chShared
}

func startPostgres(t testing.TB) *postgresShared {
	t.Helper()
	pgOnce.Do(func() { pgShared, pgErr = runPostgres(context.Background()) })
	require.NoError(t, pgErr, "testutil: Postgres не поднят (нужен Docker)")
	return pgShared
}

// runClickHouse поднимает ClickHouse с боевыми users.d/config.d. База appmetrica создаётся пустой:
// тесты грантов мигрируют её сами, остальные тесты работают в своих базах t_*.
func runClickHouse(ctx context.Context) (*clickhouseShared, error) {
	deploy := filepath.Join(BackendDir(), "..", "deploy", "clickhouse")
	ctr, err := tcclickhouse.Run(ctx, ClickHouseImage,
		tcclickhouse.WithDatabase(ClickHouseDatabase),
		tcclickhouse.WithUsername(ClickHouseAdminUser),
		tcclickhouse.WithPassword(ClickHouseAdminPassword),
		testcontainers.WithEnv(map[string]string{
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1",
			"CLICKHOUSE_RW_PASSWORD":               ClickHouseRWPassword,
			"CLICKHOUSE_RO_PASSWORD":               ClickHouseROPassword,
		}),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				HostFilePath:      filepath.Join(deploy, "users.d", "appmetrica.xml"),
				ContainerFilePath: "/etc/clickhouse-server/users.d/appmetrica.xml",
				FileMode:          0o644,
			},
			testcontainers.ContainerFile{
				HostFilePath:      filepath.Join(deploy, "config.d", "appmetrica.xml"),
				ContainerFilePath: "/etc/clickhouse-server/config.d/appmetrica.xml",
				FileMode:          0o644,
			},
		),
	)
	if err != nil {
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}
		return nil, fmt.Errorf("start clickhouse: %w", err)
	}
	addr, err := ctr.ConnectionHost(ctx)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("clickhouse addr: %w", err)
	}
	opts := chstore.Options{Addr: addr, Database: ClickHouseDatabase, User: ClickHouseAdminUser, Password: ClickHouseAdminPassword}
	admin, err := chstore.Open(ctx, opts)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, err
	}
	return &clickhouseShared{ctr: ctr, addr: addr, admin: admin}, nil
}

// runPostgres поднимает Postgres и готовит шаблонную базу appmetrica_template с миграциями.
func runPostgres(ctx context.Context) (*postgresShared, error) {
	ctr, err := postgres.Run(ctx, PostgresImage,
		postgres.WithDatabase(postgresUser),
		postgres.WithUsername(postgresUser),
		postgres.WithPassword(PostgresPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}
		return nil, fmt.Errorf("start postgres: %w", err)
	}
	adminURL, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err == nil {
		err = migrateTemplate(ctx, adminURL)
	}
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, err
	}
	return &postgresShared{ctr: ctr, adminURL: adminURL}, nil
}

func migrateTemplate(ctx context.Context, adminURL string) error {
	if err := pgExec(ctx, adminURL, "CREATE DATABASE "+postgresTemplate); err != nil {
		return err
	}
	// Пул закрывается до копирования: CREATE DATABASE … TEMPLATE требует, чтобы к шаблону не было соединений.
	store, err := pgstore.Open(ctx, withDatabase(adminURL, postgresTemplate))
	if err != nil {
		return err
	}
	defer store.Close()
	p, err := pgstore.NewMigrationProvider(store.Pool())
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	if err = errors.Join(err, p.Close()); err != nil {
		return fmt.Errorf("migrate postgres template: %w", err)
	}
	return nil
}

func pgExec(ctx context.Context, dsn, stmt string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("postgres %q: %w", stmt, err)
	}
	return nil
}

func withDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		panic("testutil: DSN контейнера не разбирается: " + err.Error())
	}
	u.Path = "/" + name
	return u.String()
}
