package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrLastPasskey — удаление единственного passkey владельца: без него вход в дашборд невозможен.
var ErrLastPasskey = errors.New("auth: last passkey")

var errNotImplemented = errors.New("auth: not implemented")

// Passkey — passkey владельца в списке настроек. credential_id и public_key наружу не отдаются.
type Passkey struct {
	ID             uuid.UUID
	Name           string
	CreatedAt      time.Time
	LastUsedAt     *time.Time
	BackupEligible bool
	BackupState    bool
	Transports     []string
}

// FinishPasskeyRegistrationInput — тело finishPasskeyRegistration.
type FinishPasskeyRegistrationInput struct {
	CeremonyID uuid.UUID
	Credential []byte // RegistrationResponseJSON как пришёл от браузера
	Name       string
}

func (s *Service) ListPasskeys(context.Context) ([]Passkey, error) { return nil, errNotImplemented }

func (s *Service) BeginPasskeyRegistration(context.Context) (Ceremony, error) {
	return Ceremony{}, errNotImplemented
}

func (s *Service) FinishPasskeyRegistration(context.Context, FinishPasskeyRegistrationInput) (Passkey, error) {
	return Passkey{}, errNotImplemented
}

func (s *Service) RenamePasskey(context.Context, uuid.UUID, string) (Passkey, error) {
	return Passkey{}, errNotImplemented
}

func (s *Service) DeletePasskey(context.Context, uuid.UUID) error { return errNotImplemented }
