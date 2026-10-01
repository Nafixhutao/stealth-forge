package repository

import (
	"context"
	"errors"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateMessagingTopic(ctx context.Context, id, projectID uuid.UUID, actor MessagingActor, input MessagingTopicInput) (domain.MessagingTopic, error) {
	name, err := normalizeMessagingName(input.Name, "name")
	if err != nil {
		return domain.MessagingTopic{}, err
	}
	description, err := normalizeMessagingDescription(input.Description)
	if err != nil {
		return domain.MessagingTopic{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MessagingTopic{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.MessagingTopic{}, err
	}
	item, err := scanMessagingTopic(tx.QueryRow(ctx, `
		INSERT INTO project_messaging_topics (id,project_id,name,description,enabled)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING `+messagingTopicProjection, id, projectID, name, description, input.Enabled))
	if err != nil {
		return domain.MessagingTopic{}, mapError(err)
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.topic.create", "messaging_topic", id, map[string]any{"name": name, "enabled": input.Enabled}); err != nil {
		return domain.MessagingTopic{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessagingTopic{}, err
	}
	return item, nil
}

func (r *Repository) ListMessagingTopics(ctx context.Context, projectID uuid.UUID, actor MessagingActor, limit int, cursor *uuid.UUID) ([]domain.MessagingTopic, string, bool, error) {
	canManage, err := r.requireMessagingRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if limit < 1 || limit > 100 {
		return nil, "", false, ErrInvalidMessaging
	}
	rows, err := r.pool.Query(ctx, `SELECT `+messagingTopicListProjection+` FROM project_messaging_topics t WHERE t.project_id=$1 AND ($3::uuid IS NULL OR t.id>$3) ORDER BY t.id LIMIT $2`, projectID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.MessagingTopic, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessagingTopic(rows)
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

func (r *Repository) GetMessagingTopic(ctx context.Context, projectID, topicID uuid.UUID, actor MessagingActor) (domain.MessagingTopic, error) {
	if _, err := r.requireMessagingRead(ctx, projectID, actor); err != nil {
		return domain.MessagingTopic{}, err
	}
	item, err := scanMessagingTopic(r.pool.QueryRow(ctx, `SELECT `+messagingTopicProjection+` FROM project_messaging_topics t WHERE t.project_id=$1 AND t.id=$2`, projectID, topicID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingTopic{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) UpdateMessagingTopic(ctx context.Context, projectID, topicID uuid.UUID, actor MessagingActor, patch MessagingTopicPatch) (domain.MessagingTopic, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MessagingTopic{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.MessagingTopic{}, err
	}
	var name, description string
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT name,description,enabled FROM project_messaging_topics WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, topicID).Scan(&name, &description, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingTopic{}, ErrNotFound
	}
	if err != nil {
		return domain.MessagingTopic{}, err
	}
	changed := make([]string, 0, 3)
	if patch.Name != nil {
		normalized, normalizeErr := normalizeMessagingName(*patch.Name, "name")
		if normalizeErr != nil {
			return domain.MessagingTopic{}, normalizeErr
		}
		if normalized != name {
			changed = append(changed, "name")
		}
		name = normalized
	}
	if patch.Description != nil {
		normalized, normalizeErr := normalizeMessagingDescription(*patch.Description)
		if normalizeErr != nil {
			return domain.MessagingTopic{}, normalizeErr
		}
		if normalized != description {
			changed = append(changed, "description")
		}
		description = normalized
	}
	if patch.Enabled != nil {
		if enabled != *patch.Enabled {
			changed = append(changed, "enabled")
		}
		enabled = *patch.Enabled
	}
	if len(changed) == 0 {
		item, scanErr := scanMessagingTopic(tx.QueryRow(ctx, `SELECT `+messagingTopicProjection+` FROM project_messaging_topics t WHERE t.project_id=$1 AND t.id=$2`, projectID, topicID))
		if scanErr != nil {
			return domain.MessagingTopic{}, scanErr
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.MessagingTopic{}, err
		}
		return item, nil
	}
	item, err := scanMessagingTopic(tx.QueryRow(ctx, `
		UPDATE project_messaging_topics
		SET name=$3,description=$4,enabled=$5,updated_at=now()
		WHERE project_id=$1 AND id=$2
		RETURNING `+messagingTopicProjection, projectID, topicID, name, description, enabled))
	if err != nil {
		return domain.MessagingTopic{}, mapError(err)
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.topic.update", "messaging_topic", topicID, map[string]any{"fields": changed}); err != nil {
		return domain.MessagingTopic{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessagingTopic{}, err
	}
	return item, nil
}

func (r *Repository) DeleteMessagingTopic(ctx context.Context, projectID, topicID uuid.UUID, actor MessagingActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_messaging_topics WHERE project_id=$1 AND id=$2)`, projectID, topicID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_messaging_topics WHERE project_id=$1 AND id=$2`, projectID, topicID); err != nil {
		return err
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.topic.delete", "messaging_topic", topicID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
