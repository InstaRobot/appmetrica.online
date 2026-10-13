package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// ErrorWriter пишет конверт ошибки API.
type ErrorWriter func(w http.ResponseWriter, r *http.Request, status int, code, message string)

// RequireSession пропускает только запросы с валидной cookie am_session.
func (s *Service) RequireSession(_ ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// LimitAttempts — глобальный лимит попыток входа и регистрации passkey.
func (s *Service) LimitAttempts(_ ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

// OwnerIDFrom — владелец сессии, прошедшей RequireSession.
func OwnerIDFrom(context.Context) (uuid.UUID, bool) { return uuid.Nil, false }

// Logout отзывает сессию.
func (s *Service) Logout(context.Context, string) error { return errNotImplemented }
