// Package telegram — клиент Telegram Bot API (sendMessage) для дайджеста, алертов и тестового сообщения.
// Токен бота входит в URL запроса, поэтому ни URL, ни токен не попадают в тексты ошибок.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// DefaultBaseURL — адрес Bot API; в тестах подменяется httptest-сервером (WithBaseURL).
	DefaultBaseURL = "https://api.telegram.org"

	// maxAttempts — попыток на сообщение (06-api-contracts.md: до 3 с учётом retry_after).
	maxAttempts = 3
	// maxRetryAfter — дольше не ждём: тестовое сообщение дашборда и цикл worker-а не должны висеть.
	maxRetryAfter = 60
	// maxResponseBytes — ответ sendMessage — небольшой JSON; больше не читаем.
	maxResponseBytes = 1 << 20
	httpTimeout      = 10 * time.Second
)

// ErrNoToken — TELEGRAM_BOT_TOKEN не задан: сообщения не отправляются.
var ErrNoToken = errors.New("telegram: TELEGRAM_BOT_TOKEN is not set")

// APIError — ответ Bot API с ok=false: error_code и description из тела.
type APIError struct {
	Code        int
	Description string
	// RetryAfter — parameters.retry_after ответа 429, секунды; 0 — не задан.
	RetryAfter int
}

// Error — description Telegram как есть («Bad Request: chat not found»): его показывает дашборд.
func (e *APIError) Error() string {
	if e.Description == "" {
		return "telegram: error " + strconv.Itoa(e.Code)
	}
	return e.Description
}

// retryable — повтор имеет смысл: 429 и ошибки на стороне Telegram.
func (e *APIError) retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= http.StatusInternalServerError
}

// Client отправляет сообщения от имени бота.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	sleep   func(ctx context.Context, d time.Duration) error
	errors  prometheus.Counter
}

type Option func(*Client)

// WithBaseURL — адрес Bot API.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithSleep — ожидание между попытками; в тестах записывает паузы вместо сна.
func WithSleep(f func(ctx context.Context, d time.Duration) error) Option {
	return func(c *Client) { c.sleep = f }
}

func New(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		token:   token,
		http:    &http.Client{Timeout: httpTimeout},
		sleep:   sleep,
		errors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "worker_telegram_send_errors_total",
			Help: "Сообщения Telegram, не отправленные после всех попыток.",
		}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// RegisterMetrics регистрирует worker_telegram_send_errors_total (вызывает worker).
func (c *Client) RegisterMetrics(reg prometheus.Registerer) error {
	if err := reg.Register(c.errors); err != nil {
		return fmt.Errorf("telegram: register metrics: %w", err)
	}
	return nil
}

// Send отправляет text (HTML) в chatID и возвращает message_id. 429 повторяется через retry_after,
// 5xx и сетевые ошибки — через 1 с и 2 с; остальные ошибки Telegram (400, 401, 403) не повторяются.
func (c *Client) Send(ctx context.Context, chatID, text string) (int64, error) {
	if c.token == "" {
		return 0, ErrNoToken
	}
	body, err := json.Marshal(sendMessageRequest{
		ChatID: chatID, Text: text, ParseMode: "HTML", DisableWebPagePreview: true,
	})
	if err != nil {
		return 0, fmt.Errorf("telegram: encode: %w", err)
	}
	backoff := time.Second
	for attempt := 1; ; attempt++ {
		id, err := c.sendOnce(ctx, body)
		if err == nil {
			return id, nil
		}
		wait, retry := retryDelay(err, backoff)
		if !retry || attempt == maxAttempts || ctx.Err() != nil {
			c.errors.Inc()
			return 0, err
		}
		if err := c.sleep(ctx, wait); err != nil {
			c.errors.Inc()
			return 0, fmt.Errorf("telegram: wait before retry: %w", err)
		}
		backoff *= 2
	}
}

// retryDelay — пауза перед следующей попыткой и стоит ли её делать.
func retryDelay(err error, backoff time.Duration) (time.Duration, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		var invalid *invalidResponseError
		return backoff, !errors.As(err, &invalid) // сеть или таймаут
	}
	switch {
	case !apiErr.retryable():
		return 0, false
	case apiErr.RetryAfter > maxRetryAfter:
		return 0, false
	case apiErr.RetryAfter > 0:
		return time.Duration(apiErr.RetryAfter) * time.Second, true
	default:
		return backoff, true
	}
}

type sendMessageRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
	Result *struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

// invalidResponseError — ответ Telegram не разобрался; не повторяется.
type invalidResponseError struct{ status int }

func (e *invalidResponseError) Error() string {
	return "telegram: invalid response (HTTP " + strconv.Itoa(e.status) + ")"
}

func (c *Client) sendOnce(ctx context.Context, body []byte) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return 0, errors.New("telegram: build request") // текст ошибки цитировал бы URL с токеном
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("telegram: send: %w", withoutURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, fmt.Errorf("telegram: read response: %w", withoutURL(err))
	}

	var r apiResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		if resp.StatusCode >= http.StatusInternalServerError {
			return 0, &APIError{Code: resp.StatusCode}
		}
		return 0, &invalidResponseError{status: resp.StatusCode}
	}
	if !r.OK {
		code := r.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		return 0, &APIError{Code: code, Description: r.Description, RetryAfter: r.Parameters.RetryAfter}
	}
	if r.Result == nil {
		return 0, &invalidResponseError{status: resp.StatusCode}
	}
	return r.Result.MessageID, nil
}

// withoutURL снимает с ошибки net/http обёртку *url.Error: её текст содержит URL, а в нём токен бота.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
