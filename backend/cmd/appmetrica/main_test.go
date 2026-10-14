package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

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

// Порт метрик занят: процесс завершается с кодом 1, а не «штатно» с 0, и основной сервер останавливается.
func TestServeProcess_MetricsListenFailureExitsNonZero(t *testing.T) {
	var lc net.ListenConfig
	busy, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = busy.Close() }()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stopped := make(chan struct{})
	background := func(ctx context.Context) { <-ctx.Done(); close(stopped) }

	done := make(chan int, 1)
	go func() {
		done <- serveProcess(t.Context(), logger, "127.0.0.1:0", http.NotFoundHandler(), busy.Addr().String(), http.NotFoundHandler(), background)
	}()
	select {
	case code := <-done:
		require.Equal(t, 1, code)
	case <-time.After(10 * time.Second):
		t.Fatal("serveProcess не завершился после ошибки сервера метрик")
	}
	<-stopped
}

// Отмена контекста (SIGTERM) — штатная остановка с кодом 0.
func TestServeProcess_StopsOnContextCancel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan int, 1)
	go func() {
		done <- serveProcess(ctx, logger, "127.0.0.1:0", http.NotFoundHandler(), "127.0.0.1:0", http.NotFoundHandler())
	}()
	cancel()
	select {
	case code := <-done:
		require.Equal(t, 0, code)
	case <-time.After(10 * time.Second):
		t.Fatal("serveProcess не завершился после отмены контекста")
	}
}
