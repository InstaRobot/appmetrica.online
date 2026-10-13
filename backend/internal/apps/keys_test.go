package apps

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWriteKey_FormatAndUniqueness(t *testing.T) {
	re := regexp.MustCompile(`^pk_[A-Za-z0-9]{32}$`)
	seen := make(map[string]bool, 2000)
	var chars strings.Builder
	for range 2000 {
		key, err := newWriteKey()
		require.NoError(t, err)
		require.Regexp(t, re, key)
		require.False(t, seen[key], key)
		seen[key] = true
		chars.WriteString(key[3:])
	}
	// 64 000 символов покрывают весь алфавит base62: смещения выборки нет.
	for _, c := range base62 {
		require.Contains(t, chars.String(), string(c))
	}
}
