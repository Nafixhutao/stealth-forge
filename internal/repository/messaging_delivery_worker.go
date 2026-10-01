package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextMessagingDelivery(ctx context.Context, workerID string) (MessagingDeliveryJob, error) {
	if !validFunctionWorkerID(workerID) {
		return MessagingDeliveryJob{}, ErrInvalidMessaging
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MessagingDeliveryJob{}, err
	}
	defer tx.Rollback(ctx)
	var job MessagingDeliveryJob
	err = tx.QueryRow(ctx, `
		SELECT d.id,d.message_id,d.project_id,d.subscriber_id,d.provider_id,d.channel,d.address_preview,d.address_ciphertext,m.payload_ciphertext,p.provider,p.enabled,p.credentials_ciphertext,d.attempt_count
		FROM project_messaging_deliveries d
		JOIN project_messaging_messages m ON m.id=d.message_id AND m.project_id=d.project_id
		LEFT JOIN project_messaging_providers p ON p.id=d.provider_id AND p.project_id=d.project_id
		WHERE d.status='pending' AND d.next_attempt_at<=now() AND m.status IN ('queued','processing')
		ORDER BY d.next_attempt_at,d.created_at,d.id
		LIMIT 1
		FOR UPDATE OF d SKIP LOCKED`).Scan(&job.DeliveryID, &job.MessageID, &job.ProjectID, &job.SubscriberID, &job.ProviderID, &job.Channel, &job.AddressPreview, &job.AddressCiphertext, &job.PayloadCiphertext, &job.Provider, &job.ProviderEnabled, &job.ProviderCredentialsCiphertext, &job.AttemptCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return MessagingDeliveryJob{}, ErrNoMessagingDelivery
	}
	if err != nil {
		return MessagingDeliveryJob{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_messaging_deliveries SET status='running',attempt_count=attempt_count+1,leased_at=now(),worker_id=$2,updated_at=now() WHERE id=$1 AND status='pending'`, job.DeliveryID, workerID); err != nil {
		return MessagingDeliveryJob{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_messaging_messages SET status='processing',updated_at=now() WHERE project_id=$1 AND id=$2 AND status='queued'`, job.ProjectID, job.MessageID); err != nil {
		return MessagingDeliveryJob{}, err
	}
	if err := r.enqueueRealtimeOnlyEventTx(ctx, tx, job.ProjectID, "messaging.delivery.updated", "messaging_delivery", job.DeliveryID, map[string]any{"message_id": job.MessageID.String(), "status": "running"}); err != nil {
		return MessagingDeliveryJob{}, err
	}
	job.AttemptCount++
	if err := tx.Commit(ctx); err != nil {
		return MessagingDeliveryJob{}, err
	}
	return job, nil
}

func (r *Repository) RequeueStaleMessagingDeliveries(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, ErrInvalidMessaging
	}
	result, err := r.pool.Exec(ctx, `UPDATE project_messaging_deliveries SET status='pending',leased_at=NULL,worker_id=NULL,next_attempt_at=LEAST(next_attempt_at,now()),updated_at=now() WHERE status='running' AND leased_at IS NOT NULL AND leased_at < now() - ($1::double precision * interval '1 second')`, maxAge.Seconds())
	if err != nil {
		return 0, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE project_messaging_messages m SET status='queued',updated_at=now() WHERE m.status='processing' AND EXISTS(SELECT 1 FROM project_messaging_deliveries d WHERE d.message_id=m.id AND d.project_id=m.project_id AND d.status='pending') AND NOT EXISTS(SELECT 1 FROM project_messaging_deliveries d WHERE d.message_id=m.id AND d.project_id=m.project_id AND d.status='running')`); err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func truncateMessagingError(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 4000 {
		value = value[:4000]
	}
	return &value
}

func (r *Repository) FinishMessagingDelivery(ctx context.Context, deliveryID uuid.UUID, workerID string, success bool, statusCode *int, lastError string, retryAt *time.Time) error {
	if !validFunctionWorkerID(workerID) {
		return ErrInvalidMessaging
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status, owner string
	var projectID, messageID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT status,COALESCE(worker_id,''),project_id,message_id FROM project_messaging_deliveries WHERE id=$1 FOR UPDATE`, deliveryID).Scan(&status, &owner, &projectID, &messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMessagingDeliveryNotFound
	}
	if err != nil {
		return err
	}
	if status != "running" || owner != workerID {
		return ErrMessagingDeliveryNotFound
	}
	if statusCode != nil && (*statusCode < 100 || *statusCode > 599) {
		statusCode = nil
	}
	errValue := truncateMessagingError(lastError)
	finalStatus := "failed"
	switch {
	case success:
		finalStatus = "succeeded"
		_, err = tx.Exec(ctx, `UPDATE project_messaging_deliveries SET status='succeeded',leased_at=NULL,worker_id=NULL,last_status_code=$2,last_error=NULL,delivered_at=now(),updated_at=now() WHERE id=$1`, deliveryID, statusCode)
	case retryAt != nil:
		finalStatus = "pending"
		_, err = tx.Exec(ctx, `UPDATE project_messaging_deliveries SET status='pending',leased_at=NULL,worker_id=NULL,last_status_code=$2,last_error=$3,next_attempt_at=$4,updated_at=now() WHERE id=$1`, deliveryID, statusCode, errValue, retryAt.UTC())
	default:
		_, err = tx.Exec(ctx, `UPDATE project_messaging_deliveries SET status='failed',leased_at=NULL,worker_id=NULL,last_status_code=$2,last_error=$3,updated_at=now() WHERE id=$1`, deliveryID, statusCode, errValue)
	}
	if err != nil {
		return err
	}
	if err := refreshMessagingMessageTx(ctx, tx, projectID, messageID); err != nil {
		return err
	}
	if err := r.enqueueRealtimeOnlyEventTx(ctx, tx, projectID, "messaging.delivery.updated", "messaging_delivery", deliveryID, map[string]any{"message_id": messageID.String(), "status": finalStatus}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func refreshMessagingMessageTx(ctx context.Context, tx pgx.Tx, projectID, messageID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE project_messaging_messages m
		SET succeeded_count=c.succeeded_count,failed_count=c.failed_count,
			status=CASE
				WHEN m.status='cancelled' THEN 'cancelled'
				WHEN c.pending_count+c.running_count>0 AND c.running_count>0 THEN 'processing'
				WHEN c.pending_count+c.running_count>0 THEN 'queued'
				WHEN c.failed_count>0 THEN 'failed'
				ELSE 'succeeded'
			END,
			updated_at=now()
		FROM (
			SELECT message_id,
				count(*) FILTER (WHERE status='succeeded')::integer AS succeeded_count,
				count(*) FILTER (WHERE status='failed')::integer AS failed_count,
				count(*) FILTER (WHERE status='pending')::integer AS pending_count,
				count(*) FILTER (WHERE status='running')::integer AS running_count
			FROM project_messaging_deliveries
			WHERE project_id=$1 AND message_id=$2
			GROUP BY message_id
		) c
		WHERE m.project_id=$1 AND m.id=c.message_id`, projectID, messageID)
	return err
}

// MessagingProviderCredentialsForDelivery decrypts a provider row only for a
// worker-owned delivery job. It is separate from the safe provider projection.
func (r *Repository) MessagingProviderCredentialsForDelivery(ctx context.Context, job MessagingDeliveryJob) (MessagingProviderCredentials, error) {
	if job.Provider == nil || job.ProviderEnabled == nil || job.ProviderID == nil {
		return MessagingProviderCredentials{}, ErrMessagingProviderUnavailable
	}
	if r.messagingCipher == nil {
		return MessagingProviderCredentials{}, ErrMessagingNotReady
	}
	plaintext, err := r.messagingCipher.Decrypt(job.ProviderCredentialsCiphertext)
	if err != nil {
		return MessagingProviderCredentials{}, fmt.Errorf("%w: decrypt provider credentials", ErrMessagingNotReady)
	}
	var values map[string]string
	if err := json.Unmarshal(plaintext, &values); err != nil || values == nil {
		return MessagingProviderCredentials{}, fmt.Errorf("%w: provider credentials are corrupt", ErrMessagingNotReady)
	}
	return MessagingProviderCredentials{ProviderID: *job.ProviderID, ProjectID: job.ProjectID, Channel: job.Channel, Provider: *job.Provider, Enabled: *job.ProviderEnabled, Values: values}, nil
}

func (r *Repository) MessagingDeliveryAddress(_ context.Context, job MessagingDeliveryJob) (string, error) {
	if r.messagingCipher == nil {
		return "", ErrMessagingNotReady
	}
	plaintext, err := r.messagingCipher.Decrypt(job.AddressCiphertext)
	if err != nil || strings.TrimSpace(string(plaintext)) == "" {
		return "", fmt.Errorf("%w: decrypt subscriber address", ErrMessagingNotReady)
	}
	return string(plaintext), nil
}

func (r *Repository) MessagingDeliveryPayload(_ context.Context, job MessagingDeliveryJob) (MessagingMessagePayload, error) {
	if r.messagingCipher == nil {
		return MessagingMessagePayload{}, ErrMessagingNotReady
	}
	plaintext, err := r.messagingCipher.Decrypt(job.PayloadCiphertext)
	if err != nil {
		return MessagingMessagePayload{}, fmt.Errorf("%w: decrypt message payload", ErrMessagingNotReady)
	}
	var payload MessagingMessagePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil || strings.TrimSpace(payload.Body) == "" {
		return MessagingMessagePayload{}, fmt.Errorf("%w: message payload is corrupt", ErrMessagingNotReady)
	}
	return payload, nil
}
