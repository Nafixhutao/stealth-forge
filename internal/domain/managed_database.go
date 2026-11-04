package domain

import "time"

// ManagedDatabase is one customer-owned MySQL database on the shared tenant
// cluster. The password never appears here; it is revealed through a separate
// audited operation.
type ManagedDatabase struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	Host              string    `json:"host"`
	Port              int       `json:"port"`
	DatabaseName      string    `json:"database_name"`
	Username          string    `json:"username"`
	PHPMyAdminURL     string    `json:"phpmyadmin_url"`
	SizeBytes         int64     `json:"size_bytes"`
	ConnectionCount   int       `json:"connection_count"`
	ConnectionLimit   int       `json:"connection_limit"`
	StorageLimitBytes int64     `json:"storage_limit_bytes"`
	WriteLocked       bool      `json:"write_locked"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ManagedDatabaseBackup is one completed snapshot in the backup ledger. It is
// safe to serialize: it carries no credentials, only a validated relative
// path inside the shared backups volume.
type ManagedDatabaseBackup struct {
	ID         string    `json:"id"`
	DatabaseID string    `json:"database_id"`
	BackupPath string    `json:"backup_path"`
	SizeBytes  int64     `json:"size_bytes"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

// ManagedDatabaseRestore is one queued or completed restore. Status moves
// queued -> running -> succeeded|failed as the worker executes it.
type ManagedDatabaseRestore struct {
	ID         string    `json:"id"`
	DatabaseID string    `json:"database_id"`
	BackupPath string    `json:"backup_path"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}