package domain

import (
	"time"

	"github.com/Stealth-deplover/stealth/internal/workloadspec"
)

// App is durable desired configuration for a persistent project workload.
// Runtime status and observed generation remain owned by a trusted runtime
// reconciler; creating or updating metadata never claims that work ran.
type App struct {
	ID                   string            `json:"id"`
	ProjectID            string            `json:"project_id"`
	Name                 string            `json:"name"`
	Enabled              bool              `json:"enabled"`
	PlatformHostname     *string           `json:"platform_hostname"`
	Workload             workloadspec.Spec `json:"workload"`
	WorkloadSpecSHA256   string            `json:"workload_spec_sha256"`
	DesiredGeneration    int64             `json:"desired_generation"`
	ObservedGeneration   int64             `json:"observed_generation"`
	DesiredDeploymentID  *string           `json:"desired_deployment_id"`
	RuntimeStatus        string            `json:"runtime_status"`
	RuntimeError         *string           `json:"runtime_error"`
	HealthStatus         string            `json:"health_status"`
	RouteStatus          string            `json:"route_status"`
	RouteDeploymentReady bool              `json:"-"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// AppDiagnosticDeploymentRef identifies a release without exposing its
// private artifact locator or Docker runtime identity.
type AppDiagnosticDeploymentRef struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

// AppDiagnosticIssue is a bounded, user-safe explanation of persisted App
// convergence state.
type AppDiagnosticIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// AppDiagnostics is a safe PostgreSQL-derived projection. Worker leases,
// Docker identities, and artifact paths are intentionally excluded.
type AppDiagnostics struct {
	AppID                string                      `json:"app_id"`
	ProjectID            string                      `json:"project_id"`
	ConvergenceStatus    string                      `json:"convergence_status"`
	DesiredGeneration    int64                       `json:"desired_generation"`
	ObservedGeneration   int64                       `json:"observed_generation"`
	AppliedGeneration    *int64                      `json:"applied_generation"`
	DesiredDeployment    *AppDiagnosticDeploymentRef `json:"desired_deployment"`
	AppliedDeployment    *AppDiagnosticDeploymentRef `json:"applied_deployment"`
	DesiredArtifactReady bool                        `json:"desired_artifact_ready"`
	RuntimeStatus        string                      `json:"runtime_status"`
	RuntimeError         *string                     `json:"runtime_error"`
	HealthStatus         string                      `json:"health_status"`
	RouteStatus          string                      `json:"route_status"`
	FailureCount         int                         `json:"failure_count"`
	NextRetryAt          *time.Time                  `json:"next_retry_at"`
	LastFailureAt        *time.Time                  `json:"last_failure_at"`
	LastInspectedAt      *time.Time                  `json:"last_inspected_at"`
	LastTransitionAt     *time.Time                  `json:"last_transition_at"`
	LastStartedAt        *time.Time                  `json:"last_started_at"`
	LastStoppedAt        *time.Time                  `json:"last_stopped_at"`
	HealthCheckedAt      *time.Time                  `json:"health_checked_at"`
	Issues               []AppDiagnosticIssue        `json:"issues"`
}

// AppEnvironmentVariable is a safe metadata projection. Values are
// write-only and ciphertext never crosses the repository HTTP boundary.
type AppEnvironmentVariable struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ProjectID   string    `json:"project_id"`
	Key         string    `json:"key"`
	IsSecret    bool      `json:"is_secret"`
	HasValue    bool      `json:"has_value"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AppDeployment is the immutable build identity and verified output for one
// App release. Private source and image storage locators and worker lease IDs
// are intentionally absent from this public projection.
type AppDeployment struct {
	ID                   string            `json:"id"`
	AppID                string            `json:"app_id"`
	ProjectID            string            `json:"project_id"`
	Version              int64             `json:"version"`
	Source               string            `json:"source"`
	SourceName           *string           `json:"source_name,omitempty"`
	SourceSizeBytes      int64             `json:"source_size_bytes"`
	SourceChecksumSHA256 string            `json:"source_checksum_sha256"`
	DockerfilePath       string            `json:"dockerfile_path"`
	ContextDirectory     string            `json:"context_directory"`
	Target               *string           `json:"target"`
	Platform             string            `json:"platform"`
	WorkloadSnapshot     workloadspec.Spec `json:"workload_snapshot"`
	WorkloadSpecSHA256   string            `json:"workload_spec_sha256"`
	Status               string            `json:"status"`
	BuildStatus          string            `json:"build_status"`
	ErrorMessage         *string           `json:"error_message"`
	ImageDigest          *string           `json:"image_digest"`
	ImageArchiveSHA256   *string           `json:"image_archive_sha256"`
	ImageSizeBytes       *int64            `json:"image_size_bytes"`
	Selected             bool              `json:"selected"`
	CreatedByAccountID   *string           `json:"created_by_account_id"`
	QueuedAt             time.Time         `json:"queued_at"`
	BuildStartedAt       *time.Time        `json:"build_started_at"`
	BuiltAt              *time.Time        `json:"built_at"`
	FinishedAt           *time.Time        `json:"finished_at"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// AppBuildLog is a bounded tenant-scoped lifecycle event from the trusted
// App build worker. It contains no command line, worker environment, or path.
type AppBuildLog struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deployment_id"`
	AppID        string    `json:"app_id"`
	ProjectID    string    `json:"project_id"`
	Sequence     int64     `json:"sequence"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}

// AppRuntimeLog is a redacted, project-scoped projection of App stdout/stderr.
// Its stable ID is opaque and does not reveal the ClickHouse event ID.
type AppRuntimeLog struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

type AppRuntimeLogsResponse struct {
	Logs       []AppRuntimeLog `json:"logs"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
