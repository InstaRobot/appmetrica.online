package ingest

import (
	"encoding/json"
	"net/http"

	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/httpx"
)

// ReadyCheck — проверка зависимости для /ready: nil — ok, ошибка — fail.
type ReadyCheck = httpx.ReadyCheck

// NewReadyHandler возвращает GET /ready по схеме ReadyStatus из ingest.yaml: 200 ready, если все
// проверки прошли, иначе 503 not_ready. Проверки выполняются параллельно.
func NewReadyHandler(checks ...ReadyCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ready, results := httpx.RunReadyChecks(r.Context(), checks)
		st := ingestapi.ReadyStatus{Status: ingestapi.ReadyStatusStatusReady, Checks: make(map[string]ingestapi.ReadyStatusChecks, len(results))}
		status := http.StatusOK
		if !ready {
			st.Status = ingestapi.ReadyStatusStatusNotReady
			status = http.StatusServiceUnavailable
		}
		for name, ok := range results {
			st.Checks[name] = ingestapi.ReadyStatusChecksFail
			if ok {
				st.Checks[name] = ingestapi.ReadyStatusChecksOk
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(st)
	})
}
