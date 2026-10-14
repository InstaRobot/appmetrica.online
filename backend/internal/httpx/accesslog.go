package httpx

import (
	"log/slog"
	"net/http"
	"strings"

	"appmetrica.online/backend/internal/clock"
)

// AccessLog пишет одну запись slog на запрос. Сознательно не логируются: IP клиента (RemoteAddr,
// X-Forwarded-For), заголовки, query string (там может оказаться ключ) и тело запроса. Вместо пути пишется
// шаблон маршрута ServeMux, если он совпал: в сегментах пути бывают секреты (токен ссылки регистрации passkey).
func AccessLog(logger *slog.Logger, c clock.Clock) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := c.Now()
			sw := &StatusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", loggedPath(r)),
				slog.Int("status", sw.status),
				slog.Int64("bytes", sw.bytes),
				slog.Int64("duration_ms", c.Now().Sub(start).Milliseconds()),
			)
		})
	}
}

// loggedPath — путь шаблона маршрута («/admin/v1/auth/setup/{token}/begin») или сырой путь, если
// ServeMux маршрут не подобрал (404/405: в таких путях параметров маршрутов нет).
func loggedPath(r *http.Request) string {
	if _, path, ok := strings.Cut(r.Pattern, " "); ok {
		return path
	}
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.URL.Path
}
