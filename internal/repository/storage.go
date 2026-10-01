package repository

import (
	"context"
	"errors"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrStorageQuotaExceeded = errors.New("storage bucket quota exceeded")
	ErrStorageFileTooLarge  = errors.New("storage file is too large")
)

// StorageActor intentionally aliases the Database actor model. Both services
// have the same four authentication provenance classes, and sharing the type
// keeps it impossible for a Console account to be fabricated for an app user.
type StorageActor = DatabaseActor

const (
	StorageConsoleActor     = DatabaseConsoleActor
	StorageAPIKeyActor      = DatabaseAPIKeyActor
	StorageApplicationActor = DatabaseApplicationActor
	StorageAnonymousActor   = DatabaseAnonymousActor
)

type StorageBucketInput struct {
	Name              string
	FileSecurity      bool
	CreatePermissions []string
	ReadPermissions   []string
	UpdatePermissions []string
	DeletePermissions []string
	MaxFileSizeBytes  int64
	QuotaBytes        int64
}

type StorageBucketPatch struct {
	Name              *string
	FileSecurity      *bool
	CreatePermissions *[]string
	ReadPermissions   *[]string
	UpdatePermissions *[]string
	DeletePermissions *[]string
	MaxFileSizeBytes  *int64
	QuotaBytes        *int64
}

type StorageFileInput struct {
	Name                 string
	MimeType             string
	SizeBytes            int64
	ChecksumSHA256       string
	StoragePath          string
	ReadPermissions      *[]string
	UpdatePermissions    *[]string
	DeletePermissions    *[]string
	CreatorProjectUserID *uuid.UUID
	PublishCleanup       *ArtifactCleanupInput
}

type StorageFilePatch struct {
	Name              *string
	ReadPermissions   *[]string
	UpdatePermissions *[]string
	DeletePermissions *[]string
}

const storageBucketProjection = `id,project_id,name,file_security,create_permissions,read_permissions,update_permissions,delete_permissions,max_file_size_bytes,quota_bytes,used_bytes,created_at,updated_at`
const storageFileProjection = `id,bucket_id,project_id,name,mime_type,size_bytes,checksum_sha256,storage_path,read_permissions,update_permissions,delete_permissions,creator_project_user_id,created_at,updated_at`

type storageScanner interface{ Scan(...any) error }

func scanStorageBucket(row storageScanner) (domain.StorageBucket, error) {
	var item domain.StorageBucket
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.FileSecurity, &item.CreatePermissions, &item.ReadPermissions, &item.UpdatePermissions, &item.DeletePermissions, &item.MaxFileSizeBytes, &item.QuotaBytes, &item.UsedBytes, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanStorageFile(row storageScanner) (domain.StorageFile, string, error) {
	var item domain.StorageFile
	var path string
	var creator *uuid.UUID
	err := row.Scan(&item.ID, &item.BucketID, &item.ProjectID, &item.Name, &item.MimeType, &item.SizeBytes, &item.ChecksumSHA256, &path, &item.ReadPermissions, &item.UpdatePermissions, &item.DeletePermissions, &creator, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, "", err
	}
	if creator != nil {
		value := creator.String()
		item.CreatorProjectUserID = &value
	}
	return item, path, nil
}

func (r *Repository) storageBucket(ctx context.Context, query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, projectID, bucketID uuid.UUID, lock bool) (domain.StorageBucket, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	item, err := scanStorageBucket(query.QueryRow(ctx, `SELECT `+storageBucketProjection+` FROM storage_buckets WHERE project_id=$1 AND id=$2`+suffix, projectID, bucketID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StorageBucket{}, ErrNotFound
	}
	return item, err
}
