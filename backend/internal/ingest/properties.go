package ingest

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"appmetrica.online/backend/internal/gen/ingestapi"
)

const (
	// MaxProperties — максимум ключей в properties события.
	MaxProperties = 50
	// MaxPropertyKeyLength — максимум символов ключа свойства.
	MaxPropertyKeyLength = 64
	// MaxPropertyValueLength — максимум символов (рун) строкового значения свойства.
	MaxPropertyValueLength = 256
)

// propertyKeyRe — шаблон ключа свойства; в контракте он описан текстом, потому что в OpenAPI 3.0 нет propertyNames.
var propertyKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// propertyPair — ключ и сырое значение в порядке документа: первое нарушение определяется порядком
// следования, а не случайным обходом map.
type propertyPair struct {
	key string
	raw json.RawMessage
}

// DecodeProperties разбирает properties события (FR-102 §1.2) в строки для events.properties. null и
// отсутствие поля — свойств нет (nil), как у context. Проверки по порядку: объект → число ключей → для каждого
// ключа в порядке документа его шаблон, затем значение (тип, длина и U+0000, PII-guard); первое нарушение
// отклоняет событие целиком. Число приводится к кратчайшей десятичной записи (-0 → "0"), boolean — к
// "true"/"false", строка хранится как есть. Числа PII-guard не проверяет. Повтор ключа: побеждает последний.
// Значения в текст отказа не попадают.
func DecodeProperties(index int, insertID string, raw json.RawMessage) (map[string]string, *ingestapi.Rejection) {
	if isNull(raw) {
		return nil, nil
	}
	pairs, ok := splitProperties(raw)
	if !ok {
		return nil, reject(index, insertID, ingestapi.RejectionCodeInvalidField, "properties",
			"properties должен быть JSON-объектом.")
	}
	if len(pairs) > MaxProperties {
		return nil, reject(index, insertID, ingestapi.RejectionCodeInvalidField, "properties",
			"properties содержит больше 50 ключей.")
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	props := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if !propertyKeyRe.MatchString(p.key) {
			return nil, reject(index, insertID, ingestapi.RejectionCodeInvalidField, "properties."+truncateRunes(p.key, MaxPropertyKeyLength),
				"Ключ свойства должен соответствовать ^[a-z][a-z0-9_]{0,63}$.")
		}
		value, rej := propertyValue(index, insertID, p)
		if rej != nil {
			return nil, rej
		}
		props[p.key] = value
	}
	return props, nil
}

// splitProperties читает объект потоком; ok=false — raw не объект JSON.
func splitProperties(raw json.RawMessage) (pairs []propertyPair, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	for dec.More() {
		tok, err := dec.Token()
		key, isString := tok.(string)
		if err != nil || !isString {
			return nil, false
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, false
		}
		pairs = append(pairs, propertyPair{key: key, raw: value})
	}
	return pairs, true
}

// propertyValue приводит значение свойства к строке или возвращает отказ.
func propertyValue(index int, insertID string, p propertyPair) (string, *ingestapi.Rejection) {
	field := "properties." + p.key
	invalid := func(message string) (string, *ingestapi.Rejection) {
		return "", reject(index, insertID, ingestapi.RejectionCodeInvalidField, field, message)
	}
	switch first := p.raw[0]; {
	case first == '"':
		var s string
		if json.Unmarshal(p.raw, &s) != nil {
			return invalid(field + " должен быть строкой, числом или boolean.")
		}
		switch {
		case utf8.RuneCountInString(s) > MaxPropertyValueLength:
			return invalid(field + " длиннее 256 символов.")
		case strings.ContainsRune(s, 0):
			return invalid(field + " содержит символ U+0000.")
		case LooksLikePII(s):
			return "", reject(index, insertID, ingestapi.RejectionCodePropertyLooksLikePii, field,
				field+" похоже на email или телефон. Не передавайте персональные данные в свойствах.")
		}
		return s, nil
	case first == 't':
		return "true", nil
	case first == 'f':
		return "false", nil
	case first == '-' || (first >= '0' && first <= '9'):
		v, err := strconv.ParseFloat(string(p.raw), 64)
		if err != nil || math.IsInf(v, 0) {
			return invalid(field + ": число вне допустимого диапазона.")
		}
		if v == 0 {
			return "0", nil // -0 и 0 — одно значение для разбивки
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default: // null, массив, объект
		return invalid(field + " должен быть строкой, числом или boolean.")
	}
}

// truncateRunes обрезает s до n символов.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
