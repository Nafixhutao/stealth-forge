package repository

// Function deployment persistence owns immutable source/build metadata,
// activation, leases, transitions, and build logs.

import (
	"context"
	"errors"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) functionDeploymentByID(ctx context.Context, query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, projectID, functionID, deploymentID uuid.UUID, lock bool, includePath bool) (domain.FunctionDeployment, string, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	projection := functionDeploymentProjection
	if includePath {
		projection += `,source_path`
	}
	var item domain.FunctionDeployment
	var sourcePath string
	var createdBy *uuid.UUID
	args := []any{projectID, functionID, deploymentID}
	var err error
	if includePath {
		err = query.QueryRow(ctx, `SELECT `+projection+` FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`+suffix, args...).Scan(&item.ID, &item.FunctionID, &item.ProjectID, &item.Version, &item.Source, &item.SourceName, &item.SizeBytes, &item.ChecksumSHA256, &item.Status, &item.BuildStatus, &item.ErrorMessage, &createdBy, &item.QueuedAt, &item.BuildStartedAt, &item.BuiltAt, &item.ActivatedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt, &sourcePath)
	} else {
		err = query.QueryRow(ctx, `SELECT `+projection+` FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`+suffix, args...).Scan(&item.ID, &item.FunctionID, &item.ProjectID, &item.Version, &item.Source, &item.SourceName, &item.SizeBytes, &item.ChecksumSHA256, &item.Status, &item.BuildStatus, &item.ErrorMessage, &createdBy, &item.QueuedAt, &item.BuildStartedAt, &item.BuiltAt, &item.ActivatedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FunctionDeployment{}, "", ErrNotFound
	}
	if err != nil {
		return domain.FunctionDeployment{}, "", err
	}
	if createdBy != nil {
		value := createdBy.String()
		item.CreatedByAccountID = &value
	}
	return item, sourcePath, nil
}

func (r *Repository) ListFunctionDeployments(ctx context.Context, projectID, functionID uuid.UUID, actor FunctionActor, limit int, cursor *uuid.UUID) ([]domain.FunctionDeployment, string, bool, error) {
	canManage, err := r.requireFunctionRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if _, err := r.functionByID(ctx, r.pool, projectID, functionID, false); err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+functionDeploymentProjection+` FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND ($3::uuid IS NULL OR id>$3) ORDER BY id LIMIT $4`, projectID, functionID, cursor, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.FunctionDeployment, 0, limit)
	for rows.Next() {
		item, scanErr := scanFunctionDeploymentPublic(rows)
		if scanErr != nil {
			return nil, "", false, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, canManage, nil
}

func (r *Repository) GetFunctionDeployment(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor) (domain.FunctionDeployment, error) {
	if _, err := r.requireFunctionRead(ctx, projectID, actor); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if _, err := r.functionByID(ctx, r.pool, projectID, functionID, false); err != nil {
		return domain.FunctionDeployment{}, err
	}
	item, _, err := r.functionDeploymentByID(ctx, r.pool, projectID, functionID, deploymentID, false, false)
	return item, err
}

// CreateFunctionDeployment reserves per-function quota and assigns a
// monotonically increasing version while holding the function row lock. The
// upload is already atomically published by the caller after recording a
// durable cleanup reservation. `Activate` is handled in the same transaction
// so the new active pointer cannot be observed before its metadata and quota
// reservation commit.
func (r *Repository) CreateFunctionDeployment(ctx context.Context, id, projectID, functionID uuid.UUID, actor FunctionActor, input FunctionDeploymentInput) (domain.FunctionDeployment, error) {
	if input.SizeBytes < 0 {
		return domain.FunctionDeployment{}, ErrFunctionArtifactTooLarge
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireFunctionWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.FunctionDeployment{}, err
	}
	function, err := r.functionByID(ctx, tx, projectID, functionID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if input.SizeBytes > function.ArtifactQuotaBytes-function.ArtifactUsedBytes {
		return domain.FunctionDeployment{}, ErrFunctionQuotaExceeded
	}
	var version int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM function_deployments WHERE project_id=$1 AND function_id=$2`, projectID, functionID).Scan(&version); err != nil {
		return domain.FunctionDeployment{}, err
	}
	item, err := scanFunctionDeploymentPublic(tx.QueryRow(ctx, `INSERT INTO function_deployments (id,function_id,project_id,version,source,source_name,size_bytes,checksum_sha256,source_path,status,build_status,created_by_account_id,runtime_snapshot,entrypoint_snapshot,commands_snapshot,timeout_seconds_snapshot,logging_snapshot) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'ready','queued',$10,$11,$12,$13,$14,$15) RETURNING `+functionDeploymentProjection, id, functionID, projectID, version, input.Source, input.SourceName, input.SizeBytes, input.ChecksumSHA256, input.SourcePath, input.CreatedByAccountID, function.Runtime, function.Entrypoint, function.Commands, function.TimeoutSeconds, function.Logging))
	if err != nil {
		return domain.FunctionDeployment{}, mapError(err)
	}
	variableRows, err := tx.Query(ctx, `SELECT key,kind,is_secret,value_ciphertext FROM function_variables WHERE project_id=$1 AND function_id=$2 ORDER BY key`, projectID, functionID)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	snapshots := make([]functionDeploymentVariableSnapshot, 0)
	for variableRows.Next() {
		var snapshot functionDeploymentVariableSnapshot
		if err := variableRows.Scan(&snapshot.Key, &snapshot.Kind, &snapshot.IsSecret, &snapshot.Ciphertext); err != nil {
			variableRows.Close()
			return domain.FunctionDeployment{}, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := variableRows.Err(); err != nil {
		variableRows.Close()
		return domain.FunctionDeployment{}, err
	}
	variableRows.Close()
	for _, snapshot := range snapshots {
		snapshotID := uuid.Must(uuid.NewV7())
		if _, err := tx.Exec(ctx, `INSERT INTO function_deployment_variables (id,deployment_id,function_id,project_id,key,kind,is_secret,value_ciphertext) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, snapshotID, id, functionID, projectID, snapshot.Key, snapshot.Kind, snapshot.IsSecret, snapshot.Ciphertext); err != nil {
			return domain.FunctionDeployment{}, mapError(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE project_functions SET artifact_used_bytes=artifact_used_bytes+$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, functionID, input.SizeBytes); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if input.Activate {
		if function.Status != "active" {
			return domain.FunctionDeployment{}, ErrFunctionDisabled
		}
		item, err = activateFunctionDeploymentTx(ctx, tx, projectID, functionID, id, actor, item)
		if err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	if err := validatePublishCleanup(input.PublishCleanup, projectID, ArtifactCleanupFunctions, input.SourcePath); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if input.PublishCleanup != nil {
		if err := finalizeArtifactPublishCleanupTx(ctx, tx, *input.PublishCleanup); err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	metadata := map[string]any{"function_id": functionID.String(), "version": item.Version, "source": input.Source, "size_bytes": input.SizeBytes, "checksum_sha256": input.ChecksumSHA256, "activated": input.Activate}
	if err := r.auditFunction(ctx, tx, projectID, actor, "function_deployment.create", "function_deployment", id, metadata); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if input.Activate {
		if err := r.auditFunction(ctx, tx, projectID, actor, "function_deployment.activate", "function_deployment", id, map[string]any{"function_id": functionID.String(), "version": item.Version}); err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionDeployment{}, err
	}
	return item, nil
}

func activateFunctionDeploymentTx(ctx context.Context, tx pgx.Tx, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor, item domain.FunctionDeployment) (domain.FunctionDeployment, error) {
	var current *uuid.UUID
	var functionStatus string
	if err := tx.QueryRow(ctx, `SELECT active_deployment_id,status FROM project_functions WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, functionID).Scan(&current, &functionStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.FunctionDeployment{}, ErrNotFound
		}
		return domain.FunctionDeployment{}, err
	}
	if functionStatus != "active" {
		return domain.FunctionDeployment{}, ErrFunctionDisabled
	}
	locked, _, err := (&Repository{pool: nil}).functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if locked.Status == "active" && current != nil && *current == deploymentID {
		return locked, nil
	}
	if locked.Status != "ready" {
		return domain.FunctionDeployment{}, ErrInvalidFunctionTransition
	}
	if current != nil && *current != deploymentID {
		if _, err := tx.Exec(ctx, `UPDATE function_deployments SET status='superseded',finished_at=now(),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 AND status='active'`, projectID, functionID, *current); err != nil {
			return domain.FunctionDeployment{}, err
		}
	}
	updated, _, err := (&Repository{pool: nil}).functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	updated, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET status='active',activated_at=COALESCE(activated_at,now()),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID))
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_functions SET active_deployment_id=$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, functionID, deploymentID); err != nil {
		return domain.FunctionDeployment{}, err
	}
	_ = actor
	return updated, nil
}

// functionDeploymentByIDTx is the transaction variant used by activation and
// worker state transitions. Keeping the project and function predicates in
// every query makes cross-tenant IDs fail closed.
func (r *Repository) functionDeploymentByIDTx(ctx context.Context, tx pgx.Tx, projectID, functionID, deploymentID uuid.UUID, lock bool) (domain.FunctionDeployment, string, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var item domain.FunctionDeployment
	var createdBy *uuid.UUID
	var path string
	err := tx.QueryRow(ctx, `SELECT `+functionDeploymentProjection+`,source_path FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`+suffix, projectID, functionID, deploymentID).Scan(&item.ID, &item.FunctionID, &item.ProjectID, &item.Version, &item.Source, &item.SourceName, &item.SizeBytes, &item.ChecksumSHA256, &item.Status, &item.BuildStatus, &item.ErrorMessage, &createdBy, &item.QueuedAt, &item.BuildStartedAt, &item.BuiltAt, &item.ActivatedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt, &path)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FunctionDeployment{}, "", ErrNotFound
	}
	if err != nil {
		return domain.FunctionDeployment{}, "", err
	}
	if createdBy != nil {
		value := createdBy.String()
		item.CreatedByAccountID = &value
	}
	return item, path, nil
}

type functionDeploymentBuildStorage struct {
	BuildPath           string
	BuildSizeBytes      int64
	BuildChecksumSHA256 string
	BuildWorkerID       string
}

func (r *Repository) functionDeploymentBuildStorageTx(ctx context.Context, tx pgx.Tx, projectID, functionID, deploymentID uuid.UUID, lock bool) (functionDeploymentBuildStorage, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var path, checksum, workerID *string
	var size int64
	err := tx.QueryRow(ctx, `SELECT build_path,build_size_bytes,build_checksum_sha256,build_worker_id FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`+suffix, projectID, functionID, deploymentID).Scan(&path, &size, &checksum, &workerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return functionDeploymentBuildStorage{}, ErrNotFound
	}
	if err != nil {
		return functionDeploymentBuildStorage{}, err
	}
	storage := functionDeploymentBuildStorage{BuildSizeBytes: size}
	if path != nil {
		storage.BuildPath = *path
	}
	if checksum != nil {
		storage.BuildChecksumSHA256 = *checksum
	}
	if workerID != nil {
		storage.BuildWorkerID = *workerID
	}
	return storage, nil
}

// functionDeploymentRuntimeConfigTx overlays immutable deployment settings
// onto the current function identity. Enabled/status and permissions remain
// live controls, while runtime/entrypoint/commands/timeout/logging cannot
// drift after a deployment has been built.
func (r *Repository) functionDeploymentRuntimeConfigTx(ctx context.Context, tx pgx.Tx, projectID, functionID, deploymentID uuid.UUID, function domain.Function) (domain.Function, error) {
	var runtime, entrypoint, commands string
	var timeout int
	var logging bool
	err := tx.QueryRow(ctx, `SELECT runtime_snapshot,entrypoint_snapshot,commands_snapshot,timeout_seconds_snapshot,logging_snapshot FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`, projectID, functionID, deploymentID).Scan(&runtime, &entrypoint, &commands, &timeout, &logging)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Function{}, ErrNotFound
	}
	if err != nil {
		return domain.Function{}, err
	}
	function.Runtime = runtime
	function.Entrypoint = entrypoint
	function.Commands = commands
	function.TimeoutSeconds = timeout
	function.Logging = logging
	return function, nil
}

// FunctionDeploymentStoragePaths returns opaque source/build paths for a
// post-commit filesystem cleanup. It is intentionally separate from the
// public deployment projection.
func (r *Repository) FunctionDeploymentStoragePaths(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor) ([]string, error) {
	if _, err := r.requireFunctionRead(ctx, projectID, actor); err != nil {
		return nil, err
	}
	if _, err := r.functionByID(ctx, r.pool, projectID, functionID, false); err != nil {
		return nil, err
	}
	var sourcePath string
	var buildPath *string
	err := r.pool.QueryRow(ctx, `SELECT source_path,build_path FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`, projectID, functionID, deploymentID).Scan(&sourcePath, &buildPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	paths := []string{sourcePath}
	if buildPath != nil && strings.TrimSpace(*buildPath) != "" {
		paths = append(paths, *buildPath)
	}
	return paths, nil
}

// ClaimNextFunctionDeployment leases one source deployment for building. A
// deployment may already be active because activation is allowed to be
// requested before the asynchronous build completes; invocations remain
// blocked until build_status becomes succeeded.
