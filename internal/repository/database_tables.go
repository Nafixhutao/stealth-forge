package repository

import (
	"context"
	"errors"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListDatabaseTables(ctx context.Context, projectID, databaseID uuid.UUID, actor DatabaseActor, limit int, cursor *uuid.UUID) ([]domain.DatabaseTable, string, bool, error) {
	canManage, err := r.requireDatabaseRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if err := r.ensureDatabaseProject(ctx, projectID, databaseID); err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+tableProjection()+` FROM database_tables WHERE project_id=$1 AND database_id=$2 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $4`, projectID, databaseID, cursor, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.DatabaseTable, 0, limit)
	for rows.Next() {
		item, scanErr := scanTable(rows)
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

func (r *Repository) GetDatabaseTable(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor) (domain.DatabaseTable, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
		return domain.DatabaseTable{}, err
	}
	item, err := scanTable(r.pool.QueryRow(ctx, `SELECT `+tableProjection()+` FROM database_tables WHERE project_id=$1 AND database_id=$2 AND id=$3`, projectID, databaseID, tableID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatabaseTable{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateDatabaseTable(ctx context.Context, id, projectID, databaseID uuid.UUID, actor DatabaseActor, input DatabaseTableInput) (domain.DatabaseTable, error) {
	permissions, err := normalizeTablePermissions(input)
	if err != nil {
		return domain.DatabaseTable{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseTable{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return domain.DatabaseTable{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseTable{}, err
	}
	if err := ensureDatabaseProjectTx(ctx, tx, projectID, databaseID); err != nil {
		return domain.DatabaseTable{}, err
	}
	item, err := scanTable(tx.QueryRow(ctx, `INSERT INTO database_tables (id,database_id,project_id,name,row_security,create_permissions,read_permissions,update_permissions,delete_permissions) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+tableProjection(), id, databaseID, projectID, input.Name, input.RowSecurity, permissions[0], permissions[1], permissions[2], permissions[3]))
	if err != nil {
		return domain.DatabaseTable{}, mapError(err)
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_table.create", "database_table", id, map[string]any{
		"database_id": databaseID.String(),
	}); err != nil {
		return domain.DatabaseTable{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseTable{}, err
	}
	return item, nil
}

func (r *Repository) UpdateDatabaseTable(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, input DatabaseTableInput) (domain.DatabaseTable, error) {
	permissions, err := normalizeTablePermissions(input)
	if err != nil {
		return domain.DatabaseTable{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseTable{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return domain.DatabaseTable{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseTable{}, err
	}
	item, err := scanTable(tx.QueryRow(ctx, `UPDATE database_tables SET row_security=$4,create_permissions=$5,read_permissions=$6,update_permissions=$7,delete_permissions=$8,updated_at=now() WHERE project_id=$1 AND database_id=$2 AND id=$3 RETURNING `+tableProjection(), projectID, databaseID, tableID, input.RowSecurity, permissions[0], permissions[1], permissions[2], permissions[3]))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatabaseTable{}, ErrNotFound
	}
	if err != nil {
		return domain.DatabaseTable{}, mapError(err)
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_table.update", "database_table", tableID, map[string]any{
		"database_id":    databaseID.String(),
		"changed_fields": []string{"row_security", "permissions"},
	}); err != nil {
		return domain.DatabaseTable{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseTable{}, err
	}
	return item, nil
}

func (r *Repository) DeleteDatabaseTable(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor) error {
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
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_tables WHERE id=$1 AND database_id=$2 AND project_id=$3)`, tableID, databaseID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if err := dropIndexesForTable(ctx, tx, tableID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM database_tables WHERE id=$1 AND database_id=$2 AND project_id=$3`, tableID, databaseID, projectID); err != nil {
		return err
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_table.delete", "database_table", tableID, map[string]any{
		"database_id": databaseID.String(),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
