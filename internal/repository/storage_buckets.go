package repository

import (
	"context"
	"errors"
	"sort"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListStorageBuckets(ctx context.Context, projectID uuid.UUID, actor StorageActor, limit int, cursor *uuid.UUID) ([]domain.StorageBucket, string, bool, error) {
	canManage, err := r.requireStorageRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+storageBucketProjection+` FROM storage_buckets WHERE project_id=$1 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $2`, projectID, limit+1, cursor)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.StorageBucket, 0, limit)
	for rows.Next() {
		item, scanErr := scanStorageBucket(rows)
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

func (r *Repository) GetStorageBucket(ctx context.Context, projectID, bucketID uuid.UUID, actor StorageActor) (domain.StorageBucket, error) {
	if _, err := r.requireStorageRead(ctx, projectID, actor); err != nil {
		return domain.StorageBucket{}, err
	}
	return r.storageBucket(ctx, r.pool, projectID, bucketID, false)
}

func (r *Repository) CreateStorageBucket(ctx context.Context, id, projectID uuid.UUID, actor StorageActor, input StorageBucketInput) (domain.StorageBucket, error) {
	permissions, err := storageBucketPermissions(input)
	if err != nil {
		return domain.StorageBucket{}, err
	}
	if input.QuotaBytes <= 0 || input.MaxFileSizeBytes <= 0 || input.MaxFileSizeBytes > input.QuotaBytes {
		return domain.StorageBucket{}, ErrStorageQuotaExceeded
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.StorageBucket{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireStorageWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.StorageBucket{}, err
	}
	organizationID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return domain.StorageBucket{}, err
	}
	if err := r.enforceOrganizationLimitTx(ctx, tx, organizationID, "storage_buckets"); err != nil {
		return domain.StorageBucket{}, err
	}
	if err := lockStorageNamespace(ctx, tx, projectID); err != nil {
		return domain.StorageBucket{}, err
	}
	item, err := scanStorageBucket(tx.QueryRow(ctx, `INSERT INTO storage_buckets (id,project_id,name,file_security,create_permissions,read_permissions,update_permissions,delete_permissions,max_file_size_bytes,quota_bytes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+storageBucketProjection, id, projectID, input.Name, input.FileSecurity, permissions[0], permissions[1], permissions[2], permissions[3], input.MaxFileSizeBytes, input.QuotaBytes))
	if err != nil {
		return domain.StorageBucket{}, mapError(err)
	}
	if err := r.auditStorage(ctx, tx, projectID, actor, "storage_bucket.create", "storage_bucket", id, map[string]any{"name": input.Name, "quota_bytes": input.QuotaBytes}); err != nil {
		return domain.StorageBucket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.StorageBucket{}, err
	}
	return item, nil
}

func (r *Repository) UpdateStorageBucket(ctx context.Context, projectID, bucketID uuid.UUID, actor StorageActor, patch StorageBucketPatch) (domain.StorageBucket, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.StorageBucket{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireStorageWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.StorageBucket{}, err
	}
	if err := lockStorageNamespace(ctx, tx, projectID); err != nil {
		return domain.StorageBucket{}, err
	}
	item, err := r.storageBucket(ctx, tx, projectID, bucketID, true)
	if err != nil {
		return domain.StorageBucket{}, err
	}
	name := item.Name
	if patch.Name != nil {
		name = *patch.Name
	}
	fileSecurity := item.FileSecurity
	if patch.FileSecurity != nil {
		fileSecurity = *patch.FileSecurity
	}
	createPermissions := item.CreatePermissions
	if patch.CreatePermissions != nil {
		createPermissions, err = normalizeStoragePermissions(*patch.CreatePermissions)
		if err != nil {
			return domain.StorageBucket{}, err
		}
	}
	readPermissions := item.ReadPermissions
	if patch.ReadPermissions != nil {
		readPermissions, err = normalizeStoragePermissions(*patch.ReadPermissions)
		if err != nil {
			return domain.StorageBucket{}, err
		}
	}
	updatePermissions := item.UpdatePermissions
	if patch.UpdatePermissions != nil {
		updatePermissions, err = normalizeStoragePermissions(*patch.UpdatePermissions)
		if err != nil {
			return domain.StorageBucket{}, err
		}
	}
	deletePermissions := item.DeletePermissions
	if patch.DeletePermissions != nil {
		deletePermissions, err = normalizeStoragePermissions(*patch.DeletePermissions)
		if err != nil {
			return domain.StorageBucket{}, err
		}
	}
	quota := item.QuotaBytes
	if patch.QuotaBytes != nil {
		quota = *patch.QuotaBytes
	}
	maxFileSize := item.MaxFileSizeBytes
	if patch.MaxFileSizeBytes != nil {
		maxFileSize = *patch.MaxFileSizeBytes
	}
	if quota <= 0 || quota < item.UsedBytes || maxFileSize <= 0 || maxFileSize > quota {
		return domain.StorageBucket{}, ErrStorageQuotaExceeded
	}
	item, err = scanStorageBucket(tx.QueryRow(ctx, `UPDATE storage_buckets SET name=$3,file_security=$4,create_permissions=$5,read_permissions=$6,update_permissions=$7,delete_permissions=$8,max_file_size_bytes=$9,quota_bytes=$10,updated_at=now() WHERE project_id=$1 AND id=$2 RETURNING `+storageBucketProjection, projectID, bucketID, name, fileSecurity, createPermissions, readPermissions, updatePermissions, deletePermissions, maxFileSize, quota))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StorageBucket{}, ErrNotFound
	}
	if err != nil {
		return domain.StorageBucket{}, mapError(err)
	}
	if err := r.auditStorage(ctx, tx, projectID, actor, "storage_bucket.update", "storage_bucket", bucketID, map[string]any{"changed_fields": storageBucketChangedFields(patch)}); err != nil {
		return domain.StorageBucket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.StorageBucket{}, err
	}
	return item, nil
}

func storageBucketChangedFields(patch StorageBucketPatch) []string {
	fields := make([]string, 0, 6)
	if patch.Name != nil {
		fields = append(fields, "name")
	}
	if patch.FileSecurity != nil {
		fields = append(fields, "file_security")
	}
	if patch.CreatePermissions != nil {
		fields = append(fields, "create_permissions")
	}
	if patch.ReadPermissions != nil {
		fields = append(fields, "read_permissions")
	}
	if patch.UpdatePermissions != nil {
		fields = append(fields, "update_permissions")
	}
	if patch.DeletePermissions != nil {
		fields = append(fields, "delete_permissions")
	}
	if patch.QuotaBytes != nil {
		fields = append(fields, "quota_bytes")
	}
	if patch.MaxFileSizeBytes != nil {
		fields = append(fields, "max_file_size_bytes")
	}
	sort.Strings(fields)
	return fields
}

// DeleteStorageBucket removes metadata/accounting and records durable
// UUID-derived cleanup jobs in the same transaction.
func (r *Repository) DeleteStorageBucket(ctx context.Context, projectID, bucketID uuid.UUID, actor StorageActor) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireStorageWriteTx(ctx, tx, projectID, actor); err != nil {
		return nil, err
	}
	if err := lockStorageNamespace(ctx, tx, projectID); err != nil {
		return nil, err
	}
	if _, err := r.storageBucket(ctx, tx, projectID, bucketID, true); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT storage_path FROM storage_files WHERE project_id=$1 AND bucket_id=$2 FOR UPDATE`, projectID, bucketID)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, path := range paths {
		if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{
			ProjectID: projectID, StoreKind: ArtifactCleanupStorage,
			Operation: ArtifactCleanupRelative, RelativePath: path,
		}); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM storage_buckets WHERE project_id=$1 AND id=$2`, projectID, bucketID); err != nil {
		return nil, err
	}
	if err := r.auditStorage(ctx, tx, projectID, actor, "storage_bucket.delete", "storage_bucket", bucketID, map[string]any{"file_count": len(paths)}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return paths, nil
}
