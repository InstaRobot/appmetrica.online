//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/testutil"
)

const (
	testRPID   = "localhost"
	testOrigin = "http://localhost:3000"
)

var t0 = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

type harness struct {
	svc   *auth.Service
	pg    *pgstore.Store
	clock *clock.Fixed
	logs  *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pg, err := pgstore.Open(t.Context(), testutil.NewPostgresDB(t, true))
	require.NoError(t, err)
	t.Cleanup(pg.Close)

	h := &harness{pg: pg, clock: clock.NewFixed(t0), logs: &bytes.Buffer{}}
	logger := slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.svc, err = auth.NewService(pg, h.clock, logger, testConfig())
	require.NoError(t, err)
	return h
}

func testConfig() config.API {
	return config.API{
		PublicBaseURL:     testOrigin,
		WebAuthnRPID:      testRPID,
		WebAuthnRPName:    "appmetrica.online",
		WebAuthnRPOrigins: []string{testOrigin},
		SessionTTL:        720 * time.Hour,
		SessionIdleTTL:    168 * time.Hour,
	}
}

// newToken выпускает ссылку как `appmetrica admin passkey-link` и возвращает токен из неё.
func (h *harness) newToken(t *testing.T) string {
	t.Helper()
	link, err := h.svc.CreatePasskeyLink(t.Context())
	require.NoError(t, err)
	token, ok := strings.CutPrefix(link, testOrigin+"/setup/")
	require.True(t, ok, link)
	return token
}

// setup проходит begin → регистрация на аутентификаторе → finish.
func (h *harness) setup(t *testing.T, token string, a *testutil.Authenticator, name string) (auth.Session, error) {
	t.Helper()
	c, err := h.svc.BeginSetup(t.Context(), token)
	require.NoError(t, err)
	return h.svc.FinishSetup(t.Context(), auth.FinishSetupInput{
		Token:      token,
		CeremonyID: c.ID,
		Credential: a.Register(t, c.Options),
		Name:       name,
		UserAgent:  "Mozilla/5.0 test",
	})
}

func (h *harness) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
	return n
}

func newAuthenticator(t *testing.T) *testutil.Authenticator {
	return testutil.NewAuthenticator(t, testRPID, testOrigin)
}

// tamper меняет первый символ токена (последний несёт неиспользуемые биты и может декодироваться так же).
func tamper(token string) string {
	if token[0] == 'A' {
		return "B" + token[1:]
	}
	return "A" + token[1:]
}

func sha(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

func TestSetup_FirstRunCreatesOwnerPasskeyAndSession(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	state, err := h.svc.State(ctx, "")
	require.NoError(t, err)
	require.Equal(t, auth.State{Authenticated: false, SetupRequired: true}, state)

	token := h.newToken(t)
	c, err := h.svc.BeginSetup(ctx, token)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, c.ID)
	opts := testutil.ParseCreationOptions(t, c.Options)
	require.Equal(t, testRPID, opts.PublicKey.RP.ID)
	require.Equal(t, "required", opts.PublicKey.AuthenticatorSelection.ResidentKey)
	require.Equal(t, "required", opts.PublicKey.AuthenticatorSelection.UserVerification)
	require.Empty(t, opts.ExcludedIDs())

	a := newAuthenticator(t)
	sess, err := h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: token, CeremonyID: c.ID, Credential: a.Register(t, c.Options), Name: "MacBook", UserAgent: "Mozilla/5.0 test",
	})
	require.NoError(t, err)
	require.Len(t, sess.Token, 43)
	require.Equal(t, t0.Add(720*time.Hour), sess.ExpiresAt)

	require.Equal(t, 1, h.count(t, "owner"))
	var (
		name                 string
		credID               []byte
		aaguid               uuid.UUID
		transports           []string
		backupElig, backupSt bool
		attType              string
	)
	require.NoError(t, h.pg.Pool().QueryRow(ctx,
		"SELECT name, credential_id, aaguid, transports, backup_eligible, backup_state, attestation_type FROM passkeys").
		Scan(&name, &credID, &aaguid, &transports, &backupElig, &backupSt, &attType))
	require.Equal(t, "MacBook", name)
	require.Equal(t, a.CredentialID, credID)
	require.Equal(t, uuid.UUID(testutil.TestAAGUID), aaguid)
	require.ElementsMatch(t, []string{"hybrid", "internal"}, transports)
	require.True(t, backupElig)
	require.True(t, backupSt)
	require.Equal(t, "none", attType)

	var handle []byte
	require.NoError(t, h.pg.Pool().QueryRow(ctx, "SELECT webauthn_user_handle FROM owner").Scan(&handle))
	require.Equal(t, a.UserHandle, handle, "user.id в опциях — handle владельца")

	var usedAt *time.Time
	require.NoError(t, h.pg.Pool().QueryRow(ctx, "SELECT used_at FROM passkey_setup_tokens").Scan(&usedAt))
	require.NotNil(t, usedAt)
	require.Equal(t, 0, h.count(t, "webauthn_ceremonies"), "церемония удалена после успеха")

	raw, err := base64.RawURLEncoding.DecodeString(sess.Token)
	require.NoError(t, err)
	var (
		expiresAt time.Time
		ua        string
	)
	require.NoError(t, h.pg.Pool().QueryRow(ctx,
		"SELECT expires_at, user_agent FROM auth_sessions WHERE token_hash = $1", sha(raw)).Scan(&expiresAt, &ua))
	require.True(t, expiresAt.Equal(t0.Add(720*time.Hour)))
	require.Equal(t, "Mozilla/5.0 test", ua)

	state, err = h.svc.State(ctx, sess.Token)
	require.NoError(t, err)
	require.Equal(t, auth.State{Authenticated: true, SetupRequired: false}, state)

	state, err = h.svc.State(ctx, "")
	require.NoError(t, err)
	require.Equal(t, auth.State{Authenticated: false, SetupRequired: false}, state)
}

func TestBeginSetup_ExpiredOrUsedToken(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	used := h.newToken(t)
	_, err := h.setup(t, used, newAuthenticator(t), "MacBook")
	require.NoError(t, err)

	expired := h.newToken(t)
	h.clock.Advance(15*time.Minute + time.Second)

	before := h.count(t, "webauthn_ceremonies")
	for name, token := range map[string]string{"used": used, "expired": expired} {
		_, err := h.svc.BeginSetup(ctx, token)
		require.ErrorIs(t, err, auth.ErrSetupTokenExpired, name)
	}
	require.Equal(t, before, h.count(t, "webauthn_ceremonies"), "новых церемоний нет")
}

func TestBeginSetup_UnknownOrMalformedToken(t *testing.T) {
	h := newHarness(t)
	h.newToken(t)

	for name, token := range map[string]string{
		"unknown":     base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		"short":       "abc",
		"not base64":  strings.Repeat("!", 43),
		"padded":      base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		"empty":       "",
		"33 bytes":    base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 33)),
		"std base64+": strings.Repeat("+", 43),
	} {
		_, err := h.svc.BeginSetup(t.Context(), token)
		require.ErrorIs(t, err, auth.ErrNotFound, name)
	}
	require.Equal(t, 0, h.count(t, "webauthn_ceremonies"))
}

func TestSetup_RecoveryAddsSecondPasskey(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	mac := newAuthenticator(t)
	_, err := h.setup(t, h.newToken(t), mac, "MacBook")
	require.NoError(t, err)

	token := h.newToken(t)
	c, err := h.svc.BeginSetup(ctx, token)
	require.NoError(t, err)
	require.Equal(t, []string{mac.CredentialIDBase64()}, testutil.ParseCreationOptions(t, c.Options).ExcludedIDs())

	iphone := newAuthenticator(t)
	_, err = h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: token, CeremonyID: c.ID, Credential: iphone.Register(t, c.Options), Name: "iPhone",
	})
	require.NoError(t, err)
	require.Equal(t, mac.UserHandle, iphone.UserHandle, "тот же владелец")

	require.Equal(t, 1, h.count(t, "owner"))
	require.Equal(t, 2, h.count(t, "passkeys"))
	var owners int
	require.NoError(t, h.pg.Pool().QueryRow(ctx, "SELECT count(DISTINCT owner_id) FROM passkeys").Scan(&owners))
	require.Equal(t, 1, owners)
}

func TestFinishSetup_DuplicateCredentialRejected(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	mac := newAuthenticator(t)
	_, err := h.setup(t, h.newToken(t), mac, "MacBook")
	require.NoError(t, err)

	token := h.newToken(t)
	c, err := h.svc.BeginSetup(ctx, token)
	require.NoError(t, err)
	// Аутентификатор проигнорировал excludeCredentials и вернул уже зарегистрированный credential.
	_, err = h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: token, CeremonyID: c.ID, Credential: mac.Register(t, c.Options), Name: "MacBook again",
	})
	require.ErrorIs(t, err, auth.ErrVerificationFailed)
	require.Equal(t, 1, h.count(t, "passkeys"))
}

func TestFinishSetup_CeremonyBoundToToken(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	tokenA, tokenB := h.newToken(t), h.newToken(t)
	c, err := h.svc.BeginSetup(ctx, tokenA)
	require.NoError(t, err)

	_, err = h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: tokenB, CeremonyID: c.ID, Credential: newAuthenticator(t).Register(t, c.Options), Name: "MacBook",
	})
	require.ErrorIs(t, err, auth.ErrVerificationFailed)
	require.Equal(t, 0, h.count(t, "passkeys"))
	require.Equal(t, 0, h.count(t, "auth_sessions"))

	var usedB int
	require.NoError(t, h.pg.Pool().QueryRow(ctx, "SELECT count(*) FROM passkey_setup_tokens WHERE used_at IS NOT NULL").Scan(&usedB))
	require.Equal(t, 0, usedB, "токены не израсходованы")
}

func TestFinishSetup_CeremonyExpired(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	token := h.newToken(t)
	c, err := h.svc.BeginSetup(ctx, token)
	require.NoError(t, err)
	h.clock.Advance(5*time.Minute + time.Second)

	_, err = h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: token, CeremonyID: c.ID, Credential: newAuthenticator(t).Register(t, c.Options), Name: "MacBook",
	})
	require.ErrorIs(t, err, auth.ErrVerificationFailed)
	require.Equal(t, 0, h.count(t, "passkeys"))

	// Токен остаётся годным до своих 15 минут.
	_, err = h.setup(t, token, newAuthenticator(t), "MacBook")
	require.NoError(t, err)
}

func TestFinishSetup_UnknownCeremony(t *testing.T) {
	h := newHarness(t)
	token := h.newToken(t)
	c, err := h.svc.BeginSetup(t.Context(), token)
	require.NoError(t, err)

	_, err = h.svc.FinishSetup(t.Context(), auth.FinishSetupInput{
		Token: token, CeremonyID: uuid.New(), Credential: newAuthenticator(t).Register(t, c.Options), Name: "MacBook",
	})
	require.ErrorIs(t, err, auth.ErrVerificationFailed)
}

func TestFinishSetup_InvalidCredential(t *testing.T) {
	h := newHarness(t)

	cases := map[string]func(a *testutil.Authenticator, options []byte) []byte{
		"not json": func(*testutil.Authenticator, []byte) []byte { return []byte("{") },
		"empty object": func(*testutil.Authenticator, []byte) []byte {
			return []byte(`{}`)
		},
		"wrong origin": func(a *testutil.Authenticator, options []byte) []byte {
			a.Origin = "https://evil.example"
			return a.Register(t, options)
		},
		"wrong rp id": func(a *testutil.Authenticator, options []byte) []byte {
			a.RPID = "evil.example"
			return a.Register(t, bytes.Replace(options, []byte(`"id":"localhost"`), []byte(`"id":""`), 1))
		},
		"wrong challenge": func(a *testutil.Authenticator, options []byte) []byte {
			o := testutil.ParseCreationOptions(t, options)
			return a.Register(t, bytes.Replace(options, []byte(o.PublicKey.Challenge),
				[]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))), 1))
		},
		"backup state without eligibility": func(a *testutil.Authenticator, options []byte) []byte {
			a.BackupEligible, a.BackupState = false, true
			return a.Register(t, options)
		},
	}
	for name, credential := range cases {
		t.Run(name, func(t *testing.T) {
			token := h.newToken(t)
			c, err := h.svc.BeginSetup(t.Context(), token)
			require.NoError(t, err)
			_, err = h.svc.FinishSetup(t.Context(), auth.FinishSetupInput{
				Token: token, CeremonyID: c.ID, Credential: credential(newAuthenticator(t), c.Options), Name: "MacBook",
			})
			require.ErrorIs(t, err, auth.ErrVerificationFailed)
		})
	}
	require.Equal(t, 0, h.count(t, "passkeys"))
}

func TestFinishSetup_NameValidation(t *testing.T) {
	h := newHarness(t)
	token := h.newToken(t)

	for name, value := range map[string]string{
		"empty":   "",
		"spaces":  "   ",
		"65 runes": strings.Repeat("я", 65),
	} {
		c, err := h.svc.BeginSetup(t.Context(), token)
		require.NoError(t, err)
		_, err = h.svc.FinishSetup(t.Context(), auth.FinishSetupInput{
			Token: token, CeremonyID: c.ID, Credential: newAuthenticator(t).Register(t, c.Options), Name: value,
		})
		var verr *auth.ValidationError
		require.ErrorAs(t, err, &verr, name)
		require.Equal(t, "name", verr.Field)
	}
	require.Equal(t, 0, h.count(t, "passkeys"))

	// 64 символа кириллицей — граница допустимого; пробелы по краям отбрасываются.
	_, err := h.setup(t, token, newAuthenticator(t), "  "+strings.Repeat("я", 64)+" ")
	require.NoError(t, err)
	var stored string
	require.NoError(t, h.pg.Pool().QueryRow(t.Context(), "SELECT name FROM passkeys").Scan(&stored))
	require.Equal(t, strings.Repeat("я", 64), stored)
}

func TestFinishSetup_SecondFinishWithSameTokenGone(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)
	token := h.newToken(t)

	// Две вкладки с одной ссылкой: обе успели начать церемонию.
	c1, err := h.svc.BeginSetup(ctx, token)
	require.NoError(t, err)
	c2, err := h.svc.BeginSetup(ctx, token)
	require.NoError(t, err)

	_, err = h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: token, CeremonyID: c1.ID, Credential: newAuthenticator(t).Register(t, c1.Options), Name: "MacBook",
	})
	require.NoError(t, err)
	_, err = h.svc.FinishSetup(ctx, auth.FinishSetupInput{
		Token: token, CeremonyID: c2.ID, Credential: newAuthenticator(t).Register(t, c2.Options), Name: "MacBook",
	})
	require.ErrorIs(t, err, auth.ErrSetupTokenExpired)
	require.Equal(t, 1, h.count(t, "passkeys"))
}

func TestPasskeyLink_StoresOnlyHash(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)

	link, err := h.svc.CreatePasskeyLink(ctx)
	require.NoError(t, err)
	token, ok := strings.CutPrefix(link, testOrigin+"/setup/")
	require.True(t, ok, link)
	require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, token)
	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	require.Len(t, raw, 32)

	rows, err := h.pg.Pool().Query(ctx, "SELECT token_hash, created_at, expires_at, used_at, row_to_json(t)::text FROM passkey_setup_tokens t")
	require.NoError(t, err)
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
		var (
			hash               []byte
			created, expiresAt time.Time
			usedAt             *time.Time
			asJSON             string
		)
		require.NoError(t, rows.Scan(&hash, &created, &expiresAt, &usedAt, &asJSON))
		require.Equal(t, sha(raw), hash)
		require.True(t, created.Equal(t0))
		require.True(t, expiresAt.Equal(t0.Add(15*time.Minute)))
		require.Nil(t, usedAt)
		require.NotContains(t, asJSON, token)
		require.NotContains(t, strings.ToLower(asJSON), strings.ToLower(base64.StdEncoding.EncodeToString(raw)))
	}
	require.NoError(t, rows.Err())
	require.Equal(t, 1, n)

	require.NotEmpty(t, h.logs.String(), "создание ссылки логируется")
	require.NotContains(t, h.logs.String(), token)
	require.NotContains(t, h.logs.String(), link)

	// Предыдущие неиспользованные ссылки не отзываются.
	second := h.newToken(t)
	for _, tok := range []string{token, second} {
		_, err := h.svc.BeginSetup(ctx, tok)
		require.NoError(t, err)
	}
}

func TestFinishSetup_ConcurrentOnlyOneWins(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t)
	token := h.newToken(t)

	const n = 2
	inputs := make([]auth.FinishSetupInput, n)
	for i := range inputs {
		c, err := h.svc.BeginSetup(ctx, token)
		require.NoError(t, err)
		inputs[i] = auth.FinishSetupInput{
			Token: token, CeremonyID: c.ID, Credential: newAuthenticator(t).Register(t, c.Options), Name: "MacBook",
		}
	}

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, n)
	)
	for i := range inputs {
		wg.Go(func() {
			<-start
			_, errs[i] = h.svc.FinishSetup(context.WithoutCancel(ctx), inputs[i])
		})
	}
	close(start)
	wg.Wait()

	var ok, gone int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, auth.ErrSetupTokenExpired):
			gone++
		default:
			t.Fatalf("неожиданная ошибка: %v", err)
		}
	}
	require.Equal(t, 1, ok)
	require.Equal(t, 1, gone)
	require.Equal(t, 1, h.count(t, "passkeys"))
	require.Equal(t, 1, h.count(t, "auth_sessions"))
}

func TestState_InvalidOrUnknownSession(t *testing.T) {
	h := newHarness(t)
	sess, err := h.setup(t, h.newToken(t), newAuthenticator(t), "MacBook")
	require.NoError(t, err)

	for name, cookie := range map[string]string{
		"garbage":  "not-a-token",
		"unknown":  base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)),
		"tampered": tamper(sess.Token),
	} {
		state, err := h.svc.State(t.Context(), cookie)
		require.NoError(t, err, name)
		require.Equal(t, auth.State{Authenticated: false, SetupRequired: false}, state, name)
	}

	h.clock.Advance(721 * time.Hour)
	state, err := h.svc.State(t.Context(), sess.Token)
	require.NoError(t, err)
	require.False(t, state.Authenticated, "абсолютный TTL сессии истёк")
}
