package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrUnindexedQuery = errors.New("unindexed query")
	ErrSchemaConflict = errors.New("schema change conflicts with existing rows")
	ErrInvalidQuery   = errors.New("invalid query")
	ErrRowHidden      = errors.New("row hidden")
)

type DatabaseActorKind string

const (
	DatabaseConsoleActor     DatabaseActorKind = "console"
	DatabaseAPIKeyActor      DatabaseActorKind = "api_key"
	DatabaseApplicationActor DatabaseActorKind = "application"
	DatabaseAnonymousActor   DatabaseActorKind = "anonymous"
)

// DatabaseActor keeps authentication provenance explicit. In particular, an
// API-key request never fabricates an accounts.id and an application cookie
// never becomes a Console membership.
type DatabaseActor struct {
	Kind          DatabaseActorKind
	AccountID     uuid.UUID
	APIKeyID      uuid.UUID
	APIKeyScopes  []string
	ProjectUserID uuid.UUID
}

func (a DatabaseActor) IsManagement() bool {
	return a.Kind == DatabaseConsoleActor || a.Kind == DatabaseAPIKeyActor
}

func (a DatabaseActor) IsApplication() bool {
	return a.Kind == DatabaseApplicationActor || a.Kind == DatabaseAnonymousActor
}

type DatabaseColumnSchema struct {
	ID          uuid.UUID
	Key         string
	Type        dbcore.ColumnType
	Required    bool
	VarcharSize *int
	Default     any
	HasDefault  bool
}

type DatabaseTableSchema struct {
	Table   domain.DatabaseTable
	Columns []DatabaseColumnSchema
}

type DatabaseTableInput struct {
	Name              string
	RowSecurity       bool
	CreatePermissions []string
	ReadPermissions   []string
	UpdatePermissions []string
	DeletePermissions []string
}

type DatabaseColumnInput struct {
	Key         string
	Type        dbcore.ColumnType
	Required    bool
	VarcharSize *int
	Default     any
	HasDefault  bool
}

type DatabaseIndexInput struct {
	Name       string
	Type       string
	ColumnKeys []string
	Directions []string
}

type DatabaseRowInput struct {
	Data              map[string]any
	ReadPermissions   *[]string
	UpdatePermissions *[]string
	DeletePermissions *[]string
}

type DatabaseRowPatch struct {
	Data              map[string]any
	ReadPermissions   *[]string
	UpdatePermissions *[]string
	DeletePermissions *[]string
}

type RowFilter struct {
	Column DatabaseColumnSchema
	Value  any
}

type RowCursor struct {
	ID    uuid.UUID `json:"id"`
	Value any       `json:"value"`
}

type RowQuery struct {
	Limit        int
	Cursor       *RowCursor
	Filters      []RowFilter
	OrderBy      *DatabaseColumnSchema
	Descending   bool
	Search       string
	SearchColumn *DatabaseColumnSchema
}

func EncodeRowCursor(cursor RowCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func DecodeRowCursor(value string) (RowCursor, error) {
	if value == "" {
		return RowCursor{}, nil
	}
	// The default id cursor remains a raw UUID for easy interoperability, but
	// a canonical UUID is also valid base64url text. Check for the raw UUID
	// form first so those cursors are never misdecoded as encoded garbage.
	if id, uuidErr := uuid.Parse(value); uuidErr == nil {
		return RowCursor{ID: id}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return RowCursor{}, fmt.Errorf("%w: cursor must be a UUID or encoded row cursor", ErrInvalidQuery)
	}
	var cursor RowCursor
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&cursor); err != nil || cursor.ID == uuid.Nil {
		return RowCursor{}, fmt.Errorf("%w: cursor is invalid", ErrInvalidQuery)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return RowCursor{}, fmt.Errorf("%w: cursor is invalid", ErrInvalidQuery)
	}
	return cursor, nil
}

func databaseProjection() string {
	return `id,project_id,name,created_at,updated_at`
}

func tableProjection() string {
	return `id,database_id,project_id,name,row_security,create_permissions,read_permissions,update_permissions,delete_permissions,created_at,updated_at`
}

func columnProjection() string {
	return `id,table_id,key,column_type,required,varchar_size,default_value,created_at,updated_at`
}

func indexProjection() string {
	return `id,table_id,name,index_type,column_keys,directions,created_at,updated_at`
}

func scanDatabase(row interface{ Scan(...any) error }) (domain.ProjectDatabase, error) {
	var item domain.ProjectDatabase
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanTable(row interface{ Scan(...any) error }) (domain.DatabaseTable, error) {
	var item domain.DatabaseTable
	err := row.Scan(&item.ID, &item.DatabaseID, &item.ProjectID, &item.Name, &item.RowSecurity, &item.CreatePermissions, &item.ReadPermissions, &item.UpdatePermissions, &item.DeletePermissions, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanColumn(row interface{ Scan(...any) error }) (domain.DatabaseColumn, error) {
	var item domain.DatabaseColumn
	var raw []byte
	err := row.Scan(&item.ID, &item.TableID, &item.Key, &item.Type, &item.Required, &item.VarcharSize, &raw, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	if len(raw) > 0 {
		item.Default = append(item.Default[:0], raw...)
	}
	return item, nil
}

func scanIndex(row interface{ Scan(...any) error }) (domain.DatabaseIndex, error) {
	var item domain.DatabaseIndex
	err := row.Scan(&item.ID, &item.TableID, &item.Name, &item.Type, &item.ColumnKeys, &item.Directions, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanRow(row interface{ Scan(...any) error }) (domain.DatabaseRow, error) {
	var item domain.DatabaseRow
	var raw []byte
	var creator *uuid.UUID
	err := row.Scan(&item.ID, &item.TableID, &item.ProjectID, &raw, &item.ReadPermissions, &item.UpdatePermissions, &item.DeletePermissions, &creator, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.Data = make(map[string]any)
	if len(raw) > 0 {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&item.Data); err != nil {
			return item, err
		}
	}
	if creator != nil {
		value := creator.String()
		item.CreatorProjectUserID = &value
	}
	return item, nil
}

func schemaFromDomain(column domain.DatabaseColumn) DatabaseColumnSchema {
	var defaultValue any
	hasDefault := len(column.Default) > 0 && string(column.Default) != "null"
	if hasDefault {
		decoder := json.NewDecoder(strings.NewReader(string(column.Default)))
		decoder.UseNumber()
		if err := decoder.Decode(&defaultValue); err != nil {
			hasDefault = false
		}
	}
	return DatabaseColumnSchema{ID: mustParseUUID(column.ID), Key: column.Key, Type: dbcore.ColumnType(column.Type), Required: column.Required, VarcharSize: column.VarcharSize, Default: defaultValue, HasDefault: hasDefault}
}

func mustParseUUID(value string) uuid.UUID {
	parsed, _ := uuid.Parse(value)
	return parsed
}

func (r *Repository) ListProjectDatabases(ctx context.Context, projectID uuid.UUID, actor DatabaseActor, limit int, cursor *uuid.UUID) ([]domain.ProjectDatabase, string, bool, error) {
	canManage, err := r.requireDatabaseRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+databaseProjection()+` FROM project_databases WHERE project_id=$1 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $2`, projectID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.ProjectDatabase, 0, limit)
	for rows.Next() {
		item, scanErr := scanDatabase(rows)
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

func (r *Repository) GetProjectDatabase(ctx context.Context, projectID, databaseID uuid.UUID, actor DatabaseActor) (domain.ProjectDatabase, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
		return domain.ProjectDatabase{}, err
	}
	item, err := scanDatabase(r.pool.QueryRow(ctx, `SELECT `+databaseProjection()+` FROM project_databases WHERE project_id=$1 AND id=$2`, projectID, databaseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProjectDatabase{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateProjectDatabase(ctx context.Context, id, projectID uuid.UUID, actor DatabaseActor, name string) (domain.ProjectDatabase, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ProjectDatabase{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return domain.ProjectDatabase{}, err
	}
	organizationID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return domain.ProjectDatabase{}, err
	}
	if err := r.enforceOrganizationLimitTx(ctx, tx, organizationID, "databases"); err != nil {
		return domain.ProjectDatabase{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, projectID); err != nil {
		return domain.ProjectDatabase{}, err
	}
	item, err := scanDatabase(tx.QueryRow(ctx, `INSERT INTO project_databases (id,project_id,name) VALUES ($1,$2,$3) RETURNING `+databaseProjection(), id, projectID, name))
	if err != nil {
		return domain.ProjectDatabase{}, mapError(err)
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database.create", "project_database", id, map[string]any{}); err != nil {
		return domain.ProjectDatabase{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ProjectDatabase{}, err
	}
	return item, nil
}

func (r *Repository) DeleteProjectDatabase(ctx context.Context, projectID, databaseID uuid.UUID, actor DatabaseActor) error {
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
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_databases WHERE id=$1 AND project_id=$2)`, databaseID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	backups, err := tx.Query(ctx, `SELECT storage_path FROM database_backups WHERE project_id=$1 AND database_id=$2 FOR UPDATE`, projectID, databaseID)
	if err != nil {
		return err
	}
	backupPaths := make([]string, 0)
	for backups.Next() {
		var path string
		if err := backups.Scan(&path); err != nil {
			backups.Close()
			return err
		}
		backupPaths = append(backupPaths, path)
	}
	if err := backups.Err(); err != nil {
		backups.Close()
		return err
	}
	backups.Close()
	for _, path := range backupPaths {
		if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{
			ProjectID: projectID, StoreKind: ArtifactCleanupStorage,
			Operation: ArtifactCleanupRelative, RelativePath: path,
		}); err != nil {
			return err
		}
	}
	if err := dropIndexesForDatabase(ctx, tx, databaseID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_databases WHERE id=$1 AND project_id=$2`, databaseID, projectID); err != nil {
		return err
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database.delete", "project_database", databaseID, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
