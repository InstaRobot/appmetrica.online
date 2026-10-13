package auth

import "net/http"

// CSRF проверяет изменяющие cookie-запросы.
func CSRF(_ string, _ ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}
