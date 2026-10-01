package repository

import (
	"context"
	"errors"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const rowProjection = `r.id,r.table_id,r.project_id,r.data,r.read_permissions,r.update_permissions,r.delete_permissions,r.creator_project_user_id,r.created_at,r.updated_at`
const rowProjectionNoAlias = `id,table_id,project_id,data,read_permissions,update_permissions,delete_permissions,creator_project_user_id,created_at,updated_at`

const (
	// DatabaseRowExportDefaultLimit keeps an accidental export from turning
	// into an unbounded database and response-buffer operation.
	DatabaseRowExportDefaultLimit = 1000
	DatabaseRowExportMaxLimit     = 10000
	DatabaseRowBulkImportMaxRows  = 1000
	DatabaseRowTransactionMaxOps  = 100
)

type DatabaseBulkRowInput struct {
	ID                uuid.UUID
	Data              map[string]any
	ReadPermissions   *[]string
	UpdatePermissions *[]string
	DeletePermissions *[]string
}

// DatabaseRowTransactionOperation is one atomic row mutation. Create and
// update return rows in the transaction result; delete returns its id in
// DeletedIDs. The repository evaluates permissions and relationships for every
// operation before committing any of them.
type DatabaseRowTransactionOperation struct {
	Action            string
	ID                uuid.UUID
	Data              map[string]any
	ReadPermissions   *[]string
	UpdatePermissions *[]string
	DeletePermissions *[]string
}

type DatabaseRowTransactionResult struct {
	Rows       []domain.DatabaseRow
	DeletedIDs []string
}

// DatabaseTableSchema loads schema metadata and performs the same project and
// table-read checks used by row reads. When row security is enabled, the
// operation may still narrow results further using each row's grant.
func (r *Repository) DatabaseTableSchema(ctx context.Context, projectID, databaseID, tableID uuid.UUID, actor DatabaseActor) (DatabaseTableSchema, error) {
	if actor.IsManagement() {
		if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
			return DatabaseTableSchema{}, err
		}
	}
	schema, err := loadTableSchema(ctx, r.pool, projectID, databaseID, tableID)
	if err != nil {
		return DatabaseTableSchema{}, err
	}
	if err := authorizeDatabaseRowRead(schema, actor); err != nil {
		return DatabaseTableSchema{}, err
	}
	return schema, nil
}

func loadTableSchema(ctx context.Context, txOrPool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, projectID, databaseID, tableID uuid.UUID) (DatabaseTableSchema, error) {
	item, err := scanTable(txOrPool.QueryRow(ctx, `SELECT `+tableProjection()+` FROM database_tables WHERE id=$1 AND database_id=$2 AND project_id=$3`, tableID, databaseID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DatabaseTableSchema{}, ErrNotFound
	}
	if err != nil {
		return DatabaseTableSchema{}, err
	}
	rows, err := txOrPool.Query(ctx, `SELECT `+columnProjection()+` FROM database_columns WHERE table_id=$1 ORDER BY id`, tableID)
	if err != nil {
		return DatabaseTableSchema{}, err
	}
	defer rows.Close()
	columns := make([]DatabaseColumnSchema, 0)
	for rows.Next() {
		column, scanErr := scanColumn(rows)
		if scanErr != nil {
			return DatabaseTableSchema{}, scanErr
		}
		columns = append(columns, schemaFromDomain(column))
	}
	if err := rows.Err(); err != nil {
		return DatabaseTableSchema{}, err
	}
	return DatabaseTableSchema{Table: item, Columns: columns}, nil
}

func authorizeDatabaseRowRead(schema DatabaseTableSchema, actor DatabaseActor) error {
	if actor.IsApplication() && !tablePermission(schema.Table.ReadPermissions, actor) && !schema.Table.RowSecurity {
		return ErrForbidden
	}
	return nil
}

func rowSQLExpression(prefix string, column DatabaseColumnSchema) string {
	literal := quoteSQLLiteral(column.Key)
	switch column.Type {
	case dbcore.TypeVarchar, dbcore.TypeText:
		return `(` + prefix + `.data->>` + literal + `)`
	case dbcore.TypeInteger:
		return `NULLIF(` + prefix + `.data->>` + literal + `,'')::bigint`
	case dbcore.TypeDouble:
		return `NULLIF(` + prefix + `.data->>` + literal + `,'')::double precision`
	case dbcore.TypeBoolean:
		return `NULLIF(` + prefix + `.data->>` + literal + `,'')::boolean`
	case dbcore.TypeDatetime:
		return `NULLIF(` + prefix + `.data->>` + literal + `,'')::timestamptz`
	case dbcore.TypeJSON:
		return `(` + prefix + `.data->` + literal + `)`
	default:
		return `NULL`
	}
}

func fullTextSQLExpression(prefix string, column DatabaseColumnSchema) string {
	return `(to_tsvector('simple', COALESCE(` + prefix + `.data->>` + quoteSQLLiteral(column.Key) + `,'')))`
}

func (r *Repository) indexedColumns(ctx context.Context, tableID uuid.UUID, keys []string) (bool, error) {
	for _, key := range keys {
		var found bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_indexes WHERE table_id=$1 AND column_keys[1]=$2)`, tableID, key).Scan(&found); err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func (r *Repository) fullTextIndexedColumn(ctx context.Context, tableID uuid.UUID, key string) (bool, error) {
	var found bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_indexes WHERE table_id=$1 AND index_type='fulltext' AND column_keys[1]=$2)`, tableID, key).Scan(&found); err != nil {
		return false, err
	}
	return found, nil
}
