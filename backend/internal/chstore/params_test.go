package chstore_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

// Значение параметра String сервер разбирает как Escaped-текст: обратный слеш и управляющие символы экранируются,
// кавычки — нет.
func TestStringParam(t *testing.T) {
	require.Equal(t, "u-42", chstore.StringParam("u-42"))
	require.Equal(t, `o'brien\\x "42"`, chstore.StringParam(`o'brien\x "42"`))
	require.Equal(t, `a\tb\nc\r\0`, chstore.StringParam("a\tb\nc\r\x00"))
	require.Empty(t, chstore.StringParam(""))
}

// Элементы Array(String) стоят в кавычках, поэтому экранируется и одинарная кавычка.
func TestStringArrayParam(t *testing.T) {
	require.Equal(t, "[]", chstore.StringArrayParam(nil))
	require.Equal(t, `['ios','o\'brien\\x','a\tb']`, chstore.StringArrayParam([]string{"ios", `o'brien\x`, "a\tb"}))
	require.Equal(t, `['a','"u-42"']`, chstore.StringArrayParam([]string{"a", `"u-42"`}), "кавычки JSON-строки не экранируются")
	require.Equal(t, `['o\'b\\r\tx\n']`, chstore.StringArrayParam([]string{"o'b\\r\tx\n"}))

	a, b := uuid.MustParse("6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21"), uuid.MustParse("9b2e4c61-0f3a-4d8b-9e71-5c2a6d8f0b13")
	require.Equal(t, "['"+a.String()+"','"+b.String()+"']", chstore.UUIDArrayParam([]uuid.UUID{a, b}))
}
