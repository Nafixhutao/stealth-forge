package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// requireInstanceAdminTx gates every instance-scoped admin write. It is shared
// by the admin feature files rather than owned by one of them, so a reader of
// any admin repository file can find the authorization primitive here.
func requireInstanceAdminTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) error {
	if accountID == uuid.Nil {
		return ErrForbidden
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM instance_roles WHERE account_id=$1 AND role IN ('instance_owner','instance_admin'))`, accountID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// writeInstanceAuditTx records an instance-level audit event and publishes it
// on the admin realtime feed inside the caller's transaction.
func writeInstanceAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	action, targetType string,
	target uuid.UUID,
	metadata map[string]any,
) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		`INSERT INTO audit_events (id,organization_id,actor_account_id,action,target_type,target_id,metadata) VALUES ($1,NULL,$2,$3,$4,$5,$6)`,
		id,
		actor,
		action,
		targetType,
		target,
		encoded,
	)
	if err != nil {
		return err
	}
	return enqueueAdminRealtimeEventTx(ctx, tx, action, targetType, target, nil)
}
