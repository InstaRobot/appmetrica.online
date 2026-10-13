package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/url"
	"time"
)

// healthcheckTimeout — общий таймаут проверки: Docker HEALTHCHECK не должен зависать.
const healthcheckTimeout = 2 * time.Second

// runHealthcheck — `appmetrica healthcheck --url <url>`: 0, если GET вернул 200, иначе 1.
// Нужна для HEALTHCHECK в distroless-образе, где нет curl и wget.
func runHealthcheck(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	rawURL := fs.String("url", "", "адрес проверки, например http://localhost:8080/health")
	timeout := fs.Duration("timeout", healthcheckTimeout, "таймаут запроса")
	if !parseFlags(fs, args, stderr) {
		return 2
	}
	u, err := url.Parse(*rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		say(stderr, "healthcheck: нужен абсолютный http(s) --url\n")
		return 2
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		say(stderr, "healthcheck: %v\n", err)
		return 2
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			say(stderr, "healthcheck: нет ответа за %s\n", *timeout)
		} else {
			say(stderr, "healthcheck: %v\n", err)
		}
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		say(stderr, "healthcheck: статус %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
