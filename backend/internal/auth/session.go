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

// userAgentMaxLen — auth_sessions.user_agent ограничен 512 символами (CHECK в схеме).
const userAgentMaxLen = 512

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
func (s *Service) State(ctx context.Context, sessionToken string) (State, error) {
	n, err := s.store.Queries().CountPasskeys(ctx)
	if err != nil {
		return State{}, fmt.Errorf("auth: count passkeys: %w", err)
	}
	authenticated, err := s.validSession(ctx, sessionToken)
	if err != nil {
		return State{}, err
	}
	return State{Authenticated: authenticated, SetupRequired: n == 0}, nil
}

// validSession: сессия не отозвана, не истекла по абсолютному сроку и простою.
func (s *Service) validSession(ctx context.Context, token string) (bool, error) {
	hash, ok := hashToken(token)
	if !ok {
		return false, nil
	}
	_, err := s.store.Queries().GetActiveSession(ctx, sqlc.GetActiveSessionParams{TokenHash: hash, Now: s.clock.Now()})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("auth: get session: %w", err)
	}
	return true, nil
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
		UserAgent: truncateRunes(userAgent, userAgentMaxLen),
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
