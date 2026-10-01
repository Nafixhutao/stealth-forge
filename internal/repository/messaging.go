package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"sort"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MessagingActor deliberately reuses the management actor shape used by the
// other project control planes. Application sessions can consume application
// data, but provider and subscriber configuration is a management operation.
type MessagingActor = DatabaseActor

const (
	MessagingConsoleActor = DatabaseConsoleActor
	MessagingAPIKeyActor  = DatabaseAPIKeyActor
)

var (
	ErrMessagingNotReady       = errors.New("messaging encryption is not ready")
	ErrInvalidMessaging        = errors.New("invalid messaging configuration")
	ErrMessagingAddressInvalid = errors.New("invalid messaging subscriber address")
)

const (
	maxMessagingCredentialKeys  = 32
	maxMessagingCredentialBytes = 16 << 10
	maxMessagingSubscriberBytes = 2048
)

var messagingCredentialKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)
var messagingSMSAddressPattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

type MessagingProviderInput struct {
	Name        string
	Channel     string
	Provider    string
	Credentials map[string]string
	Enabled     bool
}

type MessagingProviderPatch struct {
	Name        *string
	Channel     *string
	Provider    *string
	Credentials *map[string]string
	Enabled     *bool
}

type MessagingTopicInput struct {
	Name        string
	Description string
	Enabled     bool
}

type MessagingTopicPatch struct {
	Name        *string
	Description *string
	Enabled     *bool
}

type MessagingSubscriberInput struct {
	Channel string
	Address string
	Enabled bool
}

type messagingProviderScanner interface {
	Scan(dest ...any) error
}

type messagingTopicScanner interface {
	Scan(dest ...any) error
}

type messagingSubscriberScanner interface {
	Scan(dest ...any) error
}

const messagingProviderProjection = `id,project_id,name,channel,provider,credentials_present,enabled,created_at,updated_at`
const messagingTopicProjection = `id,project_id,name,description,enabled,(SELECT count(*) FROM project_messaging_subscribers s WHERE s.topic_id=project_messaging_topics.id AND s.enabled),created_at,updated_at`
const messagingTopicListProjection = `t.id,t.project_id,t.name,t.description,t.enabled,(SELECT count(*) FROM project_messaging_subscribers s WHERE s.topic_id=t.id AND s.enabled),t.created_at,t.updated_at`
const messagingSubscriberProjection = `id,project_id,topic_id,channel,address_preview,enabled,created_at,updated_at`

func scanMessagingProvider(row messagingProviderScanner) (domain.MessagingProvider, error) {
	var item domain.MessagingProvider
	var id, projectID uuid.UUID
	err := row.Scan(&id, &projectID, &item.Name, &item.Channel, &item.Provider, &item.CredentialsPresent, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	return item, err
}

func scanMessagingTopic(row messagingTopicScanner) (domain.MessagingTopic, error) {
	var item domain.MessagingTopic
	var id, projectID uuid.UUID
	err := row.Scan(&id, &projectID, &item.Name, &item.Description, &item.Enabled, &item.SubscriberCount, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	return item, err
}

func scanMessagingSubscriber(row messagingSubscriberScanner) (domain.MessagingSubscriber, error) {
	var item domain.MessagingSubscriber
	var id, projectID, topicID uuid.UUID
	err := row.Scan(&id, &projectID, &topicID, &item.Channel, &item.AddressPreview, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	item.TopicID = topicID.String()
	return item, err
}

func normalizeMessagingName(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || len(value) > 120 || strings.ContainsAny(value, "\x00\r\n\t") {
		return "", fmt.Errorf("%w: %s is invalid", ErrInvalidMessaging, field)
	}
	return value, nil
}

func normalizeMessagingDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 2000 || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("%w: description is invalid", ErrInvalidMessaging)
	}
	return value, nil
}

func normalizeMessagingChannel(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "email", "sms", "push":
		return value, nil
	default:
		return "", fmt.Errorf("%w: channel must be email, sms, or push", ErrInvalidMessaging)
	}
}

func normalizeMessagingProvider(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 1 || len(value) > 64 || strings.ContainsAny(value, "\x00\r\n\t ") {
		return "", fmt.Errorf("%w: provider is invalid", ErrInvalidMessaging)
	}
	return value, nil
}

func normalizeMessagingCredentials(raw map[string]string) ([]byte, bool, error) {
	if len(raw) > maxMessagingCredentialKeys {
		return nil, false, fmt.Errorf("%w: at most %d credential fields are allowed", ErrInvalidMessaging, maxMessagingCredentialKeys)
	}
	normalized := make(map[string]string, len(raw))
	keys := make([]string, 0, len(raw))
	for key, value := range raw {
		if !messagingCredentialKeyPattern.MatchString(key) || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, false, fmt.Errorf("%w: credential fields are invalid", ErrInvalidMessaging)
		}
		normalized[key] = value
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// encoding/json sorts string map keys, but explicitly building the map
	// above ensures callers cannot mutate the input while it is being encoded.
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, false, fmt.Errorf("%w: credentials could not be encoded", ErrInvalidMessaging)
	}
	if len(encoded) > maxMessagingCredentialBytes {
		return nil, false, fmt.Errorf("%w: credentials exceed %d bytes", ErrInvalidMessaging, maxMessagingCredentialBytes)
	}
	return encoded, len(normalized) > 0, nil
}

func normalizeMessagingAddress(channel, raw string) (string, string, []byte, error) {
	channel, err := normalizeMessagingChannel(channel)
	if err != nil {
		return "", "", nil, err
	}
	address := strings.TrimSpace(raw)
	if address == "" || len(address) > maxMessagingSubscriberBytes || strings.ContainsAny(address, "\x00\r\n\t ") {
		return "", "", nil, ErrMessagingAddressInvalid
	}
	switch channel {
	case "email":
		parsed, parseErr := mail.ParseAddress(address)
		if parseErr != nil || parsed.Address != address || !strings.Contains(address, "@") {
			return "", "", nil, ErrMessagingAddressInvalid
		}
		address = strings.ToLower(address)
	case "sms":
		if !messagingSMSAddressPattern.MatchString(address) {
			return "", "", nil, ErrMessagingAddressInvalid
		}
	case "push":
		if len(address) < 8 || strings.ContainsAny(address, "\x00\r\n") {
			return "", "", nil, ErrMessagingAddressInvalid
		}
	}
	digest := sha256.Sum256([]byte(address))
	return address, messagingAddressPreview(channel, address), digest[:], nil
}

func messagingAddressPreview(channel, address string) string {
	if channel == "email" {
		parts := strings.SplitN(address, "@", 2)
		local := parts[0]
		if len(local) <= 2 {
			local = local[:1] + "•••"
		} else {
			local = local[:1] + "•••" + local[len(local)-1:]
		}
		return local + "@" + parts[1]
	}
	if len(address) <= 4 {
		return "••••"
	}
	return address[:2] + "…" + address[len(address)-2:]
}

func (r *Repository) requireMessagingRead(ctx context.Context, projectID uuid.UUID, actor MessagingActor) (bool, error) {
	switch actor.Kind {
	case MessagingConsoleActor:
		role, err := r.projectRole(ctx, projectID, actor.AccountID)
		if err != nil {
			return false, err
		}
		return role == "owner" || role == "admin", nil
	case MessagingAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "messaging.read") {
			return false, ErrForbidden
		}
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, projectID).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, ErrNotFound
		}
		return apikey.HasScope(actor.APIKeyScopes, "messaging.write"), nil
	default:
		return false, ErrForbidden
	}
}

func (r *Repository) requireMessagingWriteTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor MessagingActor) error {
	switch actor.Kind {
	case MessagingConsoleActor:
		return requireProjectRoleTx(ctx, tx, projectID, actor.AccountID, "owner", "admin")
	case MessagingAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "messaging.write") {
			return ErrForbidden
		}
		return requireActiveProjectAPIKeyTx(ctx, tx, projectID, actor.APIKeyID, "messaging.write")
	default:
		return ErrForbidden
	}
}

func (r *Repository) encryptMessagingCredentials(raw map[string]string) ([]byte, bool, error) {
	if r.messagingCipher == nil {
		return nil, false, ErrMessagingNotReady
	}
	encoded, present, err := normalizeMessagingCredentials(raw)
	if err != nil {
		return nil, false, err
	}
	ciphertext, err := r.messagingCipher.Encrypt(encoded)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrMessagingNotReady, err)
	}
	return ciphertext, present, nil
}

func messagingAuditAccount(actor MessagingActor) uuid.UUID {
	if actor.Kind == MessagingConsoleActor {
		return actor.AccountID
	}
	return uuid.Nil
}

func messagingAuditMetadata(actor MessagingActor, metadata map[string]any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if actor.Kind == MessagingAPIKeyActor {
		metadata["actor"] = "api_key"
		metadata["api_key_id"] = actor.APIKeyID.String()
	}
	return metadata
}

func (r *Repository) auditMessaging(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor MessagingActor, action, targetType string, target uuid.UUID, metadata map[string]any) error {
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	metadata = messagingAuditMetadata(actor, metadata)
	if err := writeAuditMetadata(ctx, tx, orgID, messagingAuditAccount(actor), action, targetType, target, metadata); err != nil {
		return err
	}
	return r.enqueueWebhookEventTx(ctx, tx, projectID, action, targetType, target, metadata)
}
