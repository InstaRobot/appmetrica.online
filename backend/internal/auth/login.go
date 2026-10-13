package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var errNotImplemented = errors.New("auth: not implemented")

// FinishLoginInput — тело finishLogin и контекст запроса.
type FinishLoginInput struct {
	CeremonyID uuid.UUID
	Credential []byte // AuthenticationResponseJSON как пришёл от браузера
	UserAgent  string
}

// BeginLogin начинает вход по discoverable credential.
func (s *Service) BeginLogin(context.Context) (Ceremony, error) {
	return Ceremony{}, errNotImplemented
}

// FinishLogin проверяет assertion и создаёт сессию.
func (s *Service) FinishLogin(context.Context, FinishLoginInput) (Session, error) {
	return Session{}, errNotImplemented
}
