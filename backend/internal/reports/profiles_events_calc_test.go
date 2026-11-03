package reports

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Курсор событий профиля непрозрачен, но обратим с точностью до миллисекунды; чужая строка — ErrInvalidCursor.
func TestProfileEventsCursor_RoundTrip(t *testing.T) {
	at := time.Date(2026, 10, 5, 10, 0, 1, 234_000_000, time.UTC)
	id := uuid.MustParse("1de20000-0000-4000-8000-000000000042")

	gotAt, gotID, err := decodeProfileEventsCursor(encodeProfileEventsCursor(at, id))
	require.NoError(t, err)
	require.True(t, at.Equal(gotAt))
	require.Equal(t, id, gotID)

	for _, bad := range []string{"", "garbage", "!!!", encodeProfileEventsCursor(at, id) + "x"} {
		_, _, err := decodeProfileEventsCursor(bad)
		require.ErrorIs(t, err, ErrInvalidCursor, "%q", bad)
	}
}
