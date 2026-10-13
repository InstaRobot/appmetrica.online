package auth

import (
	"context"
	"fmt"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// ResetForE2E возвращает базу к состоянию «passkey ещё не настроен»: удаляет owner (каскадно passkey и
// сессии), setup-ссылки и церемонии, и восстанавливает лимит попыток входа — иначе повторный прогон
// Playwright в ту же минуту упирается в 429. Вызывается только служебным маршрутом стека E2E (APP_ENV=e2e).
func (s *Service) ResetForE2E(ctx context.Context) error {
	err := s.store.InTx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteOwner(ctx); err != nil {
			return fmt.Errorf("delete owner: %w", err)
		}
		if err := q.DeleteSetupTokens(ctx); err != nil {
			return fmt.Errorf("delete setup tokens: %w", err)
		}
		if err := q.DeleteCeremonies(ctx); err != nil {
			return fmt.Errorf("delete ceremonies: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("auth: reset for e2e: %w", err)
	}
	s.attempts.Store(newAttemptsLimiter())
	return nil
}
