package httpx

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"net/http"
	"time"

	"appmetrica.online/backend/internal/clock"
)

const HeaderRequestID = "X-Request-Id"

// crockford — алфавит Crockford base32 в каноническом (верхнем) регистре.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

type requestIDKey struct{}

// RequestID присваивает запросу ULID: берёт валидный ULID из заголовка X-Request-Id
// (его может прислать Traefik или клиент), иначе генерирует новый. Любое другое значение
// отбрасывается, чтобы в логи не попадал произвольный текст клиента.
func RequestID(c clock.Clock) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			if !ValidULID(id) {
				id = NewULID(c.Now())
			}
			w.Header().Set(HeaderRequestID, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
		})
	}
}

// RequestIDFrom возвращает ULID запроса или пустую строку вне цепочки RequestID.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// NewULID возвращает ULID: 48 бит миллисекунд Unix-времени и 80 случайных бит, 26 символов Crockford base32.
func NewULID(t time.Time) string {
	var b, ts [16]byte
	binary.BigEndian.PutUint64(ts[:8], uint64(max(t.UnixMilli(), 0))) // 48 младших бит — время в мс
	copy(b[:6], ts[2:8])
	_, _ = rand.Read(b[6:]) // crypto/rand.Read не возвращает ошибок (Go 1.24+)

	hi, lo := binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])
	var out [26]byte
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}

// ValidULID проверяет каноническую запись ULID: 26 символов Crockford base32 в верхнем регистре
// без переполнения 128 бит (первый символ не больше '7').
func ValidULID(s string) bool {
	if len(s) != 26 || s[0] > '7' {
		return false
	}
	for i := range len(s) {
		if !isCrockford(s[i]) {
			return false
		}
	}
	return true
}

func isCrockford(c byte) bool {
	for i := range len(crockford) {
		if crockford[i] == c {
			return true
		}
	}
	return false
}
