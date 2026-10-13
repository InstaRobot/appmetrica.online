package httpx

import "net/http"

// CORS — публичный CORS ingest: браузерные клиенты любых сайтов шлют события с write-ключом pk_,
// поэтому Origin не проверяется и credentials не разрешаются. Любой OPTIONS — preflight: ответ 204
// без вызова обработчика, кэш браузера 24 ч. Остальным ответам (включая ошибки) ставится
// Allow-Origin, чтобы клиент мог прочитать конверт ошибки, X-Request-Id и Retry-After.
func CORS() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", "*")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, X-App-Key")
				h.Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h.Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After")
			next.ServeHTTP(w, r)
		})
	}
}
