//go:build !integration

package docscheck_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// memKeys — Postgres кэша ключей: один активный ключ DEV_WRITE_KEY приложения платформы server.
type memKeys struct{}

func (memKeys) LoadWriteKeyCache(context.Context) ([]sqlc.LoadWriteKeyCacheRow, error) {
	return []sqlc.LoadWriteKeyCacheRow{{
		WriteKeyID:  uuid.MustParse("0199d5f0-0000-7000-8000-000000000001"),
		Key:         devWriteKey,
		AppID:       uuid.MustParse("0199d5f0-0000-7000-8000-000000000002"),
		AppStatus:   sqlc.AppStatusActive,
		AppPlatform: sqlc.AppPlatformServer,
	}}, nil
}

func (memKeys) TouchWriteKeysLastUsed(context.Context, sqlc.TouchWriteKeysLastUsedParams) error {
	return nil
}

// memEvents — таблица events в памяти.
type memEvents struct {
	mu   sync.Mutex
	rows []chstore.EventRow
}

func (m *memEvents) InsertEvents(_ context.Context, rows []chstore.EventRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, rows...)
	return nil
}

func (m *memEvents) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}

// discardRejected — rejected_events, которые тесту не нужны.
type discardRejected struct{}

func (discardRejected) Add(...chstore.RejectedRow) int { return 0 }

// newIngest — ingest в процессе: настоящий конвейер POST /v1/events, ключи и ClickHouse в памяти.
func newIngest(t *testing.T) (http.Handler, func(t *testing.T, accepted int)) {
	t.Helper()
	events := &memEvents{}
	h := ingestHandler(t, memKeys{}, events, discardRejected{})
	return h, func(t *testing.T, accepted int) {
		t.Helper()
		require.Equal(t, accepted, events.count(), "в events записано столько событий, сколько принято")
	}
}
