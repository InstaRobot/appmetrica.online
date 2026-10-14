package telegram_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/telegram"
)

const token = "123456:test-bot-token-SECRET"

// reply — ответ фейкового Bot API.
type reply struct {
	status int
	body   string
}

func okReply(messageID int) reply {
	return reply{http.StatusOK, `{"ok":true,"result":{"message_id":` + itoa(messageID) + `,"chat":{"id":1}}}`}
}

func errReply(status int, description string) reply {
	return reply{status, `{"ok":false,"error_code":` + itoa(status) + `,"description":"` + description + `"}`}
}

func tooManyRequests(retryAfter int) reply {
	return reply{http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after ` +
		itoa(retryAfter) + `","parameters":{"retry_after":` + itoa(retryAfter) + `}}`}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// fakeBotAPI отвечает replies по очереди (последний повторяется) и записывает тела запросов.
type fakeBotAPI struct {
	*httptest.Server
	mu       sync.Mutex
	replies  []reply
	requests []map[string]any
	paths    []string
}

func newFakeBotAPI(t *testing.T, replies ...reply) *fakeBotAPI {
	t.Helper()
	f := &fakeBotAPI{replies: replies}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.requests = append(f.requests, body)
		f.paths = append(f.paths, r.Method+" "+r.URL.Path+" "+r.Header.Get("Content-Type"))
		rep := f.replies[min(len(f.requests), len(f.replies))-1]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rep.status)
		_, _ = w.Write([]byte(rep.body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeBotAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// sleeps записывает паузы между попытками вместо настоящего ожидания.
type sleeps struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleeps) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.d = append(s.d, d)
	return nil
}

func newClient(f *fakeBotAPI, s *sleeps, opts ...telegram.Option) *telegram.Client {
	return telegram.New(token, append([]telegram.Option{telegram.WithBaseURL(f.URL), telegram.WithSleep(s.sleep)}, opts...)...)
}

func TestSend_RetriesOn429(t *testing.T) {
	f := newFakeBotAPI(t, tooManyRequests(1), okReply(4211))
	s := &sleeps{}

	id, err := newClient(f, s).Send(t.Context(), "123456789", "hello")
	require.NoError(t, err)
	require.Equal(t, int64(4211), id)
	require.Equal(t, 2, f.count())
	require.Equal(t, []time.Duration{time.Second}, s.d)
}

// Без WithSleep клиент ждёт retry_after по-настоящему.
func TestSend_RetriesOn429_RealWait(t *testing.T) {
	f := newFakeBotAPI(t, tooManyRequests(1), okReply(7))
	start := time.Now()
	id, err := telegram.New(token, telegram.WithBaseURL(f.URL)).Send(t.Context(), "1", "hello")
	require.NoError(t, err)
	require.Equal(t, int64(7), id)
	require.GreaterOrEqual(t, time.Since(start), time.Second)
}

func TestSendMessage_OK(t *testing.T) {
	f := newFakeBotAPI(t, okReply(42))
	id, err := newClient(f, &sleeps{}).Send(t.Context(), "-1001234567890", "<b>Портфель</b> за вчера")
	require.NoError(t, err)
	require.Equal(t, int64(42), id)
	require.Equal(t, []string{"POST /bot" + token + "/sendMessage application/json"}, f.paths)
	require.Equal(t, []map[string]any{{
		"chat_id":                  "-1001234567890",
		"text":                     "<b>Портфель</b> за вчера",
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}}, f.requests)
}

func TestSend_ClientErrorsAreNotRetried(t *testing.T) {
	cases := []struct {
		name        string
		reply       reply
		code        int
		description string
	}{
		{"chat_not_found", errReply(http.StatusBadRequest, "Bad Request: chat not found"), 400, "Bad Request: chat not found"},
		{"unauthorized", errReply(http.StatusUnauthorized, "Unauthorized"), 401, "Unauthorized"},
		{"bot_blocked", errReply(http.StatusForbidden, "Forbidden: bot was blocked by the user"), 403, "Forbidden: bot was blocked by the user"},
		{"no_description", reply{http.StatusBadRequest, `{"ok":false,"error_code":400}`}, 400, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeBotAPI(t, tc.reply, okReply(1))
			s := &sleeps{}
			_, err := newClient(f, s).Send(t.Context(), "1", "hello")
			var apiErr *telegram.APIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, tc.code, apiErr.Code)
			require.Equal(t, tc.description, apiErr.Description)
			if tc.description != "" {
				require.Equal(t, tc.description, err.Error())
			} else {
				require.NotEmpty(t, err.Error())
			}
			require.Equal(t, 1, f.count())
			require.Empty(t, s.d)
		})
	}
}

func TestSend_RetriesServerErrorsWithBackoff(t *testing.T) {
	t.Run("recovers", func(t *testing.T) {
		f := newFakeBotAPI(t, errReply(http.StatusBadGateway, "Bad Gateway"), reply{http.StatusInternalServerError, "oops"}, okReply(9))
		s := &sleeps{}
		id, err := newClient(f, s).Send(t.Context(), "1", "hello")
		require.NoError(t, err)
		require.Equal(t, int64(9), id)
		require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, s.d)
	})

	t.Run("gives_up_after_three_attempts", func(t *testing.T) {
		f := newFakeBotAPI(t, errReply(http.StatusBadGateway, "Bad Gateway"))
		s := &sleeps{}
		_, err := newClient(f, s).Send(t.Context(), "1", "hello")
		var apiErr *telegram.APIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadGateway, apiErr.Code)
		require.Equal(t, 3, f.count())
		require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, s.d)
	})

	t.Run("429_three_times", func(t *testing.T) {
		f := newFakeBotAPI(t, tooManyRequests(2))
		s := &sleeps{}
		_, err := newClient(f, s).Send(t.Context(), "1", "hello")
		var apiErr *telegram.APIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusTooManyRequests, apiErr.Code)
		require.Equal(t, 2, apiErr.RetryAfter)
		require.Equal(t, 3, f.count())
		require.Equal(t, []time.Duration{2 * time.Second, 2 * time.Second}, s.d)
	})

	t.Run("429_without_retry_after_uses_backoff", func(t *testing.T) {
		f := newFakeBotAPI(t, errReply(http.StatusTooManyRequests, "Too Many Requests"), okReply(3))
		s := &sleeps{}
		_, err := newClient(f, s).Send(t.Context(), "1", "hello")
		require.NoError(t, err)
		require.Equal(t, []time.Duration{time.Second}, s.d)
	})
}

// retry_after дольше минуты не ждём: тестовое сообщение дашборда и цикл worker-а не должны висеть.
func TestSend_RetryAfterTooLong(t *testing.T) {
	f := newFakeBotAPI(t, tooManyRequests(3600), okReply(1))
	s := &sleeps{}
	_, err := newClient(f, s).Send(t.Context(), "1", "hello")
	var apiErr *telegram.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 3600, apiErr.RetryAfter)
	require.Equal(t, 1, f.count())
	require.Empty(t, s.d)
}

func TestSend_InvalidResponses(t *testing.T) {
	for name, rep := range map[string]reply{
		"not_json":          {http.StatusOK, "<html>"},
		"ok_false_200":      {http.StatusOK, `{"ok":false,"error_code":400,"description":"Bad Request: message text is empty"}`},
		"ok_without_result": {http.StatusOK, `{"ok":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBotAPI(t, rep)
			_, err := newClient(f, &sleeps{}).Send(t.Context(), "1", "hello")
			require.Error(t, err)
			require.NotContains(t, err.Error(), token)
			require.Equal(t, 1, f.count())
		})
	}
}

// Ошибка сети цитирует URL запроса, а в нём токен бота: клиент его вырезает.
func TestSend_ErrorsDoNotContainToken(t *testing.T) {
	f := newFakeBotAPI(t, okReply(1))
	f.Close()
	s := &sleeps{}
	_, err := newClient(f, s).Send(t.Context(), "1", "hello")
	require.Error(t, err)
	require.NotContains(t, err.Error(), token)
	require.NotContains(t, err.Error(), "SECRET")
	require.Len(t, s.d, 2, "сетевая ошибка повторяется")
}

func TestSend_ContextCanceledStopsRetries(t *testing.T) {
	f := newFakeBotAPI(t, tooManyRequests(1), okReply(1))
	ctx, cancel := context.WithCancel(t.Context())
	c := telegram.New(token, telegram.WithBaseURL(f.URL), telegram.WithSleep(func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}))
	_, err := c.Send(ctx, "1", "hello")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, f.count())
}

func TestSend_HTTPClientTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	s := &sleeps{}
	c := telegram.New(token, telegram.WithBaseURL(slow.URL), telegram.WithSleep(s.sleep),
		telegram.WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	_, err := c.Send(t.Context(), "1", "hello")
	require.Error(t, err)
	require.NotContains(t, err.Error(), token)
}

func TestSend_CountsFailedSends(t *testing.T) {
	f := newFakeBotAPI(t, okReply(1), errReply(http.StatusBadRequest, "Bad Request: chat not found"))
	reg := prometheus.NewRegistry()
	c := newClient(f, &sleeps{})
	require.NoError(t, c.RegisterMetrics(reg))

	_, err := c.Send(t.Context(), "1", "ok")
	require.NoError(t, err)
	require.InDelta(t, 0, counterValue(t, reg, "worker_telegram_send_errors_total"), 0)

	_, err = c.Send(t.Context(), "1", "fail")
	require.Error(t, err)
	require.InDelta(t, 1, counterValue(t, reg, "worker_telegram_send_errors_total"), 0)

	require.Error(t, c.RegisterMetrics(reg), "повторная регистрация")
}

func counterValue(t *testing.T, g prometheus.Gatherer, name string) float64 {
	t.Helper()
	families, err := g.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("metric %s not found", name)
	return 0
}

// Без токена запрос не уходит.
func TestSend_EmptyToken(t *testing.T) {
	f := newFakeBotAPI(t, okReply(1))
	_, err := telegram.New("", telegram.WithBaseURL(f.URL)).Send(t.Context(), "1", "hello")
	require.ErrorIs(t, err, telegram.ErrNoToken)
	require.Equal(t, 0, f.count())
}
