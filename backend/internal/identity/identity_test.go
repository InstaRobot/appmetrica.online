package identity_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/identity"
)

func TestParseUnit(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want identity.Unit
	}{
		{"", identity.Persons},
		{"persons", identity.Persons},
		{"installs", identity.Installs},
	} {
		got, err := identity.ParseUnit(tc.in)
		require.NoError(t, err, "unit=%q", tc.in)
		require.Equal(t, tc.want, got, "unit=%q", tc.in)
	}

	for _, in := range []string{"people", "devices", "Persons", " persons", "installs "} {
		_, err := identity.ParseUnit(in)
		require.ErrorIs(t, err, identity.ErrInvalidUnit, "unit=%q", in)
	}
}

func TestPersonKey_Format(t *testing.T) {
	install := uuid.MustParse("5ce10001-0000-4000-8000-000000000007")

	require.Equal(t, "u:u-42", identity.UserKey("u-42").String())
	require.Equal(t, "i:5ce10001-0000-4000-8000-000000000007", identity.InstallKey(install).String())
	require.True(t, identity.UserKey("u-42").IsUser())
	require.False(t, identity.InstallKey(install).IsUser())
}

func TestParsePersonKey(t *testing.T) {
	install := uuid.MustParse("5ce10001-0000-4000-8000-000000000007")
	for _, tc := range []struct {
		in   string
		want identity.PersonKey
	}{
		{"u:u-42", identity.UserKey("u-42")},
		{"u:a:b", identity.UserKey("a:b")}, // двоеточие внутри user_id не разделитель
		{"u:Пользователь 7", identity.UserKey("Пользователь 7")},
		{"u:" + strings.Repeat("я", 128), identity.UserKey(strings.Repeat("я", 128))},
		{"i:5ce10001-0000-4000-8000-000000000007", identity.InstallKey(install)},
	} {
		got, err := identity.ParsePersonKey(tc.in)
		require.NoError(t, err, "key=%q", tc.in)
		require.Equal(t, tc.want, got, "key=%q", tc.in)
		require.Equal(t, tc.in, got.String(), "разбор и форматирование взаимно обратны")
	}
}

func TestParsePersonKey_Errors(t *testing.T) {
	for _, in := range []string{
		"",
		"x:1",
		"u",
		"u:",
		"i:",
		"U:u-42",
		"I:5ce10001-0000-4000-8000-000000000007",
		"u:" + strings.Repeat("я", 129),
		"i:not-a-uuid",
		"i:00000000-0000-0000-0000-000000000000",
		"i:5CE10001-0000-4000-8000-000000000007", // только каноническая запись UUID
		"i:{5ce10001-0000-4000-8000-000000000007}",           // фигурные скобки
		"i:urn:uuid:5ce10001-0000-4000-8000-000000000007",    // URN
		"i:5ce10001000040008000000000000007",                 // без дефисов
		"i:5ce10001-0000-4000-8000-000000000007 ",            // хвостовой пробел
		"5ce10001-0000-4000-8000-000000000007",               // без префикса
		"u-42",                                               // без префикса
		"i:5ce10001-0000-4000-8000-000000000007\x00trailing", // мусор после UUID
	} {
		_, err := identity.ParsePersonKey(in)
		require.ErrorIs(t, err, identity.ErrInvalidPersonKey, "key=%q", in)
	}
}
