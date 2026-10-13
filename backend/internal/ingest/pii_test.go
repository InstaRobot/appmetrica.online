package ingest_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
)

func TestPIIGuard(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		// I-07…I-09 и AC-1.5.2.
		for i, uid := range []string{
			"ivan@example.com",
			"+79161234567",
			"8 (916) 123-45-67",
			"+7 (916) 123-45-67",
			"8-916-123-45-67",
			"  ivan@example.com  ",
			"+1 202 555 0143",
			"(916)1234567",
		} {
			require.True(t, ingest.LooksLikePII(uid), "LooksLikePII(%q)", uid)

			_, rej := ingest.ValidateEvent(i, eventJSON(t, map[string]any{"user_id": uid}))
			require.NotNil(t, rej, uid)
			require.Equal(t, ingestapi.RejectionCodeUserIdLooksLikePii, rej.Code, uid)
			require.Equal(t, "events["+strconv.Itoa(i)+"].user_id", rej.Field, uid)
			require.False(t, rej.Retryable)
		}
	})

	t.Run("allowed", func(t *testing.T) {
		// I-10 и AC-1.5.1.
		for _, uid := range []string{
			"123456789012",
			"123456789",
			"user_8f3a9c",
			"user_42",
			"8f14e45f-ceea-467f-a0e6-7c9c1f2d1c01",
			strings.Repeat("a", 128),
			"1234567890123456789012345",
		} {
			require.False(t, ingest.LooksLikePII(uid), "LooksLikePII(%q)", uid)

			ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": uid}))
			require.Nil(t, rej, uid)
			require.Equal(t, uid, ev.UserID)
		}

		_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": strings.Repeat("a", 129)}))
		require.NotNil(t, rej)
		require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)
		require.Equal(t, "events[0].user_id", rej.Field)
	})

	t.Run("alnum_with_digits_allowed", func(t *testing.T) {
		for _, uid := range []string{"ID 1234567890", "tg-1234567890", "a@b", "user@localhost"} {
			require.False(t, ingest.LooksLikePII(uid), "LooksLikePII(%q)", uid)
		}
	})

	t.Run("formatted_phone_digit_count", func(t *testing.T) {
		require.False(t, ingest.LooksLikePII("916 123-45-6"), "9 цифр — не телефон")
		require.True(t, ingest.LooksLikePII("916 123-45-67"), "10 цифр")
		require.True(t, ingest.LooksLikePII("+123 456 789 012 345"), "15 цифр")
		require.False(t, ingest.LooksLikePII("1234 5678 9012 3456"), "16 цифр — не телефон")
		require.False(t, ingest.LooksLikePII("8 916 + 123 45 67"), "+ не в начале")
	})

	t.Run("e164_boundaries", func(t *testing.T) {
		require.True(t, ingest.LooksLikePII("+1234567"), "7 цифр")
		require.False(t, ingest.LooksLikePII("+123456"), "6 цифр")
		require.True(t, ingest.LooksLikePII("+123456789012345"), "15 цифр")
		require.False(t, ingest.LooksLikePII("+0123456789"), "E.164 не начинается с 0")
	})

	t.Run("length_boundary", func(t *testing.T) {
		// Длина в символах, не в байтах: 128 кириллических букв — 256 байт.
		cyr := strings.Repeat("ж", 128)
		ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": cyr}))
		require.Nil(t, rej)
		require.Equal(t, cyr, ev.UserID)

		_, rej = ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": cyr + "ж"}))
		require.NotNil(t, rej)
		require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)

		// Пробелы по краям не считаются.
		ev, rej = ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": "  " + strings.Repeat("a", 128) + "  "}))
		require.Nil(t, rej)
		require.Len(t, ev.UserID, 128)
	})
}

func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func FuzzLooksLikePII(f *testing.F) {
	for _, seed := range []string{
		"", "123456789012", "user_8f3a9c", "ivan@example.com", "+79161234567", "8 (916) 123-45-67",
		"8f14e45f-ceea-467f-a0e6-7c9c1f2d1c01", "+", "@", "a@b.c", "  ", "(((", "\xff\xfe",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := ingest.LooksLikePII(s)
		if isASCIIDigits(strings.TrimSpace(s)) && got {
			t.Fatalf("чисто цифровая строка %q отклонена", s)
		}
		if got != ingest.LooksLikePII(strings.TrimSpace(s)) {
			t.Fatalf("результат зависит от пробелов по краям: %q", s)
		}
	})
}

func BenchmarkPIIGuard(b *testing.B) {
	values := []string{"a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90", "ivan@example.com", "8 (916) 123-45-67", "123456789012"}
	for b.Loop() {
		for _, v := range values {
			ingest.LooksLikePII(v)
		}
	}
}
