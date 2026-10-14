package auth

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// auth_sessions.user_agent ограничен userAgentMaxLen символами (CHECK в схеме), а Postgres не принимает
// невалидный UTF-8: обрезка идёт по рунам, битые байты заменяются на U+FFFD.
func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short unchanged", "Mozilla/5.0 test", "Mozilla/5.0 test"},
		{"empty", "", ""},
		{"exactly at limit", strings.Repeat("я", userAgentMaxLen), strings.Repeat("я", userAgentMaxLen)},
		{"one rune over", strings.Repeat("я", userAgentMaxLen+1), strings.Repeat("я", userAgentMaxLen)},
		{"long multibyte cut by runes, not bytes", strings.Repeat("я", 600) + "x", strings.Repeat("я", userAgentMaxLen)},
		{"invalid utf-8 replaced", "Mozilla\xff\xfe/5.0", "Mozilla�/5.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateRunes(tt.in, userAgentMaxLen)
			require.Equal(t, tt.want, got)
			require.True(t, utf8.ValidString(got))
			require.LessOrEqual(t, utf8.RuneCountInString(got), userAgentMaxLen)
		})
	}
}
