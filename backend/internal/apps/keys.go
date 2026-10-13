package apps

import (
	"crypto/rand"
	"fmt"
)

const (
	base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	writeKeyPrefix = "pk_"
	writeKeyChars  = 32
	// firstKeyLabel — label первого write-ключа нового приложения (02-tech-spec.md FR-009).
	firstKeyLabel = "Основной"
)

// newWriteKey — публичный write-ключ `pk_` + 32 символа base62 из crypto/rand.
// Байты ≥ 248 отбрасываются: 248 = 4·62, так что остаток по 62 распределён равномерно.
func newWriteKey() (string, error) {
	out := make([]byte, 0, len(writeKeyPrefix)+writeKeyChars)
	out = append(out, writeKeyPrefix...)
	buf := make([]byte, writeKeyChars*2)
	for len(out) < cap(out) {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("apps: random write key: %w", err)
		}
		for _, b := range buf {
			if b < 248 && len(out) < cap(out) {
				out = append(out, base62[b%62])
			}
		}
	}
	return string(out), nil
}
