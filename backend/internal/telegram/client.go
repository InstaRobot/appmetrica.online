// Package telegram — клиент Telegram Bot API (sendMessage) для дайджеста, алертов и тестового сообщения.
// Токен бота входит в URL запроса, поэтому ни URL, ни токен не попадают в тексты ошибок.
package telegram

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DefaultBaseURL — адрес Bot API; в тестах подменяется httptest-сервером (WithBaseURL).
const DefaultBaseURL = "https://api.telegram.org"

var errNotImplemented = errors.New("telegram: not implemented")

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

// Client отправляет сообщения от имени бота.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	sleep   func(ctx context.Context, d time.Duration) error
	errors  prometheus.Counter
}

type Option func(*Client)

// WithBaseURL — адрес Bot API без завершающего «/».
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithSleep — ожидание между попытками; в тестах записывает паузы вместо сна.
func WithSleep(f func(ctx context.Context, d time.Duration) error) Option {
	return func(c *Client) { c.sleep = f }
}

func New(token string, opts ...Option) *Client {
	c := &Client{baseURL: DefaultBaseURL, token: token, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c
}

// RegisterMetrics регистрирует worker_telegram_send_errors_total.
func (c *Client) RegisterMetrics(_ prometheus.Registerer) error { return errNotImplemented }

// Send отправляет text в chatID и возвращает message_id.
func (c *Client) Send(_ context.Context, _, _ string) (int64, error) { return 0, errNotImplemented }

// ErrNoToken — TELEGRAM_BOT_TOKEN не задан: сообщения не отправляются.
var ErrNoToken = errors.New("telegram: TELEGRAM_BOT_TOKEN is not set")
