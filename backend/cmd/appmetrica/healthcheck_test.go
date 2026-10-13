package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHealthcheck_ExitCodes(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()

	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()

	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()

	tests := []struct {
		name string
		url  string
		want int
	}{
		{"200", ok.URL + "/health", 0},
		{"503", unavailable.URL + "/health", 1},
		{"unreachable", downURL + "/health", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got := runHealthcheck(t.Context(), []string{"--url", tt.url}, &stderr)
			require.Equal(t, tt.want, got, stderr.String())
		})
	}

	require.Equal(t, 2*time.Second, healthcheckTimeout)
}

func TestHealthcheck_EdgeCases(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer redirect.Close()

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"timeout", []string{"--url", slow.URL, "--timeout", "50ms"}, 1},
		{"redirect is not healthy", []string{"--url", redirect.URL}, 1},
		{"missing url", []string{}, 2},
		{"relative url", []string{"--url", "/health"}, 2},
		{"unsupported scheme", []string{"--url", "ftp://localhost/health"}, 2},
		{"unknown flag", []string{"--url", slow.URL, "--verbose"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			start := time.Now()
			got := runHealthcheck(t.Context(), tt.args, &stderr)
			require.Equal(t, tt.want, got, stderr.String())
			require.Less(t, time.Since(start), 3*time.Second)
		})
	}
}
