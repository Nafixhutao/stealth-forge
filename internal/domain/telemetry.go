package domain

import (
	"encoding/json"
	"time"
)

// HTTPTrace is the durable root-request index shown in the operator Console.
// Nested spans and attributes remain in the private OpenTelemetry backend;
// this projection contains only bounded tenant-safe request metadata.
type HTTPTrace struct {
	ID               string    `json:"id"`
	TraceID          string    `json:"trace_id"`
	SpanID           *string   `json:"span_id,omitempty"`
	OrganizationID   *string   `json:"organization_id,omitempty"`
	ProjectID        *string   `json:"project_id,omitempty"`
	OrganizationName string    `json:"organization_name,omitempty"`
	ProjectName      string    `json:"project_name,omitempty"`
	Service          string    `json:"service"`
	Method           string    `json:"method"`
	Route            string    `json:"route"`
	Status           int       `json:"status"`
	DurationMS       int64     `json:"duration_ms"`
	ResponseBytes    int64     `json:"response_bytes"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// AuditEvent is the durable, tenant-scoped activity record emitted by
// control-plane mutations. Actor and target IDs are nullable because account
// cleanup and system workers may leave an event without a live row.
type AuditEvent struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	ActorAccountID *string         `json:"actor_account_id,omitempty"`
	ActorEmail     *string         `json:"actor_email,omitempty"`
	Action         string          `json:"action"`
	TargetType     string          `json:"target_type"`
	TargetID       *string         `json:"target_id,omitempty"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
}
