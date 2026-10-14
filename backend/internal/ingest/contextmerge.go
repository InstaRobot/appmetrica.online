package ingest

import (
	"encoding/json"
	"maps"
	"strconv"
	"unicode/utf8"

	"appmetrica.online/backend/internal/gen/ingestapi"
)

// ContextFields — заданные поля EventContext: имя поля из ingest.yaml → значение. Поля, которых нет
// или которые равны null, в map отсутствуют; неизвестные поля игнорируются (как в OQ-030).
type ContextFields map[string]string

// ResolvedContext — итоговые колонки контекста события в events: слияние батча и события, значения
// по умолчанию и гео. IP здесь нет намеренно: он используется только для поиска гео.
type ResolvedContext struct {
	Platform    string
	OSName      string
	OSVersion   string
	AppVersion  string
	AppBuild    string
	DeviceModel string
	DeviceType  string
	Locale      string
	Environment string
	Country     string
	Region      string
	City        string
}

// contextFieldRule — ограничения поля EventContext из ingest.yaml: maxLength в символах или enum.
type contextFieldRule struct {
	name   string
	maxLen int
	valid  func(string) bool
	enum   string // допустимые значения для текста отклонения
}

// contextRules — в порядке схемы EventContext: при нескольких нарушениях отклонение называет первое.
var contextRules = []contextFieldRule{
	{
		name:  "platform",
		valid: func(s string) bool { return ingestapi.Platform(s).Valid() },
		enum:  "ios, android, web, macos, windows, linux, bot, server, other",
	},
	{name: "os_name", maxLen: 32},
	{name: "os_version", maxLen: 32},
	{name: "app_version", maxLen: 64},
	{name: "app_build", maxLen: 64},
	{name: "device_model", maxLen: 64},
	{
		name:  "device_type",
		valid: func(s string) bool { return ingestapi.DeviceType(s).Valid() },
		enum:  "phone, tablet, desktop, tv, watch, server, unknown",
	},
	{name: "locale", maxLen: 35},
	{
		name:  "environment",
		valid: func(s string) bool { return ingestapi.Environment(s).Valid() },
		enum:  "production, development",
	},
	{name: "ip", maxLen: 45},
}

// contextError — нарушение в context: field — "context" (не объект) или "context.<поле>".
type contextError struct {
	field   string
	message string
}

// parseContext разбирает context батча или события; raw без значения или null — пустой контекст.
// Значения полей в текст ошибки не попадают (context.ip — IP пользователя).
func parseContext(raw json.RawMessage) (ContextFields, *contextError) {
	if isNull(raw) {
		return nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, &contextError{field: "context", message: "context должен быть JSON-объектом."}
	}
	var fields ContextFields
	for _, rule := range contextRules {
		s, present, ok := stringField(obj[rule.name])
		if !present {
			continue
		}
		field := "context." + rule.name
		switch {
		case !ok:
			return nil, &contextError{field: field, message: field + " должен быть строкой."}
		case rule.valid != nil && !rule.valid(s):
			return nil, &contextError{field: field, message: field + ": допустимые значения " + rule.enum + "."}
		case rule.maxLen > 0 && utf8.RuneCountInString(s) > rule.maxLen:
			return nil, &contextError{field: field, message: field + " длиннее " + strconv.Itoa(rule.maxLen) + " символов."}
		}
		if fields == nil {
			fields = ContextFields{}
		}
		fields[rule.name] = s
	}
	return fields, nil
}

// ValidateBatchContext разбирает context батча. Нарушение отклоняет все события батча одинаковой причиной
// с field "context.<поле>" — по записи rejected[] на событие (02-tech-spec.md Story 1.4).
func ValidateBatchContext(b *Batch) (ContextFields, []ingestapi.Rejection) {
	fields, cerr := parseContext(b.Context)
	if cerr == nil {
		return fields, nil
	}
	rejected := make([]ingestapi.Rejection, len(b.Events))
	for i, raw := range b.Events {
		rejected[i] = *rejectPath(i, peekInsertID(raw), ingestapi.RejectionCodeInvalidField, cerr.field, cerr.message)
	}
	return nil, rejected
}

// peekInsertID — insert_id события для отклонения, если это валидный UUID, иначе "" (null).
func peekInsertID(raw json.RawMessage) string {
	var r struct {
		InsertID json.RawMessage `json:"insert_id"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	if id, rej := uuidField(0, "", "insert_id", r.InsertID); rej == nil {
		return id.String()
	}
	return ""
}

// MergeContext сливает context батча и события: поле события перекрывает поле батча, включая пустую строку.
// Аргументы не меняются.
func MergeContext(batch, event ContextFields) ContextFields {
	merged := make(ContextFields, len(batch)+len(event))
	maps.Copy(merged, batch)
	maps.Copy(merged, event)
	return merged
}

// resolveContext — итоговые колонки без гео; нет environment → production. Платформа (FR-3): context.platform →
// платформа ключа → единственная платформа приложения → other, и тогда missing=true (предупреждение
// platform_missing). context.platform с набором приложения не сверяется.
func resolveContext(merged ContextFields, keyPlatform string, appPlatforms []string) (rc ResolvedContext, missing bool) {
	rc = ResolvedContext{
		Platform:    merged["platform"],
		OSName:      merged["os_name"],
		OSVersion:   merged["os_version"],
		AppVersion:  merged["app_version"],
		AppBuild:    merged["app_build"],
		DeviceModel: merged["device_model"],
		DeviceType:  merged["device_type"],
		Locale:      merged["locale"],
		Environment: merged["environment"],
	}
	switch {
	case rc.Platform != "":
	case keyPlatform != "":
		rc.Platform = keyPlatform
	case len(appPlatforms) == 1:
		rc.Platform = appPlatforms[0]
	default:
		rc.Platform, missing = string(ingestapi.PlatformOther), true
	}
	if rc.Environment == "" {
		rc.Environment = string(ingestapi.EnvironmentProduction)
	}
	return rc, missing
}
