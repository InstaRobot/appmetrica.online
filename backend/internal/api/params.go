package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// commonParams — общие параметры отчётов.
type commonParams struct {
	From, To    openapi_types.Date
	Granularity *gen.Granularity
	Environment *gen.Environment
	Unit        *gen.Unit
	Platform    *[]gen.Platform
	AppVersion  *[]string
	OsName      *[]string
	Country     *[]string
}

type reportParams struct {
	reports.Params
	Granularity gen.Granularity
}

type paramError struct {
	code    string
	field   string
	message string
	maxDays int
}

// parseCommonParams — TODO(T-027).
func parseCommonParams(commonParams, uuid.UUID, *time.Location) (reportParams, *paramError) {
	return reportParams{}, &paramError{}
}

func (e *paramError) response(ctx context.Context) gen.BadRequestJSONResponse {
	return gen.BadRequestJSONResponse(apiError(ctx, e.code, e.message, nil))
}
