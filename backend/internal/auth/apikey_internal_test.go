package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRandomBase62(t *testing.T) {
	seen := make(map[string]bool)
	var all strings.Builder
	for range 200 {
		s, err := randomBase62(apiKeyChars)
		require.NoError(t, err)
		require.Len(t, s, apiKeyChars)
		require.True(t, isAPIKeySecret(apiKeyPrefix+s), s)
		require.False(t, seen[s])
		seen[s] = true
		all.WriteString(s)
	}
	for _, c := range base62 {
		require.Contains(t, all.String(), string(c), "8000 символов покрывают весь алфавит")
	}
}

func TestIsAPIKeySecret(t *testing.T) {
	valid := "sk_" + strings.Repeat("aZ9", 13) + "x"
	require.True(t, isAPIKeySecret(valid))
	for _, v := range []string{
		"",
		"sk_",
		valid[:len(valid)-1],
		valid + "a",
		"pk_" + valid[3:],
		"SK_" + valid[3:],
		valid[:42] + "-",
		valid[:42] + "ж",
		" " + valid[1:],
	} {
		require.False(t, isAPIKeySecret(v), v)
	}
}

func TestValidScopes(t *testing.T) {
	for _, tc := range []struct {
		scopes []string
		ok     bool
	}{
		{[]string{"read"}, true},
		{[]string{"delete"}, true},
		{[]string{"delete", "read"}, true},
		{nil, false},
		{[]string{}, false},
		{[]string{"read", "read"}, false},
		{[]string{"write"}, false},
		{[]string{"read", "delete", "read"}, false},
		{[]string{"READ"}, false},
	} {
		require.Equal(t, tc.ok, validScopes(tc.scopes), tc.scopes)
	}
}

func TestAPIKeyLimits_BurstAndRefill(t *testing.T) {
	var l apiKeyLimits
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	key, other := uuid.New(), uuid.New()
	for i := range apiKeyBurst {
		_, ok := l.allow(key, now)
		require.True(t, ok, "request %d", i+1)
	}
	wait, ok := l.allow(key, now)
	require.False(t, ok)
	require.Equal(t, 100*time.Millisecond, wait)

	_, ok = l.allow(other, now)
	require.True(t, ok, "у другого ключа свой бакет")

	_, ok = l.allow(key, now.Add(100*time.Millisecond))
	require.True(t, ok)
	_, ok = l.allow(key, now.Add(100*time.Millisecond))
	require.False(t, ok)
}
