package ingest

import (
	"encoding/json"
	"time"
)

// MaxBodyBytes — лимит тела после распаковки.
const MaxBodyBytes = 1 << 20

// Batch — разобранный батч; заглушка до реализации T-009.
type Batch struct {
	AppKey     string
	SentAt     time.Time
	Context    json.RawMessage
	Events     []json.RawMessage
	ReceivedAt time.Time
}
