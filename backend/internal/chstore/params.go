package chstore

import (
	"strings"

	"github.com/google/uuid"
)

// Значения параметров запросов {name:String} и {name:Array(String)}. clickhouse-go передаёт их текстом, который
// сервер разбирает в формате Escaped (у массивов — после снятия кавычек), поэтому обратный слеш и управляющие
// символы экранируются здесь: иначе `\x` в user_id превратился бы в другой символ, а табуляция — в ошибку разбора.

var (
	textEscaper   = strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`, "\x00", `\0`)
	quotedEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\t", `\t`, "\n", `\n`, "\r", `\r`, "\x00", `\0`)
)

// StringParam — значение параметра String.
func StringParam(v string) string { return textEscaper.Replace(v) }

// StringArrayParam — значение параметра Array(String): ['a','b']; пустой срез — [].
func StringArrayParam(values []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('\'')
		b.WriteString(quotedEscaper.Replace(v))
		b.WriteByte('\'')
	}
	b.WriteByte(']')
	return b.String()
}

// UUIDArrayParam — значение параметра Array(UUID) из идентификаторов.
func UUIDArrayParam(ids []uuid.UUID) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	return StringArrayParam(values)
}
