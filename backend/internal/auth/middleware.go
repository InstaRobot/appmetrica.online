package auth

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// authAttemptsPerMinute — общий лимит beginLogin, finishLogin, beginSetup и finishSetup на процесс api
// (06-api-contracts.md §1.6). По IP не лимитируем: адрес клиента в auth не попадает.
const authAttemptsPerMinute = 20

// ErrorWriter пишет конверт ошибки API. internal/api передаёт адаптер httpx.WriteError: правила зависимостей
// (07-project-structure.md §5) не дают auth импортировать httpx.
type ErrorWriter func(w http.ResponseWriter, r *http.Request, status int, code, message string)

type ownerIDKey struct{}

// OwnerIDFrom — владелец сессии, прошедшей RequireSession.
func OwnerIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ownerIDKey{}).(uuid.UUID)
	return id, ok
}

// RequireSession пропускает только запросы с валидной cookie am_session (401 unauthenticated иначе),
// продлевает простой сессии и кладёт id владельца в контекст (OwnerIDFrom).
func (s *Service) RequireSession(writeError ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var token string
			if c, err := r.Cookie(SessionCookie); err == nil {
				token = c.Value
			}
			ownerID, ok, err := s.authenticate(r.Context(), token)
			switch {
			case err != nil:
				s.logger.ErrorContext(r.Context(), "session check failed", "error", err)
				writeError(w, r, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка. Повторите запрос позже.")
			case !ok:
				writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Сессия истекла, войдите снова")
			default:
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ownerIDKey{}, ownerID)))
			}
		})
	}
}

// LimitAttempts ограничивает попытки входа и регистрации passkey: authAttemptsPerMinute в минуту суммарно
// по всем обёрнутым маршрутам; сверх лимита — 429 rate_limited с Retry-After в целых секундах (минимум 1).
func (s *Service) LimitAttempts(writeError ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := s.clock.Now()
			attempts := s.attempts.Load()
			if attempts.AllowN(now, 1) {
				next.ServeHTTP(w, r)
				return
			}
			wait := (1 - attempts.TokensAt(now)) / float64(attempts.Limit())
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(wait)))))
			writeError(w, r, http.StatusTooManyRequests, "rate_limited", "Слишком много попыток, подождите минуту")
		})
	}
}
