package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/pgstore"
)

func TestOpen_InvalidURL(t *testing.T) {
	_, err := pgstore.Open(t.Context(), "://not a url")
	require.Error(t, err)
	require.ErrorContains(t, err, "pgstore")
}

func TestOpen_UnreachableServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	// Порт 1 закрыт: Open обязан проверить соединение, а не вернуть «ленивый» пул.
	_, err := pgstore.Open(ctx, "postgres://u:secret-pass@127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-pass", "пароль не попадает в текст ошибки")
}
