package repository

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrMessagingNoRecipients        = errors.New("messaging topic has no active recipients")
	ErrMessagingProviderUnavailable = errors.New("messaging provider is unavailable")
	ErrMessagingTooManyRecipients   = errors.New("messaging topic has too many recipients")
	ErrMessagingMessageTerminal     = errors.New("messaging message is already terminal")
	ErrNoMessagingDelivery          = errors.New("no messaging delivery available")
	ErrMessagingDeliveryNotFound    = errors.New("messaging delivery was not found")
)

const (
	maxMessagingMessageSubject     = 998
	maxMessagingMessageBody        = 64 << 10
	maxMessagingMessageDataKeys    = 32
	maxMessagingMessageDataValue   = 2048
	maxMessagingMessageDataBytes   = 16 << 10
	maxMessagingMessageRecipients  = 10000
	maxMessagingMessageIdempotency = 128
)

type MessagingMessageInput struct {
	TopicID        uuid.UUID
	Channel        string
	Subject        string
	Body           string
	Data           map[string]string
	IdempotencyKey string
}

// MessagingMessagePayload is encrypted as one unit. Keeping the complete
// content out of domain.MessagingMessage prevents a handler from accidentally
// returning message text or push data to a management API caller.
type MessagingMessagePayload struct {
	Subject string            `json:"subject,omitempty"`
	Body    string            `json:"body"`
	Data    map[string]string `json:"data,omitempty"`
}

type MessagingMessageCreateResult struct {
	Message domain.MessagingMessage
	Created bool
}

// MessagingDeliveryJob contains only the ciphertext needed by a trusted
// worker. The worker owns decryption and provider network calls; this type is
// never returned by an HTTP handler.
type MessagingDeliveryJob struct {
	DeliveryID                    uuid.UUID
	MessageID                     uuid.UUID
	ProjectID                     uuid.UUID
	SubscriberID                  *uuid.UUID
	ProviderID                    *uuid.UUID
	Channel                       string
	AddressPreview                string
	AddressCiphertext             []byte
	PayloadCiphertext             []byte
	Provider                      *string
	ProviderEnabled               *bool
	ProviderCredentialsCiphertext []byte
	AttemptCount                  int
}

const messagingMessageProjection = `id,project_id,topic_id,channel,status,recipient_count,succeeded_count,failed_count,cancelled_at,created_at,updated_at`
const messagingDeliveryProjection = `id,project_id,message_id,subscriber_id,provider_id,channel,address_preview,status,attempt_count,last_status_code,last_error,delivered_at,created_at,updated_at`

type messagingMessageScanner interface {
	Scan(dest ...any) error
}

type messagingDeliveryScanner interface {
	Scan(dest ...any) error
}

func scanMessagingMessage(row messagingMessageScanner) (domain.MessagingMessage, error) {
	var item domain.MessagingMessage
	var id, projectID uuid.UUID
	var topicID *uuid.UUID
	err := row.Scan(&id, &projectID, &topicID, &item.Channel, &item.Status, &item.RecipientCount, &item.SucceededCount, &item.FailedCount, &item.CancelledAt, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	if topicID != nil {
		value := topicID.String()
		item.TopicID = &value
	}
	return item, err
}

func scanMessagingDelivery(row messagingDeliveryScanner) (domain.MessagingDelivery, error) {
	var item domain.MessagingDelivery
	var id, projectID, messageID uuid.UUID
	var subscriberID, providerID *uuid.UUID
	err := row.Scan(&id, &projectID, &messageID, &subscriberID, &providerID, &item.Channel, &item.AddressPreview, &item.Status, &item.AttemptCount, &item.LastStatusCode, &item.LastError, &item.DeliveredAt, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	item.MessageID = messageID.String()
	if subscriberID != nil {
		value := subscriberID.String()
		item.SubscriberID = &value
	}
	if providerID != nil {
		value := providerID.String()
		item.ProviderID = &value
	}
	return item, err
}

func normalizeMessagingMessageInput(input MessagingMessageInput) (string, MessagingMessagePayload, []byte, error) {
	channel, err := normalizeMessagingChannel(input.Channel)
	if err != nil {
		return "", MessagingMessagePayload{}, nil, err
	}
	subject := strings.TrimSpace(input.Subject)
	if len(subject) > maxMessagingMessageSubject || strings.ContainsAny(subject, "\x00\r\n") {
		return "", MessagingMessagePayload{}, nil, fmt.Errorf("%w: subject is invalid", ErrInvalidMessaging)
	}
	body := input.Body
	if len(body) == 0 || len(body) > maxMessagingMessageBody || strings.ContainsRune(body, '\x00') || strings.TrimSpace(body) == "" {
		return "", MessagingMessagePayload{}, nil, fmt.Errorf("%w: body is invalid", ErrInvalidMessaging)
	}
	if channel == "email" && subject == "" {
		return "", MessagingMessagePayload{}, nil, fmt.Errorf("%w: email messages require a subject", ErrInvalidMessaging)
	}
	data, err := normalizeMessagingMessageData(input.Data)
	if err != nil {
		return "", MessagingMessagePayload{}, nil, err
	}
	payload := MessagingMessagePayload{Subject: subject, Body: body, Data: data}
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > maxMessagingMessageBody+maxMessagingMessageDataBytes {
		return "", MessagingMessagePayload{}, nil, fmt.Errorf("%w: message payload is too large", ErrInvalidMessaging)
	}
	hashInput, err := json.Marshal(struct {
		TopicID string          `json:"topic_id"`
		Channel string          `json:"channel"`
		Payload json.RawMessage `json:"payload"`
	}{TopicID: input.TopicID.String(), Channel: channel, Payload: encoded})
	if err != nil {
		return "", MessagingMessagePayload{}, nil, fmt.Errorf("%w: message request could not be hashed", ErrInvalidMessaging)
	}
	hash := sha256.Sum256(hashInput)
	return channel, payload, hash[:], nil
}

func normalizeMessagingMessageData(raw map[string]string) (map[string]string, error) {
	if len(raw) > maxMessagingMessageDataKeys {
		return nil, fmt.Errorf("%w: at most %d data fields are allowed", ErrInvalidMessaging, maxMessagingMessageDataKeys)
	}
	data := make(map[string]string, len(raw))
	for key, value := range raw {
		if !messagingCredentialKeyPattern.MatchString(key) || len(value) > maxMessagingMessageDataValue || strings.ContainsAny(value, "\x00\r\n") {
			return nil, fmt.Errorf("%w: message data fields are invalid", ErrInvalidMessaging)
		}
		data[key] = value
	}
	encoded, err := json.Marshal(data)
	if err != nil || len(encoded) > maxMessagingMessageDataBytes {
		return nil, fmt.Errorf("%w: message data is too large", ErrInvalidMessaging)
	}
	return data, nil
}

func normalizeMessagingIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxMessagingMessageIdempotency || strings.ContainsAny(value, "\x00\r\n\t") {
		return "", fmt.Errorf("%w: idempotency key is invalid", ErrInvalidMessaging)
	}
	return value, nil
}
