package repository

import (
	"context"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func normalizeStoragePermissions(raw []string) ([]string, error) {
	return database.NormalizePermissions(raw)
}

func storageBucketPermissions(input StorageBucketInput) ([4][]string, error) {
	values := [][]string{input.CreatePermissions, input.ReadPermissions, input.UpdatePermissions, input.DeletePermissions}
	var result [4][]string
	for i, value := range values {
		permissions, err := normalizeStoragePermissions(value)
		if err != nil {
			return result, err
		}
		result[i] = permissions
	}
	return result, nil
}

func storageBucketCanManage(actor StorageActor, role string) bool {
	if actor.Kind == StorageConsoleActor {
		return role == "owner" || role == "admin"
	}
	return actor.Kind == StorageAPIKeyActor && apikey.HasScope(actor.APIKeyScopes, "storage.write")
}

// requireStorageRead authorizes project metadata reads and returns whether the
// actor may manage buckets. Application and anonymous callers are deliberately
// excluded from bucket-management endpoints; they use file data endpoints.
func (r *Repository) requireStorageRead(ctx context.Context, projectID uuid.UUID, actor StorageActor) (bool, error) {
	switch actor.Kind {
	case StorageConsoleActor:
		role, err := r.projectRole(ctx, projectID, actor.AccountID)
		if err != nil {
			return false, err
		}
		return storageBucketCanManage(actor, role), nil
	case StorageAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "storage.read") {
			return false, ErrForbidden
		}
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, projectID).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, ErrNotFound
		}
		return apikey.HasScope(actor.APIKeyScopes, "storage.write"), nil
	default:
		return false, ErrForbidden
	}
}

func (r *Repository) requireStorageWriteTx(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, actor StorageActor) error {
	switch actor.Kind {
	case StorageConsoleActor:
		return requireProjectRoleTx(ctx, tx, projectID, actor.AccountID, "owner", "admin")
	case StorageAPIKeyActor:
		if !apikey.HasScope(actor.APIKeyScopes, "storage.write") {
			return ErrForbidden
		}
		return requireActiveProjectAPIKeyTx(ctx, tx, projectID, actor.APIKeyID, "storage.write")
	default:
		return ErrForbidden
	}
}

func storageBucketPermission(bucket domain.StorageBucket, actor StorageActor, operation string) bool {
	var permissions []string
	switch operation {
	case "create":
		permissions = bucket.CreatePermissions
	case "read":
		permissions = bucket.ReadPermissions
	case "update":
		permissions = bucket.UpdatePermissions
	case "delete":
		permissions = bucket.DeletePermissions
	default:
		return false
	}
	return tablePermission(permissions, actor)
}

func requireStorageBucketPermission(bucket domain.StorageBucket, actor StorageActor, operation string) error {
	switch actor.Kind {
	case StorageConsoleActor, StorageAPIKeyActor:
		return nil
	case StorageApplicationActor, StorageAnonymousActor:
		if storageBucketPermission(bucket, actor, operation) {
			return nil
		}
		return ErrForbidden
	default:
		return ErrForbidden
	}
}

func (r *Repository) AuthorizeStorageBucket(ctx context.Context, projectID, bucketID uuid.UUID, actor StorageActor, operation string) (domain.StorageBucket, error) {
	if actor.IsManagement() {
		if operation == "read" {
			if _, err := r.requireStorageRead(ctx, projectID, actor); err != nil {
				return domain.StorageBucket{}, err
			}
		} else {
			tx, err := r.pool.Begin(ctx)
			if err != nil {
				return domain.StorageBucket{}, err
			}
			defer tx.Rollback(ctx)
			if err := r.requireStorageWriteTx(ctx, tx, projectID, actor); err != nil {
				return domain.StorageBucket{}, err
			}
			item, err := r.storageBucket(ctx, tx, projectID, bucketID, false)
			if err != nil {
				return domain.StorageBucket{}, err
			}
			return item, nil
		}
		return r.storageBucket(ctx, r.pool, projectID, bucketID, false)
	}
	item, err := r.storageBucket(ctx, r.pool, projectID, bucketID, false)
	if err != nil {
		return domain.StorageBucket{}, err
	}
	if err := requireStorageBucketPermission(item, actor, operation); err != nil {
		return domain.StorageBucket{}, err
	}
	return item, nil
}
