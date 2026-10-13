package httpx

import "net/http"

// BodyLimit ограничивает тело запроса n байтами. Поставленный после Gunzip, ограничивает распакованное тело.
// Превышение обработчик видит как *http.MaxBytesError при чтении и отвечает 413 payload_too_large.
func BodyLimit(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
