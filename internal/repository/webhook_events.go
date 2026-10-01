package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Stealth-deplover/stealth/internal/realtime"
	"github.com/Stealth-deplover/stealth/internal/requestcontext"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) auditWebhook(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor WebhookActor, action, targetType string, target uuid.UUID, metadata map[string]any) error {
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["project_id"] = projectID.String()
	switch actor.Kind {
	case WebhookAPIKeyActor:
		metadata["actor"] = "api_key"
		metadata["api_key_id"] = actor.APIKeyID.String()
	case WebhookApplicationActor:
		metadata["actor"] = "project_user"
		metadata["project_user_id"] = actor.ProjectUserID.String()
		metadata["source"] = "application"
	case WebhookAnonymousActor:
		metadata["actor"] = "anonymous"
		metadata["source"] = "application"
	}
	actorID := uuid.Nil
	if actor.Kind == WebhookConsoleActor {
		actorID = actor.AccountID
	}
	if err := writeAuditMetadata(ctx, tx, orgID, actorID, action, targetType, target, metadata); err != nil {
		return err
	}
	return r.enqueueWebhookEventTx(ctx, tx, projectID, action, targetType, target, metadata)
}

// enqueueWebhookEventTx records every project event in the short-lived outbox
// and creates delivery rows only for matching enabled webhooks. Keeping the
// event independent from integration configuration lets the same transactional
// stream power Realtime subscribers without making webhook configuration a
// prerequisite for observing a project mutation.
func (r *Repository) enqueueWebhookEventTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, eventName, targetType string, target uuid.UUID, metadata map[string]any) error {
	return r.enqueueEventTx(ctx, tx, projectID, eventName, targetType, target, metadata, true)
}

// enqueueRealtimeOnlyEventTx records a notification in the durable outbox
// without recursively sending that internal delivery state to user webhooks.
func (r *Repository) enqueueRealtimeOnlyEventTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, eventName, targetType string, target uuid.UUID, metadata map[string]any) error {
	return r.enqueueEventTx(ctx, tx, projectID, eventName, targetType, target, metadata, false)
}

func (r *Repository) enqueueEventTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, eventName, targetType string, target uuid.UUID, metadata map[string]any, createWebhookDeliveries bool) error {
	if len(eventName) < 3 || len(eventName) > 160 || len(targetType) < 3 || len(targetType) > 80 {
		return ErrInvalidWebhook
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	organizationID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	eventID := uuid.Must(uuid.NewV7())
	occurredAt := time.Now().UTC()
	targetValue := any(nil)
	resourceID := ""
	if target != uuid.Nil {
		targetValue = target
		resourceID = target.String()
	}
	safeMetadata := realtime.SafePayload(metadata)
	envelope := realtime.Envelope{
		ID:             eventID.String(),
		Type:           eventName,
		Version:        realtime.CurrentVersion,
		OccurredAt:     occurredAt,
		OrganizationID: organizationID.String(),
		ProjectID:      projectID.String(),
		ResourceID:     resourceID,
		CorrelationID:  requestcontext.CorrelationID(ctx),
		Payload:        safeMetadata,
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	payloadValue := map[string]any{
		"id":              eventID.String(),
		"event":           eventName, // legacy webhook consumers
		"type":            eventName,
		"version":         realtime.CurrentVersion,
		"occurred_at":     occurredAt.Format(time.RFC3339Nano),
		"organization_id": organizationID.String(),
		"project_id":      projectID.String(),
		"target": map[string]any{
			"type": targetType,
			"id": func() any {
				if target == uuid.Nil {
					return nil
				}
				return target.String()
			}(),
		},
		"resource_id": func() any {
			if resourceID == "" {
				return nil
			}
			return resourceID
		}(),
		"correlation_id": func() any {
			if envelope.CorrelationID == "" {
				return nil
			}
			return envelope.CorrelationID
		}(),
		"data":       safeMetadata, // legacy webhook consumers
		"payload":    safeMetadata,
		"created_at": occurredAt.Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return err
	}
	if len(payload) > 262144 {
		return ErrWebhookPayloadTooLarge
	}
	var insertedID uuid.UUID
	correlationValue := any(nil)
	if envelope.CorrelationID != "" {
		correlationValue = envelope.CorrelationID
	}
	err = tx.QueryRow(ctx, `INSERT INTO webhook_events (id,project_id,organization_id,event_name,target_type,target_id,event_version,occurred_at,correlation_id,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, eventID, projectID, organizationID, eventName, targetType, targetValue, realtime.CurrentVersion, occurredAt, correlationValue, payload).Scan(&insertedID)
	if err != nil {
		return err
	}
	if !createWebhookDeliveries {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT id FROM project_webhooks WHERE project_id=$1 AND enabled AND (events @> ARRAY['*']::text[] OR $2=ANY(events))`, projectID, eventName)
	if err != nil {
		return err
	}
	webhookIDs := make([]uuid.UUID, 0, 4)
	for rows.Next() {
		var webhookID uuid.UUID
		if scanErr := rows.Scan(&webhookID); scanErr != nil {
			rows.Close()
			return scanErr
		}
		webhookIDs = append(webhookIDs, webhookID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, webhookID := range webhookIDs {
		if _, insertErr := tx.Exec(ctx, `INSERT INTO webhook_deliveries (id,event_id,webhook_id) VALUES ($1,$2,$3) ON CONFLICT (event_id,webhook_id) DO NOTHING`, uuid.Must(uuid.NewV7()), insertedID, webhookID); insertErr != nil {
			return insertErr
		}
	}
	return nil
}
