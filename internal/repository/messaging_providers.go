package repository

import (
	"context"
	"errors"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateMessagingProvider(ctx context.Context, id, projectID uuid.UUID, actor MessagingActor, input MessagingProviderInput) (domain.MessagingProvider, error) {
	name, err := normalizeMessagingName(input.Name, "name")
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	channel, err := normalizeMessagingChannel(input.Channel)
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	provider, err := normalizeMessagingProvider(input.Provider)
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	ciphertext, credentialsPresent, err := r.encryptMessagingCredentials(input.Credentials)
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.MessagingProvider{}, err
	}
	item, err := scanMessagingProvider(tx.QueryRow(ctx, `
		INSERT INTO project_messaging_providers (id,project_id,name,channel,provider,credentials_ciphertext,credentials_present,enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+messagingProviderProjection, id, projectID, name, channel, provider, ciphertext, credentialsPresent, input.Enabled))
	if err != nil {
		return domain.MessagingProvider{}, mapError(err)
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.provider.create", "messaging_provider", id, map[string]any{
		"name": name, "channel": channel, "provider": provider, "credentials_present": credentialsPresent, "enabled": input.Enabled,
	}); err != nil {
		return domain.MessagingProvider{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessagingProvider{}, err
	}
	return item, nil
}

func (r *Repository) ListMessagingProviders(ctx context.Context, projectID uuid.UUID, actor MessagingActor, limit int, cursor *uuid.UUID) ([]domain.MessagingProvider, string, bool, error) {
	canManage, err := r.requireMessagingRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if limit < 1 || limit > 100 {
		return nil, "", false, ErrInvalidMessaging
	}
	rows, err := r.pool.Query(ctx, `SELECT `+messagingProviderProjection+` FROM project_messaging_providers WHERE project_id=$1 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $2`, projectID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.MessagingProvider, 0, limit)
	for rows.Next() {
		item, scanErr := scanMessagingProvider(rows)
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

func (r *Repository) GetMessagingProvider(ctx context.Context, projectID, providerID uuid.UUID, actor MessagingActor) (domain.MessagingProvider, error) {
	if _, err := r.requireMessagingRead(ctx, projectID, actor); err != nil {
		return domain.MessagingProvider{}, err
	}
	item, err := scanMessagingProvider(r.pool.QueryRow(ctx, `SELECT `+messagingProviderProjection+` FROM project_messaging_providers WHERE project_id=$1 AND id=$2`, projectID, providerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingProvider{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) UpdateMessagingProvider(ctx context.Context, projectID, providerID uuid.UUID, actor MessagingActor, patch MessagingProviderPatch) (domain.MessagingProvider, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.MessagingProvider{}, err
	}
	var name, channel, provider string
	var credentialsCiphertext []byte
	var credentialsPresent, enabled bool
	err = tx.QueryRow(ctx, `SELECT name,channel,provider,credentials_ciphertext,credentials_present,enabled FROM project_messaging_providers WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, providerID).Scan(&name, &channel, &provider, &credentialsCiphertext, &credentialsPresent, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MessagingProvider{}, ErrNotFound
	}
	if err != nil {
		return domain.MessagingProvider{}, err
	}
	originalName, originalChannel, originalProvider := name, channel, provider
	changed := make([]string, 0, 5)
	if patch.Name != nil {
		name, err = normalizeMessagingName(*patch.Name, "name")
		if err != nil {
			return domain.MessagingProvider{}, err
		}
		if name != originalName {
			changed = append(changed, "name")
		}
	}
	if patch.Channel != nil {
		channel, err = normalizeMessagingChannel(*patch.Channel)
		if err != nil {
			return domain.MessagingProvider{}, err
		}
		if channel != originalChannel {
			changed = append(changed, "channel")
		}
	}
	if patch.Provider != nil {
		provider, err = normalizeMessagingProvider(*patch.Provider)
		if err != nil {
			return domain.MessagingProvider{}, err
		}
		if provider != originalProvider {
			changed = append(changed, "provider")
		}
	}
	if patch.Credentials != nil {
		credentialsCiphertext, credentialsPresent, err = r.encryptMessagingCredentials(*patch.Credentials)
		if err != nil {
			return domain.MessagingProvider{}, err
		}
		changed = append(changed, "credentials")
	}
	if patch.Enabled != nil {
		if enabled != *patch.Enabled {
			changed = append(changed, "enabled")
		}
		enabled = *patch.Enabled
	}
	if len(changed) == 0 {
		item, scanErr := scanMessagingProvider(tx.QueryRow(ctx, `SELECT `+messagingProviderProjection+` FROM project_messaging_providers WHERE project_id=$1 AND id=$2`, projectID, providerID))
		if scanErr != nil {
			return domain.MessagingProvider{}, scanErr
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.MessagingProvider{}, err
		}
		return item, nil
	}
	item, err := scanMessagingProvider(tx.QueryRow(ctx, `
		UPDATE project_messaging_providers
		SET name=$3,channel=$4,provider=$5,credentials_ciphertext=$6,credentials_present=$7,enabled=$8,updated_at=now()
		WHERE project_id=$1 AND id=$2
		RETURNING `+messagingProviderProjection, projectID, providerID, name, channel, provider, credentialsCiphertext, credentialsPresent, enabled))
	if err != nil {
		return domain.MessagingProvider{}, mapError(err)
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.provider.update", "messaging_provider", providerID, map[string]any{"fields": changed}); err != nil {
		return domain.MessagingProvider{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MessagingProvider{}, err
	}
	return item, nil
}

func (r *Repository) DeleteMessagingProvider(ctx context.Context, projectID, providerID uuid.UUID, actor MessagingActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.requireMessagingWriteTx(ctx, tx, projectID, actor); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_messaging_providers WHERE project_id=$1 AND id=$2)`, projectID, providerID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_messaging_providers WHERE project_id=$1 AND id=$2`, projectID, providerID); err != nil {
		return err
	}
	if err := r.auditMessaging(ctx, tx, projectID, actor, "messaging.provider.delete", "messaging_provider", providerID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
