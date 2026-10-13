package httpx

import (
	"log/slog"
	"net/http"

	"appmetrica.online/backend/internal/clock"
)

// AccessLog пишет одну запись slog на запрос. Сознательно не логируются: IP клиента (RemoteAddr,
// X-Forwarded-For), заголовки, query string (там может оказаться ключ) и тело запроса.
func AccessLog(logger *slog.Logger, c clock.Clock) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := c.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int64("bytes", sw.bytes),
				slog.Int64("duration_ms", c.Now().Sub(start).Milliseconds()),
			)
		})
	}
}
