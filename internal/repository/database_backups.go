package repository

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/Stealth-deplover/stealth/internal/domain"
)

const (
	DatabaseBackupVersion        = 1
	DatabaseBackupDefaultMaxRows = 10000
	DatabaseBackupMaxRows        = 10000
	DatabaseBackupMaxTables      = 256
	DatabaseBackupMaxRelations   = 1024
	DatabaseBackupMaxBytes       = 50 << 20
)

const databaseBackupProjection = `id,project_id,database_id,storage_path,size_bytes,checksum_sha256,created_at`

// DatabaseBackupSnapshot is a portable logical snapshot. It intentionally
// contains only database metadata and typed rows; storage paths, credentials,
// and internal PostgreSQL names never leave the repository.
type DatabaseBackupSnapshot struct {
	Version       int                           `json:"version"`
	ProjectID     string                        `json:"project_id"`
	DatabaseID    string                        `json:"database_id"`
	DatabaseName  string                        `json:"database_name"`
	Tables        []DatabaseBackupTable         `json:"tables"`
	Relationships []domain.DatabaseRelationship `json:"relationships"`
}

type DatabaseBackupTable struct {
	Table   domain.DatabaseTable    `json:"table"`
	Columns []domain.DatabaseColumn `json:"columns"`
	Indexes []domain.DatabaseIndex  `json:"indexes"`
	Rows    []domain.DatabaseRow    `json:"rows"`
}

type DatabaseBackupRestoreResult struct {
	Tables        int `json:"tables"`
	Columns       int `json:"columns"`
	Indexes       int `json:"indexes"`
	Rows          int `json:"rows"`
	Relationships int `json:"relationships"`
}

func scanDatabaseBackup(row interface{ Scan(...any) error }) (domain.DatabaseBackup, string, error) {
	var item domain.DatabaseBackup
	var path string
	err := row.Scan(&item.ID, &item.ProjectID, &item.DatabaseID, &path, &item.SizeBytes, &item.ChecksumSHA256, &item.CreatedAt)
	return item, path, err
}

func BackupChecksum(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// BuildDatabaseBackup reads a bounded management snapshot. The caller stores
// the returned JSON through the configured BlobStore after this method returns.
