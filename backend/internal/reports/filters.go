package reports

import (
	"github.com/ClickHouse/clickhouse-go/v2"

	"appmetrica.online/backend/internal/chstore"
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

// arrayParam — значение параметра Array(String): сервер разбирает его как данные, а не как текст запроса.
func arrayParam(values []string) string { return chstore.StringArrayParam(values) }
