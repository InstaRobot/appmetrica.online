package auth

import (
	"bytes"
	"crypto/rand"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// userHandleBytes — длина owner.webauthn_user_handle (максимум WebAuthn, CHECK в схеме).
const userHandleBytes = 64

// newWebAuthn настраивает Relying Party из WEBAUTHN_RP_ID, WEBAUTHN_RP_NAME, WEBAUTHN_RP_ORIGINS.
// Только discoverable credentials с обязательной user verification; attestation не запрашивается.
// Срок церемонии контролирует webauthn_ceremonies.expires_at по internal/clock, а не библиотека.
func newWebAuthn(cfg config.API) (*webauthn.WebAuthn, error) {
	timeout := webauthn.TimeoutConfig{Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                  cfg.WebAuthnRPID,
		RPDisplayName:         cfg.WebAuthnRPName,
		RPOrigins:             cfg.WebAuthnRPOrigins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{Login: timeout, Registration: timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("auth: webauthn config: %w", err)
	}
	return wa, nil
}

func newUserHandle() ([]byte, error) {
	b := make([]byte, userHandleBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("auth: random user handle: %w", err)
	}
	return b, nil
}

// owner — единственный пользователь системы в терминах go-webauthn.
type owner struct {
	id          uuid.UUID
	handle      []byte
	name        string
	credentials []webauthn.Credential
	passkeyIDs  []uuid.UUID // passkeys.id в порядке credentials
}

func newOwner(id uuid.UUID, handle []byte, name string, rows []sqlc.ListPasskeyCredentialsRow) *owner {
	o := &owner{
		id: id, handle: handle, name: name,
		credentials: make([]webauthn.Credential, 0, len(rows)),
		passkeyIDs:  make([]uuid.UUID, 0, len(rows)),
	}
	for _, r := range rows {
		o.passkeyIDs = append(o.passkeyIDs, r.ID)
		transports := make([]protocol.AuthenticatorTransport, 0, len(r.Transports))
		for _, t := range r.Transports {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}
		o.credentials = append(o.credentials, webauthn.Credential{
			ID:              r.CredentialID,
			PublicKey:       r.PublicKey,
			AttestationType: r.AttestationType,
			Transport:       transports,
			Flags: webauthn.CredentialFlags{
				UserPresent:    true,
				UserVerified:   true,
				BackupEligible: r.BackupEligible,
				BackupState:    r.BackupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    r.Aaguid[:],
				SignCount: uint32(r.SignCount), //nolint:gosec // sign_count из authenticator data — uint32
			},
		})
	}
	return o
}

func (o *owner) WebAuthnID() []byte                         { return o.handle }
func (o *owner) WebAuthnName() string                       { return o.name }
func (o *owner) WebAuthnDisplayName() string                { return o.name }
func (o *owner) WebAuthnCredentials() []webauthn.Credential { return o.credentials }

func (o *owner) hasCredential(id []byte) bool {
	return o.passkeyID(id) != uuid.Nil
}

// passkeyID — passkeys.id по credential id; uuid.Nil, если такого passkey у owner нет.
func (o *owner) passkeyID(credentialID []byte) uuid.UUID {
	for i, c := range o.credentials {
		if bytes.Equal(c.ID, credentialID) {
			return o.passkeyIDs[i]
		}
	}
	return uuid.Nil
}
