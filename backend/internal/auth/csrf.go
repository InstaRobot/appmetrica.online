package auth

import (
	"net/http"
	"strings"
)

// RequestedWith — значение X-Requested-With, которое дашборд ставит на каждый запрос к API.
const RequestedWith = "appmetrica-web"

// CSRF защищает cookie-запросы, меняющие состояние: POST/PUT/PATCH/DELETE на /admin/v1/* и на /v1/*
// без Bearer-ключа обязаны нести X-Requested-With: appmetrica-web, а Origin, если он есть, — совпадать
// с PUBLIC_BASE_URL. Иначе 403 csrf_check_failed. Bearer-запросы Read API cookie не используют и не проверяются.
func CSRF(publicBaseURL string, writeError ErrorWriter) func(http.Handler) http.Handler {
	origin := strings.TrimSuffix(publicBaseURL, "/")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if needsCSRFCheck(r) && !csrfOK(r, origin) {
				writeError(w, r, http.StatusForbidden, "csrf_check_failed", "Запрос отклонён проверкой CSRF. Обновите страницу")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func needsCSRFCheck(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/admin/v1/"):
		return true
	case strings.HasPrefix(r.URL.Path, "/v1/"):
		return !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return false
}

func csrfOK(r *http.Request, origin string) bool {
	if r.Header.Get("X-Requested-With") != RequestedWith {
		return false
	}
	o := r.Header.Values("Origin")
	return len(o) == 0 || (len(o) == 1 && o[0] == origin)
}
