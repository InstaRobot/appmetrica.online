package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// ErrLastPasskey — удаление единственного passkey владельца: без него вход в дашборд невозможен,
// восстановление — только через `appmetrica admin passkey-link`.
var ErrLastPasskey = errors.New("auth: last passkey")

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

// ListPasskeys — все passkey владельца в порядке добавления.
func (s *Service) ListPasskeys(ctx context.Context) ([]Passkey, error) {
	rows, err := s.store.Queries().ListPasskeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: list passkeys: %w", err)
	}
	out := make([]Passkey, 0, len(rows))
	for _, r := range rows {
		out = append(out, passkeyFromRow(r))
	}
	return out, nil
}

// BeginPasskeyRegistration начинает добавление ещё одного passkey из вошедшей сессии.
// Все текущие passkey попадают в excludeCredentials; церемония не привязана к setup-токену.
func (s *Service) BeginPasskeyRegistration(ctx context.Context) (Ceremony, error) {
	q := s.store.Queries()
	row, err := q.GetOwner(ctx)
	if err != nil {
		return Ceremony{}, fmt.Errorf("auth: get owner: %w", err)
	}
	user, err := loadOwner(ctx, q, row.ID, row.WebauthnUserHandle, row.DisplayName)
	if err != nil {
		return Ceremony{}, err
	}
	creation, session, err := s.wa.BeginRegistration(user,
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()))
	if err != nil {
		return Ceremony{}, fmt.Errorf("auth: begin registration: %w", err)
	}
	return saveCeremony(ctx, q, sqlc.CeremonyKindRegistration, creation, session, nil, s.clock.Now())
}

// FinishPasskeyRegistration проверяет ответ аутентификатора и добавляет passkey владельцу.
// Неизвестная, истёкшая или уже использованная церемония — ErrNotFound; церемония setup-ссылки
// и любой отказ проверки WebAuthn — ErrVerificationFailed. При ошибке церемония не расходуется.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, in FinishPasskeyRegistrationInput) (Passkey, error) {
	name, err := validatePasskeyName(in.Name)
	if err != nil {
		return Passkey{}, err
	}
	now := s.clock.Now()
	var pk Passkey
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		row, err := lockOwner(ctx, q)
		if err != nil {
			return err
		}
		cer, err := q.TakeCeremony(ctx, sqlc.TakeCeremonyParams{ID: in.CeremonyID, Kind: sqlc.CeremonyKindRegistration, Now: now})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNotFound
		case err != nil:
			return fmt.Errorf("auth: take ceremony: %w", err)
		}
		if cer.SetupTokenHash != nil {
			return verificationFailed("ceremony belongs to a setup link")
		}
		var session webauthn.SessionData
		if err := json.Unmarshal(cer.SessionData, &session); err != nil {
			return fmt.Errorf("auth: decode ceremony session: %w", err)
		}
		user, err := loadOwner(ctx, q, row.ID, row.WebauthnUserHandle, row.DisplayName)
		if err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialCreationResponseBytes(in.Credential)
		if err != nil {
			return verificationFailed(webauthnReason(err))
		}
		cred, err := s.wa.CreateCredential(user, session, parsed)
		if err != nil {
			return verificationFailed(webauthnReason(err))
		}
		if user.hasCredential(cred.ID) {
			return verificationFailed("credential already registered")
		}
		pk, err = insertPasskey(ctx, q, user.id, cred, name)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrVerificationFailed) {
			s.logger.WarnContext(ctx, "passkey registration rejected", slog.String("reason", err.Error()))
		}
		return Passkey{}, err
	}
	s.logger.InfoContext(ctx, "passkey added", slog.String("passkey_id", pk.ID.String()))
	return pk, nil
}

// RenamePasskey меняет имя passkey (1..64 символа после обрезки пробелов). Нет такого — ErrNotFound.
func (s *Service) RenamePasskey(ctx context.Context, id uuid.UUID, name string) (Passkey, error) {
	name, err := validatePasskeyName(name)
	if err != nil {
		return Passkey{}, err
	}
	row, err := s.store.Queries().RenamePasskey(ctx, sqlc.RenamePasskeyParams{Name: name, ID: id})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Passkey{}, ErrNotFound
	case err != nil:
		return Passkey{}, fmt.Errorf("auth: rename passkey: %w", err)
	}
	return passkeyFromRow(sqlc.ListPasskeysRow(row)), nil
}

// DeletePasskey удаляет passkey. Нет такого — ErrNotFound; последний passkey — ErrLastPasskey.
// Проверка и удаление идут под блокировкой строки owner: два параллельных удаления последних двух
// passkey не оставят владельца без входа.
func (s *Service) DeletePasskey(ctx context.Context, id uuid.UUID) error {
	err := s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if _, err := lockOwner(ctx, q); err != nil {
			return err
		}
		total, err := q.CountPasskeys(ctx)
		if err != nil {
			return fmt.Errorf("auth: count passkeys: %w", err)
		}
		n, err := q.DeletePasskey(ctx, id)
		switch {
		case err != nil:
			return fmt.Errorf("auth: delete passkey: %w", err)
		case n == 0:
			return ErrNotFound
		case total <= 1: // удаление откатывается вместе с транзакцией
			return ErrLastPasskey
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "passkey deleted", slog.String("passkey_id", id.String()))
	return nil
}

// lockOwner блокирует строку owner до конца транзакции. Без owner нет и passkey — ErrNotFound.
func lockOwner(ctx context.Context, q *sqlc.Queries) (sqlc.GetOwnerRow, error) {
	if _, err := q.LockOwner(ctx); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.GetOwnerRow{}, ErrNotFound
		}
		return sqlc.GetOwnerRow{}, fmt.Errorf("auth: lock owner: %w", err)
	}
	row, err := q.GetOwner(ctx)
	if err != nil {
		return sqlc.GetOwnerRow{}, fmt.Errorf("auth: get owner: %w", err)
	}
	return row, nil
}

func passkeyFromRow(r sqlc.ListPasskeysRow) Passkey {
	return Passkey{
		ID:             r.ID,
		Name:           r.Name,
		CreatedAt:      r.CreatedAt,
		LastUsedAt:     r.LastUsedAt,
		BackupEligible: r.BackupEligible,
		BackupState:    r.BackupState,
		Transports:     r.Transports,
	}
}
