package domain

import "time"

// Webhook contains delivery configuration without secret material. The
// plaintext signing secret is returned only by create/rotate responses.
type Webhook struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Events         []string   `json:"events"`
	Enabled        bool       `json:"enabled"`
	FailureCount   int        `json:"failure_count"`
	LastDeliveryAt *time.Time `json:"last_delivery_at,omitempty"`
	LastFailureAt  *time.Time `json:"last_failure_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type WebhookDelivery struct {
	ID             string     `json:"id"`
	WebhookID      string     `json:"webhook_id"`
	EventID        string     `json:"event_id"`
	EventName      string     `json:"event_name"`
	Status         string     `json:"status"`
	AttemptCount   int        `json:"attempt_count"`
	LastStatusCode *int       `json:"last_status_code,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
