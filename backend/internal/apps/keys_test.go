package apps

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore/sqlc"
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

func TestPlatformLabels_CoverEveryPlatform(t *testing.T) {
	require.Len(t, platformLabels, len(sqlc.AllAppPlatformValues()))
	for _, p := range sqlc.AllAppPlatformValues() {
		require.NotEmpty(t, platformLabels[p], p)
	}
}

func TestNormalizePlatforms(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []sqlc.AppPlatform
	}{
		{"enum order", []string{"web", "ios", "android"}, []sqlc.AppPlatform{"ios", "android", "web"}},
		{"duplicates", []string{"ios", "ios", "bot", "ios"}, []sqlc.AppPlatform{"ios", "bot"}},
		{
			"all reversed",
			[]string{"other", "server", "bot", "linux", "windows", "macos", "web", "android", "ios"},
			sqlc.AllAppPlatformValues(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePlatforms(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	for _, in := range [][]string{nil, {}, {""}, {"symbian"}, {"ios", "IOS"}} {
		_, err := normalizePlatforms(in)
		var verr *ValidationError
		require.ErrorAs(t, err, &verr, in)
		require.Equal(t, "platforms", verr.Field)
	}
}
