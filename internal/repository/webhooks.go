package repository

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrWebhookNotReady          = errors.New("webhook service is not ready")
	ErrInvalidWebhook           = errors.New("invalid webhook")
	ErrNoWebhookDelivery        = errors.New("no webhook delivery available")
	ErrWebhookDeliveryNotFound  = errors.New("webhook delivery not found")
	ErrWebhookPayloadTooLarge   = errors.New("webhook payload is too large")
	ErrWebhookSecretUnavailable = errors.New("webhook secret encryption is unavailable")
)

// WebhookActor deliberately aliases the management actor used by the other
// project control planes. Application users can consume data APIs but cannot
// register or inspect delivery endpoints.
type WebhookActor = DatabaseActor

const (
	WebhookConsoleActor     = DatabaseConsoleActor
	WebhookAPIKeyActor      = DatabaseAPIKeyActor
	WebhookApplicationActor = DatabaseApplicationActor
	WebhookAnonymousActor   = DatabaseAnonymousActor
)

type WebhookInput struct {
	Name    string
	URL     string
	Events  []string
	Enabled bool
}

type WebhookPatch struct {
	Name    *string
	URL     *string
	Events  *[]string
	Enabled *bool
}

// WebhookDeliveryJob is an internal worker projection. SecretCiphertext and
// EventPayload never leave the trusted worker process.
type WebhookDeliveryJob struct {
	DeliveryID       uuid.UUID
	EventID          uuid.UUID
	WebhookID        uuid.UUID
	ProjectID        uuid.UUID
	EventName        string
	URL              string
	SecretCiphertext []byte
	EventPayload     []byte
	AttemptCount     int
}

const webhookProjection = `id,project_id,name,url,events,enabled,failure_count,last_delivery_at,last_failure_at,created_at,updated_at`
const webhookDeliveryProjection = `d.id,d.webhook_id,d.event_id,e.event_name,d.status,d.attempt_count,d.last_status_code,d.last_error,d.delivered_at,d.created_at,d.updated_at`

type webhookScanner interface {
	Scan(dest ...any) error
}

func scanWebhook(row webhookScanner) (domain.Webhook, error) {
	var item domain.Webhook
	var id, projectID uuid.UUID
	err := row.Scan(&id, &projectID, &item.Name, &item.URL, &item.Events, &item.Enabled, &item.FailureCount, &item.LastDeliveryAt, &item.LastFailureAt, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	return item, err
}

func scanWebhookDelivery(row webhookScanner) (domain.WebhookDelivery, error) {
	var item domain.WebhookDelivery
	var id, webhookID, eventID uuid.UUID
	err := row.Scan(&id, &webhookID, &eventID, &item.EventName, &item.Status, &item.AttemptCount, &item.LastStatusCode, &item.LastError, &item.DeliveredAt, &item.CreatedAt, &item.UpdatedAt)
	item.ID = id.String()
	item.WebhookID = webhookID.String()
	item.EventID = eventID.String()
	return item, err
}

// NormalizeWebhookURL validates a public configuration URL. The delivery
// worker performs a second, network-level SSRF check after DNS resolution.
func NormalizeWebhookURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) < len("https://a.co") || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n\t ") {
		return "", ErrInvalidWebhook
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", ErrInvalidWebhook
	}
	if strings.ContainsAny(u.Host, "\x00\r\n\t @") || u.Hostname() == "" {
		return "", ErrInvalidWebhook
	}
	if port := u.Port(); port != "" {
		parsed, parseErr := strconv.Atoi(port)
		if parseErr != nil || parsed < 1 || parsed > 65535 {
			return "", ErrInvalidWebhook
		}
	}
	// url.Parse accepts some malformed percent escapes in path/query only by
	// returning an error; ensure a canonical round trip before persisting.
	if u.String() != value {
		return "", ErrInvalidWebhook
	}
	return value, nil
}

func NormalizeWebhookEvents(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{"*"}, nil
	}
	if len(raw) > 64 {
		return nil, fmt.Errorf("%w: at most 64 events", ErrInvalidWebhook)
	}
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		event := strings.TrimSpace(value)
		if len(event) < 1 || len(event) > 160 || event == "*" {
			if event != "*" {
				return nil, fmt.Errorf("%w: invalid event name", ErrInvalidWebhook)
			}
		} else {
			for index, ch := range event {
				alphaNumeric := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
				if index == 0 && !alphaNumeric {
					return nil, fmt.Errorf("%w: invalid event name", ErrInvalidWebhook)
				}
				if index > 0 && !(alphaNumeric || ch == '.' || ch == '_' || ch == '-') {
					return nil, fmt.Errorf("%w: invalid event name", ErrInvalidWebhook)
				}
			}
		}
		if _, exists := seen[event]; exists {
			return nil, fmt.Errorf("%w: duplicate event name", ErrInvalidWebhook)
		}
		seen[event] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for event := range seen {
		result = append(result, event)
	}
	slices.Sort(result)
	return result, nil
}

func normalizeWebhookName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || len(value) > 120 || strings.ContainsAny(value, "\x00\r\n\t") {
		return "", ErrInvalidWebhook
	}
	return value, nil
}

func newWebhookSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (r *Repository) requireWebhookRead(ctx context.Context, projectID uuid.UUID, actor WebhookActor) (bool, error) {
	switch actor.Kind {
	case WebhookConsoleActor:
		role, err := r.projectRole(ctx, projectID, actor.AccountID)
		if err != nil {
			return false, err
		}
		return role == "owner" || role == "admin", nil
	case WebhookAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "webhooks.read") {
			return false, ErrForbidden
		}
		var active bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM project_api_keys
			WHERE id=$1 AND project_id=$2 AND revoked_at IS NULL
			  AND (expires_at IS NULL OR expires_at>now())
			  AND 'webhooks.read'=ANY(scopes)
		)`, actor.APIKeyID, projectID).Scan(&active); err != nil {
			return false, err
		}
		if !active {
			return false, ErrNotFound
		}
		return apikey.HasScope(actor.APIKeyScopes, "webhooks.write"), nil
	default:
		return false, ErrForbidden
	}
}

func (r *Repository) requireWebhookWriteTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor WebhookActor) error {
	switch actor.Kind {
	case WebhookConsoleActor:
		return requireProjectRoleTx(ctx, tx, projectID, actor.AccountID, "owner", "admin")
	case WebhookAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "webhooks.write") {
			return ErrForbidden
		}
		return requireActiveProjectAPIKeyTx(ctx, tx, projectID, actor.APIKeyID, "webhooks.write")
	default:
		return ErrForbidden
	}
}
