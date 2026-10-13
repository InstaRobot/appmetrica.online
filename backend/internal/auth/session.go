package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// SessionCookie — имя cookie сессии дашборда; значение — токен сессии (base64url, 43 символа).
const SessionCookie = "am_session"

const (
	// userAgentMaxLen — auth_sessions.user_agent ограничен 512 символами (CHECK в схеме).
	userAgentMaxLen = 512
	// sessionTouchInterval — last_seen_at обновляется не чаще, чем раз в 5 минут.
	sessionTouchInterval = 5 * time.Minute
)

// Session — новая сессия владельца: токен для cookie am_session и абсолютный срок жизни.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// State — ответ getAuthState.
type State struct {
	Authenticated bool
	SetupRequired bool // нет ни одного passkey: вход невозможен, нужна ссылка `appmetrica admin passkey-link`
}

// State сообщает, валидна ли cookie сессии и нужна ли первичная настройка passkey.
// Как и любой запрос с cookie, продлевает простой сессии.
func (s *Service) State(ctx context.Context, sessionToken string) (State, error) {
	n, err := s.store.Queries().CountPasskeys(ctx)
	if err != nil {
		return State{}, fmt.Errorf("auth: count passkeys: %w", err)
	}
	_, authenticated, err := s.authenticate(ctx, sessionToken)
	if err != nil {
		return State{}, err
	}
	return State{Authenticated: authenticated, SetupRequired: n == 0}, nil
}

// authenticate проверяет токен cookie: сессия не отозвана, не истекла по абсолютному сроку и простою.
// last_seen_at сдвигается не чаще раза в sessionTouchInterval (условие в TouchSession).
func (s *Service) authenticate(ctx context.Context, token string) (uuid.UUID, bool, error) {
	hash, ok := hashToken(token)
	if !ok {
		return uuid.Nil, false, nil
	}
	now := s.clock.Now()
	row, err := s.store.Queries().GetActiveSession(ctx, sqlc.GetActiveSessionParams{TokenHash: hash, Now: now})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return uuid.Nil, false, nil
	case err != nil:
		return uuid.Nil, false, fmt.Errorf("auth: get session: %w", err)
	}
	if now.Sub(row.LastSeenAt) > sessionTouchInterval {
		if err := s.store.Queries().TouchSession(ctx, sqlc.TouchSessionParams{Now: now, TokenHash: hash}); err != nil {
			return uuid.Nil, false, fmt.Errorf("auth: touch session: %w", err)
		}
	}
	return row.OwnerID, true, nil
}

// Logout отзывает сессию cookie. Неизвестный, некорректный или уже отозванный токен — не ошибка.
func (s *Service) Logout(ctx context.Context, token string) error {
	hash, ok := hashToken(token)
	if !ok {
		return nil
	}
	now := s.clock.Now()
	if err := s.store.Queries().RevokeSession(ctx, sqlc.RevokeSessionParams{Now: &now, TokenHash: hash}); err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

// createSession создаёт сессию владельца в транзакции q; в базе — только sha256 токена.
func (s *Service) createSession(ctx context.Context, q *sqlc.Queries, ownerID uuid.UUID, userAgent string, now time.Time) (Session, error) {
	token, hash, err := newToken()
	if err != nil {
		return Session{}, err
	}
	expires := now.Add(s.sessionTTL)
	if err := q.CreateSession(ctx, sqlc.CreateSessionParams{
		TokenHash: hash,
		OwnerID:   ownerID,
		Now:       now,
		ExpiresAt: expires,
		// Простой хранится в строке сессии: смена SESSION_IDLE_TTL не меняет уже выданные сессии.
		IdleTimeoutSeconds: s.sessionIdleTTL.Seconds(),
		UserAgent:          truncateRunes(userAgent, userAgentMaxLen),
	}); err != nil {
		return Session{}, fmt.Errorf("auth: create session: %w", err)
	}
	return Session{Token: token, ExpiresAt: expires}, nil
}

// truncateRunes обрезает строку до n символов; невалидный UTF-8 (Postgres его не примет) заменяется на U+FFFD.
func truncateRunes(s string, n int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
