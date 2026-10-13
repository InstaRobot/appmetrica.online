// Package chstore — доступ к ClickHouse: соединение clickhouse-go, вставка событий с ожиданием ack
// и goose-миграции.
package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const (
	dialTimeout = 5 * time.Second
	// maxConns — открытых и простаивающих соединений на процесс. Вставка с ожиданием ack держит соединение
	// до сброса буфера async insert (до 1 с): 500 rps ingest (NFR-001) — до ~500 одновременных вставок.
	// Равные лимиты: пул драйвера по умолчанию (10/5) закрывал соединения после каждой вставки.
	maxConns = 512
)

// Options — параметры подключения (CLICKHOUSE_ADDR, CLICKHOUSE_DATABASE и пользователь процесса).
type Options struct {
	Addr     string
	Database string
	User     string
	Password string
}

func (o Options) clickhouse() *clickhouse.Options {
	return &clickhouse.Options{
		Addr: []string{o.Addr},
		Auth: clickhouse.Auth{
			Database: o.Database,
			Username: o.User,
			Password: o.Password,
		},
		DialTimeout:  dialTimeout,
		MaxOpenConns: maxConns,
		MaxIdleConns: maxConns,
		Compression:  &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	}
}

// Store владеет пулом нативных соединений ClickHouse.
type Store struct {
	conn driver.Conn
}

// Open подключается по нативному протоколу и проверяет соединение.
// Тексты ошибок не содержат пароль: clickhouse-go его не цитирует, а Options не форматируется.
func Open(ctx context.Context, opts Options) (*Store, error) {
	conn, err := clickhouse.Open(opts.clickhouse())
	if err != nil {
		return nil, fmt.Errorf("chstore: open: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("chstore: ping %s as %s: %w", opts.Addr, opts.User, err)
	}
	return &Store{conn: conn}, nil
}

func (s *Store) Close() error { return s.conn.Close() }

// Conn — соединение для запросов отчётов и служебных операций.
func (s *Store) Conn() driver.Conn { return s.conn }

func (s *Store) Ping(ctx context.Context) error {
	if err := s.conn.Ping(ctx); err != nil {
		return fmt.Errorf("chstore: ping: %w", err)
	}
	return nil
}
