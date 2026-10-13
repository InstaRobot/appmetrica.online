package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// tokenBytes — длина setup-токенов и токенов сессии: 32 случайных байта, base64url без padding — 43 символа.
const tokenBytes = 32

var tokenEncoding = base64.RawURLEncoding.Strict()

// newToken создаёт случайный токен и его sha256. В базе хранится только хэш.
func newToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: random token: %w", err)
	}
	sum := sha256.Sum256(raw)
	return tokenEncoding.EncodeToString(raw), sum[:], nil
}

// hashToken декодирует токен из ссылки или cookie и возвращает sha256 его байтов; false — токен некорректен.
func hashToken(token string) ([]byte, bool) {
	if len(token) != tokenEncoding.EncodedLen(tokenBytes) {
		return nil, false
	}
	raw, err := tokenEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}
