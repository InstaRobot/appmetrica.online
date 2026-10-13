package ingest

import (
	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/geo"
)

// MaxPayloadSampleBytes — лимит payload_sample.
const MaxPayloadSampleBytes = 4096

// Enricher — шаг обогащения.
type Enricher struct {
	Geo               *geo.Reader
	AcceptDevelopment bool
}

// Enrich обогащает событие.
func (e Enricher) Enrich(*Batch, ContextFields, *Event) (*ingestapi.IngestWarning, bool) {
	return nil, false
}

// NewEventRow — строка events.
func NewEventRow(*Batch, *Event) chstore.EventRow { return chstore.EventRow{} }

// PayloadSample — сэмпл для rejected_events.
func PayloadSample([]byte) string { return "" }
