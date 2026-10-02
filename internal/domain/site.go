package domain

import "time"

// Site is project-scoped static hosting metadata. Files are kept in a
// private immutable directory and are intentionally absent from this DTO.
type Site struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	// PlatformHostname is derived from the stable database platform label and
	// the current optional instance workload domain. It is null until an
	// operator configures that domain; the label itself is intentionally not a
	// public API field.
	PlatformHostname      *string   `json:"platform_hostname"`
	Framework             string    `json:"framework"`
	Enabled               bool      `json:"enabled"`
	Status                string    `json:"status"`
	ArtifactQuotaBytes    int64     `json:"artifact_quota_bytes"`
	ArtifactUsedBytes     int64     `json:"artifact_used_bytes"`
	ArtifactReservedBytes int64     `json:"artifact_reserved_bytes"`
	ActiveDeploymentID    *string   `json:"active_deployment_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// SiteDomain binds a verified DNS hostname to a Site. The verification token
// is a public challenge value, not an API credential; it is returned so an
// operator can publish the required TXT record.
type SiteDomain struct {
	ID                      string     `json:"id"`
	ProjectID               string     `json:"project_id"`
	SiteID                  string     `json:"site_id"`
	Hostname                string     `json:"hostname"`
	Status                  string     `json:"status"`
	VerificationToken       string     `json:"verification_token"`
	VerificationRecordName  string     `json:"verification_record_name"`
	VerificationRecordType  string     `json:"verification_record_type"`
	VerificationRecordValue string     `json:"verification_record_value"`
	VerifiedAt              *time.Time `json:"verified_at,omitempty"`
	TLSStatus               string     `json:"tls_status"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// SiteDeployment exposes publication metadata while keeping the internal
// UUID-derived artifact path private.
type SiteDeployment struct {
	ID                 string     `json:"id"`
	SiteID             string     `json:"site_id"`
	ProjectID          string     `json:"project_id"`
	Version            int64      `json:"version"`
	Source             string     `json:"source"`
	SourceName         *string    `json:"source_name,omitempty"`
	GitRepository      *string    `json:"git_repository,omitempty"`
	GitRef             *string    `json:"git_ref,omitempty"`
	SizeBytes          int64      `json:"size_bytes"`
	ArchiveSizeBytes   int64      `json:"archive_size_bytes"`
	ChecksumSHA256     string     `json:"checksum_sha256"`
	Status             string     `json:"status"`
	BuildRuntime       string     `json:"build_runtime,omitempty"`
	BuildCommand       string     `json:"build_command,omitempty"`
	OutputDirectory    string     `json:"output_directory,omitempty"`
	BuildStatus        string     `json:"build_status"`
	ActivateRequested  bool       `json:"activate_requested,omitempty"`
	ReservedBytes      int64      `json:"reserved_bytes,omitempty"`
	ErrorMessage       *string    `json:"error_message,omitempty"`
	CreatedByAccountID *string    `json:"created_by_account_id,omitempty"`
	QueuedAt           time.Time  `json:"queued_at"`
	BuildStartedAt     *time.Time `json:"build_started_at,omitempty"`
	BuiltAt            *time.Time `json:"built_at,omitempty"`
	ActivatedAt        *time.Time `json:"activated_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// SiteBuildLog is a bounded, secret-safe lifecycle message emitted by the
// trusted Site build worker. Deployment and tenant identifiers are included
// so callers can safely render logs without joining private storage paths.
type SiteBuildLog struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	SiteID       string    `json:"site_id"`
	ProjectID    string    `json:"project_id"`
	Sequence     int64     `json:"sequence"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}
