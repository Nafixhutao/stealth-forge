package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateWebhook(ctx context.Context, id, projectID uuid.UUID, actor WebhookActor, input WebhookInput) (domain.Webhook, string, error) {
	if r.webhookCipher == nil {
		return domain.Webhook{}, "", ErrWebhookNotReady
	}
	name, err := normalizeWebhookName(input.Name)
	if err != nil {
		return domain.Webhook{}, "", err
	}
	webhookURL, err := NormalizeWebhookURL(input.URL)
	if err != nil {
		return domain.Webhook{}, "", err
	}
	events, err := NormalizeWebhookEvents(input.Events)
	if err != nil {
		return domain.Webhook{}, "", err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return domain.Webhook{}, "", err
	}
	ciphertext, err := r.webhookCipher.Encrypt([]byte(secret))
	if err != nil {
		return domain.Webhook{}, "", fmt.Errorf("%w: %v", ErrWebhookSecretUnavailable, err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Webhook{}, "", err
	}
	defer tx.Rollback(ctx)
	if err := r.requireWebhookWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.Webhook{}, "", err
	}
	item, err := scanWebhook(tx.QueryRow(ctx, `INSERT INTO project_webhooks (id,project_id,name,url,secret_ciphertext,events,enabled) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+webhookProjection, id, projectID, name, webhookURL, ciphertext, events, input.Enabled))
	if err != nil {
		return domain.Webhook{}, "", mapError(err)
	}
	if err := r.auditWebhook(ctx, tx, projectID, actor, "webhook.create", "webhook", id, map[string]any{"name": name, "url": webhookURL, "events": events, "enabled": input.Enabled}); err != nil {
		return domain.Webhook{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Webhook{}, "", err
	}
	return item, secret, nil
}

func (r *Repository) ListWebhooks(ctx context.Context, projectID uuid.UUID, actor WebhookActor, limit int, cursor *uuid.UUID) ([]domain.Webhook, string, bool, error) {
	canManage, err := r.requireWebhookRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if limit < 1 || limit > 100 {
		return nil, "", false, ErrInvalidWebhook
	}
	rows, err := r.pool.Query(ctx, `SELECT `+webhookProjection+` FROM project_webhooks WHERE project_id=$1 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $2`, projectID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.Webhook, 0, limit)
	for rows.Next() {
		item, scanErr := scanWebhook(rows)
		if scanErr != nil {
			return nil, "", false, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, canManage, nil
}

func (r *Repository) GetWebhook(ctx context.Context, projectID, webhookID uuid.UUID, actor WebhookActor) (domain.Webhook, error) {
	if _, err := r.requireWebhookRead(ctx, projectID, actor); err != nil {
		return domain.Webhook{}, err
	}
	item, err := scanWebhook(r.pool.QueryRow(ctx, `SELECT `+webhookProjection+` FROM project_webhooks WHERE project_id=$1 AND id=$2`, projectID, webhookID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Webhook{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) UpdateWebhook(ctx context.Context, projectID, webhookID uuid.UUID, actor WebhookActor, patch WebhookPatch) (domain.Webhook, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Webhook{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireWebhookWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.Webhook{}, err
	}
	current, err := scanWebhook(tx.QueryRow(ctx, `SELECT `+webhookProjection+` FROM project_webhooks WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, webhookID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Webhook{}, ErrNotFound
	}
	if err != nil {
		return domain.Webhook{}, err
	}
	name, webhookURL, events := current.Name, current.URL, append([]string(nil), current.Events...)
	enabled := current.Enabled
	changed := make([]string, 0, 4)
	if patch.Name != nil {
		name, err = normalizeWebhookName(*patch.Name)
		if err != nil {
			return domain.Webhook{}, err
		}
		if name != current.Name {
			changed = append(changed, "name")
		}
	}
	if patch.URL != nil {
		webhookURL, err = NormalizeWebhookURL(*patch.URL)
		if err != nil {
			return domain.Webhook{}, err
		}
		if webhookURL != current.URL {
			changed = append(changed, "url")
		}
	}
	if patch.Events != nil {
		events, err = NormalizeWebhookEvents(*patch.Events)
		if err != nil {
			return domain.Webhook{}, err
		}
		if strings.Join(events, "\x00") != strings.Join(current.Events, "\x00") {
			changed = append(changed, "events")
		}
	}
	if patch.Enabled != nil {
		enabled = *patch.Enabled
		if enabled != current.Enabled {
			changed = append(changed, "enabled")
		}
	}
	item := current
	if len(changed) > 0 {
		item, err = scanWebhook(tx.QueryRow(ctx, `UPDATE project_webhooks SET name=$3,url=$4,events=$5,enabled=$6,updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING `+webhookProjection, projectID, webhookID, name, webhookURL, events, enabled))
		if err != nil {
			return domain.Webhook{}, mapError(err)
		}
		if err := r.auditWebhook(ctx, tx, projectID, actor, "webhook.update", "webhook", webhookID, map[string]any{"changed_fields": changed}); err != nil {
			return domain.Webhook{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Webhook{}, err
	}
	return item, nil
}

func (r *Repository) DeleteWebhook(ctx context.Context, projectID, webhookID uuid.UUID, actor WebhookActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.requireWebhookWriteTx(ctx, tx, projectID, actor); err != nil {
		return err
	}
	item, err := scanWebhook(tx.QueryRow(ctx, `SELECT `+webhookProjection+` FROM project_webhooks WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, webhookID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_webhooks WHERE project_id=$1 AND id=$2`, projectID, webhookID); err != nil {
		return err
	}
	if err := r.auditWebhook(ctx, tx, projectID, actor, "webhook.delete", "webhook", webhookID, map[string]any{"name": item.Name, "url": item.URL}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RotateWebhookSecret(ctx context.Context, projectID, webhookID uuid.UUID, actor WebhookActor) (domain.Webhook, string, error) {
	if r.webhookCipher == nil {
		return domain.Webhook{}, "", ErrWebhookNotReady
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return domain.Webhook{}, "", err
	}
	ciphertext, err := r.webhookCipher.Encrypt([]byte(secret))
	if err != nil {
		return domain.Webhook{}, "", fmt.Errorf("%w: %v", ErrWebhookSecretUnavailable, err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Webhook{}, "", err
	}
	defer tx.Rollback(ctx)
	if err := r.requireWebhookWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.Webhook{}, "", err
	}
	if _, err := scanWebhook(tx.QueryRow(ctx, `SELECT `+webhookProjection+` FROM project_webhooks WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, webhookID)); errors.Is(err, pgx.ErrNoRows) {
		return domain.Webhook{}, "", ErrNotFound
	} else if err != nil {
		return domain.Webhook{}, "", err
	}
	item, err := scanWebhook(tx.QueryRow(ctx, `UPDATE project_webhooks SET secret_ciphertext=$3,updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING `+webhookProjection, projectID, webhookID, ciphertext))
	if err != nil {
		return domain.Webhook{}, "", err
	}
	if err := r.auditWebhook(ctx, tx, projectID, actor, "webhook.secret_rotate", "webhook", webhookID, nil); err != nil {
		return domain.Webhook{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Webhook{}, "", err
	}
	return item, secret, nil
}

func (r *Repository) ListWebhookDeliveries(ctx context.Context, projectID, webhookID uuid.UUID, actor WebhookActor, limit int, cursor *uuid.UUID) ([]domain.WebhookDelivery, string, error) {
	if _, err := r.requireWebhookRead(ctx, projectID, actor); err != nil {
		return nil, "", err
	}
	if limit < 1 || limit > 100 {
		return nil, "", ErrInvalidWebhook
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_webhooks WHERE project_id=$1 AND id=$2)`, projectID, webhookID).Scan(&exists); err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+webhookDeliveryProjection+` FROM webhook_deliveries d JOIN webhook_events e ON e.id=d.event_id JOIN project_webhooks w ON w.id=d.webhook_id WHERE w.project_id=$1 AND d.webhook_id=$2 AND ($3::uuid IS NULL OR d.id<$3) ORDER BY d.id DESC LIMIT $4`, projectID, webhookID, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.WebhookDelivery, 0, limit)
	for rows.Next() {
		item, scanErr := scanWebhookDelivery(rows)
		if scanErr != nil {
			return nil, "", scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, nil
}

// ClaimNextWebhookDelivery atomically leases a pending delivery. The event,
// webhook and project predicates are repeated in the same query so a stale
// row can never cross a tenant boundary.
