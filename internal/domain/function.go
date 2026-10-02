package domain

import (
	"encoding/json"
	"time"
)

// Function is the tenant-scoped metadata for a deployable function. Source
// bytes and variable values are intentionally absent from this DTO.
type Function struct {
	ID                    string    `json:"id"`
	ProjectID             string    `json:"project_id"`
	Name                  string    `json:"name"`
	Runtime               string    `json:"runtime"`
	Entrypoint            string    `json:"entrypoint"`
	Commands              string    `json:"commands"`
	TimeoutSeconds        int       `json:"timeout_seconds"`
	Enabled               bool      `json:"enabled"`
	Logging               bool      `json:"logging"`
	ExecutePermissions    []string  `json:"execute_permissions"`
	Description           *string   `json:"description,omitempty"`
	Status                string    `json:"status"`
	ArtifactQuotaBytes    int64     `json:"artifact_quota_bytes"`
	ArtifactUsedBytes     int64     `json:"artifact_used_bytes"`
	ArtifactReservedBytes int64     `json:"artifact_reserved_bytes"`
	ActiveDeploymentID    *string   `json:"active_deployment_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// FunctionVariable is metadata only. HasValue lets a caller distinguish an
// unset variable from an explicitly configured one without exposing either a
// plaintext value or encrypted/hash material.
type FunctionVariable struct {
	ID          string    `json:"id"`
	FunctionID  string    `json:"function_id"`
	ProjectID   string    `json:"project_id"`
	Key         string    `json:"key"`
	Kind        string    `json:"kind"`
	IsSecret    bool      `json:"is_secret"`
	HasValue    bool      `json:"has_value"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FunctionDeployment exposes build/activation metadata while keeping the
// UUID-derived source path private.
type FunctionDeployment struct {
	ID                 string     `json:"id"`
	FunctionID         string     `json:"function_id"`
	ProjectID          string     `json:"project_id"`
	Version            int64      `json:"version"`
	Source             string     `json:"source"`
	SourceName         *string    `json:"source_name,omitempty"`
	SizeBytes          int64      `json:"size_bytes"`
	ChecksumSHA256     string     `json:"checksum_sha256"`
	Status             string     `json:"status"`
	BuildStatus        string     `json:"build_status"`
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

type FunctionBuildLog struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	FunctionID   string    `json:"function_id"`
	ProjectID    string    `json:"project_id"`
	Sequence     int64     `json:"sequence"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}

type FunctionExecution struct {
	ID                string          `json:"id"`
	DeploymentID      string          `json:"deployment_id"`
	FunctionID        string          `json:"function_id"`
	ProjectID         string          `json:"project_id"`
	Status            string          `json:"status"`
	Trigger           string          `json:"trigger"`
	InputJSON         json.RawMessage `json:"input_json,omitempty"`
	ResponseStatus    *int            `json:"response_status,omitempty"`
	OutputJSON        json.RawMessage `json:"output_json,omitempty"`
	OutputContentType *string         `json:"output_content_type,omitempty"`
	ErrorMessage      *string         `json:"error_message,omitempty"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type FunctionExecutionLog struct {
	ID          string    `json:"id"`
	ExecutionID string    `json:"execution_id"`
	FunctionID  string    `json:"function_id"`
	ProjectID   string    `json:"project_id"`
	Sequence    int64     `json:"sequence"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
}
