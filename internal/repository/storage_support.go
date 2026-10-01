package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func lockStorageNamespace(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "storage:"+projectID.String())
	return err
}

func (r *Repository) auditStorage(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor StorageActor, action, targetType string, target uuid.UUID, metadata map[string]any) error {
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["project_id"] = projectID.String()
	switch actor.Kind {
	case StorageAPIKeyActor:
		metadata["actor"] = "api_key"
		metadata["api_key_id"] = actor.APIKeyID.String()
	case StorageApplicationActor:
		metadata["actor"] = "project_user"
		metadata["project_user_id"] = actor.ProjectUserID.String()
		metadata["source"] = "application"
	case StorageAnonymousActor:
		metadata["actor"] = "anonymous"
		metadata["source"] = "application"
	}
	actorID := uuid.Nil
	if actor.Kind == StorageConsoleActor {
		actorID = actor.AccountID
	}
	if err := writeAuditMetadata(ctx, tx, orgID, actorID, action, targetType, target, metadata); err != nil {
		return err
	}
	return r.enqueueWebhookEventTx(ctx, tx, projectID, action, targetType, target, metadata)
}
