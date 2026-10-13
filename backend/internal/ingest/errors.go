package ingest

// Error — ошибка уровня батча; заглушка до реализации T-009.
type Error struct {
	Status     int
	Code       string
	Message    string
	Details    map[string]any
	RetryAfter int
}

func (e *Error) Error() string { return e.Code }
