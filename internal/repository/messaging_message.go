package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateMessagingMessage(ctx context.Context, id, projectID uuid.UUID, actor MessagingActor, input MessagingMessageInput) (MessagingMessageCreateResult, error) {
	channel, payload, requestHash, err := normalizeMessagingMessageInput(input)
	if err != nil {
		return MessagingMessageCreateResult{}, err
	}
	idempotencyKey, err := normalizeMessagingIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		return MessagingMessageCreateResult{}, err
	}
	if r.messagingCipher == nil {
		return MessagingMessageCreateResult{}, ErrMessagingNotReady
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return MessagingMessageCreateResult{}, fmt.Errorf("%w: message payload could not be encoded", ErrInvalidMessaging)
	}
	payloadCiphertext, err := r.messagingCipher.Encrypt(payloadBytes)
	if err != nil {
		return MessagingMessageCreateResult{}, fmt.Errorf("%w: message payload could not be encrypted", ErrMessagingNotReady)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MessagingMessageCreateResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return MessagingMessageCreateResult{}, err
	}
	if idempotencyKey != "" {
		// Serialize requests sharing a tenant/key pair. Without a transaction
		// advisory lock, concurrent first attempts could both miss the lookup
		// and turn a valid idempotent replay into a unique-constraint conflict.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, messagingIdempotencyLockKey(projectID, idempotencyKey)); err != nil {
			return MessagingMessageCreateResult{}, err
		}
		var existingHash []byte
		var existing domain.MessagingMessage
		row := tx.QueryRow(ctx, `SELECT `+messagingMessageProjection+`,request_hash FROM project_messaging_messages WHERE project_id=$1 AND idempotency_key=$2`, projectID, idempotencyKey)
		var scanErr error
		existing, scanErr = scanMessagingMessageWithHash(row, &existingHash)
		if scanErr == nil {
			if !bytes.Equal(existingHash, requestHash) {
				return MessagingMessageCreateResult{}, ErrConflict
			}
			if err := tx.Commit(ctx); err != nil {
				return MessagingMessageCreateResult{}, err
			}
			return MessagingMessageCreateResult{Message: existing, Created: false}, nil
		}
		if !errors.Is(scanErr, pgx.ErrNoRows) {
			return MessagingMessageCreateResult{}, scanErr
		}
	}
	var topicEnabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM project_messaging_topics WHERE project_id=$1 AND id=$2 FOR SHARE`, projectID, input.TopicID).Scan(&topicEnabled); errors.Is(err, pgx.ErrNoRows) {
		return MessagingMessageCreateResult{}, ErrNotFound
	} else if err != nil {
		return MessagingMessageCreateResult{}, err
	}
	if !topicEnabled {
		return MessagingMessageCreateResult{}, fmt.Errorf("%w: topic is disabled", ErrInvalidMessaging)
	}
	var providerID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM project_messaging_providers WHERE project_id=$1 AND channel=$2 AND enabled ORDER BY id LIMIT 1`, projectID, channel).Scan(&providerID); errors.Is(err, pgx.ErrNoRows) {
		return MessagingMessageCreateResult{}, ErrMessagingProviderUnavailable
	} else if err != nil {
		return MessagingMessageCreateResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id,address_ciphertext,address_preview FROM project_messaging_subscribers WHERE project_id=$1 AND topic_id=$2 AND channel=$3 AND enabled ORDER BY id LIMIT $4`, projectID, input.TopicID, channel, maxMessagingMessageRecipients+1)
	if err != nil {
		return MessagingMessageCreateResult{}, err
	}
	type recipient struct {
		id      uuid.UUID
		address []byte
		preview string
	}
	recipients := make([]recipient, 0)
	for rows.Next() {
		var item recipient
		if err := rows.Scan(&item.id, &item.address, &item.preview); err != nil {
			rows.Close()
			return MessagingMessageCreateResult{}, err
		}
		recipients = append(recipients, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MessagingMessageCreateResult{}, err
	}
	rows.Close()
	if len(recipients) == 0 {
		return MessagingMessageCreateResult{}, ErrMessagingNoRecipients
	}
	if len(recipients) > maxMessagingMessageRecipients {
		return MessagingMessageCreateResult{}, ErrMessagingTooManyRecipients
	}
	item, err := scanMessagingMessage(tx.QueryRow(ctx, `
		INSERT INTO project_messaging_messages (id,project_id,topic_id,channel,payload_ciphertext,request_hash,idempotency_key,status,recipient_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'queued',$8)
		RETURNING `+messagingMessageProjection, id, projectID, input.TopicID, channel, payloadCiphertext, requestHash, nullableMessagingIdempotencyKey(idempotencyKey), len(recipients)))
	if err != nil {
		return MessagingMessageCreateResult{}, mapError(err)
	}
	for _, recipient := range recipients {
		if _, err := tx.Exec(ctx, `INSERT INTO project_messaging_deliveries (id,project_id,message_id,subscriber_id,provider_id,channel,address_ciphertext,address_preview,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending')`, uuid.Must(uuid.NewV7()), projectID, id, recipient.id, providerID, channel, recipient.address, recipient.preview); err != nil {
			return MessagingMessageCreateResult{}, mapError(err)
		}
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.message.create", "messaging_message", id, map[string]any{"topic_id": input.TopicID.String(), "channel": channel, "recipient_count": len(recipients)}); err != nil {
		return MessagingMessageCreateResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MessagingMessageCreateResult{}, err
	}
	return MessagingMessageCreateResult{Message: item, Created: true}, nil
}

func messagingIdempotencyLockKey(projectID uuid.UUID, idempotencyKey string) int64 {
	digest := sha256.Sum256([]byte(projectID.String() + "\x00" + idempotencyKey))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}

func scanMessagingMessageWithHash(row messagingMessageScanner, hash *[]byte) (domain.MessagingMessage, error) {
	var item domain.MessagingMessage
	var id, projectID uuid.UUID
	var topicID *uuid.UUID
	err := row.Scan(&id, &projectID, &topicID, &item.Channel, &item.Status, &item.RecipientCount, &item.SucceededCount, &item.FailedCount, &item.CancelledAt, &item.CreatedAt, &item.UpdatedAt, hash)
	item.ID = id.String()
	item.ProjectID = projectID.String()
	if topicID != nil {
		value := topicID.String()
		item.TopicID = &value
	}
	return item, err
}

func nullableMessagingIdempotencyKey(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (r *Repository) ListMessagingMessages(ctx context.Context, projectID uuid.UUID, actor MessagingActor, limit int, cursor *uuid.UUID) ([]domain.MessagingMessage, string, bool, error) {
	canManage, err := r.requireMessagingRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if limit < 1 || limit > 100 {
		return nil, "", false, ErrInvalidMessaging
	}
	rows, err := r.pool.Query(ctx, `SELECT `+messagingMessageProjection+` FROM project_messaging_messages WHERE project_id=$1 AND ($3::uuid IS NULL OR id<$3) ORDER BY id DESC LIMIT $2`, projectID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.MessagingMessage, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessagingMessage(rows)
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

func (r *Repository) GetMessagingMessage(ctx context.Context, projectID, messageID uuid.UUID, actor MessagingActor) (domain.MessagingMessage, error) {
	if _, err := r.requireMessagingRead(ctx, projectID, actor); err != nil {
		return domain.MessagingMessage{}, err
	}
	item, err := scanMessagingMessage(r.pool.QueryRow(ctx, `SELECT `+messagingMessageProjection+` FROM project_messaging_messages WHERE project_id=$1 AND id=$2`, projectID, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingMessage{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) ListMessagingDeliveries(ctx context.Context, projectID, messageID uuid.UUID, actor MessagingActor, limit int, cursor *uuid.UUID) ([]domain.MessagingDelivery, string, error) {
	if _, err := r.requireMessagingRead(ctx, projectID, actor); err != nil {
		return nil, "", err
	}
	if limit < 1 || limit > 100 {
		return nil, "", ErrInvalidMessaging
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_messaging_messages WHERE project_id=$1 AND id=$2)`, projectID, messageID).Scan(&exists); err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+messagingDeliveryProjection+` FROM project_messaging_deliveries WHERE project_id=$1 AND message_id=$2 AND ($3::uuid IS NULL OR id<$3) ORDER BY id DESC LIMIT $4`, projectID, messageID, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.MessagingDelivery, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessagingDelivery(rows)
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

func (r *Repository) CancelMessagingMessage(ctx context.Context, projectID, messageID uuid.UUID, actor MessagingActor) (domain.MessagingMessage, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MessagingMessage{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.MessagingMessage{}, err
	}
	// Delivery rows are updated before the message row so cancellation and the
	// worker's delivery claim acquire locks in the same order (delivery, then
	// message), avoiding a tenant-wide queue deadlock.
	if _, err := tx.Exec(ctx, `UPDATE project_messaging_deliveries SET status='cancelled',updated_at=now() WHERE project_id=$1 AND message_id=$2 AND status='pending'`, projectID, messageID); err != nil {
		return domain.MessagingMessage{}, err
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM project_messaging_messages WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, messageID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingMessage{}, ErrNotFound
	} else if err != nil {
		return domain.MessagingMessage{}, err
	}
	if status == "succeeded" || status == "failed" {
		return domain.MessagingMessage{}, ErrMessagingMessageTerminal
	}
	if status != "cancelled" {
		if _, err := tx.Exec(ctx, `UPDATE project_messaging_messages SET status='cancelled',cancelled_at=now(),updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, messageID); err != nil {
			return domain.MessagingMessage{}, err
		}
		if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.message.cancel", "messaging_message", messageID, nil); err != nil {
			return domain.MessagingMessage{}, err
		}
	}
	item, err := scanMessagingMessage(tx.QueryRow(ctx, `SELECT `+messagingMessageProjection+` FROM project_messaging_messages WHERE project_id=$1 AND id=$2`, projectID, messageID))
	if err != nil {
		return domain.MessagingMessage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessagingMessage{}, err
	}
	return item, nil
}

// ClaimNextMessagingDelivery atomically leases one pending recipient. Provider
// and message predicates are tenant-bound in the same query; provider secrets
// remain ciphertext until the trusted worker decrypts them.
