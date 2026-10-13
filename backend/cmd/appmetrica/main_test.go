package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun_Help(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		var stdout, stderr bytes.Buffer
		code := run(t.Context(), args, &stdout, &stderr)
		require.Equal(t, 0, code, args)
		for _, sub := range []string{"ingest", "api", "worker", "migrate", "admin", "healthcheck"} {
			require.Contains(t, stdout.String(), sub)
		}
	}
}

func TestRun_UsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no subcommand", nil},
		{"unknown subcommand", []string{"serve"}},
		{"migrate without action", []string{"migrate"}},
		{"migrate unknown action", []string{"migrate", "down"}},
		{"admin without action", []string{"admin"}},
		{"admin unknown action", []string{"admin", "reset-everything"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(t.Context(), tt.args, &stdout, &stderr)
			require.Equal(t, 2, code)
			require.NotEmpty(t, stderr.String())
		})
	}
}

func TestRun_ConfigErrorExitsWithOne(t *testing.T) {
	t.Setenv("POSTGRES_URL", "")
	for _, sub := range []string{"ingest", "api", "worker"} {
		var stdout, stderr bytes.Buffer
		code := run(t.Context(), []string{sub}, &stdout, &stderr)
		require.Equal(t, 1, code, sub)
		require.Contains(t, stderr.String(), "POSTGRES_URL is required", sub)
	}
}

func TestRun_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(t.Context(), []string{"--version"}, &stdout, &stderr))
	require.Equal(t, version+"\n", stdout.String())
}
