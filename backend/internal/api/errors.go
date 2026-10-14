package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5/pgconn"

	"appmetrica.online/backend/internal/httpx"
)

// storageRetryAfter — Retry-After ответа 503 storage_unavailable в секундах, как у ingest.
const storageRetryAfter = 5

var (
	// chUnavailableCodes — исключения ClickHouse о перегрузке, а не об ошибке запроса:
	// TIMEOUT_EXCEEDED, TOO_MANY_SIMULTANEOUS_QUERIES, SOCKET_TIMEOUT, NETWORK_ERROR.
	chUnavailableCodes = []int32{159, 202, 209, 210}
	// pgUnavailableCodes — SQLSTATE остановки или перегрузки Postgres (кроме класса 08 — connection exception):
	// too_many_connections, admin_shutdown, crash_shutdown, cannot_connect_now.
	pgUnavailableCodes = []string{"53300", "57P01", "57P02", "57P03"}
)

// storageUnavailable — ошибка означает недоступность ClickHouse или Postgres (соединение, таймаут, перегрузка),
// а не ошибку в запросе или коде: клиенту стоит повторить запрос позже.
func storageUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var (
		netErr  net.Error
		connErr *pgconn.ConnectError
		pgErr   *pgconn.PgError
		chErr   *clickhouse.Exception
	)
	switch {
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, clickhouse.ErrConnectionClosed),
		errors.Is(err, clickhouse.ErrAcquireConnTimeout),
		errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.As(err, &netErr),
		errors.As(err, &connErr),
		pgconn.Timeout(err):
		return true
	case errors.As(err, &pgErr):
		return strings.HasPrefix(pgErr.Code, "08") || slices.Contains(pgUnavailableCodes, pgErr.Code)
	case errors.As(err, &chErr):
		return slices.Contains(chUnavailableCodes, chErr.Code)
	default:
		return false
	}
}

// responseError — обработчик вернул ошибку вместо ответа из контракта: недоступное хранилище — 503
// storage_unavailable с Retry-After, остальное — 500 internal_error. Текст ошибки клиенту не отдаётся.
func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	if !storageUnavailable(err) {
		s.internalError(w, r, err)
		return
	}
	s.logger.WarnContext(r.Context(), "storage unavailable",
		slog.String("request_id", httpx.RequestIDFrom(r.Context())), slog.Any("error", err))
	w.Header().Set("Retry-After", strconv.Itoa(storageRetryAfter))
	httpx.WriteError(w, r, http.StatusServiceUnavailable, "storage_unavailable",
		"Хранилище временно недоступно. Повторите запрос позже.", nil)
}

// internalError — 500 internal_error: ошибка в коде или запросе, а не в доступности хранилища.
func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.ErrorContext(r.Context(), "request failed",
		slog.String("request_id", httpx.RequestIDFrom(r.Context())), slog.Any("error", err))
	httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка. Повторите запрос позже.", nil)
}
