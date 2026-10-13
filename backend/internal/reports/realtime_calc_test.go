package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPerMinute(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	require.NoError(t, err)
	now := time.Date(2026, 9, 10, 12, 0, 30, 500, time.UTC)
	first := time.Date(2026, 9, 10, 11, 30, 0, 0, time.UTC)

	got := perMinute(now, kolkata, map[int64]uint64{
		first.Unix():                       2,
		first.Add(29 * time.Minute).Unix(): 5,
		first.Add(-time.Minute).Unix():     7, // до окна
		first.Add(30 * time.Minute).Unix(): 9, // текущая минута
	})
	require.Len(t, got, RealtimeMinutes)
	for i, m := range got {
		require.True(t, first.Add(time.Duration(i)*time.Minute).Equal(m.Minute), "минута %d", i)
		require.Equal(t, kolkata, m.Minute.Location())
		switch i {
		case 0:
			require.Equal(t, uint64(2), m.SessionsStarted)
		case RealtimeMinutes - 1:
			require.Equal(t, uint64(5), m.SessionsStarted)
		default:
			require.Zero(t, m.SessionsStarted, "минута %d", i)
		}
	}
	require.Equal(t, "2026-09-10T17:00:00+05:30", got[0].Minute.Format(time.RFC3339))

	t.Run("minute_boundary", func(t *testing.T) {
		got := perMinute(first.Add(30*time.Minute), nil, nil)
		require.Len(t, got, RealtimeMinutes)
		require.True(t, first.Equal(got[0].Minute))
		require.Equal(t, time.UTC, got[0].Minute.Location(), "nil — UTC")
		require.True(t, first.Add(29*time.Minute).Equal(got[RealtimeMinutes-1].Minute))
	})
}
