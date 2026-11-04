package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const managedDatabaseProjection = `
  id,project_id,name,database_name,username,password_ciphertext,status,connection_limit,write_locked,created_at,updated_at
`

// ManagedDatabaseRecord is the private persistence shape. PasswordCiphertext
// must never be serialized by an HTTP handler.
type ManagedDatabaseRecord struct {
	ID                 uuid.UUID
	ProjectID          uuid.UUID
	Name               string
	DatabaseName       string
	Username           string
	PasswordCiphertext []byte
	Status             string
	ConnectionLimit    int
	WriteLocked        bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func scanManagedDatabase(row pgx.Row) (ManagedDatabaseRecord, error) {
	var item ManagedDatabaseRecord
	err := row.Scan(
		&item.ID,
		&item.ProjectID,
		&item.Name,
		&item.DatabaseName,
		&item.Username,
		&item.PasswordCiphertext,
		&item.Status,
		&item.ConnectionLimit,
		&item.WriteLocked,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) CreateManagedDatabase(
	ctx context.Context,
	item ManagedDatabaseRecord,
) (ManagedDatabaseRecord, error) {
	created, err := scanManagedDatabase(r.pool.QueryRow(ctx, `
		INSERT INTO managed_databases (
		  id,project_id,name,database_name,username,password_ciphertext,status,connection_limit
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+managedDatabaseProjection,
		item.ID,
		item.ProjectID,
		item.Name,
		item.DatabaseName,
		item.Username,
		item.PasswordCiphertext,
		item.Status,
		item.ConnectionLimit,
	))
	if err != nil {
		return ManagedDatabaseRecord{}, mapError(err)
	}
	return created, nil
}

func (r *Repository) ListManagedDatabases(
	ctx context.Context,
	projectID uuid.UUID,
) ([]ManagedDatabaseRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+managedDatabaseProjection+`
		FROM managed_databases
		WHERE project_id=$1
		ORDER BY created_at ASC, id ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ManagedDatabaseRecord, 0, 1)
	for rows.Next() {
		item, scanErr := scanManagedDatabase(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListAllReadyManagedDatabases returns every ready database across projects.
// It backs the maintenance worker's usage sweep; the Console never calls it.
func (r *Repository) ListAllReadyManagedDatabases(
	ctx context.Context,
) ([]ManagedDatabaseRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+managedDatabaseProjection+`
		FROM managed_databases
		WHERE status='ready'
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ManagedDatabaseRecord, 0, 8)
	for rows.Next() {
		item, scanErr := scanManagedDatabase(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ManagedDatabaseByID(
	ctx context.Context,
	projectID,
	databaseID uuid.UUID,
) (ManagedDatabaseRecord, error) {
	item, err := scanManagedDatabase(r.pool.QueryRow(ctx, `
		SELECT `+managedDatabaseProjection+`
		FROM managed_databases
		WHERE project_id=$1 AND id=$2
	`, projectID, databaseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedDatabaseRecord{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) UpdateManagedDatabaseStatus(
	ctx context.Context,
	projectID,
	databaseID uuid.UUID,
	status string,
) (ManagedDatabaseRecord, error) {
	item, err := scanManagedDatabase(r.pool.QueryRow(ctx, `
		UPDATE managed_databases
		SET status=$3, updated_at=now()
		WHERE project_id=$1 AND id=$2
		RETURNING `+managedDatabaseProjection,
		projectID,
		databaseID,
		status,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedDatabaseRecord{}, ErrNotFound
	}
	return item, err
}

// SetManagedDatabaseWriteLock flips the fair-use write lock. The worker calls
// it after measuring usage; the API only reads the flag.
func (r *Repository) SetManagedDatabaseWriteLock(
	ctx context.Context,
	databaseID uuid.UUID,
	writeLocked bool,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE managed_databases
		SET write_locked=$2, updated_at=now()
		WHERE id=$1
	`, databaseID, writeLocked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateManagedDatabasePassword(
	ctx context.Context,
	projectID,
	databaseID uuid.UUID,
	ciphertext []byte,
) (ManagedDatabaseRecord, error) {
	item, err := scanManagedDatabase(r.pool.QueryRow(ctx, `
		UPDATE managed_databases
		SET password_ciphertext=$3, updated_at=now()
		WHERE project_id=$1 AND id=$2
		RETURNING `+managedDatabaseProjection,
		projectID,
		databaseID,
		ciphertext,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedDatabaseRecord{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) DeleteManagedDatabase(
	ctx context.Context,
	projectID,
	databaseID uuid.UUID,
) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM managed_databases
		WHERE project_id=$1 AND id=$2
	`, projectID, databaseID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AuditManagedDatabase writes a tenant audit event for a managed-database
// operation. It is separate from the App/database audit helpers because those
// require an App or a DatabaseActor.
func (r *Repository) AuditManagedDatabase(
	ctx context.Context,
	projectID,
	actorAccountID,
	targetID uuid.UUID,
	action string,
	metadata map[string]any,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["project_id"] = projectID.String()
	if err := writeAuditMetadata(ctx, tx, orgID, actorAccountID, action, "managed_database", targetID, metadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
