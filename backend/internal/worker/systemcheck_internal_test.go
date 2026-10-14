package worker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseSample(t *testing.T) {
	tests := []struct {
		line  string
		name  string
		value float64
		ok    bool
	}{
		{"ingest_clickhouse_insert_errors_total 3", "ingest_clickhouse_insert_errors_total", 3, true},
		{"ingest_clickhouse_insert_duration_seconds_count 1.5e+06", "ingest_clickhouse_insert_duration_seconds_count", 1.5e6, true},
		{`ingest_requests_total{code="ok",status="200"} 42 1760000000000`, "ingest_requests_total", 42, true},
		{`metric{label="with } brace"} 7`, "metric", 7, true},
		{"# TYPE ingest_clickhouse_insert_errors_total counter", "", 0, false},
		{"", "", 0, false},
		{"metric_without_value", "", 0, false},
		{"metric NaNx", "", 0, false},
		{`metric{label="unterminated" 1`, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			name, value, ok := parseSample(tt.line)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.name, name)
			require.InDelta(t, tt.value, value, 0)
		})
	}
}

func TestFormatPct(t *testing.T) {
	for v, want := range map[float64]string{15: "15 %", 14.24: "14,2 %", 19.96: "20 %", 0: "0 %", 3.05: "3,1 %"} {
		require.Equal(t, want, formatPct(v), "%v", v)
	}
}

func TestFormatDuration(t *testing.T) {
	require.Equal(t, "10 мин", formatDuration(10*time.Minute+59*time.Second))
	require.Equal(t, "1 ч", formatDuration(time.Hour))
	require.Equal(t, "6 ч", formatDuration(6*time.Hour+59*time.Minute))
}
