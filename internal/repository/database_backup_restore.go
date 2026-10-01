package repository

import (
	"context"
	"encoding/json"
	"strings"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) RestoreDatabaseBackup(ctx context.Context, projectID, databaseID uuid.UUID, actor DatabaseActor, snapshot DatabaseBackupSnapshot) (DatabaseBackupRestoreResult, error) {
	if snapshot.Version != DatabaseBackupVersion || snapshot.ProjectID != projectID.String() || snapshot.DatabaseID != databaseID.String() || len(snapshot.Tables) > DatabaseBackupMaxTables || len(snapshot.Relationships) > DatabaseBackupMaxRelations {
		return DatabaseBackupRestoreResult{}, ErrInvalidBackup
	}
	rowCount := 0
	for _, table := range snapshot.Tables {
		rowCount += len(table.Rows)
	}
	if rowCount > DatabaseBackupMaxRows {
		return DatabaseBackupRestoreResult{}, ErrBackupTooLarge
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	if err := ensureDatabaseProjectTx(ctx, tx, projectID, databaseID); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	if err := dropIndexesForDatabase(ctx, tx, databaseID); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM database_tables WHERE project_id=$1 AND database_id=$2`, projectID, databaseID); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	tableSchemas := make(map[uuid.UUID]DatabaseTableSchema, len(snapshot.Tables))
	result := DatabaseBackupRestoreResult{}
	for _, backupTable := range snapshot.Tables {
		tableID, err := validateBackupUUID(backupTable.Table.ID)
		if err != nil || backupTable.Table.DatabaseID != databaseID.String() || backupTable.Table.ProjectID != projectID.String() {
			return DatabaseBackupRestoreResult{}, ErrInvalidBackup
		}
		name, err := dbcore.ValidateName(backupTable.Table.Name)
		if err != nil {
			return DatabaseBackupRestoreResult{}, ErrInvalidBackup
		}
		permissions, err := normalizeTablePermissions(DatabaseTableInput{CreatePermissions: backupTable.Table.CreatePermissions, ReadPermissions: backupTable.Table.ReadPermissions, UpdatePermissions: backupTable.Table.UpdatePermissions, DeletePermissions: backupTable.Table.DeletePermissions})
		if err != nil {
			return DatabaseBackupRestoreResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO database_tables (id,database_id,project_id,name,row_security,create_permissions,read_permissions,update_permissions,delete_permissions,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, tableID, databaseID, projectID, name, backupTable.Table.RowSecurity, permissions[0], permissions[1], permissions[2], permissions[3], backupTime(backupTable.Table.CreatedAt), backupTime(backupTable.Table.UpdatedAt)); err != nil {
			return DatabaseBackupRestoreResult{}, mapError(err)
		}
		schema := DatabaseTableSchema{Table: backupTable.Table, Columns: make([]DatabaseColumnSchema, 0, len(backupTable.Columns))}
		for _, backupColumn := range backupTable.Columns {
			columnID, err := validateBackupUUID(backupColumn.ID)
			if err != nil || backupColumn.TableID != tableID.String() {
				return DatabaseBackupRestoreResult{}, ErrInvalidBackup
			}
			var defaultValue any
			hasDefault := len(backupColumn.Default) > 0 && string(backupColumn.Default) != "null"
			if hasDefault {
				decoder := json.NewDecoder(strings.NewReader(string(backupColumn.Default)))
				decoder.UseNumber()
				if err := decoder.Decode(&defaultValue); err != nil {
					return DatabaseBackupRestoreResult{}, ErrInvalidBackup
				}
			}
			column := DatabaseColumnInput{Key: backupColumn.Key, Type: dbcore.ColumnType(backupColumn.Type), Required: backupColumn.Required, VarcharSize: backupColumn.VarcharSize, Default: defaultValue, HasDefault: hasDefault}
			if err := dbcore.ValidateColumn(dbcore.ColumnDefinition{Key: column.Key, Type: column.Type, Required: column.Required, VarcharSize: column.VarcharSize, Default: column.Default, HasDefault: column.HasDefault}); err != nil {
				return DatabaseBackupRestoreResult{}, ErrInvalidBackup
			}
			defaultJSON := []byte(nil)
			if hasDefault {
				defaultJSON = backupColumn.Default
			}
			if _, err := tx.Exec(ctx, `INSERT INTO database_columns (id,table_id,key,column_type,required,varchar_size,default_value,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)`, columnID, tableID, column.Key, column.Type, column.Required, column.VarcharSize, defaultJSON, backupTime(backupColumn.CreatedAt), backupTime(backupColumn.UpdatedAt)); err != nil {
				return DatabaseBackupRestoreResult{}, mapError(err)
			}
			schema.Columns = append(schema.Columns, DatabaseColumnSchema{ID: columnID, Key: column.Key, Type: column.Type, Required: column.Required, VarcharSize: column.VarcharSize, Default: defaultValue, HasDefault: hasDefault})
			result.Columns++
		}
		tableSchemas[tableID] = schema
		result.Tables++
	}
	for _, backupTable := range snapshot.Tables {
		tableID := mustParseUUID(backupTable.Table.ID)
		schema := tableSchemas[tableID]
		for _, backupRow := range backupTable.Rows {
			rowID, err := validateBackupUUID(backupRow.ID)
			if err != nil || backupRow.TableID != tableID.String() || backupRow.ProjectID != projectID.String() {
				return DatabaseBackupRestoreResult{}, ErrInvalidBackup
			}
			data, err := dbcore.NormalizeCreate(backupRow.Data, columnDefinitions(schema.Columns))
			if err != nil {
				return DatabaseBackupRestoreResult{}, ErrInvalidBackup
			}
			readPermissions, err := dbcore.NormalizePermissions(backupRow.ReadPermissions)
			if err != nil {
				return DatabaseBackupRestoreResult{}, err
			}
			updatePermissions, err := dbcore.NormalizePermissions(backupRow.UpdatePermissions)
			if err != nil {
				return DatabaseBackupRestoreResult{}, err
			}
			deletePermissions, err := dbcore.NormalizePermissions(backupRow.DeletePermissions)
			if err != nil {
				return DatabaseBackupRestoreResult{}, err
			}
			dataJSON, err := json.Marshal(data)
			if err != nil {
				return DatabaseBackupRestoreResult{}, err
			}
			var creator any
			if backupRow.CreatorProjectUserID != nil {
				creatorID, parseErr := uuid.Parse(*backupRow.CreatorProjectUserID)
				if parseErr != nil || creatorID == uuid.Nil {
					return DatabaseBackupRestoreResult{}, ErrInvalidBackup
				}
				creator = creatorID
			}
			if _, err := tx.Exec(ctx, `INSERT INTO database_rows (id,table_id,project_id,data,read_permissions,update_permissions,delete_permissions,creator_project_user_id,created_at,updated_at) VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10)`, rowID, tableID, projectID, dataJSON, readPermissions, updatePermissions, deletePermissions, creator, backupTime(backupRow.CreatedAt), backupTime(backupRow.UpdatedAt)); err != nil {
				return DatabaseBackupRestoreResult{}, mapError(err)
			}
			result.Rows++
		}
	}
	for _, relationship := range snapshot.Relationships {
		relationshipID, err := validateBackupUUID(relationship.ID)
		if err != nil || relationship.ProjectID != projectID.String() || relationship.DatabaseID != databaseID.String() {
			return DatabaseBackupRestoreResult{}, ErrInvalidBackup
		}
		sourceTableUUID, sourceErr := validateBackupUUID(relationship.SourceTableID)
		targetTableUUID, targetErr := validateBackupUUID(relationship.TargetTableID)
		if sourceErr != nil || targetErr != nil {
			return DatabaseBackupResultInvalid()
		}
		sourceTable, sourceOK := tableSchemas[sourceTableUUID]
		_, targetOK := tableSchemas[targetTableUUID]
		if !sourceOK || !targetOK || relationship.RelationshipType != DatabaseRelationshipManyToOne || relationship.OnDelete != DatabaseRelationshipRestrict {
			return DatabaseBackupResultInvalid()
		}
		var sourceColumn *DatabaseColumnSchema
		for i := range sourceTable.Columns {
			if sourceTable.Columns[i].Key == relationship.SourceColumnKey {
				sourceColumn = &sourceTable.Columns[i]
				break
			}
		}
		if sourceColumn == nil || (sourceColumn.Type != dbcore.TypeText && sourceColumn.Type != dbcore.TypeVarchar) {
			return DatabaseBackupResultInvalid()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO database_relationships (id,project_id,database_id,source_table_id,source_column_key,target_table_id,relationship_type,on_delete) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, relationshipID, projectID, databaseID, sourceTableUUID, relationship.SourceColumnKey, targetTableUUID, relationship.RelationshipType, relationship.OnDelete); err != nil {
			return DatabaseBackupResultInvalidWithError(err)
		}
		result.Relationships++
	}
	for _, relationship := range snapshot.Relationships {
		sourceTableID, err := validateBackupUUID(relationship.SourceTableID)
		if err != nil {
			return DatabaseBackupResultInvalid()
		}
		if err := validateDatabaseRowRelationshipsForTableTx(ctx, tx, sourceTableID); err != nil {
			return DatabaseBackupResultInvalidWithError(err)
		}
	}
	for _, backupTable := range snapshot.Tables {
		tableID := mustParseUUID(backupTable.Table.ID)
		schema := tableSchemas[tableID]
		byKey := make(map[string]DatabaseColumnSchema, len(schema.Columns))
		for _, column := range schema.Columns {
			byKey[column.Key] = column
		}
		for _, backupIndex := range backupTable.Indexes {
			indexID, err := validateBackupUUID(backupIndex.ID)
			if err != nil || backupIndex.TableID != tableID.String() {
				return DatabaseBackupResultInvalid()
			}
			input := DatabaseIndexInput{Name: backupIndex.Name, Type: backupIndex.Type, ColumnKeys: backupIndex.ColumnKeys, Directions: backupIndex.Directions}
			if len(input.Directions) == 0 && len(input.ColumnKeys) > 0 {
				input.Directions = make([]string, len(input.ColumnKeys))
				for i := range input.Directions {
					input.Directions[i] = "asc"
				}
			}
			if err := validateBackupIndex(input, byKey); err != nil {
				return DatabaseBackupResultInvalidWithError(err)
			}
			ddl, err := buildIndexDDL(internalIndexName(indexID), tableID, input, byKey)
			if err != nil {
				return DatabaseBackupResultInvalidWithError(err)
			}
			if _, err := tx.Exec(ctx, ddl); err != nil {
				return DatabaseBackupResultInvalidWithError(mapError(err))
			}
			if _, err := tx.Exec(ctx, `INSERT INTO database_indexes (id,table_id,name,index_type,column_keys,directions,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, indexID, tableID, input.Name, input.Type, input.ColumnKeys, input.Directions, backupTime(backupIndex.CreatedAt), backupTime(backupIndex.UpdatedAt)); err != nil {
				return DatabaseBackupResultInvalidWithError(mapError(err))
			}
			result.Indexes++
		}
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_backup.restore", "project_database", databaseID, map[string]any{
		"database_id":   databaseID.String(),
		"tables":        result.Tables,
		"rows":          result.Rows,
		"relationships": result.Relationships,
	}); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DatabaseBackupRestoreResult{}, err
	}
	return result, nil
}

// These small helpers keep restore validation errors mapped through the same
// resource error path while preserving the original PostgreSQL conflict when
// one is available.
func DatabaseBackupResultInvalid() (DatabaseBackupRestoreResult, error) {
	return DatabaseBackupRestoreResult{}, ErrInvalidBackup
}

func DatabaseBackupResultInvalidWithError(err error) (DatabaseBackupRestoreResult, error) {
	if err == nil {
		return DatabaseBackupRestoreResult{}, ErrInvalidBackup
	}
	return DatabaseBackupRestoreResult{}, err
}

func validateBackupIndex(input DatabaseIndexInput, columns map[string]DatabaseColumnSchema) error {
	if _, err := dbcore.ValidateName(input.Name); err != nil {
		return err
	}
	if input.Type != "key" && input.Type != "unique" && input.Type != "fulltext" {
		return ErrInvalidBackup
	}
	if len(input.ColumnKeys) == 0 || len(input.ColumnKeys) > 16 || len(input.ColumnKeys) != len(input.Directions) {
		return ErrInvalidBackup
	}
	if input.Type == "fulltext" && len(input.ColumnKeys) != 1 {
		return ErrInvalidBackup
	}
	seen := make(map[string]struct{}, len(input.ColumnKeys))
	for i, key := range input.ColumnKeys {
		if _, err := dbcore.ValidateIdentifier(key); err != nil {
			return err
		}
		if _, exists := seen[key]; exists {
			return ErrInvalidBackup
		}
		seen[key] = struct{}{}
		column, exists := columns[key]
		if !exists {
			return ErrInvalidBackup
		}
		direction := strings.ToLower(input.Directions[i])
		if direction != "asc" && direction != "desc" || input.Type == "fulltext" && direction != "asc" {
			return ErrInvalidBackup
		}
		if input.Type == "fulltext" && column.Type != dbcore.TypeText && column.Type != dbcore.TypeVarchar {
			return ErrInvalidBackup
		}
	}
	return nil
}

func validateDatabaseRowRelationshipsForTableTx(ctx context.Context, tx pgx.Tx, tableID uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT data FROM database_rows WHERE table_id=$1`, tableID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var data map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&data); err != nil {
			return err
		}
		if err := validateDatabaseRowRelationshipsTx(ctx, tx, tableID, data); err != nil {
			return err
		}
	}
	return rows.Err()
}
