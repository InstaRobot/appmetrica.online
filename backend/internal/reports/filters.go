package reports

import (
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Filters — фильтры отчёта: значения внутри поля объединяются OR, поля между собой — AND, пустое поле
// фильтр не включает.
type Filters struct {
	Platform   []string
	AppVersion []string
	OSName     []string
	Country    []string
}

// bind записывает фильтры в параметры {f_platform}, {f_app_version}, {f_os_name}, {f_country}
// (Array(String); пустой массив выключает фильтр в eventsFilter).
func (f Filters) bind(params clickhouse.Parameters) {
	params["f_platform"] = arrayParam(f.Platform)
	params["f_app_version"] = arrayParam(f.AppVersion)
	params["f_os_name"] = arrayParam(f.OSName)
	params["f_country"] = arrayParam(f.Country)
}

// arrayEscaper — экранирование строки внутри кавычек текстового формата ClickHouse.
var arrayEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\t", `\t`, "\n", `\n`, "\r", `\r`, "\x00", `\0`)

// arrayParam — значение параметра Array(String) в текстовом формате ClickHouse: ['a','b'].
// Это значение параметра, а не текст запроса: сервер разбирает его как данные типа Array(String).
func arrayParam(values []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('\'')
		b.WriteString(arrayEscaper.Replace(v))
		b.WriteByte('\'')
	}
	b.WriteByte(']')
	return b.String()
}
