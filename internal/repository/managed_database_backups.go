package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ManagedDatabaseBackupRecord is one completed mysqldump snapshot. Rows are
// pruned by the maintenance worker when a database exceeds its retention
// count; nothing here stores credentials.
type ManagedDatabaseBackupRecord struct {
	ID         uuid.UUID
	DatabaseID uuid.UUID
	ProjectID  uuid.UUID
	BackupPath string
	SizeBytes  int64
	Source     string
	CreatedAt  time.Time
}

const managedDatabaseBackupProjection = `
  id,database_id,project_id,backup_path,size_bytes,source,created_at
`

func scanManagedDatabaseBackup(row pgx.Row) (ManagedDatabaseBackupRecord, error) {
	var item ManagedDatabaseBackupRecord
	err := row.Scan(
		&item.ID,
		&item.DatabaseID,
		&item.ProjectID,
		&item.BackupPath,
		&item.SizeBytes,
		&item.Source,
		&item.CreatedAt,
	)
	return item, err
}

func (r *Repository) CreateManagedDatabaseBackup(
	ctx context.Context,
	item ManagedDatabaseBackupRecord,
) (ManagedDatabaseBackupRecord, error) {
	created, err := scanManagedDatabaseBackup(r.pool.QueryRow(ctx, `
		INSERT INTO managed_database_backups (
		  id,database_id,project_id,backup_path,size_bytes,source
		) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING `+managedDatabaseBackupProjection,
		item.ID,
		item.DatabaseID,
		item.ProjectID,
		item.BackupPath,
		item.SizeBytes,
		item.Source,
	))
	if err != nil {
		return ManagedDatabaseBackupRecord{}, mapError(err)
	}
	return created, nil
}

func (r *Repository) ListManagedDatabaseBackups(
	ctx context.Context,
	projectID, databaseID uuid.UUID,
	limit int,
) ([]ManagedDatabaseBackupRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+managedDatabaseBackupProjection+`
		FROM managed_database_backups
		WHERE project_id=$1 AND database_id=$2
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	`, projectID, databaseID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ManagedDatabaseBackupRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanManagedDatabaseBackup(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) OldestManagedDatabaseBackups(
	ctx context.Context,
	databaseID uuid.UUID,
	keep int,
) ([]ManagedDatabaseBackupRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+managedDatabaseBackupProjection+`
		FROM managed_database_backups
		WHERE database_id=$1
		ORDER BY created_at ASC, id ASC
		LIMIT GREATEST((
		  SELECT COUNT(*) FROM managed_database_backups WHERE database_id=$1
		) - $2, 0)
	`, databaseID, keep)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ManagedDatabaseBackupRecord{}
	for rows.Next() {
		item, scanErr := scanManagedDatabaseBackup(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) DeleteManagedDatabaseBackup(
	ctx context.Context,
	backupID uuid.UUID,
) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM managed_database_backups WHERE id=$1
	`, backupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ManagedDatabaseRestoreRecord is one queued or completed restore request.
type ManagedDatabaseRestoreRecord struct {
	ID          uuid.UUID
	DatabaseID  uuid.UUID
	ProjectID   uuid.UUID
	BackupPath  string
	Status      string
	Error       string
	RequestedBy uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const managedDatabaseRestoreProjection = `
  id,database_id,project_id,backup_path,status,error,requested_by,created_at,updated_at
`

func scanManagedDatabaseRestore(row pgx.Row) (ManagedDatabaseRestoreRecord, error) {
	var item ManagedDatabaseRestoreRecord
	err := row.Scan(
		&item.ID,
		&item.DatabaseID,
		&item.ProjectID,
		&item.BackupPath,
		&item.Status,
		&item.Error,
		&item.RequestedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) CreateManagedDatabaseRestore(
	ctx context.Context,
	item ManagedDatabaseRestoreRecord,
) (ManagedDatabaseRestoreRecord, error) {
	created, err := scanManagedDatabaseRestore(r.pool.QueryRow(ctx, `
		INSERT INTO managed_database_restores (
		  id,database_id,project_id,backup_path,status,requested_by
		) VALUES ($1,$2,$3,$4,'queued',$5)
		RETURNING `+managedDatabaseRestoreProjection,
		item.ID,
		item.DatabaseID,
		item.ProjectID,
		item.BackupPath,
		item.RequestedBy,
	))
	if err != nil {
		return ManagedDatabaseRestoreRecord{}, mapError(err)
	}
	return created, nil
}

// ClaimNextManagedDatabaseRestore leases the oldest queued restore. SKIP
// LOCKED keeps two workers from running the same restore against the tenant
// cluster, and only restores whose database is still owned by the same
// project are eligible.
func (r *Repository) ClaimNextManagedDatabaseRestore(
	ctx context.Context,
	leaseAge time.Duration,
) (ManagedDatabaseRestoreRecord, error) {
	item, err := scanManagedDatabaseRestore(r.pool.QueryRow(ctx, `
		UPDATE managed_database_restores AS restore
		SET status='running', updated_at=now()
		WHERE id = (
		  SELECT candidate.id
		  FROM managed_database_restores AS candidate
		  JOIN managed_databases AS db ON db.id = candidate.database_id
		  WHERE candidate.status = 'queued'
		    AND candidate.created_at <= now() - ($1::double precision * interval '1 second')
		    AND db.status = 'ready'
		  ORDER BY candidate.created_at ASC, candidate.id ASC
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED
		)
		RETURNING `+managedDatabaseRestoreProjection,
		leaseAge.Seconds()))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ManagedDatabaseRestoreRecord{}, ErrNotFound
		}
		return ManagedDatabaseRestoreRecord{}, err
	}
	return item, nil
}

func (r *Repository) FinishManagedDatabaseRestore(
	ctx context.Context,
	restoreID uuid.UUID,
	status string,
	restoreErr string,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE managed_database_restores
		SET status=$2, error=$3, updated_at=now()
		WHERE id=$1
	`, restoreID, status, restoreErr)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) LatestManagedDatabaseRestore(
	ctx context.Context,
	projectID, databaseID uuid.UUID,
) (ManagedDatabaseRestoreRecord, error) {
	item, err := scanManagedDatabaseRestore(r.pool.QueryRow(ctx, `
		SELECT `+managedDatabaseRestoreProjection+`
		FROM managed_database_restores
		WHERE project_id=$1 AND database_id=$2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, projectID, databaseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedDatabaseRestoreRecord{}, ErrNotFound
	}
	return item, err
}
