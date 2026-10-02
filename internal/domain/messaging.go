package domain

import "time"

// MessagingProvider is a safe project provider projection. Credentials are
// encrypted at rest and represented only by the configured flag; plaintext
// values and ciphertext are never serialized.
type MessagingProvider struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"project_id"`
	Name               string    `json:"name"`
	Channel            string    `json:"channel"`
	Provider           string    `json:"provider"`
	CredentialsPresent bool      `json:"credentials_present"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// MessagingTopic is a project-scoped fan-out target. SubscriberCount is
// computed from durable subscriber rows and is safe to show in the Console.
type MessagingTopic struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Enabled         bool      `json:"enabled"`
	SubscriberCount int64     `json:"subscriber_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// MessagingSubscriber keeps the recipient secret while returning a bounded
// masked preview for operators. The full address is never returned by the API.
type MessagingSubscriber struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	TopicID        string    `json:"topic_id"`
	Channel        string    `json:"channel"`
	AddressPreview string    `json:"address_preview"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// MessagingMessage is a safe delivery projection. The message body, subject,
// and optional data are encrypted at rest and are intentionally omitted from
// API responses.
type MessagingMessage struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	TopicID        *string    `json:"topic_id"`
	Channel        string     `json:"channel"`
	Status         string     `json:"status"`
	RecipientCount int64      `json:"recipient_count"`
	SucceededCount int64      `json:"succeeded_count"`
	FailedCount    int64      `json:"failed_count"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// MessagingDelivery is a safe per-recipient status projection. The recipient
// address is represented only by the masked preview captured at enqueue time.
type MessagingDelivery struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	MessageID      string     `json:"message_id"`
	SubscriberID   *string    `json:"subscriber_id,omitempty"`
	ProviderID     *string    `json:"provider_id,omitempty"`
	Channel        string     `json:"channel"`
	AddressPreview string     `json:"address_preview"`
	Status         string     `json:"status"`
	AttemptCount   int        `json:"attempt_count"`
	LastStatusCode *int       `json:"last_status_code,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
