package reports

import "github.com/ClickHouse/clickhouse-go/v2"

// Filters — фильтры отчёта.
type Filters struct {
	Platform   []string
	AppVersion []string
	OSName     []string
	Country    []string
}

func (f Filters) bind(clickhouse.Parameters) {}
