package httpx

import (
	"context"
	"sync"
	"time"
)

// readyCheckTimeout ограничивает каждую проверку /ready, чтобы зависшая зависимость не держала запрос.
const readyCheckTimeout = 2 * time.Second

// ReadyCheck — проверка зависимости для /ready: nil — ok, ошибка — fail.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// RunReadyChecks выполняет проверки параллельно, каждую не дольше readyCheckTimeout.
// results — ok по имени проверки; ready — все проверки прошли.
func RunReadyChecks(ctx context.Context, checks []ReadyCheck) (ready bool, results map[string]bool) {
	ctx, cancel := context.WithTimeout(ctx, readyCheckTimeout)
	defer cancel()

	ok := make([]bool, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Go(func() { ok[i] = c.Check(ctx) == nil })
	}
	wg.Wait()

	ready = true
	results = make(map[string]bool, len(checks))
	for i, c := range checks {
		results[c.Name] = ok[i]
		ready = ready && ok[i]
	}
	return ready, results
}
