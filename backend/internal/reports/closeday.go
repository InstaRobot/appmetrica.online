package reports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// CloseDay — job close_day (FR-022): 4 строки daily_stats дня day (календарная дата в TZ loc).
func (s *Service) CloseDay(ctx context.Context, appID uuid.UUID, loc *time.Location, day time.Time) error {
	return errors.New("reports: close day: not implemented")
}
