package httpx

import "net/http"

// CORS — заглушка до реализации T-009.
func CORS() Middleware {
	return func(next http.Handler) http.Handler { return next }
}
