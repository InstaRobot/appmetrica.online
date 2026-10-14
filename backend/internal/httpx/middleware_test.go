package httpx_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/httpx"
)

var fixedNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

type errorEnvelope struct {
	Error struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		RequestID string         `json:"request_id"`
		Details   map[string]any `json:"details"`
	} `json:"error"`
}

func newLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// stack — цепочка в том же порядке, что и в cmd/appmetrica.
func stack(logger *slog.Logger, h http.Handler) http.Handler {
	return httpx.Chain(h,
		httpx.RequestID(clock.NewFixed(fixedNow)),
		httpx.AccessLog(logger, clock.NewFixed(fixedNow)),
		httpx.Recover(logger),
	)
}

func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), line)
		lines = append(lines, rec)
	}
	return lines
}

func requireULID(t *testing.T, id string) {
	t.Helper()
	require.Len(t, id, 26)
	for _, c := range id {
		require.True(t, strings.ContainsRune(crockford, c), "символ %q не из алфавита Crockford base32 в %q", c, id)
	}
	// Первый символ кодирует старшие 3 бита 48-битного времени — не больше '7'.
	require.LessOrEqual(t, id[0], byte('7'))
}

func TestRequestID_GeneratesULID(t *testing.T) {
	var buf bytes.Buffer
	var seen string
	h := stack(newLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/events", nil))

	id := rec.Header().Get("X-Request-Id")
	requireULID(t, id)
	require.Equal(t, id, seen)

	lines := decodeLogLines(t, &buf)
	require.Len(t, lines, 1)
	require.Equal(t, id, lines[0]["request_id"])
	require.Equal(t, "GET", lines[0]["method"])
	require.Equal(t, httpx.UnmatchedPath, lines[0]["path"], "без ServeMux нет Pattern — сырой путь не пишется")
	require.EqualValues(t, http.StatusNoContent, lines[0]["status"])
}

func TestRequestID_UniqueAndTimeOrdered(t *testing.T) {
	c := clock.NewFixed(fixedNow)
	h := httpx.RequestID(c)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	seen := map[string]bool{}
	prev := ""
	for i := range 100 {
		c.Advance(time.Millisecond)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		id := rec.Header().Get("X-Request-Id")
		requireULID(t, id)
		require.False(t, seen[id], "повтор id на итерации %d", i)
		seen[id] = true
		require.Greater(t, id, prev, "ULID должны расти вместе со временем")
		prev = id
	}
}

func TestRequestID_IncomingHeader(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{"valid ulid is kept", "01JA8Z3K7Q2M9V4T6R1X5N8C0B", true},
		{"lowercase ulid is rejected", "01ja8z3k7q2m9v4t6r1x5n8c0b", false},
		{"wrong length", "01JA8Z3K7Q2M9V4T6R1X5N8C0", false},
		{"forbidden letter", "01JA8Z3K7Q2M9V4T6R1X5N8C0U", false},
		{"log injection", "abc\n{\"request_id\":\"x\"}", false},
		{"overflowing timestamp", "81JA8Z3K7Q2M9V4T6R1X5N8C0B", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := httpx.RequestID(clock.NewFixed(fixedNow))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req.Header.Set("X-Request-Id", tt.incoming)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Request-Id")
			requireULID(t, got)
			if tt.keep {
				require.Equal(t, tt.incoming, got)
			} else {
				require.NotEqual(t, tt.incoming, got)
			}
		})
	}
}

func TestAccessLog_NoClientIP(t *testing.T) {
	var buf bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	h := stack(newLogger(&buf), mux)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events?app_key=pk_secret", strings.NewReader("{}"))
	req.RemoteAddr = "203.0.113.7:5000"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("X-Real-Ip", "203.0.113.7")
	req.Header.Set("X-App-Key", "pk_secret")
	req.Header.Set("User-Agent", "probe/1.0")
	h.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	require.NotEmpty(t, out)
	require.NotContains(t, out, "203.0.113.7")
	require.NotContains(t, out, "pk_secret")

	lines := decodeLogLines(t, &buf)
	require.Len(t, lines, 1)
	require.Equal(t, "/v1/events", lines[0]["path"])
	require.EqualValues(t, 200, lines[0]["status"])
	require.EqualValues(t, 2, lines[0]["bytes"])
	require.Contains(t, lines[0], "duration_ms")
}

func TestAccessLog_DurationFromClock(t *testing.T) {
	var buf bytes.Buffer
	c := clock.NewFixed(fixedNow)
	h := httpx.AccessLog(newLogger(&buf), c)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		c.Advance(1500 * time.Millisecond)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	lines := decodeLogLines(t, &buf)
	require.Len(t, lines, 1)
	require.EqualValues(t, 1500, lines[0]["duration_ms"])
	require.EqualValues(t, 200, lines[0]["status"], "без WriteHeader статус по умолчанию 200")
}

func TestRecover_ReturnsErrorEnvelope(t *testing.T) {
	var buf bytes.Buffer
	h := stack(newLogger(&buf), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	srv := httptest.NewServer(h)
	defer srv.Close()

	for range 2 { // второй запрос доказывает, что процесс жив после паники
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/v1/apps", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		var env errorEnvelope
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
		require.NoError(t, resp.Body.Close())

		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		require.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"))
		require.Equal(t, "internal_error", env.Error.Code)
		require.NotEmpty(t, env.Error.Message)
		require.Equal(t, resp.Header.Get("X-Request-Id"), env.Error.RequestID)
		requireULID(t, env.Error.RequestID)
	}

	var panics, access int
	for _, l := range decodeLogLines(t, &buf) {
		switch l["msg"] {
		case "panic recovered":
			panics++
			require.Equal(t, "ERROR", l["level"])
			require.Contains(t, l["panic"], "boom")
			require.NotEmpty(t, l["request_id"])
		case "http request":
			access++
			require.EqualValues(t, 500, l["status"])
		}
	}
	require.Equal(t, 2, panics)
	require.Equal(t, 2, access)
}

func TestRecover_AfterHeadersWritten(t *testing.T) {
	var buf bytes.Buffer
	h := stack(newLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("late boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	require.Equal(t, http.StatusAccepted, rec.Code, "ответ уже начат — статус не переписывается")
	require.NotContains(t, rec.Body.String(), "internal_error")
	require.Contains(t, buf.String(), "late boom")
}

func TestRecover_ErrAbortHandlerPropagates(t *testing.T) {
	h := httpx.Recover(newLogger(&bytes.Buffer{}))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	require.PanicsWithError(t, http.ErrAbortHandler.Error(), func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	})
}

func TestWriteError_Envelope(t *testing.T) {
	h := httpx.RequestID(clock.NewFixed(fixedNow))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_batch", "Поле events обязательно.", map[string]any{"field": "events"})
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", nil))

	var env errorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_batch", env.Error.Code)
	require.Equal(t, "Поле events обязательно.", env.Error.Message)
	require.Equal(t, rec.Header().Get("X-Request-Id"), env.Error.RequestID)
	require.Equal(t, map[string]any{"field": "events"}, env.Error.Details)

	// details не передан — поля нет в JSON.
	rec = httptest.NewRecorder()
	httpx.WriteError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), http.StatusNotFound, "not_found", "Не найдено.", nil)
	require.NotContains(t, rec.Body.String(), "details")
}

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(b)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// echoBody возвращает прочитанное тело или 413, если сработал лимит.
func echoBody() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", "too large", nil)
			return
		}
		if err != nil || r.Header.Get("Content-Encoding") != "" {
			http.Error(w, "unexpected body state", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(b)
	})
}

func TestGzip_DecodesBody(t *testing.T) {
	payload := []byte(`{"events":[]}`)
	h := httpx.Chain(echoBody(), httpx.Gunzip(), httpx.BodyLimit(1024))

	tests := []struct {
		name     string
		encoding string
		body     []byte
	}{
		{"gzip", "gzip", gzipBytes(t, payload)},
		{"gzip uppercase", "GZIP", gzipBytes(t, payload)},
		{"identity", "identity", payload},
		{"no encoding", "", payload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", bytes.NewReader(tt.body))
			if tt.encoding != "" {
				req.Header.Set("Content-Encoding", tt.encoding)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, string(payload), rec.Body.String())
		})
	}
}

func TestGzip_Rejects(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		body     []byte
		code     string
	}{
		{"brotli", "br", []byte("x"), "unsupported_content_encoding"},
		{"deflate", "deflate", []byte("x"), "unsupported_content_encoding"},
		{"stacked encodings", "gzip, gzip", []byte("x"), "unsupported_content_encoding"},
		{"corrupt gzip header", "gzip", []byte("not gzip at all"), "invalid_json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := httpx.Chain(echoBody(), httpx.RequestID(clock.NewFixed(fixedNow)), httpx.Gunzip())
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", bytes.NewReader(tt.body))
			req.Header.Set("Content-Encoding", tt.encoding)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			var env errorEnvelope
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, tt.code, env.Error.Code)
			require.Equal(t, rec.Header().Get("X-Request-Id"), env.Error.RequestID)
		})
	}
}

func TestBodyLimit_AppliesAfterGunzip(t *testing.T) {
	const limit = 1 << 20
	h := httpx.Chain(echoBody(), httpx.Gunzip(), httpx.BodyLimit(limit))

	tests := []struct {
		name   string
		size   int
		status int
	}{
		{"exactly at limit", limit, http.StatusOK},
		{"one byte over", limit + 1, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := gzipBytes(t, bytes.Repeat([]byte("a"), tt.size))
			require.Less(t, len(body), 20*1024, "сжатое тело маленькое, лимит считается по распакованному")
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", bytes.NewReader(body))
			req.Header.Set("Content-Encoding", "gzip")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, tt.status, rec.Code)
		})
	}
}

func TestBodyLimit_PlainBody(t *testing.T) {
	h := httpx.Chain(echoBody(), httpx.BodyLimit(4))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("1234")))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("12345")))
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        []string
		want       string
		ok         bool
	}{
		{"remote addr only", "203.0.113.7:5000", nil, "203.0.113.7", true},
		{"last forwarded hop wins", "10.0.0.2:443", []string{"10.0.0.1, 81.2.69.142"}, "81.2.69.142", true},
		{"multiple headers", "10.0.0.2:443", []string{"1.1.1.1", "10.0.0.1,  81.2.69.142 "}, "81.2.69.142", true},
		{"ipv6 remote", "[2001:db8::1]:5000", nil, "2001:db8::1", true},
		{"ipv6 forwarded", "10.0.0.2:443", []string{"2001:db8::2"}, "2001:db8::2", true},
		{"ipv4-mapped ipv6 unmapped", "[::ffff:203.0.113.9]:1", nil, "203.0.113.9", true},
		{"garbage forwarded falls back", "203.0.113.7:5000", []string{"unknown"}, "203.0.113.7", true},
		{"empty forwarded falls back", "203.0.113.7:5000", []string{""}, "203.0.113.7", true},
		{"no address", "", nil, "", false},
		{"garbage remote", "pipe", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for _, v := range tt.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			got, ok := httpx.ClientIP(req)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				require.Equal(t, netip.MustParseAddr(tt.want), got)
			}
		})
	}
}

// Токен ссылки регистрации passkey стоит в пути: в лог попадает шаблон маршрута, а не сам путь.
func TestAccessLog_RoutePatternInsteadOfPath(t *testing.T) {
	var buf bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/v1/auth/setup/{token}/begin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := stack(newLogger(&buf), mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/v1/auth/setup/SECRET-TOKEN/begin", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotContains(t, buf.String(), "SECRET-TOKEN")
	lines := decodeLogLines(t, &buf)
	require.Len(t, lines, 1)
	require.Equal(t, "/admin/v1/auth/setup/{token}/begin", lines[0]["path"])

	// Маршрут не совпал — сырой путь не пишется: при 404/405 ServeMux не заполняет Pattern, а в пути
	// может стоять секрет (тот же токен ссылки регистрации с другим методом или лишним слэшем).
	buf.Reset()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil))
	require.Equal(t, httpx.UnmatchedPath, decodeLogLines(t, &buf)[0]["path"])
}

// Запрос с другим методом на маршрут с параметром получает 405 от ServeMux без Pattern:
// сырой путь с токеном ссылки регистрации passkey не должен попасть в access-лог.
func TestAccessLog_UnmatchedRouteHidesPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/v1/auth/setup/{token}/begin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "method mismatch 405", method: http.MethodGet, path: "/admin/v1/auth/setup/SECRET-TOKEN/begin", status: http.StatusMethodNotAllowed},
		{name: "head 405", method: http.MethodHead, path: "/admin/v1/auth/setup/SECRET-TOKEN/begin", status: http.StatusMethodNotAllowed},
		{name: "trailing slash 404", method: http.MethodPost, path: "/admin/v1/auth/setup/SECRET-TOKEN/begin/", status: http.StatusNotFound},
		{name: "near miss 404", method: http.MethodPost, path: "/admin/v1/auth/setup/SECRET-TOKEN", status: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := stack(newLogger(&buf), mux)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			require.Equal(t, tt.status, rec.Code)
			require.NotContains(t, buf.String(), "SECRET-TOKEN")
			lines := decodeLogLines(t, &buf)
			require.Len(t, lines, 1)
			require.Equal(t, httpx.UnmatchedPath, lines[0]["path"])
			require.EqualValues(t, tt.status, lines[0]["status"])
		})
	}
}

// Два заголовка Content-Encoding — не identity и не один gzip: 400 unsupported_content_encoding.
func TestGzip_RejectsRepeatedHeader(t *testing.T) {
	h := httpx.Chain(echoBody(), httpx.RequestID(clock.NewFixed(fixedNow)), httpx.Gunzip())
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/events", bytes.NewReader(gzipBytes(t, []byte("{}"))))
	req.Header.Add("Content-Encoding", "gzip")
	req.Header.Add("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var env errorEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "unsupported_content_encoding", env.Error.Code)
}
