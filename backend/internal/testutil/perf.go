package testutil

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
)

const PerfSeed uint64 = 42

const PerfPowerUser = "perf-power-user"

var PerfPowerInstall = uuid.Nil

type PerfApp struct {
	Code       string
	ID         uuid.UUID
	Platform   string
	Timezone   string
	WriteKeyID uuid.UUID
}

var PerfApps []PerfApp

type PerfConfig struct {
	Seed    uint64
	DAU     int
	Days    int
	LastDay time.Time
}

func DefaultPerfConfig() PerfConfig { return PerfConfig{} }

func (c PerfConfig) FirstDay() time.Time { return time.Time{} }

func (c PerfConfig) Now() time.Time { return time.Time{} }

func (c PerfConfig) Validate() error { return errors.New("not implemented") }

type PerfStats struct {
	Days     int
	Events   int
	Installs int
	Links    int
	Users    int
	AvgDAU   []int
}

func GeneratePerf(PerfConfig, func([]chstore.EventRow) error) (PerfStats, error) {
	return PerfStats{}, errors.New("not implemented")
}

func WritePerf(io.Writer, PerfConfig) (PerfStats, error) {
	return PerfStats{}, errors.New("not implemented")
}

func PerfStatsTable(PerfConfig, PerfStats) string { return "" }

func (e *Env) LoadPerf(t testing.TB, _ PerfConfig) PerfStats {
	t.Helper()
	t.Fatal("not implemented")
	return PerfStats{}
}

type PerfResult struct {
	Name        string  `json:"name"`
	Runs        int     `json:"runs"`
	ThresholdMs float64 `json:"threshold_ms"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	MaxMs       float64 `json:"max_ms"`
	Pass        bool    `json:"pass"`
}

func Percentile([]time.Duration, int) time.Duration { return -1 }

func SummarizeRuns(string, time.Duration, []time.Duration) PerfResult { return PerfResult{} }
