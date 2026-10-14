//go:build integration

package pgstore_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/testutil"
)

// Advisory lock close_day (02-tech-spec.md FR-022): второй процесс lock не получает, после release — получает.
func TestTryAdvisoryLock_Exclusive(t *testing.T) {
	url := testutil.NewPostgresDB(t, false)
	a, err := pgstore.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(a.Close)
	b, err := pgstore.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(b.Close)

	release, ok, err := a.TryAdvisoryLock(t.Context(), "close_day")
	require.NoError(t, err)
	require.True(t, ok)

	_, ok, err = b.TryAdvisoryLock(t.Context(), "close_day")
	require.NoError(t, err)
	require.False(t, ok, "lock держит a")

	other, ok, err := b.TryAdvisoryLock(t.Context(), "digest")
	require.NoError(t, err)
	require.True(t, ok, "другой ключ свободен")
	other()

	release()
	again, ok, err := b.TryAdvisoryLock(t.Context(), "close_day")
	require.NoError(t, err)
	require.True(t, ok, "после release lock свободен")
	again()
}
