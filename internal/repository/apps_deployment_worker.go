package repository

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/buildkitmetadata"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextAppDeployment(ctx context.Context, workerID string) (AppBuildJob, error) {
	if !validFunctionWorkerID(workerID) {
		return AppBuildJob{}, ErrInvalidAppDeployment
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AppBuildJob{}, err
	}
	defer tx.Rollback(ctx)
	var projectID, appID, deploymentID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT d.project_id,d.app_id,d.id
		FROM app_deployments d
		WHERE d.status='queued' AND d.build_status IN ('queued','deferred')
		ORDER BY d.queued_at,d.id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&projectID, &appID, &deploymentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AppBuildJob{}, ErrNoAppDeploymentJob
	}
	if err != nil {
		return AppBuildJob{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE app_deployments SET status='building',build_status='running',build_worker_id=$4,build_started_at=now(),error_message=NULL,updated_at=now() WHERE project_id=$1 AND app_id=$2 AND id=$3 AND status='queued' AND build_status IN ('queued','deferred')`, projectID, appID, deploymentID, workerID); err != nil {
		return AppBuildJob{}, err
	}
	app, err := appByID(ctx, tx, projectID, appID, false)
	if err != nil {
		return AppBuildJob{}, err
	}
	deployment, sourcePath, _, _, _, _, _, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, false, true)
	if err != nil {
		return AppBuildJob{}, err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return AppBuildJob{}, ErrInvalidAppDeployment
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "app_deployment.updated", "app_deployment", deploymentID, map[string]any{"app_id": appID.String(), "version": deployment.Version, "status": deployment.Status, "build_status": deployment.BuildStatus}); err != nil {
		return AppBuildJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AppBuildJob{}, err
	}
	return AppBuildJob{App: app, Deployment: deployment, SourcePath: sourcePath, WorkerID: workerID}, nil
}

func (r *Repository) RequeueStaleAppDeployments(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, ErrInvalidAppDeployment
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT project_id,app_id,id
		FROM app_deployments
		WHERE status='building' AND build_status='running'
		  AND build_started_at < now()-($1::double precision*interval '1 second')
		ORDER BY build_started_at,id`, maxAge.Seconds())
	if err != nil {
		return 0, err
	}
	type stale struct{ projectID, appID, deploymentID uuid.UUID }
	items := make([]stale, 0)
	for rows.Next() {
		var item stale
		if err := rows.Scan(&item.projectID, &item.appID, &item.deploymentID); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	changed := int64(0)
	for _, item := range items {
		if _, err := appByID(ctx, tx, item.projectID, item.appID, true); errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			return 0, err
		}
		_, _, _, _, _, _, reserved, err := appDeploymentByID(ctx, tx, item.projectID, item.appID, item.deploymentID, true, true)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		var reservedImagePath *string
		if err := tx.QueryRow(ctx, `SELECT reserved_image_path FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, item.projectID, item.appID, item.deploymentID).Scan(&reservedImagePath); err != nil {
			return 0, err
		}
		var version int64
		update, err := tx.Exec(ctx, `
			UPDATE app_deployments
			SET status='queued',build_status='deferred',build_worker_id=NULL,build_started_at=NULL,reserved_image_bytes=0,reserved_image_path=NULL,updated_at=now()
			WHERE project_id=$1 AND app_id=$2 AND id=$3 AND status='building' AND build_status='running'
			  AND build_started_at < now()-($4::double precision*interval '1 second')`, item.projectID, item.appID, item.deploymentID, maxAge.Seconds())
		if err != nil {
			return 0, err
		}
		if update.RowsAffected() == 0 {
			continue
		}
		if reserved > 0 {
			quotaUpdate, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_reserved_bytes=artifact_reserved_bytes-$3,updated_at=now() WHERE project_id=$1 AND id=$2 AND artifact_reserved_bytes >= $3`, item.projectID, item.appID, reserved)
			if err != nil {
				return 0, err
			}
			if quotaUpdate.RowsAffected() != 1 {
				return 0, ErrInvalidAppDeployment
			}
			if reservedImagePath == nil {
				return 0, ErrArtifactPublishLost
			}
			if err := promoteAppImagePublishCleanupTx(ctx, tx, item.projectID, item.appID, *reservedImagePath, reserved); err != nil {
				return 0, err
			}
		}
		if err := tx.QueryRow(ctx, `SELECT version FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, item.projectID, item.appID, item.deploymentID).Scan(&version); err != nil {
			return 0, err
		}
		if err := r.enqueueWebhookEventTx(ctx, tx, item.projectID, "app_deployment.updated", "app_deployment", item.deploymentID, map[string]any{"app_id": item.appID.String(), "version": version, "build_status": "deferred"}); err != nil {
			return 0, err
		}
		changed++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return changed, nil
}

// DeferAppDeploymentBuild releases the current lease without making a
// terminal claim when the dedicated BuildKit daemon became unavailable after
// the readiness check. The same immutable build input remains queued.
func (r *Repository) DeferAppDeploymentBuild(ctx context.Context, projectID, appID, deploymentID uuid.UUID, workerID, diagnostic string) error {
	if !validFunctionWorkerID(workerID) {
		return ErrInvalidAppDeployment
	}
	diagnostic = normalizeAppBuildLogMessage(diagnostic)
	if diagnostic == "" || len(diagnostic) > 512 {
		return ErrInvalidAppDeployment
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := appByID(ctx, tx, projectID, appID, true); err != nil {
		return err
	}
	_, _, _, claimedBy, _, _, reserved, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return err
	}
	if claimedBy == nil || *claimedBy != workerID {
		return ErrAppBuildNotOwned
	}
	var reservedImagePath *string
	if err := tx.QueryRow(ctx, `SELECT reserved_image_path FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, projectID, appID, deploymentID).Scan(&reservedImagePath); err != nil {
		return err
	}
	update, err := tx.Exec(ctx, `
		UPDATE app_deployments
		SET status='queued',build_status='deferred',build_worker_id=NULL,build_started_at=NULL,
		    reserved_image_bytes=0,reserved_image_path=NULL,error_message=$4,updated_at=now()
		WHERE project_id=$1 AND app_id=$2 AND id=$3 AND status='building' AND build_status='running' AND build_worker_id=$5`,
		projectID, appID, deploymentID, diagnostic, workerID)
	if err != nil {
		return err
	}
	if update.RowsAffected() != 1 {
		return ErrAppBuildNotOwned
	}
	if reserved > 0 {
		quotaUpdate, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_reserved_bytes=artifact_reserved_bytes-$3,updated_at=now() WHERE project_id=$1 AND id=$2 AND artifact_reserved_bytes >= $3`, projectID, appID, reserved)
		if err != nil {
			return err
		}
		if quotaUpdate.RowsAffected() != 1 {
			return ErrInvalidAppDeployment
		}
		if reservedImagePath == nil {
			return ErrArtifactPublishLost
		}
		if err := promoteAppImagePublishCleanupTx(ctx, tx, projectID, appID, *reservedImagePath, reserved); err != nil {
			return err
		}
	}
	if _, err := appendAppBuildLogTx(ctx, tx, projectID, appID, deploymentID, "warn", diagnostic); err != nil {
		return err
	}
	item, err := publicAppDeploymentByID(ctx, tx, projectID, appID, deploymentID)
	if err != nil {
		return err
	}
	metadata := appDeploymentAuditMetadata(item)
	metadata["build_status"] = "deferred"
	if err := r.auditAppDeploymentTx(ctx, tx, projectID, nil, "app_deployment.updated", deploymentID, metadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ReserveAppImagePublish(ctx context.Context, projectID, appID, deploymentID uuid.UUID, workerID string, imageSize int64, cleanup ArtifactCleanupInput) error {
	if !validFunctionWorkerID(workerID) || imageSize <= 0 || cleanup.ProjectID != projectID || cleanup.StoreKind != ArtifactCleanupAppImages || cleanup.Operation != ArtifactCleanupRelative || !validAppArtifactPath(cleanup.RelativePath) {
		return ErrInvalidAppDeployment
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := appByID(ctx, tx, projectID, appID, true); err != nil {
		return err
	}
	_, _, _, claimedBy, _, _, reserved, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return err
	}
	var buildStatus, status string
	if err := tx.QueryRow(ctx, `SELECT status,build_status FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, projectID, appID, deploymentID).Scan(&status, &buildStatus); err != nil {
		return err
	}
	if claimedBy == nil || *claimedBy != workerID || status != "building" || buildStatus != "running" || reserved != 0 {
		return ErrAppBuildNotOwned
	}
	var quota, used, currentReserved int64
	if err := tx.QueryRow(ctx, `SELECT artifact_quota_bytes,artifact_used_bytes,artifact_reserved_bytes FROM project_apps WHERE project_id=$1 AND id=$2`, projectID, appID).Scan(&quota, &used, &currentReserved); err != nil {
		return err
	}
	if imageSize > quota-used-currentReserved {
		return ErrAppArtifactQuotaExceeded
	}
	if _, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_reserved_bytes=artifact_reserved_bytes+$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, appID, imageSize); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE app_deployments SET reserved_image_bytes=$4,reserved_image_path=$5,updated_at=now() WHERE project_id=$1 AND app_id=$2 AND id=$3`, projectID, appID, deploymentID, imageSize, cleanup.RelativePath); err != nil {
		return err
	}
	if err := reserveArtifactPublishCleanupTx(ctx, tx, cleanup); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE artifact_cleanup_jobs
		SET quota_app_id=$5,quota_reserved_bytes=$6,updated_at=now()
		WHERE project_id=$1 AND store_kind=$2 AND operation=$3 AND relative_path=$4
		  AND status='reserved' AND quota_app_id IS NULL AND quota_reserved_bytes=0`,
		projectID, cleanup.StoreKind, cleanup.Operation, cleanup.RelativePath, appID, imageSize)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrArtifactPublishConflict
	}
	return tx.Commit(ctx)
}

func (r *Repository) CompleteAppDeploymentBuildWithCleanup(ctx context.Context, projectID, appID, deploymentID uuid.UUID, workerID, imageDigest, imagePath, archiveChecksum string, imageSize int64, cleanup ArtifactCleanupInput) (domain.AppDeployment, error) {
	if !validFunctionWorkerID(workerID) || !buildkitmetadata.ValidDigest(imageDigest) || !validAppSHA256(archiveChecksum) || !validAppArtifactPath(imagePath) || imageSize <= 0 || cleanup.ProjectID != projectID || cleanup.StoreKind != ArtifactCleanupAppImages || cleanup.Operation != ArtifactCleanupRelative || cleanup.RelativePath != imagePath {
		return domain.AppDeployment{}, ErrInvalidAppDeployment
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	defer tx.Rollback(ctx)
	app, err := appByID(ctx, tx, projectID, appID, true)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	item, _, _, claimedBy, selectRequested, selectionBaseDeploymentID, reserved, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if claimedBy == nil || *claimedBy != workerID || item.Status != "building" || item.BuildStatus != "running" {
		return domain.AppDeployment{}, ErrAppBuildNotOwned
	}
	if reserved != imageSize {
		return domain.AppDeployment{}, ErrAppArtifactQuotaExceeded
	}
	if err := validatePublishCleanup(&cleanup, projectID, ArtifactCleanupAppImages, imagePath); err != nil {
		return domain.AppDeployment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_reserved_bytes=artifact_reserved_bytes-$3,artifact_used_bytes=artifact_used_bytes+$3,updated_at=now() WHERE project_id=$1 AND id=$2 AND artifact_reserved_bytes >= $3`, projectID, appID, imageSize); err != nil {
		return domain.AppDeployment{}, err
	}
	update, err := tx.Exec(ctx, `
		UPDATE app_deployments
		SET status='ready',build_status='succeeded',build_worker_id=NULL,reserved_image_bytes=0,reserved_image_path=NULL,
		    image_digest=$4,image_archive_sha256=$5,image_size_bytes=$6,image_path=$7,
		    error_message=NULL,built_at=now(),finished_at=now(),updated_at=now()
		WHERE project_id=$1 AND app_id=$2 AND id=$3 AND status='building' AND build_status='running' AND build_worker_id=$8 AND reserved_image_path=$9`,
		projectID, appID, deploymentID, imageDigest, archiveChecksum, imageSize, imagePath, workerID, imagePath)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if update.RowsAffected() != 1 {
		return domain.AppDeployment{}, ErrAppBuildNotOwned
	}
	if err := finalizeArtifactPublishCleanupTx(ctx, tx, cleanup); err != nil {
		return domain.AppDeployment{}, err
	}
	autoSelected := false
	if selectRequested && sameOptionalID(app.DesiredDeploymentID, selectionBaseDeploymentID) && app.DesiredGeneration < math.MaxInt64 {
		if _, err := tx.Exec(ctx, `UPDATE project_apps SET desired_deployment_id=$3,desired_generation=desired_generation+1,runtime_status='pending',runtime_error=NULL,updated_at=now() WHERE project_id=$1 AND id=$2 AND desired_generation=$4`, projectID, appID, deploymentID, app.DesiredGeneration); err != nil {
			return domain.AppDeployment{}, err
		}
		if err := resetAppRuntimeRetryTx(ctx, tx, appID); err != nil {
			return domain.AppDeployment{}, err
		}
		autoSelected = true
	}
	item, _, _, _, _, _, _, err = appDeploymentByID(ctx, tx, projectID, appID, deploymentID, false, false)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	metadata := appDeploymentAuditMetadata(item)
	metadata["auto_selected"] = autoSelected
	if autoSelected {
		metadata["desired_generation"] = app.DesiredGeneration + 1
	}
	if err := r.auditAppDeploymentTx(ctx, tx, projectID, nil, "app_deployment.updated", deploymentID, metadata); err != nil {
		return domain.AppDeployment{}, err
	}
	if _, err := appendAppBuildLogTx(ctx, tx, projectID, appID, deploymentID, "info", "Build completed; verified OCI artifact persisted"); err != nil {
		return domain.AppDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppDeployment{}, err
	}
	return item, nil
}

func sameOptionalID(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (r *Repository) FailAppDeploymentBuild(ctx context.Context, projectID, appID, deploymentID uuid.UUID, workerID, message string) (domain.AppDeployment, error) {
	if !validFunctionWorkerID(workerID) {
		return domain.AppDeployment{}, ErrInvalidAppDeployment
	}
	message = normalizeAppBuildLogMessage(message)
	if message == "" {
		message = "App build failed"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := appByID(ctx, tx, projectID, appID, true); err != nil {
		return domain.AppDeployment{}, err
	}
	item, _, _, claimedBy, _, _, reserved, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if claimedBy == nil || *claimedBy != workerID || item.Status != "building" || item.BuildStatus != "running" {
		return domain.AppDeployment{}, ErrAppBuildNotOwned
	}
	if reserved > 0 {
		quotaUpdate, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_reserved_bytes=artifact_reserved_bytes-$3,updated_at=now() WHERE project_id=$1 AND id=$2 AND artifact_reserved_bytes >= $3`, projectID, appID, reserved)
		if err != nil {
			return domain.AppDeployment{}, err
		}
		if quotaUpdate.RowsAffected() != 1 {
			return domain.AppDeployment{}, ErrInvalidAppDeployment
		}
		var reservedImagePath *string
		if err := tx.QueryRow(ctx, `SELECT reserved_image_path FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, projectID, appID, deploymentID).Scan(&reservedImagePath); err != nil {
			return domain.AppDeployment{}, err
		}
		if reservedImagePath == nil {
			return domain.AppDeployment{}, ErrArtifactPublishLost
		}
		if err := promoteAppImagePublishCleanupTx(ctx, tx, projectID, appID, *reservedImagePath, reserved); err != nil {
			return domain.AppDeployment{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE app_deployments SET status='failed',build_status='failed',build_worker_id=NULL,reserved_image_bytes=0,reserved_image_path=NULL,error_message=$4,finished_at=now(),updated_at=now() WHERE project_id=$1 AND app_id=$2 AND id=$3 AND build_worker_id=$5 AND status='building' AND build_status='running'`, projectID, appID, deploymentID, message, workerID); err != nil {
		return domain.AppDeployment{}, err
	}
	item, _, _, _, _, _, _, err = appDeploymentByID(ctx, tx, projectID, appID, deploymentID, false, false)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	metadata := appDeploymentAuditMetadata(item)
	metadata["error"] = message
	if err := r.auditAppDeploymentTx(ctx, tx, projectID, nil, "app_deployment.updated", deploymentID, metadata); err != nil {
		return domain.AppDeployment{}, err
	}
	if _, err := appendAppBuildLogTx(ctx, tx, projectID, appID, deploymentID, "error", message); err != nil {
		return domain.AppDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppDeployment{}, err
	}
	return item, nil
}

func (r *Repository) AppendAppBuildLog(ctx context.Context, projectID, appID, deploymentID uuid.UUID, workerID string, id uuid.UUID, level, message string) (domain.AppBuildLog, error) {
	if !validFunctionWorkerID(workerID) || id == uuid.Nil || id.Version() != uuid.Version(7) {
		return domain.AppBuildLog{}, ErrInvalidAppDeployment
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if level != "info" && level != "warn" && level != "error" {
		return domain.AppBuildLog{}, ErrInvalidAppDeployment
	}
	message = normalizeAppBuildLogMessage(message)
	if message == "" {
		return domain.AppBuildLog{}, ErrInvalidAppDeployment
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AppBuildLog{}, err
	}
	defer tx.Rollback(ctx)
	_, _, _, claimedBy, _, _, _, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return domain.AppBuildLog{}, err
	}
	var status, buildStatus string
	if err := tx.QueryRow(ctx, `SELECT status,build_status FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, projectID, appID, deploymentID).Scan(&status, &buildStatus); err != nil {
		return domain.AppBuildLog{}, err
	}
	if claimedBy == nil || *claimedBy != workerID || status != "building" || buildStatus != "running" {
		return domain.AppBuildLog{}, ErrAppBuildNotOwned
	}
	item, err := appendAppBuildLogTxWithID(ctx, tx, projectID, appID, deploymentID, id, level, message)
	if err != nil {
		return domain.AppBuildLog{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppBuildLog{}, err
	}
	return item, nil
}

func appendAppBuildLogTx(ctx context.Context, tx pgx.Tx, projectID, appID, deploymentID uuid.UUID, level, message string) (domain.AppBuildLog, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return domain.AppBuildLog{}, err
	}
	return appendAppBuildLogTxWithID(ctx, tx, projectID, appID, deploymentID, id, level, message)
}

func appendAppBuildLogTxWithID(ctx context.Context, tx pgx.Tx, projectID, appID, deploymentID, id uuid.UUID, level, message string) (domain.AppBuildLog, error) {
	level = strings.ToLower(strings.TrimSpace(level))
	if level != "info" && level != "warn" && level != "error" {
		return domain.AppBuildLog{}, ErrInvalidAppDeployment
	}
	message = normalizeAppBuildLogMessage(message)
	if message == "" {
		return domain.AppBuildLog{}, ErrInvalidAppDeployment
	}
	var sequence int64
	if err := tx.QueryRow(ctx, `UPDATE app_deployments SET next_log_sequence=next_log_sequence+1 WHERE project_id=$1 AND app_id=$2 AND id=$3 RETURNING next_log_sequence-1`, projectID, appID, deploymentID).Scan(&sequence); err != nil {
		return domain.AppBuildLog{}, err
	}
	var item domain.AppBuildLog
	err := tx.QueryRow(ctx, `INSERT INTO app_build_logs (id,deployment_id,app_id,project_id,sequence,level,message) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+appBuildLogProjection, id, deploymentID, appID, projectID, sequence, level, message).Scan(&item.ID, &item.DeploymentID, &item.AppID, &item.ProjectID, &item.Sequence, &item.Level, &item.Message, &item.CreatedAt)
	if err != nil {
		return domain.AppBuildLog{}, err
	}
	if sequence > AppBuildLogRetentionRecords {
		if _, err := tx.Exec(ctx, `DELETE FROM app_build_logs WHERE deployment_id=$1 AND sequence <= $2`, deploymentID, sequence-AppBuildLogRetentionRecords); err != nil {
			return domain.AppBuildLog{}, err
		}
	}
	return item, nil
}
