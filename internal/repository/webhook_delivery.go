package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextWebhookDelivery(ctx context.Context, workerID string) (WebhookDeliveryJob, error) {
	if !validFunctionWorkerID(workerID) {
		return WebhookDeliveryJob{}, ErrInvalidWebhook
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WebhookDeliveryJob{}, err
	}
	defer tx.Rollback(ctx)
	var job WebhookDeliveryJob
	err = tx.QueryRow(ctx, `
		SELECT d.id,d.event_id,d.webhook_id,e.project_id,e.event_name,w.url,w.secret_ciphertext,e.payload,d.attempt_count
		FROM webhook_deliveries d
		JOIN webhook_events e ON e.id=d.event_id
		JOIN project_webhooks w ON w.id=d.webhook_id AND w.project_id=e.project_id
		WHERE d.status='pending' AND d.next_attempt_at<=now() AND w.enabled AND e.expires_at>now()
		ORDER BY d.next_attempt_at,d.created_at,d.id
		LIMIT 1
		FOR UPDATE OF d SKIP LOCKED`).Scan(&job.DeliveryID, &job.EventID, &job.WebhookID, &job.ProjectID, &job.EventName, &job.URL, &job.SecretCiphertext, &job.EventPayload, &job.AttemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookDeliveryJob{}, ErrNoWebhookDelivery
	}
	if err != nil {
		return WebhookDeliveryJob{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET status='running',attempt_count=attempt_count+1,leased_at=now(),worker_id=$2,updated_at=now() WHERE id=$1 AND status='pending'`, job.DeliveryID, workerID); err != nil {
		return WebhookDeliveryJob{}, err
	}
	job.AttemptCount++
	if err := tx.Commit(ctx); err != nil {
		return WebhookDeliveryJob{}, err
	}
	return job, nil
}

func (r *Repository) RequeueStaleWebhookDeliveries(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, ErrInvalidWebhook
	}
	result, err := r.pool.Exec(ctx, `UPDATE webhook_deliveries SET status='pending',leased_at=NULL,worker_id=NULL,next_attempt_at=LEAST(next_attempt_at,now()),updated_at=now() WHERE status='running' AND leased_at IS NOT NULL AND leased_at < now() - ($1 * interval '1 second')`, maxAge.Seconds())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (r *Repository) ExpireWebhookDeliveries(ctx context.Context) (int64, error) {
	result, err := r.pool.Exec(ctx, `UPDATE webhook_deliveries d SET status='failed',last_error='event expired before delivery',updated_at=now() FROM webhook_events e WHERE e.id=d.event_id AND d.status='pending' AND e.expires_at<=now()`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func truncateWebhookError(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 4000 {
		value = value[:4000]
	}
	return &value
}

func (r *Repository) FinishWebhookDelivery(ctx context.Context, deliveryID uuid.UUID, workerID string, success bool, statusCode *int, lastError string, retryAt *time.Time) error {
	if !validFunctionWorkerID(workerID) {
		return ErrInvalidWebhook
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status, owner string
	var webhookID, projectID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT d.status,COALESCE(d.worker_id,''),d.webhook_id,w.project_id FROM webhook_deliveries d JOIN project_webhooks w ON w.id=d.webhook_id WHERE d.id=$1 FOR UPDATE`, deliveryID).Scan(&status, &owner, &webhookID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrWebhookDeliveryNotFound
	}
	if err != nil {
		return err
	}
	if status != "running" || owner != workerID {
		return ErrWebhookDeliveryNotFound
	}
	if statusCode != nil && (*statusCode < 100 || *statusCode > 599) {
		statusCode = nil
	}
	errValue := truncateWebhookError(lastError)
	finalStatus := "failed"
	if success {
		finalStatus = "succeeded"
		if _, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET status='succeeded',leased_at=NULL,worker_id=NULL,last_status_code=$2,last_error=NULL,delivered_at=now(),updated_at=now() WHERE id=$1`, deliveryID, statusCode); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE project_webhooks SET failure_count=0,last_delivery_at=now(),updated_at=now() WHERE id=$1`, webhookID)
	} else if retryAt != nil {
		finalStatus = "pending"
		if _, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET status='pending',leased_at=NULL,worker_id=NULL,last_status_code=$2,last_error=$3,next_attempt_at=$4,updated_at=now() WHERE id=$1`, deliveryID, statusCode, errValue, retryAt.UTC()); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE project_webhooks SET failure_count=failure_count+1,last_failure_at=now(),updated_at=now() WHERE id=$1`, webhookID)
	} else {
		if _, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET status='failed',leased_at=NULL,worker_id=NULL,last_status_code=$2,last_error=$3,updated_at=now() WHERE id=$1`, deliveryID, statusCode, errValue); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE project_webhooks SET failure_count=failure_count+1,last_failure_at=now(),updated_at=now() WHERE id=$1`, webhookID)
	}
	if err != nil {
		return err
	}
	if err := r.enqueueRealtimeOnlyEventTx(ctx, tx, projectID, "webhook.delivery.updated", "webhook_delivery", deliveryID, map[string]any{"webhook_id": webhookID.String(), "status": finalStatus}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// auditWebhook appends to both the existing audit stream and the transactional
// webhook outbox. It is also used by the other project control planes.
