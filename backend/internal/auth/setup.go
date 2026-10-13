// Package auth — аутентификация владельца: passkey (WebAuthn), setup-ссылки из CLI и сессии am_session.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
)

var (
	ErrNotFound           = errors.New("auth: not found")
	ErrSetupTokenExpired  = errors.New("auth: setup token expired or used")
	ErrVerificationFailed = errors.New("auth: webauthn verification failed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "auth: invalid " + e.Field + ": " + e.Message }

type Service struct{}

func NewService(_ *pgstore.Store, _ clock.Clock, _ *slog.Logger, _ config.API) (*Service, error) {
	return &Service{}, nil
}

type State struct {
	Authenticated bool
	SetupRequired bool
}

type Ceremony struct {
	ID      uuid.UUID
	Options json.RawMessage
}

type FinishSetupInput struct {
	Token      string
	CeremonyID uuid.UUID
	Credential []byte
	Name       string
	UserAgent  string
}

type Session struct {
	Token     string
	ExpiresAt time.Time
}

var errNotImplemented = errors.New("auth: not implemented")

func (s *Service) CreatePasskeyLink(context.Context) (string, error) { return "", errNotImplemented }
func (s *Service) State(context.Context, string) (State, error)      { return State{}, errNotImplemented }
func (s *Service) BeginSetup(context.Context, string) (Ceremony, error) {
	return Ceremony{}, errNotImplemented
}

func (s *Service) FinishSetup(context.Context, FinishSetupInput) (Session, error) {
	return Session{}, errNotImplemented
}
