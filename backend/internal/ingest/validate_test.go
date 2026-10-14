package ingest_test

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/ingest"
)

const (
	validInsertID  = "0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61"
	validInstallID = "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21"
	validSessionID = "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f"
	nilUUID        = "00000000-0000-0000-0000-000000000000"
)

// absent — значение-маркер для eventJSON: поле удаляется из события.
type absentField struct{}

var absent absentField

// eventJSON — валидный session_start из I-01 с перекрытыми полями; absent удаляет поле.
func eventJSON(t *testing.T, overrides map[string]any) json.RawMessage {
	t.Helper()
	ev := map[string]any{
		"insert_id":  validInsertID,
		"name":       "session_start",
		"timestamp":  "2026-10-13T09:11:58.004Z",
		"install_id": validInstallID,
		"session_id": validSessionID,
		"user_id":    "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
	}
	maps.Copy(ev, overrides)
	for k, v := range ev {
		if v == absent {
			delete(ev, k)
		}
	}
	raw, err := json.Marshal(ev)
	require.NoError(t, err)
	return raw
}

func TestValidateEvent(t *testing.T) {
	t.Run("session_start_valid", func(t *testing.T) {
		raw := eventJSON(t, map[string]any{"context": map[string]any{"app_version": "3.5.0"}})
		ev, rej := ingest.ValidateEvent(3, raw)
		require.Nil(t, rej)
		require.Equal(t, 3, ev.Index)
		require.Equal(t, uuid.MustParse(validInsertID), ev.InsertID)
		require.Equal(t, uuid.MustParse(validInstallID), ev.InstallID)
		require.Equal(t, uuid.MustParse(validSessionID), ev.SessionID)
		require.Equal(t, ingestapi.EventNameSessionStart, ev.Name)
		require.True(t, mustTime(t, "2026-10-13T09:11:58.004Z").Equal(ev.Timestamp))
		require.Equal(t, "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90", ev.UserID)
		require.Zero(t, ev.DurationMs)
		require.Equal(t, ingest.ContextFields{"app_version": "3.5.0"}, ev.Context)
	})

	t.Run("session_end_valid", func(t *testing.T) {
		raw := eventJSON(t, map[string]any{"name": "session_end", "duration_ms": 183986, "user_id": absent})
		ev, rej := ingest.ValidateEvent(0, raw)
		require.Nil(t, rej)
		require.Equal(t, ingestapi.EventNameSessionEnd, ev.Name)
		require.Equal(t, int64(183986), ev.DurationMs)
		require.Empty(t, ev.UserID)
		require.Empty(t, ev.Context)
	})

	t.Run("duration_bounds_accepted", func(t *testing.T) {
		for _, d := range []int64{0, 86_400_000} {
			ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"name": "session_end", "duration_ms": d}))
			require.Nil(t, rej, "duration_ms=%d", d)
			require.Equal(t, d, ev.DurationMs)
		}
	})

	t.Run("timestamp_offsets_accepted", func(t *testing.T) {
		for _, ts := range []string{"2026-10-13T12:11:58+03:00", "2026-10-13T09:11:58.123456789Z", "2026-10-13T04:11:58-05:00"} {
			_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"timestamp": ts}))
			require.Nil(t, rej, ts)
		}
	})

	t.Run("uppercase_uuid_accepted", func(t *testing.T) {
		ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"install_id": strings.ToUpper(validInstallID)}))
		require.Nil(t, rej)
		require.Equal(t, uuid.MustParse(validInstallID), ev.InstallID)
	})

	t.Run("null_optional_fields_accepted", func(t *testing.T) {
		ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": nil, "duration_ms": nil, "context": nil}))
		require.Nil(t, rej)
		require.Empty(t, ev.UserID)
		require.Empty(t, ev.Context)
	})

	t.Run("unknown_fields_ignored", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"properties": map[string]any{"x": 1}}))
		require.Nil(t, rej)
	})

	t.Run("blank_user_id_is_absent", func(t *testing.T) {
		ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": "   "}))
		require.Nil(t, rej)
		require.Empty(t, ev.UserID)
	})

	t.Run("user_id_trimmed", func(t *testing.T) {
		ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": "  user_42\t"}))
		require.Nil(t, rej)
		require.Equal(t, "user_42", ev.UserID)
	})

	rejected := []struct {
		name         string
		index        int
		raw          json.RawMessage
		wantCode     ingestapi.RejectionCode
		wantField    string
		wantInsertID string // "" — ожидается null
	}{
		{
			// I-11.
			name: "nil_install_id", raw: eventJSON(t, map[string]any{"install_id": nilUUID}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].install_id", wantInsertID: validInsertID,
		},
		{
			// I-13.
			name: "end_without_duration", raw: eventJSON(t, map[string]any{"name": "session_end"}),
			wantCode: ingestapi.RejectionCodeDurationRequired, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			name: "end_with_null_duration", raw: eventJSON(t, map[string]any{"name": "session_end", "duration_ms": nil}),
			wantCode: ingestapi.RejectionCodeDurationRequired, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			// I-14.
			name: "unknown_name", raw: eventJSON(t, map[string]any{"name": "purchase"}),
			wantCode: ingestapi.RejectionCodeUnknownEventName, wantField: "events[0].name", wantInsertID: validInsertID,
		},
		{
			// I-15.
			name: "missing_session_id", raw: eventJSON(t, map[string]any{"session_id": absent}),
			wantCode: ingestapi.RejectionCodeMissingField, wantField: "events[0].session_id", wantInsertID: validInsertID,
		},
		{
			// AC-1.2.5.
			name: "timestamp_without_offset", raw: eventJSON(t, map[string]any{"timestamp": "2026-10-13T11:59:00"}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].timestamp", wantInsertID: validInsertID,
		},
		{
			// AC-1.3.4.
			name: "insert_id_not_uuid", raw: eventJSON(t, map[string]any{"insert_id": "abc"}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].insert_id",
		},
		{
			name: "missing_insert_id", index: 1, raw: eventJSON(t, map[string]any{"insert_id": absent}),
			wantCode: ingestapi.RejectionCodeMissingField, wantField: "events[1].insert_id",
		},
		{
			name: "null_insert_id", raw: eventJSON(t, map[string]any{"insert_id": nil}),
			wantCode: ingestapi.RejectionCodeMissingField, wantField: "events[0].insert_id",
		},
		{
			name: "nil_insert_id", raw: eventJSON(t, map[string]any{"insert_id": nilUUID}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].insert_id",
		},
		{
			name: "insert_id_not_string", raw: eventJSON(t, map[string]any{"insert_id": 42}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].insert_id",
		},
		{
			name: "insert_id_without_dashes", raw: eventJSON(t, map[string]any{"insert_id": strings.ReplaceAll(validInsertID, "-", "")}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].insert_id",
		},
		{
			name: "insert_id_urn_form", raw: eventJSON(t, map[string]any{"insert_id": "urn:uuid:" + validInsertID}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].insert_id",
		},
		{
			name: "insert_id_braced", raw: eventJSON(t, map[string]any{"insert_id": "{" + validInsertID[:34] + "}"}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].insert_id",
		},
		{
			name: "missing_name", raw: eventJSON(t, map[string]any{"name": absent}),
			wantCode: ingestapi.RejectionCodeMissingField, wantField: "events[0].name", wantInsertID: validInsertID,
		},
		{
			name: "name_not_string", raw: eventJSON(t, map[string]any{"name": true}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].name", wantInsertID: validInsertID,
		},
		{
			name: "name_wrong_case", raw: eventJSON(t, map[string]any{"name": "Session_Start"}),
			wantCode: ingestapi.RejectionCodeUnknownEventName, wantField: "events[0].name", wantInsertID: validInsertID,
		},
		{
			name: "missing_timestamp", raw: eventJSON(t, map[string]any{"timestamp": absent}),
			wantCode: ingestapi.RejectionCodeMissingField, wantField: "events[0].timestamp", wantInsertID: validInsertID,
		},
		{
			name: "timestamp_not_string", raw: eventJSON(t, map[string]any{"timestamp": 1760346718}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].timestamp", wantInsertID: validInsertID,
		},
		{
			name: "timestamp_date_only", raw: eventJSON(t, map[string]any{"timestamp": "2026-10-13"}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].timestamp", wantInsertID: validInsertID,
		},
		{
			name: "missing_install_id", raw: eventJSON(t, map[string]any{"install_id": absent}),
			wantCode: ingestapi.RejectionCodeMissingField, wantField: "events[0].install_id", wantInsertID: validInsertID,
		},
		{
			name: "nil_session_id", raw: eventJSON(t, map[string]any{"session_id": nilUUID}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].session_id", wantInsertID: validInsertID,
		},
		{
			name: "duration_negative", raw: eventJSON(t, map[string]any{"name": "session_end", "duration_ms": -1}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			name: "duration_over_day", raw: eventJSON(t, map[string]any{"name": "session_end", "duration_ms": 86_400_001}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			name: "duration_fractional", raw: eventJSON(t, map[string]any{"name": "session_end", "duration_ms": 1.5}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			name: "duration_string", raw: eventJSON(t, map[string]any{"name": "session_end", "duration_ms": "4000"}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			name: "duration_invalid_on_start", raw: eventJSON(t, map[string]any{"duration_ms": -5}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].duration_ms", wantInsertID: validInsertID,
		},
		{
			name: "user_id_not_string", raw: eventJSON(t, map[string]any{"user_id": 123456789}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].user_id", wantInsertID: validInsertID,
		},
		{
			name: "user_id_too_long", raw: eventJSON(t, map[string]any{"user_id": strings.Repeat("a", 129)}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].user_id", wantInsertID: validInsertID,
		},
		{
			// Ревью: NUL принимался, но deletion_requests.subject_value (Postgres text) его не хранит.
			name: "user_id_nul", raw: eventJSON(t, map[string]any{"user_id": "user\x00suffix"}),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0].user_id", wantInsertID: validInsertID,
		},
		{
			name: "user_id_email", index: 5, raw: eventJSON(t, map[string]any{"user_id": "ivan@example.com"}),
			wantCode: ingestapi.RejectionCodeUserIdLooksLikePii, wantField: "events[5].user_id", wantInsertID: validInsertID,
		},
		{
			name: "event_not_object", index: 2, raw: json.RawMessage(`"session_start"`),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[2]",
		},
		{
			name: "event_null", raw: json.RawMessage(`null`),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0]",
		},
		{
			name: "event_array", raw: json.RawMessage(`[]`),
			wantCode: ingestapi.RejectionCodeInvalidField, wantField: "events[0]",
		},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			ev, rej := ingest.ValidateEvent(tc.index, tc.raw)
			require.Nil(t, ev)
			require.NotNil(t, rej)
			require.Equal(t, tc.wantCode, rej.Code)
			require.Equal(t, tc.wantField, rej.Field)
			require.Equal(t, tc.index, rej.Index)
			require.False(t, rej.Retryable)
			require.NotEmpty(t, rej.Message)
			require.True(t, rej.InsertId.IsSpecified(), "insert_id обязателен в Rejection (null или строка)")
			if tc.wantInsertID == "" {
				require.True(t, rej.InsertId.IsNull())
			} else {
				require.Equal(t, tc.wantInsertID, rej.InsertId.MustGet())
			}
		})
	}
}

// Rejection сериализуется ровно по схеме ingest.yaml: insert_id присутствует, даже если null.
func TestValidateEvent_RejectionJSON(t *testing.T) {
	_, rej := ingest.ValidateEvent(1, eventJSON(t, map[string]any{"insert_id": absent}))
	require.NotNil(t, rej)
	out, err := json.Marshal(rej)
	require.NoError(t, err)
	require.JSONEq(t, `{"index":1,"insert_id":null,"code":"missing_field","field":"events[1].insert_id",
		"message":"Поле insert_id обязательно.","retryable":false}`, string(out))
}

// Значение user_id не попадает в текст отклонения (AC-1.5.2: PII не утекает в логи и ответы).
func TestValidateEvent_RejectionDoesNotEchoUserID(t *testing.T) {
	for _, uid := range []string{"ivan@example.com", strings.Repeat("x", 200)} {
		_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"user_id": uid}))
		require.NotNil(t, rej)
		require.NotContains(t, rej.Message, uid)
	}
}

func TestEventTimestampKeepsClientOffset(t *testing.T) {
	ev, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"timestamp": "2026-10-13T12:11:58+03:00"}))
	require.Nil(t, rej)
	_, offset := ev.Timestamp.Zone()
	require.Equal(t, 3*int(time.Hour/time.Second), offset)
}

// batchWithContext — батч для ValidateBatchContext: context (nil — поля нет) и сырые события.
func batchWithContext(t *testing.T, ctx any, events ...json.RawMessage) *ingest.Batch {
	t.Helper()
	b := &ingest.Batch{Events: events}
	if ctx != nil {
		raw, err := json.Marshal(ctx)
		require.NoError(t, err)
		b.Context = raw
	}
	return b
}

func TestValidateContext(t *testing.T) {
	t.Run("batch_platform_invalid", func(t *testing.T) {
		b := batchWithContext(t, map[string]any{"platform": "tvos", "app_version": "3.4.0"},
			eventJSON(t, nil), eventJSON(t, map[string]any{"insert_id": "not-a-uuid"}))
		fields, rejected := ingest.ValidateBatchContext(b)
		require.Nil(t, fields)
		require.Len(t, rejected, 2)
		for i, rej := range rejected {
			require.Equal(t, i, rej.Index)
			require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)
			require.Equal(t, "context.platform", rej.Field)
			require.False(t, rej.Retryable)
			require.NotContains(t, rej.Message, "tvos")
		}
		require.Equal(t, validInsertID, rejected[0].InsertId.MustGet())
		require.True(t, rejected[1].InsertId.IsNull(), "insert_id не UUID — null")
	})

	t.Run("batch_context_valid", func(t *testing.T) {
		b := batchWithContext(t, map[string]any{
			"platform": "ios", "os_name": "iOS", "os_version": "26.0.1", "app_version": "3.4.0", "app_build": "341",
			"device_model": "iPhone17,3", "device_type": "phone", "locale": "ru-RU", "environment": "development",
			"ip": "none", "unknown_field": 42, "os_name_typo": "x",
		}, eventJSON(t, nil))
		fields, rejected := ingest.ValidateBatchContext(b)
		require.Empty(t, rejected)
		require.Equal(t, ingest.ContextFields{
			"platform": "ios", "os_name": "iOS", "os_version": "26.0.1", "app_version": "3.4.0", "app_build": "341",
			"device_model": "iPhone17,3", "device_type": "phone", "locale": "ru-RU", "environment": "development",
			"ip": "none",
		}, fields, "неизвестные поля игнорируются (OQ-030)")
	})

	t.Run("batch_context_absent_or_null", func(t *testing.T) {
		for _, b := range []*ingest.Batch{
			batchWithContext(t, nil, eventJSON(t, nil)),
			{Context: json.RawMessage(`null`), Events: []json.RawMessage{eventJSON(t, nil)}},
			batchWithContext(t, map[string]any{}, eventJSON(t, nil)),
		} {
			fields, rejected := ingest.ValidateBatchContext(b)
			require.Empty(t, rejected)
			require.Empty(t, fields)
		}
	})

	t.Run("batch_null_fields_are_absent", func(t *testing.T) {
		b := batchWithContext(t, map[string]any{"platform": nil, "app_version": "3.4.0"}, eventJSON(t, nil))
		fields, rejected := ingest.ValidateBatchContext(b)
		require.Empty(t, rejected)
		require.Equal(t, ingest.ContextFields{"app_version": "3.4.0"}, fields)
	})

	t.Run("batch_context_not_object", func(t *testing.T) {
		for _, raw := range []string{`[1]`, `"ios"`, `42`, `true`} {
			b := &ingest.Batch{Context: json.RawMessage(raw), Events: []json.RawMessage{eventJSON(t, nil)}}
			_, rejected := ingest.ValidateBatchContext(b)
			require.Len(t, rejected, 1, raw)
			require.Equal(t, "context", rejected[0].Field, raw)
			require.Equal(t, ingestapi.RejectionCodeInvalidField, rejected[0].Code)
		}
	})

	t.Run("batch_rejects_every_event", func(t *testing.T) {
		events := make([]json.RawMessage, 5)
		for i := range events {
			events[i] = eventJSON(t, nil)
		}
		_, rejected := ingest.ValidateBatchContext(batchWithContext(t, map[string]any{"device_type": "fridge"}, events...))
		require.Len(t, rejected, 5)
		require.Equal(t, "context.device_type", rejected[4].Field)
		require.Equal(t, 4, rejected[4].Index)
	})

	t.Run("event_platform_invalid", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(2, eventJSON(t, map[string]any{"context": map[string]any{"platform": "tvos"}}))
		require.NotNil(t, rej)
		require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)
		require.Equal(t, "events[2].context.platform", rej.Field)
		require.Equal(t, validInsertID, rej.InsertId.MustGet())
	})

	t.Run("event_context_not_object", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"context": "ios"}))
		require.NotNil(t, rej)
		require.Equal(t, "events[0].context", rej.Field)
	})

	t.Run("event_context_checked_after_user_id", func(t *testing.T) {
		_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{
			"user_id": "ivan@example.com", "context": map[string]any{"platform": "tvos"},
		}))
		require.NotNil(t, rej)
		require.Equal(t, ingestapi.RejectionCodeUserIdLooksLikePii, rej.Code)
	})

	t.Run("field_rules", func(t *testing.T) {
		long := func(n int) string { return strings.Repeat("я", n) }
		tests := []struct {
			name  string
			field string
			value any
			ok    bool
		}{
			{"platform_each_value", "platform", "android", true},
			{"platform_case_sensitive", "platform", "iOS", false},
			{"platform_empty", "platform", "", false},
			{"platform_not_string", "platform", 1, false},
			{"device_type_valid", "device_type", "tablet", true},
			{"device_type_invalid", "device_type", "fridge", false},
			{"environment_valid", "environment", "production", true},
			{"environment_invalid", "environment", "staging", false},
			{"os_name_32_chars", "os_name", long(32), true},
			{"os_name_33_chars", "os_name", long(33), false},
			{"os_version_33_chars", "os_version", long(33), false},
			{"app_version_64_chars", "app_version", long(64), true},
			{"app_version_65_chars", "app_version", long(65), false},
			{"app_build_65_chars", "app_build", long(65), false},
			{"device_model_65_chars", "device_model", long(65), false},
			{"locale_35_chars", "locale", long(35), true},
			{"locale_36_chars", "locale", long(36), false},
			{"ip_45_chars", "ip", long(45), true},
			{"ip_46_chars", "ip", long(46), false},
			{"ip_not_string", "ip", []int{1, 2}, false},
			{"empty_string_allowed", "app_version", "", true},
			{"app_version_number", "app_version", 3.4, false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"context": map[string]any{tc.field: tc.value}}))
				if tc.ok {
					require.Nil(t, rej)
					return
				}
				require.NotNil(t, rej)
				require.Equal(t, ingestapi.RejectionCodeInvalidField, rej.Code)
				require.Equal(t, "events[0].context."+tc.field, rej.Field)
			})
		}
	})

	t.Run("all_platforms_accepted", func(t *testing.T) {
		for _, p := range []string{"ios", "android", "web", "macos", "windows", "linux", "bot", "server", "other"} {
			_, rej := ingest.ValidateEvent(0, eventJSON(t, map[string]any{"context": map[string]any{"platform": p}}))
			require.Nil(t, rej, p)
		}
	})
}
