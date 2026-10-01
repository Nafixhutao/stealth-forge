package repository

import (
	"context"
	"errors"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) DeleteDatabaseRow(ctx context.Context, projectID, databaseID, tableID, rowID uuid.UUID, actor DatabaseActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return err
	}
	schema, err := loadTableSchema(ctx, tx, projectID, databaseID, tableID)
	if err != nil {
		return err
	}
	if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "delete"); err != nil {
		return err
	}
	if err := r.deleteDatabaseRowTx(ctx, tx, projectID, tableID, rowID, actor, schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) deleteDatabaseRowTx(ctx context.Context, tx pgx.Tx, projectID, tableID, rowID uuid.UUID, actor DatabaseActor, schema DatabaseTableSchema) error {
	selectSQL := `SELECT ` + rowProjection + ` FROM database_rows r WHERE r.project_id=$1 AND r.table_id=$2 AND r.id=$3`
	selectArgs := []any{projectID, tableID, rowID}
	if actor.IsApplication() && schema.Table.RowSecurity && !tablePermission(schema.Table.DeletePermissions, actor) {
		selectSQL += ` AND ` + rowPermissionSQL("r.delete_permissions", actor, &selectArgs)
	}
	item, err := scanRow(tx.QueryRow(ctx, selectSQL+` FOR UPDATE`, selectArgs...))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := ensureNoDatabaseRowReferencesTx(ctx, tx, tableID, rowID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM database_rows WHERE project_id=$1 AND table_id=$2 AND id=$3`, projectID, tableID, rowID); err != nil {
		return err
	}
	metadata := buildDatabaseRowEventMetadata(actor, schema.Table, item.ReadPermissions, nil)
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_row.delete", "database_row", rowID, metadata); err != nil {
		return err
	}
	return nil
}

func authorizeRowOperationTx(ctx context.Context, tx pgx.Tx, table domain.DatabaseTable, actor DatabaseActor, operation string) error {
	switch actor.Kind {
	case DatabaseConsoleActor:
		if operation != "read" {
			if actor.AccountID == uuid.Nil {
				return ErrForbidden
			}
			return requireProjectRoleTx(ctx, tx, mustParseUUID(table.ProjectID), actor.AccountID, "owner", "admin")
		}
		return requireProjectAccessTx(ctx, tx, mustParseUUID(table.ProjectID), actor.AccountID)
	case DatabaseAPIKeyActor:
		scope := "databases.read"
		if operation != "read" {
			scope = "databases.write"
		}
		if !apikey.HasScope(actor.APIKeyScopes, scope) {
			return ErrForbidden
		}
		return requireActiveProjectAPIKeyTx(ctx, tx, mustParseUUID(table.ProjectID), actor.APIKeyID, scope)
	case DatabaseApplicationActor, DatabaseAnonymousActor:
		var permissions []string
		switch operation {
		case "create":
			permissions = table.CreatePermissions
		case "read":
			permissions = table.ReadPermissions
		case "update":
			permissions = table.UpdatePermissions
		case "delete":
			permissions = table.DeletePermissions
		default:
			return ErrForbidden
		}
		if tablePermission(permissions, actor) {
			return nil
		}
		// With row security enabled a row grant may authorize update/delete
		// (and read) even when the table grant is absent. Creation is always
		// gated by the table's create permissions.
		if operation == "create" || !table.RowSecurity {
			return ErrForbidden
		}
		return nil
	default:
		return ErrForbidden
	}
}

func requireProjectAccessTx(ctx context.Context, tx pgx.Tx, projectID, accountID uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN organization_memberships m ON m.organization_id=p.organization_id WHERE p.id=$1 AND m.account_id=$2)`, projectID, accountID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
