package httpx

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover перехватывает панику обработчика, пишет её в лог и отвечает 500 internal_error,
// если ответ ещё не начат. http.ErrAbortHandler пробрасывается дальше: это штатный способ оборвать ответ.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &StatusWriter{ResponseWriter: w}
			defer recoverPanic(logger, sw, r)
			next.ServeHTTP(sw, r)
		})
	}
}

// recoverPanic вызывается через defer напрямую, поэтому recover() в нём перехватывает панику обработчика.
func recoverPanic(logger *slog.Logger, sw *StatusWriter, r *http.Request) {
	v := recover()
	if v == nil {
		return
	}
	if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		panic(v)
	}
	logger.ErrorContext(r.Context(), "panic recovered",
		slog.String("request_id", RequestIDFrom(r.Context())),
		slog.String("panic", fmt.Sprint(v)),
		slog.String("stack", string(debug.Stack())),
	)
	if !sw.wroteHeader {
		WriteError(sw, r, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка. Повторите запрос позже.", nil)
	}
}
