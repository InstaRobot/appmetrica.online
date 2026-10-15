package apps

import (
	"crypto/rand"
	"fmt"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	writeKeyPrefix = "pk_"
	writeKeyChars  = 32
)

// platformLabels — метки write-ключей, которые Create выпускает на каждую платформу приложения.
var platformLabels = map[sqlc.AppPlatform]string{
	sqlc.AppPlatformIos:     "iOS",
	sqlc.AppPlatformAndroid: "Android",
	sqlc.AppPlatformWeb:     "Веб",
	sqlc.AppPlatformMacos:   "macOS",
	sqlc.AppPlatformWindows: "Windows",
	sqlc.AppPlatformLinux:   "Linux",
	sqlc.AppPlatformBot:     "Бот",
	sqlc.AppPlatformServer:  "Бэкенд",
	sqlc.AppPlatformOther:   "Другое",
}

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
