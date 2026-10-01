package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListDatabaseRows(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, query RowQuery) ([]domain.DatabaseRow, string, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil && actor.IsManagement() {
		return nil, "", err
	}
	schema, err := loadTableSchema(ctx, r.pool, projectID, databaseID, tableID)
	if err != nil {
		return nil, "", err
	}
	if err := authorizeDatabaseRowRead(schema, actor); err != nil {
		return nil, "", ErrForbidden
	}
	tableReadGranted := tablePermission(schema.Table.ReadPermissions, actor)
	indexKeys := make([]string, 0, len(query.Filters)+1)
	for _, filter := range query.Filters {
		indexKeys = append(indexKeys, filter.Column.Key)
	}
	if query.OrderBy != nil {
		indexKeys = append(indexKeys, query.OrderBy.Key)
	}
	if query.SearchColumn != nil {
		indexed, err := r.fullTextIndexedColumn(ctx, tableID, query.SearchColumn.Key)
		if err != nil {
			return nil, "", err
		}
		if !indexed {
			return nil, "", ErrUnindexedQuery
		}
	}
	if len(indexKeys) > 0 {
		indexed, err := r.indexedColumns(ctx, tableID, indexKeys)
		if err != nil {
			return nil, "", err
		}
		if !indexed {
			return nil, "", ErrUnindexedQuery
		}
	}
	args := []any{projectID, tableID}
	where := []string{"r.project_id=$1", "r.table_id=$2"}
	if actor.IsApplication() && schema.Table.RowSecurity && !tableReadGranted {
		where = append(where, rowPermissionSQL("r.read_permissions", actor, &args))
	}
	for _, filter := range query.Filters {
		args = append(args, filter.Value)
		where = append(where, rowSQLExpression("r", filter.Column)+"=$"+strconv.Itoa(len(args)))
	}
	if query.SearchColumn != nil && strings.TrimSpace(query.Search) != "" {
		args = append(args, strings.TrimSpace(query.Search))
		where = append(where, fullTextSQLExpression("r", *query.SearchColumn)+" @@ plainto_tsquery('simple', $"+strconv.Itoa(len(args))+")")
	}
	orderExpression := "r.id"
	if query.OrderBy != nil {
		orderExpression = rowSQLExpression("r", *query.OrderBy)
	}
	if query.Cursor != nil && query.Cursor.ID != uuid.Nil {
		op := ">"
		if query.Descending {
			op = "<"
		}
		if query.OrderBy == nil {
			args = append(args, query.Cursor.ID)
			where = append(where, "r.id "+op+" $"+strconv.Itoa(len(args)))
		} else {
			if query.Cursor.Value == nil {
				return nil, "", fmt.Errorf("%w: ordered cursor has no value", ErrInvalidQuery)
			}
			args = append(args, query.Cursor.Value)
			valueArg := strconv.Itoa(len(args))
			args = append(args, query.Cursor.ID)
			idArg := strconv.Itoa(len(args))
			where = append(where, "("+orderExpression+" "+op+" $"+valueArg+" OR ("+orderExpression+" = $"+valueArg+" AND r.id "+op+" $"+idArg+"))")
		}
	}
	direction := "ASC"
	if query.Descending {
		direction = "DESC"
	}
	if query.Limit < 1 || query.Limit > 100 {
		return nil, "", fmt.Errorf("%w: limit must be between 1 and 100", ErrInvalidQuery)
	}
	args = append(args, query.Limit+1)
	sql := `SELECT ` + rowProjection + ` FROM database_rows r WHERE ` + strings.Join(where, " AND ") + ` ORDER BY ` + orderExpression + ` ` + direction + ` NULLS LAST, r.id ` + direction + ` LIMIT $` + strconv.Itoa(len(args))
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.DatabaseRow, 0, query.Limit)
	for rows.Next() {
		item, scanErr := scanRow(rows)
		if scanErr != nil {
			return nil, "", scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > query.Limit {
		last := items[query.Limit-1]
		if query.OrderBy == nil {
			next = last.ID
		} else {
			next = EncodeRowCursor(RowCursor{ID: mustParseUUID(last.ID), Value: last.Data[query.OrderBy.Key]})
		}
		items = items[:query.Limit]
	}
	return items, next, nil
}

// StreamDatabaseRows emits rows in stable id order while applying the same
// table and row permission rules as ListDatabaseRows. The callback runs while
// the PostgreSQL cursor is open, so callers can write an export without first
// materialising the entire table in memory.
func (r *Repository) StreamDatabaseRows(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor, limit int, emit func(domain.DatabaseRow) error) (int, error) {
	if limit < 1 || limit > DatabaseRowExportMaxLimit {
		return 0, fmt.Errorf("%w: export limit must be between 1 and %d", ErrInvalidQuery, DatabaseRowExportMaxLimit)
	}
	if emit == nil {
		return 0, fmt.Errorf("%w: export callback is required", ErrInvalidQuery)
	}
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil && actor.IsManagement() {
		return 0, err
	}
	schema, err := loadTableSchema(ctx, r.pool, projectID, databaseID, tableID)
	if err != nil {
		return 0, err
	}
	if err := authorizeDatabaseRowRead(schema, actor); err != nil {
		return 0, err
	}
	tableReadGranted := tablePermission(schema.Table.ReadPermissions, actor)
	args := []any{projectID, tableID}
	where := []string{"r.project_id=$1", "r.table_id=$2"}
	if actor.IsApplication() && schema.Table.RowSecurity && !tableReadGranted {
		where = append(where, rowPermissionSQL("r.read_permissions", actor, &args))
	}
	args = append(args, limit)
	query := `SELECT ` + rowProjection + ` FROM database_rows r WHERE ` + strings.Join(where, " AND ") + ` ORDER BY r.id ASC LIMIT $` + strconv.Itoa(len(args))
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		item, scanErr := scanRow(rows)
		if scanErr != nil {
			return count, scanErr
		}
		if callbackErr := emit(item); callbackErr != nil {
			return count, callbackErr
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func (r *Repository) GetDatabaseRow(ctx context.Context, projectID, databaseID, tableID, rowID uuid.UUID, actor DatabaseActor) (domain.DatabaseRow, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil && actor.IsManagement() {
		return domain.DatabaseRow{}, err
	}
	schema, err := loadTableSchema(ctx, r.pool, projectID, databaseID, tableID)
	if err != nil {
		return domain.DatabaseRow{}, err
	}
	if err := authorizeDatabaseRowRead(schema, actor); err != nil {
		return domain.DatabaseRow{}, err
	}
	tableReadGranted := tablePermission(schema.Table.ReadPermissions, actor)
	sql := `SELECT ` + rowProjection + ` FROM database_rows r WHERE r.project_id=$1 AND r.table_id=$2 AND r.id=$3`
	args := []any{projectID, tableID, rowID}
	if actor.IsApplication() && schema.Table.RowSecurity && !tableReadGranted {
		sql += ` AND ` + rowPermissionSQL("r.read_permissions", actor, &args)
	}
	item, err := scanRow(r.pool.QueryRow(ctx, sql, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatabaseRow{}, ErrNotFound
	}
	return item, err
}
