package docscheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Публичная документация не называет хостинг и дата-центр (решение владельца 2026-10-15, как и на лендинге).
var hostingName = regexp.MustCompile(`(?i)hetzner|fsn1|falkenstein`)

func TestDocsDoNotNameHosting(t *testing.T) {
	err := filepath.WalkDir(docsDir(), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".mdx") {
			return err
		}
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotRegexp(t, hostingName, string(src), path)
		return nil
	})
	require.NoError(t, err)
}
