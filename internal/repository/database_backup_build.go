package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) BuildDatabaseBackup(ctx context.Context, projectID, databaseID uuid.UUID, actor DatabaseActor, maxRows int) (DatabaseBackupSnapshot, []byte, error) {
	if maxRows <= 0 {
		maxRows = DatabaseBackupDefaultMaxRows
	}
	if maxRows > DatabaseBackupMaxRows {
		return DatabaseBackupSnapshot{}, nil, fmt.Errorf("%w: maximum row count is %d", ErrBackupTooLarge, DatabaseBackupMaxRows)
	}
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	database, err := scanDatabase(tx.QueryRow(ctx, `SELECT `+databaseProjection()+` FROM project_databases WHERE id=$1 AND project_id=$2`, databaseID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DatabaseBackupSnapshot{}, nil, ErrNotFound
	}
	if err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	snapshot := DatabaseBackupSnapshot{
		Version:       DatabaseBackupVersion,
		ProjectID:     projectID.String(),
		DatabaseID:    databaseID.String(),
		DatabaseName:  database.Name,
		Tables:        make([]DatabaseBackupTable, 0),
		Relationships: make([]domain.DatabaseRelationship, 0),
	}
	tableRows, err := tx.Query(ctx, `SELECT `+tableProjection()+` FROM database_tables WHERE project_id=$1 AND database_id=$2 ORDER BY id`, projectID, databaseID)
	if err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	tables := make([]domain.DatabaseTable, 0)
	for tableRows.Next() {
		if len(tables) >= DatabaseBackupMaxTables {
			tableRows.Close()
			return DatabaseBackupSnapshot{}, nil, ErrBackupTooLarge
		}
		table, scanErr := scanTable(tableRows)
		if scanErr != nil {
			tableRows.Close()
			return DatabaseBackupSnapshot{}, nil, scanErr
		}
		tables = append(tables, table)
	}
	if err := tableRows.Err(); err != nil {
		tableRows.Close()
		return DatabaseBackupSnapshot{}, nil, err
	}
	tableRows.Close()
	rowCount := 0
	for _, table := range tables {
		item := DatabaseBackupTable{Table: table, Columns: make([]domain.DatabaseColumn, 0), Indexes: make([]domain.DatabaseIndex, 0), Rows: make([]domain.DatabaseRow, 0)}
		tableID := mustParseUUID(table.ID)
		columns, err := tx.Query(ctx, `SELECT `+columnProjection()+` FROM database_columns WHERE table_id=$1 ORDER BY id`, tableID)
		if err != nil {
			return DatabaseBackupSnapshot{}, nil, err
		}
		for columns.Next() {
			column, scanErr := scanColumn(columns)
			if scanErr != nil {
				columns.Close()
				return DatabaseBackupSnapshot{}, nil, scanErr
			}
			item.Columns = append(item.Columns, column)
		}
		if err := columns.Err(); err != nil {
			columns.Close()
			return DatabaseBackupSnapshot{}, nil, err
		}
		columns.Close()
		indexes, err := tx.Query(ctx, `SELECT `+indexProjection()+` FROM database_indexes WHERE table_id=$1 ORDER BY id`, tableID)
		if err != nil {
			return DatabaseBackupSnapshot{}, nil, err
		}
		for indexes.Next() {
			index, scanErr := scanIndex(indexes)
			if scanErr != nil {
				indexes.Close()
				return DatabaseBackupSnapshot{}, nil, scanErr
			}
			item.Indexes = append(item.Indexes, index)
		}
		if err := indexes.Err(); err != nil {
			indexes.Close()
			return DatabaseBackupSnapshot{}, nil, err
		}
		indexes.Close()
		remaining := maxRows - rowCount
		if remaining < 0 {
			return DatabaseBackupSnapshot{}, nil, ErrBackupTooLarge
		}
		rows, err := tx.Query(ctx, `SELECT `+rowProjection+` FROM database_rows r WHERE r.project_id=$1 AND r.table_id=$2 ORDER BY r.id LIMIT $3`, projectID, tableID, remaining+1)
		if err != nil {
			return DatabaseBackupSnapshot{}, nil, err
		}
		for rows.Next() {
			if rowCount >= maxRows {
				rows.Close()
				return DatabaseBackupSnapshot{}, nil, ErrBackupTooLarge
			}
			row, scanErr := scanRow(rows)
			if scanErr != nil {
				rows.Close()
				return DatabaseBackupSnapshot{}, nil, scanErr
			}
			item.Rows = append(item.Rows, row)
			rowCount++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return DatabaseBackupSnapshot{}, nil, err
		}
		rows.Close()
		snapshot.Tables = append(snapshot.Tables, item)
	}
	relationships, err := tx.Query(ctx, `SELECT `+databaseRelationshipProjection+` FROM database_relationships WHERE project_id=$1 AND database_id=$2 ORDER BY id`, projectID, databaseID)
	if err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	// The rows cursor must be released before the transaction is rolled back on
	// any early return, otherwise pgx cannot cleanly return the connection.
	defer relationships.Close()
	for relationships.Next() {
		if len(snapshot.Relationships) >= DatabaseBackupMaxRelations {
			return DatabaseBackupSnapshot{}, nil, ErrBackupTooLarge
		}
		item, scanErr := scanDatabaseRelationship(relationships)
		if scanErr != nil {
			return DatabaseBackupSnapshot{}, nil, scanErr
		}
		snapshot.Relationships = append(snapshot.Relationships, item)
	}
	if err := relationships.Err(); err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	if len(payload) == 0 || len(payload) > DatabaseBackupMaxBytes {
		return DatabaseBackupSnapshot{}, nil, ErrBackupTooLarge
	}
	if err := tx.Commit(ctx); err != nil {
		return DatabaseBackupSnapshot{}, nil, err
	}
	return snapshot, payload, nil
}

func (r *Repository) CreateDatabaseBackup(ctx context.Context, id, projectID, databaseID uuid.UUID, actor DatabaseActor, storagePath string, sizeBytes int64, checksum string) (domain.DatabaseBackup, error) {
	return r.createDatabaseBackup(ctx, id, projectID, databaseID, actor, storagePath, sizeBytes, checksum, nil)
}

func (r *Repository) CreateDatabaseBackupWithCleanup(ctx context.Context, id, projectID, databaseID uuid.UUID, actor DatabaseActor, storagePath string, sizeBytes int64, checksum string, cleanup ArtifactCleanupInput) (domain.DatabaseBackup, error) {
	return r.createDatabaseBackup(ctx, id, projectID, databaseID, actor, storagePath, sizeBytes, checksum, &cleanup)
}

func (r *Repository) createDatabaseBackup(ctx context.Context, id, projectID, databaseID uuid.UUID, actor DatabaseActor, storagePath string, sizeBytes int64, checksum string, cleanup *ArtifactCleanupInput) (domain.DatabaseBackup, error) {
	if strings.TrimSpace(storagePath) == "" || strings.Contains(storagePath, "..") || sizeBytes < 1 || sizeBytes > DatabaseBackupMaxBytes || len(checksum) != 64 || checksum != strings.ToLower(checksum) {
		return domain.DatabaseBackup{}, ErrInvalidBackup
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return domain.DatabaseBackup{}, ErrInvalidBackup
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DatabaseBackup{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return domain.DatabaseBackup{}, err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return domain.DatabaseBackup{}, err
	}
	if err := ensureDatabaseProjectTx(ctx, tx, projectID, databaseID); err != nil {
		return domain.DatabaseBackup{}, err
	}
	item, _, err := scanDatabaseBackup(tx.QueryRow(ctx, `INSERT INTO database_backups (id,project_id,database_id,storage_path,size_bytes,checksum_sha256) VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+databaseBackupProjection, id, projectID, databaseID, storagePath, sizeBytes, checksum))
	if err != nil {
		return domain.DatabaseBackup{}, mapError(err)
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_backup.create", "database_backup", id, map[string]any{
		"database_id":     databaseID.String(),
		"size_bytes":      sizeBytes,
		"checksum_sha256": checksum,
	}); err != nil {
		return domain.DatabaseBackup{}, err
	}
	if err := validatePublishCleanup(cleanup, projectID, ArtifactCleanupStorage, storagePath); err != nil {
		return domain.DatabaseBackup{}, err
	}
	if cleanup != nil {
		if err := finalizeArtifactPublishCleanupTx(ctx, tx, *cleanup); err != nil {
			return domain.DatabaseBackup{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DatabaseBackup{}, err
	}
	return item, nil
}

func (r *Repository) ListDatabaseBackups(ctx context.Context, projectID, databaseID uuid.UUID, actor DatabaseActor, limit int, cursor *uuid.UUID) ([]domain.DatabaseBackup, string, bool, error) {
	canManage, err := r.requireDatabaseRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if err := r.ensureDatabaseProject(ctx, projectID, databaseID); err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+databaseBackupProjection+` FROM database_backups WHERE project_id=$1 AND database_id=$2 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $4`, projectID, databaseID, cursor, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.DatabaseBackup, 0, limit)
	for rows.Next() {
		item, _, scanErr := scanDatabaseBackup(rows)
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

func (r *Repository) GetDatabaseBackup(ctx context.Context, projectID, databaseID, backupID uuid.UUID, actor DatabaseActor) (domain.DatabaseBackup, string, error) {
	if _, err := r.requireDatabaseRead(ctx, projectID, actor); err != nil {
		return domain.DatabaseBackup{}, "", err
	}
	item, path, err := scanDatabaseBackup(r.pool.QueryRow(ctx, `SELECT `+databaseBackupProjection+` FROM database_backups WHERE project_id=$1 AND database_id=$2 AND id=$3`, projectID, databaseID, backupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DatabaseBackup{}, "", ErrNotFound
	}
	return item, path, err
}

func (r *Repository) DeleteDatabaseBackup(ctx context.Context, projectID, databaseID, backupID uuid.UUID, actor DatabaseActor) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := r.requireDatabaseWriteTx(ctx, tx, projectID, actor, "databases.write"); err != nil {
		return "", err
	}
	if err := lockDatabaseNamespace(ctx, tx, databaseID); err != nil {
		return "", err
	}
	if err := ensureDatabaseProjectTx(ctx, tx, projectID, databaseID); err != nil {
		return "", err
	}
	var path string
	if err := tx.QueryRow(ctx, `SELECT storage_path FROM database_backups WHERE project_id=$1 AND database_id=$2 AND id=$3 FOR UPDATE`, projectID, databaseID, backupID).Scan(&path); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM database_backups WHERE project_id=$1 AND database_id=$2 AND id=$3`, projectID, databaseID, backupID); err != nil {
		return "", err
	}
	if err := r.auditDatabase(ctx, tx, projectID, actor, "database_backup.delete", "database_backup", backupID, map[string]any{
		"database_id": databaseID.String(),
	}); err != nil {
		return "", err
	}
	if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{
		ProjectID: projectID, StoreKind: ArtifactCleanupStorage,
		Operation: ArtifactCleanupRelative, RelativePath: path,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return path, nil
}

func validateBackupUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.Version() != uuid.Version(7) {
		return uuid.Nil, ErrInvalidBackup
	}
	return id, nil
}

func backupTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}

// RestoreDatabaseBackup replaces every table in the target database in one
// transaction. The caller must have an explicit databases.write grant; a
// failed schema, row, relationship, or index aborts the complete restore.
