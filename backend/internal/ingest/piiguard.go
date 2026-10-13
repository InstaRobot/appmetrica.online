package ingest

import (
	"regexp"
	"strings"
)

// Регулярки PII-guard из 02-tech-spec.md Story 1.5; компилируются один раз.
var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	e164Re  = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
)

// LooksLikePII сообщает, похож ли user_id на email или телефон (после strings.TrimSpace).
// Чисто цифровые строки (ID из БД, Telegram ID) не считаются PII.
func LooksLikePII(s string) bool {
	s = strings.TrimSpace(s)
	return emailRe.MatchString(s) || e164Re.MatchString(s) || isFormattedPhone(s)
}

// isFormattedPhone: только цифры и разделители «(», «)», «-», пробел с необязательным ведущим «+»,
// хотя бы один разделитель и от 10 до 15 цифр.
func isFormattedPhone(s string) bool {
	digits, separators := 0, 0
	for i := range len(s) {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			digits++
		case c == '(' || c == ')' || c == '-' || c == ' ':
			separators++
		case c == '+' && i == 0:
		default:
			return false
		}
	}
	return separators > 0 && digits >= 10 && digits <= 15
}
