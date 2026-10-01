package repository

import (
	"context"
	"encoding/json"
	"errors"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListDatabaseColumns(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, limit int, cursor *uuid.UUID) ([]domain.DatabaseColumn, string, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
		return nil, "", err
	}
	if err := r.ensureTableProject(ctx, projectID, databaseID, tableID); err != nil {
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+columnProjection()+` FROM database_columns WHERE table_id=$1 AND ($2::uuid IS NULL OR id>$2) ORDER BY id LIMIT $3`, tableID, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.DatabaseColumn, 0, limit)
	for rows.Next() {
		item, scanErr := scanColumn(rows)
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

func (r *Repository) CreateDatabaseColumn(ctx context.Context, id, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, input DatabaseColumnInput) (domain.DatabaseColumn, error) {
	if err := dbcore.ValidateColumn(dbcore.ColumnDefinition{Key: input.Key, Type: input.Type, Required: input.Required, VarcharSize: input.VarcharSize, Default: input.Default, HasDefault: input.HasDefault}); err != nil {
		return domain.DatabaseColumn{}, err
	}
	defaultJSON, err := json.Marshal(input.Default)
	if !input.HasDefault {
		defaultJSON = nil
	}
	if err != nil {
		return domain.DatabaseColumn{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseColumn{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return domain.DatabaseColumn{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseColumn{}, err
	}
	if err := ensureTableProjectTx(ctx, tx, projectID, databaseID, tableID); err != nil {
		return domain.DatabaseColumn{}, err
	}
	var rowCount int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM database_rows WHERE table_id=$1`, tableID).Scan(&rowCount); err != nil {
		return domain.DatabaseColumn{}, err
	}
	if rowCount > 0 && input.Required && !input.HasDefault {
		return domain.DatabaseColumn{}, ErrSchemaConflict
	}
	item, err := scanColumn(tx.QueryRow(ctx, `INSERT INTO database_columns (id,table_id,key,column_type,required,varchar_size,default_value) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+columnProjection(), id, tableID, input.Key, input.Type, input.Required, input.VarcharSize, defaultJSON))
	if err != nil {
		return domain.DatabaseColumn{}, mapError(err)
	}
	if input.HasDefault {
		if _, err := tx.Exec(ctx, `UPDATE database_rows SET data=data || jsonb_build_object($2,$3::jsonb),updated_at=now() WHERE table_id=$1 AND NOT (data ? $2)`, tableID, input.Key, defaultJSON); err != nil {
			return domain.DatabaseColumn{}, err
		}
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_column.create", "database_column", id, map[string]any{
		"database_id": databaseID.String(),
		"table_id":    tableID.String(),
	}); err != nil {
		return domain.DatabaseColumn{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseColumn{}, err
	}
	return item, nil
}

func (r *Repository) DeleteDatabaseColumn(ctx context.Context, projectID, databaseID, tableID, columnID uuid.UUID, actor DatabaseActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return err
	}
	if err := ensureTableProjectTx(ctx, tx, projectID, databaseID, tableID); err != nil {
		return err
	}
	var dependentIndex bool
	var key string
	if err := tx.QueryRow(ctx, `SELECT key FROM database_columns WHERE id=$1 AND table_id=$2 FOR UPDATE`, columnID, tableID).Scan(&key); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_indexes WHERE table_id=$1 AND $2=ANY(column_keys))`, tableID, key).Scan(&dependentIndex); err != nil {
		return err
	}
	if dependentIndex {
		return ErrSchemaConflict
	}
	var dependentRelationship bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_relationships WHERE source_table_id=$1 AND source_column_key=$2)`, tableID, key).Scan(&dependentRelationship); err != nil {
		return err
	}
	if dependentRelationship {
		return ErrSchemaConflict
	}
	if _, err := tx.Exec(ctx, `DELETE FROM database_columns WHERE id=$1 AND table_id=$2`, columnID, tableID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE database_rows SET data=data-$2,updated_at=now() WHERE table_id=$1`, tableID, key); err != nil {
		return err
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_column.delete", "database_column", columnID, map[string]any{
		"database_id":    databaseID.String(),
		"table_id":       tableID.String(),
		"changed_fields": []string{"key"},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
