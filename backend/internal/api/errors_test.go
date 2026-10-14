package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/httpx"
)

// closedAddr — адрес, на котором гарантированно никто не слушает.
func closedAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestStorageUnavailable(t *testing.T) {
	addr := closedAddr(t)
	_, pgErr := pgconn.Connect(t.Context(), "postgres://u:p@"+addr+"/db?connect_timeout=2")
	require.Error(t, pgErr)
	ch, err := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, DialTimeout: 2 * time.Second})
	require.NoError(t, err)
	chErr := ch.Ping(t.Context())
	require.Error(t, chErr)
	require.NoError(t, ch.Close())

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"postgres_connect_refused", fmt.Errorf("apps: list: %w", pgErr), true},
		{"clickhouse_dial_refused", fmt.Errorf("reports: portfolio: %w", chErr), true},
		{"clickhouse_closed", fmt.Errorf("reports: %w", clickhouse.ErrConnectionClosed), true},
		{"clickhouse_acquire_timeout", clickhouse.ErrAcquireConnTimeout, true},
		{"clickhouse_timeout_exceeded", &clickhouse.Exception{Code: 159, Name: "DB::Exception"}, true},
		{"clickhouse_too_many_queries", &clickhouse.Exception{Code: 202, Name: "DB::Exception"}, true},
		{"postgres_admin_shutdown", &pgconn.PgError{Code: "57P01"}, true},
		{"postgres_connection_failure", &pgconn.PgError{Code: "08006"}, true},
		{"postgres_too_many_connections", &pgconn.PgError{Code: "53300"}, true},
		{"deadline", fmt.Errorf("reports: %w", context.DeadlineExceeded), true},
		{"unexpected_eof", io.ErrUnexpectedEOF, true},
		{"net_op_error", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}, true},

		{"nil", nil, false},
		{"plain_bug", errors.New("api: report day: parsing time"), false},
		{"clickhouse_syntax_error", &clickhouse.Exception{Code: 62, Name: "DB::Exception"}, false},
		{"postgres_unique_violation", &pgconn.PgError{Code: "23505"}, false},
		{"no_rows", fmt.Errorf("apps: get: %w", pgx.ErrNoRows), false},
		{"client_gone", context.Canceled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, storageUnavailable(tc.err))
		})
	}
}

func TestResponseError(t *testing.T) {
	s := &Server{logger: slog.New(slog.DiscardHandler)}
	h := httpx.RequestID(clock.NewFixed(time.Date(2026, 10, 13, 5, 0, 0, 0, time.UTC)))

	cases := []struct {
		name       string
		err        error
		status     int
		code       string
		retryAfter string
	}{
		{"storage", fmt.Errorf("reports: %w", clickhouse.ErrConnectionClosed), http.StatusServiceUnavailable, "storage_unavailable", "5"},
		{"bug", errors.New("api: report day: parsing time"), http.StatusInternalServerError, "internal_error", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/portfolio", nil)
			h(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.responseError(w, r, tc.err) })).ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, tc.retryAfter, rec.Header().Get("Retry-After"))
			require.Contains(t, rec.Body.String(), `"code":"`+tc.code+`"`)
			require.NotContains(t, rec.Body.String(), "parsing time", "текст ошибки не уходит клиенту")
			require.Contains(t, rec.Body.String(), rec.Header().Get(httpx.HeaderRequestID))
		})
	}
}
