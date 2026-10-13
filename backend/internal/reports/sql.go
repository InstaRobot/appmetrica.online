// Package reports — канонические запросы отчётов ClickHouse (05-data-model.md §10.4) и их параметры.
package reports

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/identity"
)

const (
	EnvProduction  = "production"
	EnvDevelopment = "development"
)

// TODO(T-027): фрагменты.
const (
	eventsFilter = ``
	evCTE        = ``
)

// Period — календарные даты from..to включительно в TZ приложения.
type Period struct {
	From, To time.Time
}

func (p Period) Days() int { return 0 }

func (p Period) Bounds(*time.Location) (start, end time.Time) { return time.Time{}, time.Time{} }

// Params — общие параметры отчёта.
type Params struct {
	AppID    uuid.UUID
	Location *time.Location
	Env      string
	Unit     identity.Unit
	Period   Period
	Filters  Filters
}

func (p Params) Parameters() clickhouse.Parameters { return nil }

func (p Params) Bind(ctx context.Context) context.Context { return ctx }
