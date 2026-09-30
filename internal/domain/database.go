package domain

import (
	"encoding/json"
	"time"
)

type ProjectDatabase struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DatabaseBackup describes an immutable logical snapshot. The storage path is
// intentionally omitted; callers receive a download endpoint instead of an
// internal filesystem/S3 key.
type DatabaseBackup struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	DatabaseID     string    `json:"database_id"`
	SizeBytes      int64     `json:"size_bytes"`
	ChecksumSHA256 string    `json:"checksum_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

type DatabaseTable struct {
	ID                string    `json:"id"`
	DatabaseID        string    `json:"database_id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	RowSecurity       bool      `json:"row_security"`
	CreatePermissions []string  `json:"create_permissions"`
	ReadPermissions   []string  `json:"read_permissions"`
	UpdatePermissions []string  `json:"update_permissions"`
	DeletePermissions []string  `json:"delete_permissions"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type DatabaseColumn struct {
	ID          string          `json:"id"`
	TableID     string          `json:"table_id"`
	Key         string          `json:"key"`
	Type        string          `json:"type"`
	Required    bool            `json:"required"`
	VarcharSize *int            `json:"varchar_size,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type DatabaseIndex struct {
	ID         string    `json:"id"`
	TableID    string    `json:"table_id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	ColumnKeys []string  `json:"column_keys"`
	Directions []string  `json:"directions"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DatabaseRelationship is a tenant-scoped many-to-one reference. The source
// value is the UUID of a row in TargetTableID; the repository validates that
// target on every source-row write and protects it from deletion while used.
type DatabaseRelationship struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	DatabaseID       string    `json:"database_id"`
	SourceTableID    string    `json:"source_table_id"`
	SourceColumnKey  string    `json:"source_column_key"`
	TargetTableID    string    `json:"target_table_id"`
	RelationshipType string    `json:"type"`
	OnDelete         string    `json:"on_delete"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type DatabaseRow struct {
	ID                   string         `json:"id"`
	TableID              string         `json:"table_id"`
	ProjectID            string         `json:"project_id"`
	Data                 map[string]any `json:"data"`
	ReadPermissions      []string       `json:"read_permissions"`
	UpdatePermissions    []string       `json:"update_permissions"`
	DeletePermissions    []string       `json:"delete_permissions"`
	CreatorProjectUserID *string        `json:"creator_project_user_id,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}
