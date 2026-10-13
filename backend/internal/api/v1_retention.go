package api

import (
	"time"

	"github.com/google/uuid"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/reports"
)

// parseRetentionParams — параметры getRetention.
func parseRetentionParams(_ gen.GetRetentionParams, _ uuid.UUID, _ *time.Location) (reports.Params, reports.RetentionQuery, *paramError) {
	return reports.Params{}, reports.RetentionQuery{}, nil
}
