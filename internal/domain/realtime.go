package domain

import (
	"encoding/json"
	"time"
)

// RealtimeEvent is the short-lived project event envelope consumed by the
// Realtime SSE transport. Payload contains the exact JSON envelope persisted
// in the transactional outbox and is kept out of ordinary JSON projections so
// handlers can stream it without re-encoding or changing signatures.
type RealtimeEvent struct {
	ID             string
	OrganizationID string
	ProjectID      string
	EventName      string
	Version        int
	TargetType     string
	TargetID       *string
	ResourceID     *string
	CorrelationID  string
	Data           map[string]any
	OccurredAt     time.Time
	CreatedAt      time.Time
	Payload        json.RawMessage
}
