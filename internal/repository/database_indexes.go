package repository

import (
	"context"
	"fmt"
	"strings"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListDatabaseIndexes(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, limit int, cursor *uuid.UUID) ([]domain.DatabaseIndex, string, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
		return nil, "", err
	}
	if err := r.ensureTableProject(ctx, projectID, databaseID, tableID); err != nil {
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+indexProjection()+` FROM database_indexes WHERE table_id=$1 AND ($2::uuid IS NULL OR id>$2) ORDER BY id LIMIT $3`, tableID, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.DatabaseIndex, 0, limit)
	for rows.Next() {
		item, scanErr := scanIndex(rows)
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

func (r *Repository) CreateDatabaseIndex(ctx context.Context, id, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, input DatabaseIndexInput) (domain.DatabaseIndex, error) {
	if _, err := dbcore.ValidateName(input.Name); err != nil {
		return domain.DatabaseIndex{}, err
	}
	if input.Type != "key" && input.Type != "unique" && input.Type != "fulltext" {
		return domain.DatabaseIndex{}, fmt.Errorf("%w: index type must be key, unique, or fulltext", dbcore.ErrInvalidIdentifier)
	}
	if len(input.Directions) == 0 && len(input.ColumnKeys) > 0 {
		input.Directions = make([]string, len(input.ColumnKeys))
		for i := range input.Directions {
			input.Directions[i] = "asc"
		}
	}
	if len(input.ColumnKeys) == 0 || len(input.ColumnKeys) > 16 || len(input.ColumnKeys) != len(input.Directions) {
		return domain.DatabaseIndex{}, fmt.Errorf("%w: index columns and directions must contain 1 to 16 matching entries", dbcore.ErrInvalidIdentifier)
	}
	if input.Type == "fulltext" && len(input.ColumnKeys) != 1 {
		return domain.DatabaseIndex{}, fmt.Errorf("%w: fulltext indexes must contain exactly one text column", dbcore.ErrInvalidIdentifier)
	}
	seen := make(map[string]struct{}, len(input.ColumnKeys))
	for i, key := range input.ColumnKeys {
		if _, err := dbcore.ValidateIdentifier(key); err != nil {
			return domain.DatabaseIndex{}, err
		}
		if _, ok := seen[key]; ok {
			return domain.DatabaseIndex{}, fmt.Errorf("%w: duplicate index column", dbcore.ErrInvalidIdentifier)
		}
		seen[key] = struct{}{}
		input.Directions[i] = strings.ToLower(input.Directions[i])
		if input.Directions[i] != "asc" && input.Directions[i] != "desc" {
			return domain.DatabaseIndex{}, fmt.Errorf("%w: index direction must be asc or desc", dbcore.ErrInvalidIdentifier)
		}
		if input.Type == "fulltext" && input.Directions[i] != "asc" {
			return domain.DatabaseIndex{}, fmt.Errorf("%w: fulltext index direction must be asc", dbcore.ErrInvalidIdentifier)
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseIndex{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return domain.DatabaseIndex{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseIndex{}, err
	}
	if err := ensureTableProjectTx(ctx, tx, projectID, databaseID, tableID); err != nil {
		return domain.DatabaseIndex{}, err
	}
	columns, err := columnsForTableTx(ctx, tx, tableID)
	if err != nil {
		return domain.DatabaseIndex{}, err
	}
	byKey := make(map[string]DatabaseColumnSchema, len(columns))
	for _, column := range columns {
		byKey[column.Key] = column
	}
	for _, key := range input.ColumnKeys {
		if _, ok := byKey[key]; !ok {
			return domain.DatabaseIndex{}, ErrNotFound
		}
	}
	internalName := internalIndexName(id)
	ddl, err := buildIndexDDL(internalName, tableID, input, byKey)
	if err != nil {
		return domain.DatabaseIndex{}, err
	}
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return domain.DatabaseIndex{}, mapError(err)
	}
	item, err := scanIndex(tx.QueryRow(ctx, `INSERT INTO database_indexes (id,table_id,name,index_type,column_keys,directions) VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+indexProjection(), id, tableID, input.Name, input.Type, input.ColumnKeys, input.Directions))
	if err != nil {
		// The DDL is transactional in PostgreSQL, so rollback removes the
		// physical index together with failed metadata insertion.
		return domain.DatabaseIndex{}, mapError(err)
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_index.create", "database_index", id, map[string]any{
		"database_id": databaseID.String(),
		"table_id":    tableID.String(),
		"columns":     input.ColumnKeys,
	}); err != nil {
		return domain.DatabaseIndex{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseIndex{}, err
	}
	return item, nil
}

func (r *Repository) DeleteDatabaseIndex(ctx context.Context, projectID, databaseID, tableID, indexID uuid.UUID, actor DatabaseActor) error {
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
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_indexes WHERE id=$1 AND table_id=$2)`, indexID, tableID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS "`+internalIndexName(indexID)+`"`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM database_indexes WHERE id=$1 AND table_id=$2`, indexID, tableID); err != nil {
		return err
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_index.delete", "database_index", indexID, map[string]any{
		"database_id": databaseID.String(),
		"table_id":    tableID.String(),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func dropIndexesForTable(ctx context.Context, tx pgx.Tx, tableID uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id FROM database_indexes WHERE table_id=$1 FOR UPDATE`, tableID)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS "`+internalIndexName(id)+`"`); err != nil {
			return err
		}
	}
	return nil
}

func dropIndexesForDatabase(ctx context.Context, tx pgx.Tx, databaseID uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT i.id FROM database_indexes i JOIN database_tables t ON t.id=i.table_id WHERE t.database_id=$1 FOR UPDATE`, databaseID)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `DROP INDEX IF EXISTS "`+internalIndexName(id)+`"`); err != nil {
			return err
		}
	}
	return nil
}

func internalIndexName(id uuid.UUID) string {
	return "stealth_db_row_idx_" + strings.ReplaceAll(id.String(), "-", "")
}

func quoteSQLLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func buildIndexDDL(internalName string, tableID uuid.UUID, input DatabaseIndexInput, columns map[string]DatabaseColumnSchema) (string, error) {
	if input.Type == "fulltext" {
		column := columns[input.ColumnKeys[0]]
		if column.Type != dbcore.TypeVarchar && column.Type != dbcore.TypeText {
			return "", fmt.Errorf("%w: fulltext indexes require a varchar or text column", dbcore.ErrInvalidIdentifier)
		}
		keyLiteral := quoteSQLLiteral(column.Key)
		return `CREATE INDEX "` + internalName + `" ON database_rows USING GIN (to_tsvector('simple', COALESCE(data->>` + keyLiteral + `,''))) WHERE table_id = ` + quoteSQLLiteral(tableID.String()), nil
	}
	parts := make([]string, 0, len(input.ColumnKeys))
	for i, key := range input.ColumnKeys {
		column := columns[key]
		keyLiteral := quoteSQLLiteral(key)
		var expression string
		switch column.Type {
		case dbcore.TypeVarchar, dbcore.TypeText:
			expression = `(data->>` + keyLiteral + `)`
		case dbcore.TypeInteger:
			expression = `NULLIF(data->>` + keyLiteral + `,'')::bigint`
		case dbcore.TypeDouble:
			expression = `NULLIF(data->>` + keyLiteral + `,'')::double precision`
		case dbcore.TypeBoolean:
			expression = `NULLIF(data->>` + keyLiteral + `,'')::boolean`
		case dbcore.TypeDatetime:
			expression = `NULLIF(data->>` + keyLiteral + `,'')::timestamptz`
		case dbcore.TypeJSON:
			expression = `(data->` + keyLiteral + `)`
		default:
			return "", fmt.Errorf("%w: unsupported index column type", ErrInvalidQuery)
		}
		parts = append(parts, "("+expression+") "+strings.ToUpper(input.Directions[i]))
	}
	return `CREATE ` + map[bool]string{true: "UNIQUE ", false: ""}[input.Type == "unique"] + `INDEX "` + internalName + `" ON database_rows (` + strings.Join(parts, ",") + `) WHERE table_id = ` + quoteSQLLiteral(tableID.String()), nil
}
