package repository

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func normalizeTablePermissions(input DatabaseTableInput) ([4][]string, error) {
	var result [4][]string
	values := [][]string{input.CreatePermissions, input.ReadPermissions, input.UpdatePermissions, input.DeletePermissions}
	for i, raw := range values {
		permissions, err := dbcore.NormalizePermissions(raw)
		if err != nil {
			return result, err
		}
		result[i] = permissions
	}
	return result, nil
}

func (r *Repository) requireDatabaseRead(ctx context.Context, projectID uuid.UUID, actor DatabaseActor) (bool, error) {
	switch actor.Kind {
	case DatabaseConsoleActor:
		role, err := r.projectRole(ctx, projectID, actor.AccountID)
		if err != nil {
			return false, err
		}
		return role == "owner" || role == "admin", nil
	case DatabaseAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "databases.read") {
			return false, ErrForbidden
		}
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, projectID).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, ErrNotFound
		}
		return apikey.HasScope(actor.APIKeyScopes, "databases.write"), nil
	default:
		return false, ErrForbidden
	}
}

func (r *Repository) requireDatabaseWriteTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor DatabaseActor, scope string) error {
	switch actor.Kind {
	case DatabaseConsoleActor:
		return requireProjectRoleTx(ctx, tx, projectID, actor.AccountID, "owner", "admin")
	case DatabaseAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, scope) {
			return ErrForbidden
		}
		return requireActiveProjectAPIKeyTx(ctx, tx, projectID, actor.APIKeyID, scope)
	default:
		return ErrForbidden
	}
}

func (r *Repository) ensureDatabaseProject(ctx context.Context, projectID, databaseID uuid.UUID) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_databases WHERE id=$1 AND project_id=$2)`, databaseID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func ensureDatabaseProjectTx(ctx context.Context, tx pgx.Tx, projectID, databaseID uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_databases WHERE id=$1 AND project_id=$2)`, databaseID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ensureTableProject(ctx context.Context, projectID, databaseID, tableID uuid.UUID) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_tables WHERE id=$1 AND database_id=$2 AND project_id=$3)`, tableID, databaseID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func ensureTableProjectTx(ctx context.Context, tx pgx.Tx, projectID, databaseID, tableID uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM database_tables WHERE id=$1 AND database_id=$2 AND project_id=$3)`, tableID, databaseID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func lockDatabaseNamespace(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id.String())
	return err
}

func columnsForTableTx(ctx context.Context, tx pgx.Tx, tableID uuid.UUID) ([]DatabaseColumnSchema, error) {
	rows, err := tx.Query(ctx, `SELECT `+columnProjection()+` FROM database_columns WHERE table_id=$1 ORDER BY id`, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make([]DatabaseColumnSchema, 0)
	for rows.Next() {
		item, err := scanColumn(rows)
		if err != nil {
			return nil, err
		}
		columns = append(columns, schemaFromDomain(item))
	}
	return columns, rows.Err()
}

func (r *Repository) auditDatabase(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor DatabaseActor, action, targetType string, target uuid.UUID, metadata map[string]any) error {
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["project_id"] = projectID.String()
	switch actor.Kind {
	case DatabaseAPIKeyActor:
		metadata["actor"] = "api_key"
		metadata["api_key_id"] = actor.APIKeyID.String()
	case DatabaseApplicationActor:
		metadata["actor"] = "project_user"
		metadata["project_user_id"] = actor.ProjectUserID.String()
		metadata["source"] = "application"
	case DatabaseAnonymousActor:
		metadata["actor"] = "anonymous"
		metadata["source"] = "application"
	}
	actorID := uuid.Nil
	if actor.Kind == DatabaseConsoleActor {
		actorID = actor.AccountID
	}
	if err := writeAuditMetadata(ctx, tx, orgID, actorID, action, targetType, target, metadata); err != nil {
		return err
	}
	return r.enqueueWebhookEventTx(ctx, tx, projectID, action, targetType, target, metadata)
}

func rowPermissionSQL(column string, actor DatabaseActor, args *[]any) string {
	if actor.Kind == DatabaseConsoleActor || actor.Kind == DatabaseAPIKeyActor {
		return "TRUE"
	}
	if actor.Kind == DatabaseAnonymousActor {
		return column + " @> ARRAY['any']::text[]"
	}
	userPermission := "user:" + actor.ProjectUserID.String()
	*args = append(*args, userPermission)
	return "(" + column + " @> ARRAY['any']::text[] OR " + column + " @> ARRAY['users']::text[] OR $" + strconv.Itoa(len(*args)) + " = ANY(" + column + "))"
}

func tablePermission(perms []string, actor DatabaseActor) bool {
	if actor.Kind == DatabaseConsoleActor || actor.Kind == DatabaseAPIKeyActor {
		return true
	}
	return dbcore.Grants(perms, dbcore.Actor{Authenticated: actor.Kind == DatabaseApplicationActor, UserID: actor.ProjectUserID})
}

func normalizeRowPermissions(raw *[]string, actor DatabaseActor, defaultForUser bool) ([]string, error) {
	if raw == nil {
		if defaultForUser && actor.Kind == DatabaseApplicationActor {
			return []string{"user:" + actor.ProjectUserID.String()}, nil
		}
		return []string{}, nil
	}
	permissions, err := dbcore.NormalizePermissions(*raw)
	if err != nil {
		return nil, err
	}
	if actor.Kind == DatabaseAnonymousActor {
		for _, permission := range permissions {
			if permission == "users" {
				return nil, fmt.Errorf("%w: anonymous rows cannot grant users", dbcore.ErrInvalidPermissions)
			}
		}
	}
	return permissions, nil
}

func buildRowSourceMetadata(actor DatabaseActor, changed []string) map[string]any {
	copyChanged := append([]string(nil), changed...)
	sort.Strings(copyChanged)
	return map[string]any{"changed_fields": copyChanged}
}

// buildDatabaseRowEventMetadata adds the minimum permission snapshot needed
// for a Realtime application subscriber to decide whether a row event is
// visible. The actual row data is intentionally never copied into the audit or
// integration payload; consumers can fetch it through the normal permissioned
// row API after receiving an event.
func buildDatabaseRowEventMetadata(actor DatabaseActor, table domain.DatabaseTable, rowReadPermissions, changed []string) map[string]any {
	metadata := buildRowSourceMetadata(actor, changed)
	// Keep the query scope in the public notification metadata as well as in
	// the permission marker below. The Console must be able to invalidate the
	// affected table without depending on authorization-only fields.
	metadata["database_id"] = table.DatabaseID
	metadata["table_id"] = table.ID
	metadata["realtime"] = map[string]any{
		"database_id":            table.DatabaseID,
		"table_id":               table.ID,
		"row_security":           table.RowSecurity,
		"table_read_permissions": append([]string(nil), table.ReadPermissions...),
		"row_read_permissions":   append([]string(nil), rowReadPermissions...),
	}
	return metadata
}

// Keep these references in this file so future schema adapters cannot forget
// that database writes are checked against the same API-key implementation.
