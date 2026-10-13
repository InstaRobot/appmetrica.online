package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// FinishLoginInput — тело finishLogin и контекст запроса.
type FinishLoginInput struct {
	CeremonyID uuid.UUID
	Credential []byte // AuthenticationResponseJSON как пришёл от браузера
	UserAgent  string
}

// BeginLogin начинает вход по discoverable credential: allowCredentials пуст, браузер сам предлагает passkey
// (в том числе через автозаполнение — mediation "conditional").
func (s *Service) BeginLogin(ctx context.Context) (Ceremony, error) {
	assertion, session, err := s.wa.BeginDiscoverableMediatedLogin(protocol.MediationConditional)
	if err != nil {
		return Ceremony{}, fmt.Errorf("auth: begin login: %w", err)
	}
	return saveCeremony(ctx, s.store.Queries(), sqlc.CeremonyKindLogin, assertion, session, nil, s.clock.Now())
}

// FinishLogin проверяет assertion (ValidatePasskeyLogin на сырых байтах), обновляет счётчик passkey и создаёт сессию.
// Церемония одноразовая и расходуется до проверки: повтор или истёкшая церемония — ErrNotFound.
// Счётчик подписей, не выросший относительно сохранённого (CloneWarning), — ErrVerificationFailed.
func (s *Service) FinishLogin(ctx context.Context, in FinishLoginInput) (Session, error) {
	now := s.clock.Now()
	cer, err := s.store.Queries().TakeCeremony(ctx, sqlc.TakeCeremonyParams{ID: in.CeremonyID, Kind: sqlc.CeremonyKindLogin, Now: now})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Session{}, ErrNotFound
	case err != nil:
		return Session{}, fmt.Errorf("auth: take ceremony: %w", err)
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(cer.SessionData, &session); err != nil {
		return Session{}, fmt.Errorf("auth: decode ceremony session: %w", err)
	}

	user, cred, err := s.validateLogin(ctx, session, in.Credential)
	if err != nil {
		if errors.Is(err, ErrVerificationFailed) {
			s.logger.WarnContext(ctx, "passkey login rejected", slog.String("reason", err.Error()))
		}
		return Session{}, err
	}
	passkeyID := user.passkeyID(cred.ID)
	if cred.Authenticator.CloneWarning {
		s.logCloneWarning(ctx, passkeyID)
		return Session{}, verificationFailed("sign count did not increase")
	}

	var sess Session
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		n, err := q.UpdatePasskeyAfterLogin(ctx, sqlc.UpdatePasskeyAfterLoginParams{
			SignCount:   int64(cred.Authenticator.SignCount),
			BackupState: cred.Flags.BackupState,
			Now:         &now,
			ID:          passkeyID,
		})
		switch {
		case err != nil:
			return fmt.Errorf("auth: update passkey: %w", err)
		case n == 0: // параллельный вход успел записать счётчик не меньше нашего
			s.logCloneWarning(ctx, passkeyID)
			return verificationFailed("sign count did not increase")
		}
		sess, err = s.createSession(ctx, q, user.id, in.UserAgent, now)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	s.logger.InfoContext(ctx, "passkey login", slog.String("passkey_id", passkeyID.String()))
	return sess, nil
}

// validateLogin разбирает ответ браузера и проверяет его против owner и его passkey.
// Ошибка чтения Postgres из обработчика go-webauthn возвращается как есть, а не как отказ проверки.
func (s *Service) validateLogin(ctx context.Context, session webauthn.SessionData, credential []byte) (*owner, *webauthn.Credential, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return nil, nil, verificationFailed(webauthnReason(err))
	}
	var (
		user     *owner
		storeErr error
	)
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		row, err := s.store.Queries().GetOwner(ctx)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, errors.New("owner not found")
		case err != nil:
			storeErr = fmt.Errorf("auth: get owner: %w", err)
			return nil, storeErr
		case !bytes.Equal(row.WebauthnUserHandle, userHandle):
			return nil, errors.New("user handle does not match owner")
		}
		user, storeErr = loadOwner(ctx, s.store.Queries(), row.ID, row.WebauthnUserHandle, row.DisplayName)
		if storeErr != nil {
			return nil, storeErr
		}
		return user, nil
	}
	_, cred, err := s.wa.ValidatePasskeyLogin(handler, session, parsed)
	switch {
	case storeErr != nil:
		return nil, nil, storeErr
	case err != nil:
		return nil, nil, verificationFailed(webauthnReason(err))
	}
	return user, cred, nil
}

func (s *Service) logCloneWarning(ctx context.Context, passkeyID uuid.UUID) {
	s.logger.WarnContext(ctx, "passkey sign count did not increase",
		slog.String("event", "passkey_clone_warning"), slog.String("passkey_id", passkeyID.String()))
}
