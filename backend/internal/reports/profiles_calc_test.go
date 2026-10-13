package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCeilMillis(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 250_000_000, time.UTC)
	msk := time.FixedZone("MSK", 3*3600)
	for _, tc := range []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"aligned", base, base},
		{"sub_ms", base.Add(500 * time.Microsecond), base.Add(time.Millisecond)},
		{"one_ns", base.Add(time.Nanosecond), base.Add(time.Millisecond)},
		{"just_below_next", base.Add(time.Millisecond - time.Nanosecond), base.Add(time.Millisecond)},
		{"offset_to_utc", base.Add(time.Nanosecond).In(msk), base.Add(time.Millisecond)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ceilMillis(tc.in)
			require.Equal(t, tc.want, got)
			require.Equal(t, time.UTC, got.Location())
		})
	}
}
