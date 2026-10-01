package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateDatabaseRow(ctx context.Context, id, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, input DatabaseRowInput) (domain.DatabaseRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseRow{}, err
	}
	schema, err := loadTableSchema(ctx, tx, projectID, databaseID, tableID)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "create"); err != nil {
		return domain.DatabaseRow{}, err
	}
	item, err := r.createDatabaseRowTx(ctx, tx, projectID, tableID, schema, id, actor, input)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseRow{}, err
	}
	return item, nil
}

func (r *Repository) CreateDatabaseRows(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, inputs []DatabaseBulkRowInput) ([]domain.DatabaseRow, error) {
	if len(inputs) == 0 || len(inputs) > DatabaseRowBulkImportMaxRows {
		return nil, fmt.Errorf("%w: import rows must contain between 1 and %d items", dbcore.ErrInvalidRow, DatabaseRowBulkImportMaxRows)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return nil, err
	}
	schema, err := loadTableSchema(ctx, tx, projectID, databaseID, tableID)
	if err != nil {
		return nil, err
	}
	if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "create"); err != nil {
		return nil, err
	}
	items := make([]domain.DatabaseRow, 0, len(inputs))
	for _, input := range inputs {
		id := input.ID
		if id == uuid.Nil {
			id = uuid.Must(uuid.NewV7())
		}
		item, err := r.createDatabaseRowTx(ctx, tx, projectID, tableID, schema, id, actor, DatabaseRowInput{
			Data: input.Data, ReadPermissions: input.ReadPermissions,
			UpdatePermissions: input.UpdatePermissions, DeletePermissions: input.DeletePermissions,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

// TransactDatabaseRows applies a bounded sequence of creates, updates, and
// deletes under one database transaction. The database namespace lock also
// serializes this batch with relationship creation and target-row deletion.
func (r *Repository) TransactDatabaseRows(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, operations []DatabaseRowTransactionOperation) (DatabaseRowTransactionResult, error) {
	if len(operations) == 0 || len(operations) > DatabaseRowTransactionMaxOps {
		return DatabaseRowTransactionResult{}, fmt.Errorf("%w: transaction operations must contain between 1 and %d items", dbcore.ErrInvalidRow, DatabaseRowTransactionMaxOps)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DatabaseRowTransactionResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return DatabaseRowTransactionResult{}, err
	}
	schema, err := loadTableSchema(ctx, tx, projectID, databaseID, tableID)
	if err != nil {
		return DatabaseRowTransactionResult{}, err
	}
	result := DatabaseRowTransactionResult{
		Rows:       make([]domain.DatabaseRow, 0, len(operations)),
		DeletedIDs: make([]string, 0),
	}
	for _, operation := range operations {
		switch operation.Action {
		case "create":
			if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "create"); err != nil {
				return DatabaseRowTransactionResult{}, err
			}
			id := operation.ID
			if id == uuid.Nil {
				id = uuid.Must(uuid.NewV7())
			}
			item, err := r.createDatabaseRowTx(ctx, tx, projectID, tableID, schema, id, actor, DatabaseRowInput{
				Data: operation.Data, ReadPermissions: operation.ReadPermissions,
				UpdatePermissions: operation.UpdatePermissions, DeletePermissions: operation.DeletePermissions,
			})
			if err != nil {
				return DatabaseRowTransactionResult{}, err
			}
			result.Rows = append(result.Rows, item)
		case "update":
			if operation.ID == uuid.Nil {
				return DatabaseRowTransactionResult{}, fmt.Errorf("%w: update operation requires id", dbcore.ErrInvalidRow)
			}
			if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "update"); err != nil {
				return DatabaseRowTransactionResult{}, err
			}
			item, err := r.updateDatabaseRowTx(ctx, tx, projectID, tableID, operation.ID, actor, schema, DatabaseRowPatch{
				Data: operation.Data, ReadPermissions: operation.ReadPermissions,
				UpdatePermissions: operation.UpdatePermissions, DeletePermissions: operation.DeletePermissions,
			})
			if err != nil {
				return DatabaseRowTransactionResult{}, err
			}
			result.Rows = append(result.Rows, item)
		case "delete":
			if operation.ID == uuid.Nil {
				return DatabaseRowTransactionResult{}, fmt.Errorf("%w: delete operation requires id", dbcore.ErrInvalidRow)
			}
			if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "delete"); err != nil {
				return DatabaseRowTransactionResult{}, err
			}
			if err := r.deleteDatabaseRowTx(ctx, tx, projectID, tableID, operation.ID, actor, schema); err != nil {
				return DatabaseRowTransactionResult{}, err
			}
			result.DeletedIDs = append(result.DeletedIDs, operation.ID.String())
		default:
			return DatabaseRowTransactionResult{}, fmt.Errorf("%w: transaction action must be create, update, or delete", dbcore.ErrInvalidRow)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DatabaseRowTransactionResult{}, err
	}
	return result, nil
}

func (r *Repository) createDatabaseRowTx(ctx context.Context, tx pgx.Tx, projectID, tableID uuid.UUID, schema DatabaseTableSchema, id uuid.UUID, actor DatabaseActor, input DatabaseRowInput) (domain.DatabaseRow, error) {
	data, err := dbcore.NormalizeCreate(input.Data, columnDefinitions(schema.Columns))
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := validateDatabaseRowRelationshipsTx(ctx, tx, tableID, data); err != nil {
		return domain.DatabaseRow{}, err
	}
	if actor.Kind == DatabaseAnonymousActor && (input.ReadPermissions == nil || input.UpdatePermissions == nil || input.DeletePermissions == nil) {
		return domain.DatabaseRow{}, fmt.Errorf("%w: anonymous rows must specify read, update, and delete permissions", dbcore.ErrInvalidPermissions)
	}
	readPermissions, err := normalizeRowPermissions(input.ReadPermissions, actor, true)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	updatePermissions, err := normalizeRowPermissions(input.UpdatePermissions, actor, true)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	deletePermissions, err := normalizeRowPermissions(input.DeletePermissions, actor, true)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	var creator any
	if actor.Kind == DatabaseApplicationActor {
		creator = actor.ProjectUserID
	}
	item, err := scanRow(tx.QueryRow(ctx, `INSERT INTO database_rows (id,table_id,project_id,data,read_permissions,update_permissions,delete_permissions,creator_project_user_id) VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8) RETURNING `+rowProjectionNoAlias, id, tableID, projectID, dataJSON, readPermissions, updatePermissions, deletePermissions, creator))
	if err != nil {
		return domain.DatabaseRow{}, mapError(err)
	}
	changed := make([]string, 0, len(data))
	for key := range data {
		changed = append(changed, key)
	}
	metadata := buildDatabaseRowEventMetadata(actor, schema.Table, item.ReadPermissions, changed)
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_row.create", "database_row", id, metadata); err != nil {
		return domain.DatabaseRow{}, err
	}
	return item, nil
}

func (r *Repository) UpdateDatabaseRow(ctx context.Context, projectID, databaseID, tableID, rowID uuid.UUID, actor DatabaseActor, input DatabaseRowPatch) (domain.DatabaseRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseRow{}, err
	}
	schema, err := loadTableSchema(ctx, tx, projectID, databaseID, tableID)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := authorizeRowOperationTx(ctx, tx, schema.Table, actor, "update"); err != nil {
		return domain.DatabaseRow{}, err
	}
	item, err := r.updateDatabaseRowTx(ctx, tx, projectID, tableID, rowID, actor, schema, input)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseRow{}, err
	}
	return item, nil
}

func (r *Repository) updateDatabaseRowTx(ctx context.Context, tx pgx.Tx, projectID, tableID, rowID uuid.UUID, actor DatabaseActor, schema DatabaseTableSchema, input DatabaseRowPatch) (domain.DatabaseRow, error) {
	selectSQL := `SELECT ` + rowProjection + ` FROM database_rows r WHERE r.project_id=$1 AND r.table_id=$2 AND r.id=$3`
	selectArgs := []any{projectID, tableID, rowID}
	if actor.IsApplication() && schema.Table.RowSecurity && !tablePermission(schema.Table.UpdatePermissions, actor) {
		selectSQL += ` AND ` + rowPermissionSQL("r.update_permissions", actor, &selectArgs)
	}
	selectSQL += ` FOR UPDATE`
	existing, err := scanRow(tx.QueryRow(ctx, selectSQL, selectArgs...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatabaseRow{}, ErrNotFound
	}
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	data, changed, err := dbcore.NormalizeUpdate(existing.Data, input.Data, columnDefinitions(schema.Columns))
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := validateDatabaseRowRelationshipsTx(ctx, tx, tableID, data); err != nil {
		return domain.DatabaseRow{}, err
	}
	readPermissions := existing.ReadPermissions
	updatePermissions := existing.UpdatePermissions
	deletePermissions := existing.DeletePermissions
	if actor.IsApplication() && (input.ReadPermissions != nil || input.UpdatePermissions != nil || input.DeletePermissions != nil) {
		return domain.DatabaseRow{}, ErrForbidden
	}
	if input.ReadPermissions != nil {
		readPermissions, err = normalizeRowPermissions(input.ReadPermissions, actor, false)
		if err != nil {
			return domain.DatabaseRow{}, err
		}
		changed = append(changed, "read_permissions")
	}
	if input.UpdatePermissions != nil {
		updatePermissions, err = normalizeRowPermissions(input.UpdatePermissions, actor, false)
		if err != nil {
			return domain.DatabaseRow{}, err
		}
		changed = append(changed, "update_permissions")
	}
	if input.DeletePermissions != nil {
		deletePermissions, err = normalizeRowPermissions(input.DeletePermissions, actor, false)
		if err != nil {
			return domain.DatabaseRow{}, err
		}
		changed = append(changed, "delete_permissions")
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	item, err := scanRow(tx.QueryRow(ctx, `UPDATE database_rows SET data=$4::jsonb,read_permissions=$5,update_permissions=$6,delete_permissions=$7,updated_at=now() WHERE project_id=$1 AND table_id=$2 AND id=$3 RETURNING `+rowProjectionNoAlias, projectID, tableID, rowID, dataJSON, readPermissions, updatePermissions, deletePermissions))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatabaseRow{}, ErrNotFound
	}
	if err != nil {
		return domain.DatabaseRow{}, mapError(err)
	}
	metadata := buildDatabaseRowEventMetadata(actor, schema.Table, item.ReadPermissions, changed)
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_row.update", "database_row", rowID, metadata); err != nil {
		return domain.DatabaseRow{}, err
	}
	return item, nil
}

func columnDefinitions(columns []DatabaseColumnSchema) []dbcore.ColumnDefinition {
	definitions := make([]dbcore.ColumnDefinition, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, dbcore.ColumnDefinition{
			Key: column.Key, Type: column.Type, Required: column.Required,
			VarcharSize: column.VarcharSize, Default: column.Default, HasDefault: column.HasDefault,
		})
	}
	return definitions
}
