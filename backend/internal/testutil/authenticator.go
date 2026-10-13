package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"slices"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/stretchr/testify/require"
)

// TestAAGUID — AAGUID программного аутентификатора.
var TestAAGUID = [16]byte{0x0a, 0x9e, 0x7c, 0x1d, 0x41, 0x6f, 0x4b, 0x2e, 0x9d, 0x55, 0x30, 0x6c, 0x7e, 0x11, 0x42, 0x08}

// Флаги authenticator data (WebAuthn §6.1).
const (
	flagUP byte = 0x01
	flagUV byte = 0x04
	flagBE byte = 0x08
	flagBS byte = 0x10
	flagAT byte = 0x40
)

// Authenticator — программный WebAuthn-аутентификатор для тестов: ключ ES256, attestation "none",
// discoverable credential с user verification. Отвечает на опции go-webauthn так же, как браузер
// с @simplewebauthn/browser: возвращает RegistrationResponseJSON.
type Authenticator struct {
	Origin         string
	RPID           string
	CredentialID   []byte
	Transports     []string
	BackupEligible bool
	BackupState    bool
	// UserHandle — user.id из опций последней регистрации; возвращается в assertion при входе.
	UserHandle []byte
	// SignCount — счётчик подписей в authenticator data. 0 — как у synced passkey (iCloud, Google).
	SignCount uint32

	key *ecdsa.PrivateKey
}

// NewAuthenticator создаёт аутентификатор со случайными ключом и credential id (synced passkey: BE и BS выставлены).
func NewAuthenticator(t testing.TB, rpID, origin string) *Authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id := make([]byte, 32)
	_, err = rand.Read(id)
	require.NoError(t, err)
	return &Authenticator{
		Origin:         origin,
		RPID:           rpID,
		CredentialID:   id,
		Transports:     []string{"hybrid", "internal"},
		BackupEligible: true,
		BackupState:    true,
		key:            key,
	}
}

// CreationOptions — нужная тестам часть опций регистрации {"publicKey": PublicKeyCredentialCreationOptionsJSON}.
type CreationOptions struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"rp"`
		User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
		AuthenticatorSelection struct {
			ResidentKey      string `json:"residentKey"`
			UserVerification string `json:"userVerification"`
		} `json:"authenticatorSelection"`
		ExcludeCredentials []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"excludeCredentials"`
	} `json:"publicKey"`
}

// ParseCreationOptions разбирает опции регистрации из ответа begin.
func ParseCreationOptions(t testing.TB, options []byte) CreationOptions {
	t.Helper()
	var o CreationOptions
	require.NoError(t, json.Unmarshal(options, &o))
	return o
}

// ExcludedIDs — base64url credential id из excludeCredentials.
func (o CreationOptions) ExcludedIDs() []string {
	ids := make([]string, 0, len(o.PublicKey.ExcludeCredentials))
	for _, c := range o.PublicKey.ExcludeCredentials {
		ids = append(ids, c.ID)
	}
	return ids
}

// CredentialIDBase64 — credential id в base64url без padding, как в JSON WebAuthn.
func (a *Authenticator) CredentialIDBase64() string {
	return base64.RawURLEncoding.EncodeToString(a.CredentialID)
}

// Register выполняет navigator.credentials.create по опциям begin и возвращает RegistrationResponseJSON.
func (a *Authenticator) Register(t testing.TB, options []byte) []byte {
	t.Helper()
	o := ParseCreationOptions(t, options)
	rpID := o.PublicKey.RP.ID
	if rpID == "" {
		rpID = a.RPID
	}
	var err error
	a.UserHandle, err = base64.RawURLEncoding.DecodeString(o.PublicKey.User.ID)
	require.NoError(t, err, "user.id")

	clientData, err := json.Marshal(map[string]any{
		"type":        "webauthn.create",
		"challenge":   o.PublicKey.Challenge,
		"origin":      a.Origin,
		"crossOrigin": false,
	})
	require.NoError(t, err)

	attObj, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": a.authData(t, rpID),
	})
	require.NoError(t, err)

	b64 := base64.RawURLEncoding.EncodeToString
	resp, err := json.Marshal(map[string]any{
		"id":    a.CredentialIDBase64(),
		"rawId": a.CredentialIDBase64(),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData),
			"attestationObject": b64(attObj),
			"transports":        a.Transports,
		},
		"clientExtensionResults":  map[string]any{},
		"authenticatorAttachment": "platform",
	})
	require.NoError(t, err)
	return resp
}

// RequestOptions — нужная тестам часть опций входа {"publicKey": PublicKeyCredentialRequestOptionsJSON, "mediation": …}.
type RequestOptions struct {
	PublicKey struct {
		Challenge        string `json:"challenge"`
		RPID             string `json:"rpId"`
		Timeout          int    `json:"timeout"`
		UserVerification string `json:"userVerification"`
		AllowCredentials []struct {
			ID string `json:"id"`
		} `json:"allowCredentials"`
	} `json:"publicKey"`
	Mediation string `json:"mediation"`
}

// ParseRequestOptions разбирает опции входа из ответа beginLogin.
func ParseRequestOptions(t testing.TB, options []byte) RequestOptions {
	t.Helper()
	var o RequestOptions
	require.NoError(t, json.Unmarshal(options, &o))
	return o
}

// Login выполняет navigator.credentials.get по опциям beginLogin и возвращает AuthenticationResponseJSON
// с userHandle (discoverable credential) и текущим SignCount.
func (a *Authenticator) Login(t testing.TB, options []byte) []byte {
	t.Helper()
	o := ParseRequestOptions(t, options)
	rpID := o.PublicKey.RPID
	if rpID == "" {
		rpID = a.RPID
	}
	clientData, err := json.Marshal(map[string]any{
		"type":        "webauthn.get",
		"challenge":   o.PublicKey.Challenge,
		"origin":      a.Origin,
		"crossOrigin": false,
	})
	require.NoError(t, err)

	rpHash := sha256.Sum256([]byte(rpID))
	authData := slices.Concat(rpHash[:], []byte{a.flags()})
	authData = binary.BigEndian.AppendUint32(authData, a.SignCount)
	clientHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(slices.Concat(authData, clientHash[:]))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	require.NoError(t, err)

	b64 := base64.RawURLEncoding.EncodeToString
	resp, err := json.Marshal(map[string]any{
		"id":    a.CredentialIDBase64(),
		"rawId": a.CredentialIDBase64(),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData),
			"authenticatorData": b64(authData),
			"signature":         b64(sig),
			"userHandle":        b64(a.UserHandle),
		},
		"clientExtensionResults":  map[string]any{},
		"authenticatorAttachment": "platform",
	})
	require.NoError(t, err)
	return resp
}

func (a *Authenticator) flags() byte {
	flags := flagUP | flagUV
	if a.BackupEligible {
		flags |= flagBE
	}
	if a.BackupState {
		flags |= flagBS
	}
	return flags
}

// authData: rpIdHash | flags | signCount | AAGUID | len(credentialId) | credentialId | COSE-ключ.
func (a *Authenticator) authData(t testing.TB, rpID string) []byte {
	t.Helper()
	rpHash := sha256.Sum256([]byte(rpID))
	pub, err := a.key.PublicKey.Bytes() // 0x04 | X | Y
	require.NoError(t, err)
	coseKey, err := webauthncbor.Marshal(map[int]any{
		1:  2,  // kty: EC2
		3:  -7, // alg: ES256
		-1: 1,  // crv: P-256
		-2: pub[1:33],
		-3: pub[33:65],
	})
	require.NoError(t, err)

	out := make([]byte, 0, 37+16+2+len(a.CredentialID)+len(coseKey))
	out = append(out, rpHash[:]...)
	out = append(out, a.flags()|flagAT)
	out = binary.BigEndian.AppendUint32(out, a.SignCount)
	out = append(out, TestAAGUID[:]...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(a.CredentialID))) //nolint:gosec // 32 байта
	out = append(out, a.CredentialID...)
	return append(out, coseKey...)
}
