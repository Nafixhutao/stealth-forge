package repository

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextFunctionDeployment(ctx context.Context, workerID string) (FunctionBuildJob, error) {
	if !validFunctionWorkerID(workerID) {
		return FunctionBuildJob{}, ErrInvalidFunctionSettings
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return FunctionBuildJob{}, err
	}
	defer tx.Rollback(ctx)
	var deploymentID, projectID, functionID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT d.id,d.project_id,d.function_id
		FROM function_deployments d
		JOIN project_functions f ON f.id=d.function_id AND f.project_id=d.project_id
		WHERE d.status IN ('ready','active') AND d.build_status IN ('queued','deferred')
		ORDER BY d.queued_at,d.id
		LIMIT 1
		FOR UPDATE OF f SKIP LOCKED`).Scan(&deploymentID, &projectID, &functionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return FunctionBuildJob{}, ErrNoDeploymentJob
	}
	if err != nil {
		return FunctionBuildJob{}, err
	}
	function, err := r.functionByID(ctx, tx, projectID, functionID, true)
	if err != nil {
		return FunctionBuildJob{}, err
	}
	deployment, sourcePath, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return FunctionBuildJob{}, err
	}
	if deployment.BuildStatus != "queued" && deployment.BuildStatus != "deferred" {
		return FunctionBuildJob{}, ErrNoDeploymentJob
	}
	function, err = r.functionDeploymentRuntimeConfigTx(ctx, tx, projectID, functionID, deploymentID, function)
	if err != nil {
		return FunctionBuildJob{}, err
	}
	deployment, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET build_status='running',build_started_at=now(),build_worker_id=$4,updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 AND build_status IN ('queued','deferred') RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID, workerID))
	if err != nil {
		return FunctionBuildJob{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "function_deployment.updated", "function_deployment", deploymentID, map[string]any{"function_id": functionID.String(), "status": deployment.Status, "build_status": deployment.BuildStatus}); err != nil {
		return FunctionBuildJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FunctionBuildJob{}, err
	}
	return FunctionBuildJob{Function: function, Deployment: deployment, SourcePath: sourcePath}, nil
}

// RequeueStaleFunctionDeployments makes a crashed builder's work available to
// another worker. It does not alter activation status, only build lease data.
func (r *Repository) RequeueStaleFunctionDeployments(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, ErrInvalidFunctionSettings
	}
	result, err := r.pool.Exec(ctx, `UPDATE function_deployments SET build_status='deferred',build_started_at=NULL,build_worker_id=NULL,updated_at=now() WHERE build_status='running' AND build_started_at IS NOT NULL AND build_started_at < now() - ($1::double precision * interval '1 second')`, maxAge.Seconds())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

// CompleteFunctionDeploymentBuild publishes the worker-produced immutable
// archive and adjusts artifact quota for its final size in one transaction.
func (r *Repository) CompleteFunctionDeploymentBuild(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, workerID, buildPath string, buildSizeBytes int64, buildChecksumSHA256 string) (domain.FunctionDeployment, error) {
	return r.completeFunctionDeploymentBuild(ctx, projectID, functionID, deploymentID, workerID, buildPath, buildSizeBytes, buildChecksumSHA256, nil)
}

func (r *Repository) CompleteFunctionDeploymentBuildWithCleanup(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, workerID, buildPath string, buildSizeBytes int64, buildChecksumSHA256 string, cleanup ArtifactCleanupInput) (domain.FunctionDeployment, error) {
	return r.completeFunctionDeploymentBuild(ctx, projectID, functionID, deploymentID, workerID, buildPath, buildSizeBytes, buildChecksumSHA256, &cleanup)
}

func (r *Repository) completeFunctionDeploymentBuild(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, workerID, buildPath string, buildSizeBytes int64, buildChecksumSHA256 string, cleanup *ArtifactCleanupInput) (domain.FunctionDeployment, error) {
	if !validFunctionWorkerID(workerID) || !validFunctionArtifactPath(buildPath) || buildSizeBytes <= 0 || !validSHA256(buildChecksumSHA256) {
		return domain.FunctionDeployment{}, ErrInvalidFunctionSettings
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	defer tx.Rollback(ctx)
	function, err := r.functionByID(ctx, tx, projectID, functionID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	item, _, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	storage, err := r.functionDeploymentBuildStorageTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if item.BuildStatus != "running" || storage.BuildWorkerID != workerID {
		return domain.FunctionDeployment{}, ErrExecutionNotAvailable
	}
	if storage.BuildSizeBytes > function.ArtifactUsedBytes {
		return domain.FunctionDeployment{}, ErrInvalidFunctionSettings
	}
	newUsedBytes := function.ArtifactUsedBytes - storage.BuildSizeBytes + buildSizeBytes
	if newUsedBytes > function.ArtifactQuotaBytes {
		return domain.FunctionDeployment{}, ErrFunctionQuotaExceeded
	}
	if _, err := tx.Exec(ctx, `UPDATE project_functions SET artifact_used_bytes=$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, functionID, newUsedBytes); err != nil {
		return domain.FunctionDeployment{}, err
	}
	updated, err := scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET build_path=$4,build_size_bytes=$5,build_checksum_sha256=$6,build_status='succeeded',build_worker_id=NULL,error_message=NULL,built_at=COALESCE(built_at,now()),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID, buildPath, buildSizeBytes, strings.ToLower(buildChecksumSHA256)))
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	// A deployment created with Activate=true carries status='active' as its
	// deferred activation request but does not become the function's active
	// pointer until its immutable artifact exists. The function row is already
	// locked, so the previous active deployment keeps serving and is only
	// superseded now that the build has succeeded.
	if item.Status == "active" && (function.ActiveDeploymentID == nil || *function.ActiveDeploymentID != deploymentID.String()) {
		if function.ActiveDeploymentID != nil {
			if _, err := tx.Exec(ctx, `UPDATE function_deployments SET status='superseded',finished_at=now(),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 AND status='active'`, projectID, functionID, *function.ActiveDeploymentID); err != nil {
				return domain.FunctionDeployment{}, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE project_functions SET active_deployment_id=$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, functionID, deploymentID); err != nil {
			return domain.FunctionDeployment{}, err
		}
		updated, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET activated_at=COALESCE(activated_at,now()),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID))
		if err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	if err := validatePublishCleanup(cleanup, projectID, ArtifactCleanupFunctions, buildPath); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if cleanup != nil {
		if err := finalizeArtifactPublishCleanupTx(ctx, tx, *cleanup); err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "function_deployment.updated", "function_deployment", deploymentID, map[string]any{"function_id": functionID.String(), "status": updated.Status, "build_status": updated.BuildStatus}); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionDeployment{}, err
	}
	return updated, nil
}

// FailFunctionDeploymentBuild records a bounded build failure while releasing
// the builder lease. A failed build is not executable and must be replaced by
// a new deployment; the previously active deployment remains represented by
// the function pointer until an operator activates another ready build.
func (r *Repository) FailFunctionDeploymentBuild(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, workerID, errorMessage string) (domain.FunctionDeployment, error) {
	if !validFunctionWorkerID(workerID) {
		return domain.FunctionDeployment{}, ErrInvalidFunctionSettings
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := r.functionByID(ctx, tx, projectID, functionID, true); err != nil {
		return domain.FunctionDeployment{}, err
	}
	item, _, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	storage, err := r.functionDeploymentBuildStorageTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if item.BuildStatus != "running" || storage.BuildWorkerID != workerID {
		return domain.FunctionDeployment{}, ErrExecutionNotAvailable
	}
	failureMessage := normalizeFunctionBuildError(errorMessage)
	// Invocations accepted while the build was queued must not remain stuck
	// forever when the immutable artifact cannot be produced. They are fenced
	// to this deployment and transition atomically with its failed state.
	failedExecutions, err := tx.Query(ctx, `UPDATE function_executions SET status='failed',error_message=$4,finished_at=now(),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND deployment_id=$3 AND status='accepted' RETURNING started_at,finished_at`, projectID, functionID, deploymentID, failureMessage)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	type failedExecutionWindow struct {
		startedAt  *time.Time
		finishedAt *time.Time
	}
	windows := make([]failedExecutionWindow, 0)
	for failedExecutions.Next() {
		var window failedExecutionWindow
		if err := failedExecutions.Scan(&window.startedAt, &window.finishedAt); err != nil {
			failedExecutions.Close()
			return domain.FunctionDeployment{}, err
		}
		windows = append(windows, window)
	}
	if err := failedExecutions.Err(); err != nil {
		failedExecutions.Close()
		return domain.FunctionDeployment{}, err
	}
	// The returned rows must be drained and the cursor closed before usage is
	// incremented on the same connection, otherwise pgx reports "conn busy".
	failedExecutions.Close()
	for _, window := range windows {
		if window.finishedAt == nil {
			continue
		}
		delta := UsageDelta{FunctionFailureCount: 1}
		if window.startedAt != nil {
			if computeMS := window.finishedAt.Sub(*window.startedAt).Milliseconds(); computeMS > 0 {
				delta.FunctionComputeMS = computeMS
			}
		}
		if err := incrementUsageTx(ctx, tx, projectID, *window.finishedAt, delta); err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	updated, err := scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET status='failed',build_status='failed',build_worker_id=NULL,error_message=$4,updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID, failureMessage))
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "function_deployment.updated", "function_deployment", deploymentID, map[string]any{"function_id": functionID.String(), "status": updated.Status, "build_status": updated.BuildStatus, "has_error": true}); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionDeployment{}, err
	}
	return updated, nil
}

func normalizeFunctionBuildError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "function build failed"
	}
	if len(value) > 4000 {
		return value[:4000]
	}
	return value
}

func validFunctionArtifactPath(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		parsed, err := uuid.Parse(part)
		if err != nil || parsed.Version() != uuid.Version(7) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
