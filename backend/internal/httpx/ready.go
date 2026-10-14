package httpx

import "context"

// ReadyCheck — проверка зависимости для /ready: nil — ok, ошибка — fail.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// RunReadyChecks — заглушка до реализации (T-034).
func RunReadyChecks(_ context.Context, _ []ReadyCheck) (bool, map[string]bool) {
	return false, nil
}
