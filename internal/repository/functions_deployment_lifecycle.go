package repository

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
)

func (r *Repository) ActivateFunctionDeployment(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor) (domain.FunctionDeployment, domain.Function, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireFunctionWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	item, _, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, false)
	if err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	item, err = activateFunctionDeploymentTx(ctx, tx, projectID, functionID, deploymentID, actor, item)
	if err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	function, err := r.functionByID(ctx, tx, projectID, functionID, false)
	if err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	if err := r.auditFunction(ctx, tx, projectID, actor, "function_deployment.activate", "function_deployment", deploymentID, map[string]any{"function_id": functionID.String(), "version": item.Version}); err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionDeployment{}, domain.Function{}, err
	}
	return item, function, nil
}

// DeleteFunctionDeployment preserves the original single-path return for
// callers that only know about source artifacts. New callers should use
// DeleteFunctionDeploymentWithArtifacts so source and build bytes are cleaned
// up atomically with the metadata deletion.
func (r *Repository) DeleteFunctionDeployment(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor) (string, error) {
	paths, err := r.DeleteFunctionDeploymentWithArtifacts(ctx, projectID, functionID, deploymentID, actor)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", nil
	}
	return paths[0], nil
}

// DeleteFunctionDeploymentWithArtifacts removes metadata/accounting and
// records durable cleanup jobs for its source/build paths in one transaction.
func (r *Repository) DeleteFunctionDeploymentWithArtifacts(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireFunctionWriteTx(ctx, tx, projectID, actor); err != nil {
		return nil, err
	}
	function, err := r.functionByID(ctx, tx, projectID, functionID, true)
	if err != nil {
		return nil, err
	}
	item, path, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return nil, err
	}
	if (function.ActiveDeploymentID != nil && *function.ActiveDeploymentID == deploymentID.String()) || item.Status == "active" {
		return nil, ErrDeploymentActive
	}
	buildStorage, err := r.functionDeploymentBuildStorageTx(ctx, tx, projectID, functionID, deploymentID, false)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM function_deployments WHERE project_id=$1 AND function_id=$2 AND id=$3`, projectID, functionID, deploymentID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_functions SET artifact_used_bytes=GREATEST(0,artifact_used_bytes-$3),updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, functionID, item.SizeBytes+buildStorage.BuildSizeBytes); err != nil {
		return nil, err
	}
	if err := r.auditFunction(ctx, tx, projectID, actor, "function_deployment.delete", "function_deployment", deploymentID, map[string]any{"function_id": functionID.String(), "version": item.Version, "size_bytes": item.SizeBytes}); err != nil {
		return nil, err
	}
	for _, artifactPath := range []string{path, buildStorage.BuildPath} {
		if strings.TrimSpace(artifactPath) == "" {
			continue
		}
		if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{
			ProjectID: projectID, StoreKind: ArtifactCleanupFunctions,
			Operation: ArtifactCleanupRelative, RelativePath: artifactPath,
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	paths := []string{path}
	if strings.TrimSpace(buildStorage.BuildPath) != "" {
		paths = append(paths, buildStorage.BuildPath)
	}
	return paths, nil
}

// TransitionFunctionDeployment is an internal builder boundary. It performs
// no source extraction or process execution; a trusted builder can call it
// after doing that work outside this API process.
func (r *Repository) TransitionFunctionDeployment(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, next, errorMessage string) (domain.FunctionDeployment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	defer tx.Rollback(ctx)
	item, _, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true)
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if !validFunctionDeploymentTransition(item.Status, next) {
		return domain.FunctionDeployment{}, ErrInvalidFunctionTransition
	}
	var updated domain.FunctionDeployment
	switch next {
	case "building":
		updated, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET status='building',build_status='running',build_started_at=COALESCE(build_started_at,now()),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID))
	case "ready":
		updated, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET status='ready',build_status='succeeded',built_at=COALESCE(built_at,now()),error_message=NULL,updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID))
	case "failed":
		updated, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET status='failed',build_status='failed',error_message=$4,finished_at=now(),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID, nullableError(errorMessage)))
	case "cancelled":
		updated, err = scanFunctionDeploymentPublic(tx.QueryRow(ctx, `UPDATE function_deployments SET status='cancelled',finished_at=now(),updated_at=now() WHERE project_id=$1 AND function_id=$2 AND id=$3 RETURNING `+functionDeploymentProjection, projectID, functionID, deploymentID))
	default:
		return domain.FunctionDeployment{}, ErrInvalidFunctionTransition
	}
	if err != nil {
		return domain.FunctionDeployment{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "function_deployment.updated", "function_deployment", deploymentID, map[string]any{"function_id": functionID.String(), "status": updated.Status, "build_status": updated.BuildStatus}); err != nil {
		return domain.FunctionDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionDeployment{}, err
	}
	return updated, nil
}

func validFunctionDeploymentTransition(current, next string) bool {
	switch current {
	case "queued":
		return next == "building" || next == "failed" || next == "cancelled" || next == "ready"
	case "building":
		return next == "ready" || next == "failed" || next == "cancelled"
	case "ready":
		return next == "active" || next == "cancelled"
	case "active":
		return next == "superseded"
	default:
		return false
	}
}

func nullableError(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

func (r *Repository) AppendFunctionBuildLog(ctx context.Context, projectID, functionID, deploymentID, id uuid.UUID, sequence int64, level, message string) (domain.FunctionBuildLog, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionBuildLog{}, err
	}
	defer tx.Rollback(ctx)
	if sequence <= 0 {
		if _, _, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, true); err != nil {
			return domain.FunctionBuildLog{}, err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM function_build_logs WHERE project_id=$1 AND deployment_id=$2`, projectID, deploymentID).Scan(&sequence); err != nil {
			return domain.FunctionBuildLog{}, err
		}
	} else if _, _, err := r.functionDeploymentByIDTx(ctx, tx, projectID, functionID, deploymentID, false); err != nil {
		return domain.FunctionBuildLog{}, err
	}
	item, err := scanFunctionBuildLog(tx.QueryRow(ctx, `INSERT INTO function_build_logs (id,deployment_id,function_id,project_id,sequence,level,message) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+functionBuildLogProjection, id, deploymentID, functionID, projectID, sequence, level, message))
	if err != nil {
		return domain.FunctionBuildLog{}, mapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionBuildLog{}, err
	}
	return item, nil
}

func (r *Repository) ListFunctionBuildLogs(ctx context.Context, projectID, functionID, deploymentID uuid.UUID, actor FunctionActor, limit int, after int64) ([]domain.FunctionBuildLog, error) {
	if _, err := r.requireFunctionRead(ctx, projectID, actor); err != nil {
		return nil, err
	}
	if _, err := r.functionByID(ctx, r.pool, projectID, functionID, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+functionBuildLogProjection+` FROM function_build_logs WHERE project_id=$1 AND function_id=$2 AND deployment_id=$3 AND sequence>$4 ORDER BY sequence LIMIT $5`, projectID, functionID, deploymentID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FunctionBuildLog, 0, limit)
	for rows.Next() {
		item, scanErr := scanFunctionBuildLog(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
