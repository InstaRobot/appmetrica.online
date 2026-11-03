package ingest_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
)

// propsEvent — валидное событие purchase с сырым JSON свойств.
func propsEvent(t *testing.T, rawProps string) json.RawMessage {
	t.Helper()
	return eventJSON(t, map[string]any{"name": "purchase", "properties": json.RawMessage(rawProps)})
}

func manyProps(n int) string {
	var b strings.Builder
	b.WriteByte('{')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"k` + strings.Repeat("a", i%3) + string(rune('a'+i/26)) + string(rune('a'+i%26)) + `":1`)
	}
	b.WriteByte('}')
	return b.String()
}

// AC-003.2: таблица нарушений FR-102 §1.2.
func TestValidateProperties(t *testing.T) {
	longKey := strings.Repeat("k", 64)
	cases := []struct {
		name      string
		props     string
		wantField string // "" — принято
	}{
		{"not_object_string", `"x"`, "events[0].properties"},
		{"not_object_array", `[1]`, "events[0].properties"},
		{"not_object_number", `5`, "events[0].properties"},
		{"51_keys", manyProps(51), "events[0].properties"},
		{"50_keys_accepted", manyProps(50), ""},
		{"bad_key_uppercase", `{"Bad":1}`, "events[0].properties.Bad"},
		{"bad_key_digit_first", `{"1a":1}`, "events[0].properties.1a"},
		{"bad_key_empty", `{"":1}`, "events[0].properties."},
		{"key_65_chars", `{"` + longKey + `k":1}`, "events[0].properties." + longKey},
		{"key_64_chars_accepted", `{"` + longKey + `":1}`, ""},
		{"null_value", `{"a":null}`, "events[0].properties.a"},
		{"array_value", `{"a":[1]}`, "events[0].properties.a"},
		{"object_value", `{"a":{"b":1}}`, "events[0].properties.a"},
		{"string_257_runes", `{"a":"` + strings.Repeat("я", 257) + `"}`, "events[0].properties.a"},
		{"string_256_runes_accepted", `{"a":"` + strings.Repeat("я", 256) + `"}`, ""},
		{"nul_char", `{"a":"x\u0000y"}`, "events[0].properties.a"},
		{"empty_object_accepted", `{}`, ""},
		{"empty_string_accepted", `{"a":""}`, ""},
		{"number_out_of_range", `{"a":1e999}`, "events[0].properties.a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, rej := ingest.ValidateEvent(0, propsEvent(t, tc.props))
			if tc.wantField == "" {
				require.Nil(t, rej)
				require.NotNil(t, ev)
				return
			}
			require.Nil(t, ev)
			require.NotNil(t, rej)
			require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)
			require.Equal(t, tc.wantField, rej.Field)
			require.False(t, rej.Retryable)
			require.NotEmpty(t, rej.Message)
		})
	}

	t.Run("first_violation_in_document_order", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, propsEvent(t, `{"ok":1,"zbad":null,"Abad":1}`))
		require.NotNil(t, rej)
		require.Equal(t, "events[0].properties.zbad", rej.Field)
	})
	t.Run("too_many_keys_before_key_checks", func(t *testing.T) {
		raw := strings.TrimSuffix(manyProps(50), "}") + `,"Bad":1}`
		_, rej := ingest.ValidateEvent(0, propsEvent(t, raw))
		require.NotNil(t, rej)
		require.Equal(t, "events[0].properties", rej.Field)
	})
	t.Run("absent_and_null_are_no_properties", func(t *testing.T) {
		for _, raw := range []json.RawMessage{
			eventJSON(t, map[string]any{"name": "purchase"}),
			eventJSON(t, map[string]any{"name": "purchase", "properties": nil}),
		} {
			ev, rej := ingest.ValidateEvent(0, raw)
			require.Nil(t, rej)
			require.Empty(t, ev.Properties)
		}
	})
	t.Run("message_has_no_values", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, propsEvent(t, `{"a":"`+strings.Repeat("secret", 60)+`"}`))
		require.NotNil(t, rej)
		require.NotContains(t, rej.Message, "secret")
	})
	t.Run("system_events_keep_properties", func(t *testing.T) {
		ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"properties": map[string]any{"a": 1}}))
		require.Nil(t, rej)
		require.Equal(t, map[string]string{"a": "1"}, ev.Properties)
	})
}

// AC-003.3: значения хранятся строками.
func TestNormalizeProperties(t *testing.T) {
	ev, rej := ingest.ValidateEvent(0, propsEvent(t,
		`{"seconds": 12, "ratio": 0.5, "big": 1e21, "ok": true, "no": false, "s": " a ", "neg0": -0, "small": 0.001, "one": 1.0, "exp": 1.5e2}`))
	require.Nil(t, rej)
	require.Equal(t, map[string]string{
		"seconds": "12", "ratio": "0.5", "big": "1000000000000000000000", "ok": "true", "no": "false",
		"s": " a ", "neg0": "0", "small": "0.001", "one": "1", "exp": "150",
	}, ev.Properties)
}

func TestNormalizeProperties_DuplicateKeyLastWins(t *testing.T) {
	ev, rej := ingest.ValidateEvent(0, propsEvent(t, `{"a":1,"a":2}`))
	require.Nil(t, rej)
	require.Equal(t, map[string]string{"a": "2"}, ev.Properties)
}

// AC-003.4: строки проверяются PII-guard, числа — никогда.
func TestValidateProperties_PII(t *testing.T) {
	for _, value := range []string{"user@example.com", "+381641234567", "(064) 123-4567"} {
		t.Run(value, func(t *testing.T) {
			ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{
				"name": "purchase", "properties": map[string]any{"plan": "pro", "contact": value},
			}))
			require.Nil(t, ev)
			require.NotNil(t, rej)
			require.Equal(t, ingestapi.RejectionCodePropertyLooksLikePii, rej.Code)
			require.Equal(t, "events[0].properties.contact", rej.Field)
			require.False(t, rej.Retryable)
			require.NotContains(t, rej.Message, value)
		})
	}
	t.Run("json_number_is_not_pii", func(t *testing.T) {
		ev, rej := ingest.ValidateEvent(0, propsEvent(t, `{"contact":381641234567}`))
		require.Nil(t, rej)
		require.Equal(t, map[string]string{"contact": "381641234567"}, ev.Properties)
	})
	t.Run("digit_string_is_not_pii", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, propsEvent(t, `{"contact":"381641234567"}`))
		require.Nil(t, rej)
	})
	t.Run("length_checked_before_pii", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, propsEvent(t, `{"a":"`+strings.Repeat("x", 257)+`@b.co"}`))
		require.NotNil(t, rej)
		require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)
	})
}

var propertyKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// FuzzDecodeProperties — разбор свойств не паникует, а принятый результат подчиняется всем лимитам.
func FuzzDecodeProperties(f *testing.F) {
	for _, seed := range []string{
		`{}`, `{"a":1}`, `{"a":"x"}`, `{"a":true}`, `{"a":null}`, `{"a":[1]}`, `{"Bad":1}`, `[]`, `"x"`, `5`,
		`{"a":"user@example.com"}`, `{"a":"+381641234567"}`, `{"a":"(064) 123-4567"}`, `{"a":"x\u0000"}`,
		`{"a":1e999}`, `{"a":1e21,"b":-0,"c":0.001}`, `{"a":1,"a":2}`, `{"":1}`, "{\"a\":\"\xff\"}",
		`{"a":{"b":{"c":[{"d":1}]}}}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if !json.Valid(raw) {
			t.Skip()
		}
		props, rej := ingest.DecodeProperties(0, "id", json.RawMessage(raw))
		if rej != nil {
			require.Nil(t, props)
			require.True(t, rej.Code == ingestapi.RejectionCodeInvalidField || rej.Code == ingestapi.RejectionCodePropertyLooksLikePii)
			require.True(t, rej.Field == "events[0].properties" || strings.HasPrefix(rej.Field, "events[0].properties."))
			require.False(t, rej.Retryable)
			return
		}
		require.LessOrEqual(t, len(props), 50)
		for k, v := range props {
			require.Regexp(t, propertyKeyRe, k)
			require.True(t, utf8.ValidString(v), "значение %q", v)
			if !strings.ContainsAny(v, "0123456789") || utf8.RuneCountInString(v) <= 256 {
				require.NotContains(t, v, "\x00")
			}
		}
	})
}
