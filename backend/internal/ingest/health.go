package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"appmetrica.online/backend/internal/gen/ingestapi"
)

// readyCheckTimeout ограничивает каждую проверку /ready, чтобы зависшая зависимость не держала запрос.
const readyCheckTimeout = 2 * time.Second

// ReadyCheck — проверка зависимости для /ready: nil — ok, ошибка — fail.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// NewReadyHandler возвращает GET /ready по схеме ReadyStatus из ingest.yaml: 200 ready, если все
// проверки прошли, иначе 503 not_ready. Проверки выполняются параллельно.
func NewReadyHandler(checks ...ReadyCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyCheckTimeout)
		defer cancel()

		results := make([]ingestapi.ReadyStatusChecks, len(checks))
		var wg sync.WaitGroup
		for i, c := range checks {
			wg.Go(func() {
				results[i] = ingestapi.ReadyStatusChecksOk
				if err := c.Check(ctx); err != nil {
					results[i] = ingestapi.ReadyStatusChecksFail
				}
			})
		}
		wg.Wait()

		st := ingestapi.ReadyStatus{Status: ingestapi.ReadyStatusStatusReady, Checks: make(map[string]ingestapi.ReadyStatusChecks, len(checks))}
		status := http.StatusOK
		for i, c := range checks {
			st.Checks[c.Name] = results[i]
			if results[i] != ingestapi.ReadyStatusChecksOk {
				st.Status = ingestapi.ReadyStatusStatusNotReady
				status = http.StatusServiceUnavailable
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(st)
	})
}
