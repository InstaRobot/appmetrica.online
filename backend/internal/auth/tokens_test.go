package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewToken_FormatAndHash(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		token, hash, err := newToken()
		require.NoError(t, err)
		require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, token)
		require.False(t, seen[token], "токены уникальны")
		seen[token] = true

		raw, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err)
		require.Len(t, raw, tokenBytes)
		sum := sha256.Sum256(raw)
		require.Equal(t, sum[:], hash)

		parsed, ok := hashToken(token)
		require.True(t, ok)
		require.Equal(t, hash, parsed)
	}
}

func TestHashToken_RejectsMalformed(t *testing.T) {
	for name, token := range map[string]string{
		"empty":        "",
		"short":        "abc",
		"31 bytes":     base64.RawURLEncoding.EncodeToString(make([]byte, 31)),
		"33 bytes":     base64.RawURLEncoding.EncodeToString(make([]byte, 33)),
		"padded":       base64.URLEncoding.EncodeToString(make([]byte, 32)),
		"std alphabet": strings.Repeat("+", 43),
		"bang":         strings.Repeat("!", 43),
		"whitespace":   " " + base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
	} {
		_, ok := hashToken(token)
		require.False(t, ok, name)
	}
}
