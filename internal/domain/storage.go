package domain

import "time"

// StorageBucket is safe to return to Console and project data callers. The
// filesystem path is intentionally not part of this DTO.
type StorageBucket struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	FileSecurity      bool      `json:"file_security"`
	CreatePermissions []string  `json:"create_permissions"`
	ReadPermissions   []string  `json:"read_permissions"`
	UpdatePermissions []string  `json:"update_permissions"`
	DeletePermissions []string  `json:"delete_permissions"`
	MaxFileSizeBytes  int64     `json:"max_file_size_bytes"`
	QuotaBytes        int64     `json:"quota_bytes"`
	UsedBytes         int64     `json:"used_bytes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// StorageFile contains metadata only. Blob bytes are always streamed from the
// local store and are never serialized into PostgreSQL or this response.
type StorageFile struct {
	ID                   string    `json:"id"`
	BucketID             string    `json:"bucket_id"`
	ProjectID            string    `json:"project_id"`
	Name                 string    `json:"name"`
	MimeType             string    `json:"mime_type"`
	SizeBytes            int64     `json:"size_bytes"`
	ChecksumSHA256       string    `json:"checksum_sha256"`
	ReadPermissions      []string  `json:"read_permissions"`
	UpdatePermissions    []string  `json:"update_permissions"`
	DeletePermissions    []string  `json:"delete_permissions"`
	CreatorProjectUserID *string   `json:"creator_project_user_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
