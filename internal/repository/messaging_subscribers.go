package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateMessagingSubscriber(ctx context.Context, id, projectID, topicID uuid.UUID, actor MessagingActor, input MessagingSubscriberInput) (domain.MessagingSubscriber, error) {
	channel, err := normalizeMessagingChannel(input.Channel)
	if err != nil {
		return domain.MessagingSubscriber{}, err
	}
	address, preview, addressHash, err := normalizeMessagingAddress(channel, input.Address)
	if err != nil {
		return domain.MessagingSubscriber{}, err
	}
	if r.messagingCipher == nil {
		return domain.MessagingSubscriber{}, ErrMessagingNotReady
	}
	addressCiphertext, err := r.messagingCipher.Encrypt([]byte(address))
	if err != nil {
		return domain.MessagingSubscriber{}, fmt.Errorf("%w: %v", ErrMessagingNotReady, err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MessagingSubscriber{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.MessagingSubscriber{}, err
	}
	item, err := scanMessagingSubscriber(tx.QueryRow(ctx, `
		INSERT INTO project_messaging_subscribers (id,project_id,topic_id,channel,address_ciphertext,address_hash,address_preview,enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+messagingSubscriberProjection, id, projectID, topicID, channel, addressCiphertext, addressHash, preview, input.Enabled))
	if err != nil {
		return domain.MessagingSubscriber{}, mapError(err)
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.subscriber.create", "messaging_subscriber", id, map[string]any{"topic_id": topicID.String(), "channel": channel, "address_preview": preview, "enabled": input.Enabled}); err != nil {
		return domain.MessagingSubscriber{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessagingSubscriber{}, err
	}
	return item, nil
}

func (r *Repository) ListMessagingSubscribers(ctx context.Context, projectID, topicID uuid.UUID, actor MessagingActor, limit int, cursor *uuid.UUID) ([]domain.MessagingSubscriber, string, bool, error) {
	canManage, err := r.requireMessagingRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if limit < 1 || limit > 100 {
		return nil, "", false, ErrInvalidMessaging
	}
	var topicExists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_messaging_topics WHERE project_id=$1 AND id=$2)`, projectID, topicID).Scan(&topicExists); err != nil {
		return nil, "", false, err
	}
	if !topicExists {
		return nil, "", false, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+messagingSubscriberProjection+` FROM project_messaging_subscribers WHERE project_id=$1 AND topic_id=$2 AND ($4::uuid IS NULL OR id>$4) ORDER BY id LIMIT $3`, projectID, topicID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.MessagingSubscriber, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessagingSubscriber(rows)
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

func (r *Repository) GetMessagingSubscriber(ctx context.Context, projectID, topicID, subscriberID uuid.UUID, actor MessagingActor) (domain.MessagingSubscriber, error) {
	if _, err := r.requireMessagingRead(ctx, projectID, actor); err != nil {
		return domain.MessagingSubscriber{}, err
	}
	item, err := scanMessagingSubscriber(r.pool.QueryRow(ctx, `SELECT `+messagingSubscriberProjection+` FROM project_messaging_subscribers WHERE project_id=$1 AND topic_id=$2 AND id=$3`, projectID, topicID, subscriberID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingSubscriber{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) DeleteMessagingSubscriber(ctx context.Context, projectID, topicID, subscriberID uuid.UUID, actor MessagingActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_messaging_subscribers WHERE project_id=$1 AND topic_id=$2 AND id=$3)`, projectID, topicID, subscriberID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_messaging_subscribers WHERE project_id=$1 AND topic_id=$2 AND id=$3`, projectID, topicID, subscriberID); err != nil {
		return err
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.subscriber.delete", "messaging_subscriber", subscriberID, map[string]any{"topic_id": topicID.String()}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MessagingProviderCredentials is an internal worker projection. It is kept
// separate from domain.MessagingProvider so handlers cannot accidentally
// serialize the decrypted secret material.
