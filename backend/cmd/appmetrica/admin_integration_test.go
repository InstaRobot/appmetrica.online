//go:build integration

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/testutil"
)

func TestAdminPasskeyLink_PrintsLinkAndStoresHash(t *testing.T) {
	ctx := t.Context()
	pgURL := testutil.NewPostgresDB(t, true)
	t.Setenv("POSTGRES_URL", pgURL)
	t.Setenv("PUBLIC_BASE_URL", "https://appmetrica.online")
	t.Setenv("KEY_HASH_PEPPER", "ZGV2LXBlcHBlci1kby1ub3QtdXNlLWluLXByb2R1Y3Rpb24=")
	t.Setenv("APP_ENV", "production")

	code, stdout, stderr := runCLI(t, "admin", "passkey-link")
	require.Equal(t, 0, code, stderr)
	m := regexp.MustCompile(`^https://appmetrica\.online/setup/([A-Za-z0-9_-]{43})\n$`).FindStringSubmatch(stdout)
	require.NotNil(t, m, stdout)
	token := m[1]
	require.NotContains(t, stderr, token, "токен не попадает в лог")
	require.True(t, strings.Contains(stderr, "15m") || strings.Contains(stderr, "expires_at"), stderr)

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	want := sha256.Sum256(raw)

	pg, err := pgstore.Open(ctx, pgURL)
	require.NoError(t, err)
	defer pg.Close()
	var hash []byte
	require.NoError(t, pg.Pool().QueryRow(ctx, "SELECT token_hash FROM passkey_setup_tokens").Scan(&hash))
	require.Equal(t, want[:], hash)
}

func TestAdminPasskeyLink_ConfigErrors(t *testing.T) {
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("KEY_HASH_PEPPER", "ZGV2LXBlcHBlci1kby1ub3QtdXNlLWluLXByb2R1Y3Rpb24=")
	code, _, stderr := runCLI(t, "admin", "passkey-link")
	require.Equal(t, 1, code)
	require.Contains(t, stderr, "POSTGRES_URL is required")

	code, _, stderr = runCLI(t, "admin", "passkey-link", "extra")
	require.Equal(t, 2, code)
	require.Contains(t, stderr, "лишние аргументы")
}
