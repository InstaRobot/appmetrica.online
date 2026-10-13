// Package auth — аутентификация владельца: passkey (WebAuthn), setup-ссылки из CLI и сессии am_session.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/time/rate"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	// SetupTokenTTL — срок жизни ссылки `appmetrica admin passkey-link`.
	SetupTokenTTL = 15 * time.Minute
	// ceremonyTTL — сколько живёт церемония WebAuthn между begin и finish.
	ceremonyTTL = 5 * time.Minute

	passkeyNameMaxLen = 64
)

var (
	// ErrNotFound — setup-токен не существует (или имеет неверный формат).
	ErrNotFound = errors.New("auth: not found")
	// ErrSetupTokenExpired — setup-токен уже использован или истёк.
	ErrSetupTokenExpired = errors.New("auth: setup token expired or used")
	// ErrVerificationFailed — церемония не найдена, истекла, принадлежит другому токену или ответ
	// аутентификатора не прошёл проверку WebAuthn.
	ErrVerificationFailed = errors.New("auth: webauthn verification failed")
)

// ValidationError — некорректное поле запроса.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "auth: invalid " + e.Field + ": " + e.Message }

// Service — регистрация passkey и сессии владельца поверх Postgres.
type Service struct {
	store      *pgstore.Store
	clock      clock.Clock
	logger     *slog.Logger
	wa         *webauthn.WebAuthn
	baseURL    string
	sessionTTL time.Duration
	// sessionIdleTTL — простой сессии (SESSION_IDLE_TTL), пишется в auth_sessions.idle_timeout.
	sessionIdleTTL time.Duration
	// attempts — общий лимит попыток входа и регистрации по setup-ссылке на процесс (LimitAttempts).
	attempts *rate.Limiter
}

func NewService(store *pgstore.Store, c clock.Clock, logger *slog.Logger, cfg config.API) (*Service, error) {
	wa, err := newWebAuthn(cfg)
	if err != nil {
		return nil, err
	}
	return &Service{
		store:          store,
		clock:          c,
		logger:         logger,
		wa:             wa,
		baseURL:        strings.TrimSuffix(cfg.PublicBaseURL, "/"),
		sessionTTL:     cfg.SessionTTL,
		sessionIdleTTL: cfg.SessionIdleTTL,
		attempts:       rate.NewLimiter(rate.Every(time.Minute/authAttemptsPerMinute), authAttemptsPerMinute),
	}, nil
}

// Ceremony — начатая церемония WebAuthn: id для finish и опции для браузера
// ({"publicKey": …} — передаются в @simplewebauthn/browser как optionsJSON).
type Ceremony struct {
	ID      uuid.UUID
	Options json.RawMessage
}

// FinishSetupInput — тело finishSetup и контекст запроса.
type FinishSetupInput struct {
	Token      string
	CeremonyID uuid.UUID
	Credential []byte // RegistrationResponseJSON как пришёл от браузера
	Name       string
	UserAgent  string
}

// CreatePasskeyLink выпускает одноразовую ссылку регистрации passkey на SetupTokenTTL.
// В базе — только sha256 токена; ссылка возвращается вызывающему и не логируется.
// Предыдущие неиспользованные ссылки остаются действительными.
func (s *Service) CreatePasskeyLink(ctx context.Context) (string, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	expires := now.Add(SetupTokenTTL)
	if err := s.store.Queries().CreateSetupToken(ctx, sqlc.CreateSetupTokenParams{
		TokenHash: hash, Now: now, ExpiresAt: expires,
	}); err != nil {
		return "", fmt.Errorf("auth: create setup token: %w", err)
	}
	s.logger.InfoContext(ctx, "passkey setup link created", slog.Time("expires_at", expires))
	return s.baseURL + "/setup/" + token, nil
}

// BeginSetup начинает регистрацию passkey по setup-токену. При первом запуске создаёт owner;
// уже зарегистрированные passkey попадают в excludeCredentials.
func (s *Service) BeginSetup(ctx context.Context, token string) (Ceremony, error) {
	hash, ok := hashToken(token)
	if !ok {
		return Ceremony{}, ErrNotFound
	}
	handle, err := newUserHandle()
	if err != nil {
		return Ceremony{}, err
	}
	now := s.clock.Now()
	var c Ceremony
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if err := checkSetupToken(ctx, q, hash, now); err != nil {
			return err
		}
		row, err := q.GetOrCreateOwner(ctx, handle)
		if err != nil {
			return fmt.Errorf("auth: get or create owner: %w", err)
		}
		user, err := loadOwner(ctx, q, row.ID, row.WebauthnUserHandle, row.DisplayName)
		if err != nil {
			return err
		}
		creation, session, err := s.wa.BeginRegistration(user,
			webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()))
		if err != nil {
			return fmt.Errorf("auth: begin registration: %w", err)
		}
		c, err = saveCeremony(ctx, q, sqlc.CeremonyKindRegistration, creation, session, hash, now)
		return err
	})
	if err != nil {
		return Ceremony{}, err
	}
	return c, nil
}

// FinishSetup проверяет ответ аутентификатора, сохраняет passkey, расходует setup-токен и создаёт сессию —
// в одной транзакции. Строка токена блокируется (FOR UPDATE): из параллельных finish с одним токеном
// успешен ровно один, остальные получают ErrSetupTokenExpired.
func (s *Service) FinishSetup(ctx context.Context, in FinishSetupInput) (Session, error) {
	name, err := validatePasskeyName(in.Name)
	if err != nil {
		return Session{}, err
	}
	hash, ok := hashToken(in.Token)
	if !ok {
		return Session{}, ErrNotFound
	}
	now := s.clock.Now()
	var (
		sess      Session
		passkeyID uuid.UUID
		recovery  bool
	)
	err = s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if err := checkSetupToken(ctx, q, hash, now); err != nil {
			return err
		}
		cer, err := q.TakeCeremony(ctx, sqlc.TakeCeremonyParams{ID: in.CeremonyID, Kind: sqlc.CeremonyKindRegistration, Now: now})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return verificationFailed("ceremony not found or expired")
		case err != nil:
			return fmt.Errorf("auth: take ceremony: %w", err)
		}
		if !bytes.Equal(cer.SetupTokenHash, hash) {
			return verificationFailed("ceremony belongs to another setup token")
		}
		var session webauthn.SessionData
		if err := json.Unmarshal(cer.SessionData, &session); err != nil {
			return fmt.Errorf("auth: decode ceremony session: %w", err)
		}

		row, err := q.GetOwner(ctx)
		if err != nil {
			return fmt.Errorf("auth: get owner: %w", err)
		}
		user, err := loadOwner(ctx, q, row.ID, row.WebauthnUserHandle, row.DisplayName)
		if err != nil {
			return err
		}
		recovery = len(user.credentials) > 0

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

		pk, err := insertPasskey(ctx, q, user.id, cred, name)
		if err != nil {
			return err
		}
		passkeyID = pk.ID
		if _, err := q.MarkSetupTokenUsed(ctx, sqlc.MarkSetupTokenUsedParams{Now: &now, TokenHash: hash}); err != nil {
			return fmt.Errorf("auth: mark setup token used: %w", err)
		}
		sess, err = s.createSession(ctx, q, user.id, in.UserAgent, now)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrVerificationFailed) {
			s.logger.WarnContext(ctx, "passkey setup rejected", slog.String("reason", err.Error()))
		}
		return Session{}, err
	}
	s.logger.InfoContext(ctx, "passkey registered", slog.String("passkey_id", passkeyID.String()), slog.Bool("recovery", recovery))
	return sess, nil
}

// checkSetupToken блокирует строку токена до конца транзакции и проверяет, что он годен.
func checkSetupToken(ctx context.Context, q *sqlc.Queries, hash []byte, now time.Time) error {
	t, err := q.LockSetupToken(ctx, hash)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("auth: lock setup token: %w", err)
	case t.UsedAt != nil || !t.ExpiresAt.After(now):
		return ErrSetupTokenExpired
	}
	return nil
}

func loadOwner(ctx context.Context, q *sqlc.Queries, id uuid.UUID, handle []byte, name string) (*owner, error) {
	rows, err := q.ListPasskeyCredentials(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("auth: list passkey credentials: %w", err)
	}
	return newOwner(id, handle, name, rows), nil
}

// saveCeremony сохраняет SessionData go-webauthn в webauthn_ceremonies и возвращает опции для браузера
// (CredentialCreation или CredentialAssertion). setupTokenHash — только у регистрации по setup-ссылке.
func saveCeremony(ctx context.Context, q *sqlc.Queries, kind sqlc.CeremonyKind, browserOptions any,
	session *webauthn.SessionData, setupTokenHash []byte, now time.Time,
) (Ceremony, error) {
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return Ceremony{}, fmt.Errorf("auth: encode ceremony session: %w", err)
	}
	options, err := json.Marshal(browserOptions)
	if err != nil {
		return Ceremony{}, fmt.Errorf("auth: encode ceremony options: %w", err)
	}
	id, err := q.CreateCeremony(ctx, sqlc.CreateCeremonyParams{
		Kind:           kind,
		SessionData:    sessionJSON,
		SetupTokenHash: setupTokenHash,
		Now:            now,
		ExpiresAt:      now.Add(ceremonyTTL),
	})
	if err != nil {
		return Ceremony{}, fmt.Errorf("auth: create ceremony: %w", err)
	}
	return Ceremony{ID: id, Options: options}, nil
}

func insertPasskey(ctx context.Context, q *sqlc.Queries, ownerID uuid.UUID, cred *webauthn.Credential, name string) (Passkey, error) {
	aaguid, err := uuid.FromBytes(cred.Authenticator.AAGUID)
	if err != nil {
		aaguid = uuid.Nil
	}
	attestation := cred.AttestationType
	if attestation == "" {
		attestation = "none"
	}
	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	row, err := q.CreatePasskey(ctx, sqlc.CreatePasskeyParams{
		OwnerID:         ownerID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: attestation,
		Aaguid:          aaguid,
		SignCount:       int64(cred.Authenticator.SignCount),
		Transports:      transports,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
		Name:            name,
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.ConstraintName == "passkeys_credential_id_uq":
		return Passkey{}, verificationFailed("credential already registered")
	case err != nil:
		return Passkey{}, fmt.Errorf("auth: create passkey: %w", err)
	}
	return passkeyFromRow(sqlc.ListPasskeysRow(row)), nil
}

// validatePasskeyName: 1..64 символа после обрезки пробелов по краям.
func validatePasskeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > passkeyNameMaxLen || !utf8.ValidString(name) {
		return "", &ValidationError{Field: "name", Message: "Название passkey — от 1 до 64 символов"}
	}
	return name, nil
}

func verificationFailed(reason string) error {
	return fmt.Errorf("%w: %s", ErrVerificationFailed, reason)
}

// webauthnReason — причина отказа go-webauthn для лога (тип и подробности protocol.Error).
func webauthnReason(err error) string {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return strings.TrimSpace(perr.Type + ": " + perr.Details + " " + perr.DevInfo)
	}
	return err.Error()
}
