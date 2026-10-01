package repository

// App deployment persistence owns immutable build inputs, PostgreSQL queue
// leases, verified artifact publication, desired image selection, and bounded
// build logs. It does not own or expose a persistent container lifecycle.

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Stealth-deplover/stealth/internal/appbuildspec"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/Stealth-deplover/stealth/internal/workloadspec"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	DefaultAppArtifactQuotaBytes int64 = 5 << 30
	AppBuildLogMessageMaxBytes         = 4096
	AppBuildLogRetentionRecords        = 2000
)

var (
	ErrAppArtifactQuotaExceeded     = errors.New("App artifact quota exceeded")
	ErrAppDeploymentSelected        = errors.New("selected App deployment cannot be deleted")
	ErrAppDeploymentRunning         = errors.New("App deployment build is running")
	ErrAppArtifactPublishInProgress = errors.New("App artifact publication is in progress")
	ErrAppDeploymentNotReady        = errors.New("App deployment is not ready for selection")
	ErrAppDeploymentSnapshotInvalid = errors.New("App deployment WorkloadSpec snapshot is invalid")
	ErrAppRollbackNotAvailable      = errors.New("App deployment is not available for rollback")
	ErrAppDeploymentAlreadySelected = errors.New("App deployment is already selected")
	ErrAppRollbackGenerationLimit   = errors.New("App desired generation cannot be incremented")
	ErrNoAppDeploymentJob           = errors.New("no App deployment build job available")
	ErrAppBuildNotOwned             = errors.New("App deployment build lease is no longer owned by this worker")
	ErrAppBuildTransition           = errors.New("invalid App deployment build transition")
	ErrInvalidAppDeployment         = errors.New("invalid App deployment settings")
)

type AppDeploymentInput struct {
	SourceName       string
	SourceSizeBytes  int64
	SourceChecksum   string
	SourcePath       string
	SourceReserved   bool
	BuildSpec        appbuildspec.Spec
	Select           bool
	CreatedByAccount *uuid.UUID
	PublishCleanup   *ArtifactCleanupInput
}

// AppBuildJob is private to trusted workers. SourcePath never crosses an HTTP
// or client projection.
type AppBuildJob struct {
	App        domain.App
	Deployment domain.AppDeployment
	SourcePath string
	WorkerID   string
}

type appDeploymentScanner interface{ Scan(...any) error }

const appDeploymentProjection = `d.id::text,d.app_id::text,d.project_id::text,d.version,d.source,d.source_name,d.source_size_bytes,d.source_checksum_sha256,d.dockerfile_path,d.context_directory,d.target,d.platform,d.workload_spec,d.workload_spec_sha256,d.status,d.build_status,d.error_message,d.image_digest,d.image_archive_sha256,d.image_size_bytes,COALESCE(a.desired_deployment_id=d.id,false),d.created_by_account_id,d.queued_at,d.build_started_at,d.built_at,d.finished_at,d.created_at,d.updated_at`
const appBuildLogProjection = `id,deployment_id,app_id,project_id,sequence,level,message,created_at`

func scanAppDeployment(row appDeploymentScanner) (domain.AppDeployment, error) {
	item, _, _, _, _, _, _, err := scanAppDeploymentFields(row, false)
	return item, err
}

func scanAppDeploymentFields(row appDeploymentScanner, includePrivate bool) (domain.AppDeployment, string, *string, *string, bool, *string, int64, error) {
	var item domain.AppDeployment
	var rawSpec []byte
	var sourceName, target, errorMessage, imageDigest, archiveChecksum *string
	var imageSize *int64
	var createdBy *uuid.UUID
	var sourcePath string
	var imagePath, workerID *string
	var selectRequested bool
	var selectionBaseDeploymentID *string
	var reservedBytes int64
	args := []any{
		&item.ID, &item.AppID, &item.ProjectID, &item.Version, &item.Source, &sourceName,
		&item.SourceSizeBytes, &item.SourceChecksumSHA256, &item.DockerfilePath, &item.ContextDirectory,
		&target, &item.Platform, &rawSpec, &item.WorkloadSpecSHA256, &item.Status, &item.BuildStatus,
		&errorMessage, &imageDigest, &archiveChecksum, &imageSize, &item.Selected, &createdBy,
		&item.QueuedAt, &item.BuildStartedAt, &item.BuiltAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt,
	}
	if includePrivate {
		args = append(args, &sourcePath, &imagePath, &workerID, &selectRequested, &selectionBaseDeploymentID, &reservedBytes)
	}
	if err := row.Scan(args...); err != nil {
		return domain.AppDeployment{}, "", nil, nil, false, nil, 0, err
	}
	item.SourceName = sourceName
	item.Target = target
	item.ErrorMessage = errorMessage
	item.ImageDigest = imageDigest
	item.ImageArchiveSHA256 = archiveChecksum
	item.ImageSizeBytes = imageSize
	if createdBy != nil {
		value := createdBy.String()
		item.CreatedByAccountID = &value
	}
	spec, err := workloadspec.Decode(rawSpec)
	if err != nil {
		return domain.AppDeployment{}, "", nil, nil, false, nil, 0, fmt.Errorf("%w: stored AppDeployment WorkloadSpec: %v", ErrAppDeploymentSnapshotInvalid, err)
	}
	digest, err := workloadspec.Digest(spec)
	if err != nil || digest != item.WorkloadSpecSHA256 {
		return domain.AppDeployment{}, "", nil, nil, false, nil, 0, ErrAppDeploymentSnapshotInvalid
	}
	item.WorkloadSnapshot = spec
	return item, sourcePath, imagePath, workerID, selectRequested, selectionBaseDeploymentID, reservedBytes, nil
}

func appDeploymentByID(ctx context.Context, query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, projectID, appID, deploymentID uuid.UUID, lock, includePrivate bool) (domain.AppDeployment, string, *string, *string, bool, *string, int64, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF d"
	}
	projection := appDeploymentProjection
	if includePrivate {
		projection += `,d.source_path,d.image_path,d.build_worker_id,d.select_requested,d.selection_base_deployment_id::text,d.reserved_image_bytes`
	}
	row := query.QueryRow(ctx, `
			SELECT `+projection+`
			FROM app_deployments d JOIN project_apps a ON a.id=d.app_id AND a.project_id=d.project_id
			WHERE d.project_id=$1 AND d.app_id=$2 AND d.id=$3`+suffix,
		projectID, appID, deploymentID)
	item, sourcePath, imagePath, workerID, selectRequested, selectionBaseDeploymentID, reservedBytes, err := scanAppDeploymentFields(row, includePrivate)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AppDeployment{}, "", nil, nil, false, nil, 0, ErrNotFound
	}
	return item, sourcePath, imagePath, workerID, selectRequested, selectionBaseDeploymentID, reservedBytes, err
}

func (r *Repository) CreateAppDeployment(ctx context.Context, id, projectID, appID uuid.UUID, actor AppActor, input AppDeploymentInput) (domain.AppDeployment, error) {
	buildSpec, err := appbuildspec.Normalize(input.BuildSpec)
	if err != nil || id == uuid.Nil || id.Version() != uuid.Version(7) || input.SourceSizeBytes <= 0 || !validAppSHA256(input.SourceChecksum) || !validAppArtifactPath(input.SourcePath) || storage.ValidateFilename(input.SourceName) != nil {
		return domain.AppDeployment{}, ErrInvalidAppDeployment
	}
	cleanup := ArtifactCleanupInput{ProjectID: projectID, StoreKind: ArtifactCleanupAppSources, Operation: ArtifactCleanupRelative, RelativePath: input.SourcePath}
	if input.PublishCleanup == nil || *input.PublishCleanup != cleanup {
		return domain.AppDeployment{}, ErrInvalidAppDeployment
	}
	return r.createAppDeployment(ctx, id, projectID, appID, actor, input, buildSpec)
}

func (r *Repository) createAppDeployment(ctx context.Context, id, projectID, appID uuid.UUID, actor AppActor, input AppDeploymentInput, buildSpec appbuildspec.Spec) (domain.AppDeployment, error) {
	cleanup := *input.PublishCleanup
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireAppWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.AppDeployment{}, err
	}
	app, err := appByID(ctx, tx, projectID, appID, true)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if !input.SourceReserved {
		return domain.AppDeployment{}, ErrAppArtifactQuotaExceeded
	}
	var version int64
	if err := tx.QueryRow(ctx, `UPDATE project_apps SET next_deployment_version=next_deployment_version+1 WHERE project_id=$1 AND id=$2 AND next_deployment_version<9223372036854775807 RETURNING next_deployment_version-1`, projectID, appID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AppDeployment{}, ErrInvalidAppDeployment
		}
		return domain.AppDeployment{}, err
	}
	workloadJSON, err := workloadspec.MarshalCanonical(app.Workload)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	var target any
	if buildSpec.Target != nil {
		target = *buildSpec.Target
	}
	var createdBy any
	if input.CreatedByAccount != nil {
		createdBy = *input.CreatedByAccount
	}
	var selectionBaseDeploymentID any
	if input.Select {
		if app.DesiredDeploymentID != nil {
			selectionBaseDeploymentID, err = uuid.Parse(*app.DesiredDeploymentID)
			if err != nil {
				return domain.AppDeployment{}, ErrInvalidAppDeployment
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO app_deployments (
			id,app_id,project_id,version,source,source_name,source_size_bytes,source_checksum_sha256,source_path,
			dockerfile_path,context_directory,target,platform,workload_spec,workload_spec_sha256,status,build_status,
			select_requested,selection_base_deployment_id,created_by_account_id
		) VALUES ($1,$2,$3,$4,'upload',$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,'queued','queued',$15,$16,$17)`,
		id, appID, projectID, version, input.SourceName, input.SourceSizeBytes, input.SourceChecksum,
		input.SourcePath, buildSpec.DockerfilePath, buildSpec.ContextDirectory, target, buildSpec.Platform,
		workloadJSON, app.WorkloadSpecSHA256, input.Select, selectionBaseDeploymentID, createdBy,
	); err != nil {
		return domain.AppDeployment{}, mapError(err)
	}
	if err := consumeAppSourceUploadReservationTx(ctx, tx, cleanup, appID, input.SourceSizeBytes); err != nil {
		return domain.AppDeployment{}, err
	}
	quotaUpdate, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_reserved_bytes=artifact_reserved_bytes-$3,artifact_used_bytes=artifact_used_bytes+$3,updated_at=now() WHERE project_id=$1 AND id=$2 AND artifact_reserved_bytes >= $3`, projectID, appID, input.SourceSizeBytes)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if quotaUpdate.RowsAffected() != 1 {
		return domain.AppDeployment{}, ErrAppArtifactQuotaExceeded
	}
	item, _, _, _, _, _, _, err := appDeploymentByID(ctx, tx, projectID, appID, id, false, false)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	metadata := appDeploymentAuditMetadata(item)
	metadata["select_requested"] = input.Select
	if err := r.auditAppDeploymentTx(ctx, tx, projectID, &actor, "app_deployment.create", id, metadata); err != nil {
		return domain.AppDeployment{}, err
	}
	if _, err := appendAppBuildLogTx(ctx, tx, projectID, appID, id, "info", "Build queued"); err != nil {
		return domain.AppDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppDeployment{}, err
	}
	return item, nil
}

func consumeAppSourceUploadReservationTx(ctx context.Context, tx pgx.Tx, cleanup ArtifactCleanupInput, appID uuid.UUID, size int64) error {
	var reservedAppID *uuid.UUID
	var reservedBytes int64
	err := tx.QueryRow(ctx, `SELECT quota_app_id,quota_reserved_bytes FROM artifact_cleanup_jobs WHERE project_id=$1 AND store_kind=$2 AND operation=$3 AND relative_path=$4 AND status='reserved' AND leased_at IS NULL FOR UPDATE`, cleanup.ProjectID, cleanup.StoreKind, cleanup.Operation, cleanup.RelativePath).Scan(&reservedAppID, &reservedBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrArtifactPublishLost
	}
	if err != nil {
		return err
	}
	if reservedAppID == nil || *reservedAppID != appID || reservedBytes != size {
		return ErrInvalidAppDeployment
	}
	result, err := tx.Exec(ctx, `DELETE FROM artifact_cleanup_jobs WHERE project_id=$1 AND store_kind=$2 AND operation=$3 AND relative_path=$4 AND status='reserved' AND leased_at IS NULL`, cleanup.ProjectID, cleanup.StoreKind, cleanup.Operation, cleanup.RelativePath)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrArtifactPublishLost
	}
	return nil
}

// promoteAppImagePublishCleanupTx releases an image publish reservation when
// its owning build fails or becomes stale. The immutable path is tied to the
// deployment row so one build cannot consume another build's quota reservation.
func promoteAppImagePublishCleanupTx(ctx context.Context, tx pgx.Tx, projectID, appID uuid.UUID, imagePath string, reserved int64) error {
	if !validAppArtifactPath(imagePath) || reserved <= 0 {
		return ErrArtifactPublishLost
	}
	result, err := tx.Exec(ctx, `
		UPDATE artifact_cleanup_jobs
		SET status='pending',available_at=now(),quota_app_id=NULL,quota_reserved_bytes=0,updated_at=now()
		WHERE project_id=$1 AND store_kind=$2 AND operation='relative' AND relative_path=$3
		  AND status='reserved' AND leased_at IS NULL AND quota_app_id=$4 AND quota_reserved_bytes=$5`,
		projectID, ArtifactCleanupAppImages, imagePath, appID, reserved)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrArtifactPublishLost
	}
	return nil
}

func (r *Repository) ListAppDeployments(ctx context.Context, projectID, appID uuid.UUID, actor AppActor, limit int, cursor *int64) ([]domain.AppDeployment, string, bool, error) {
	if limit < 1 || limit > 100 {
		return nil, "", false, ErrInvalidAppDeployment
	}
	canManage, err := r.requireAppRead(ctx, projectID, actor)
	if err != nil {
		return nil, "", false, err
	}
	if _, err := appByID(ctx, r.pool, projectID, appID, false); err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+appDeploymentProjection+`
		FROM app_deployments d JOIN project_apps a ON a.id=d.app_id AND a.project_id=d.project_id
		WHERE d.project_id=$1 AND d.app_id=$2 AND ($3::bigint IS NULL OR d.version<$3)
		ORDER BY d.version DESC LIMIT $4`, projectID, appID, cursor, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.AppDeployment, 0, limit)
	for rows.Next() {
		item, scanErr := scanAppDeployment(rows)
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
		next = fmt.Sprint(items[limit-1].Version)
		items = items[:limit]
	}
	return items, next, canManage, nil
}

func (r *Repository) GetAppDeployment(ctx context.Context, projectID, appID, deploymentID uuid.UUID, actor AppActor) (domain.AppDeployment, error) {
	if _, err := r.requireAppRead(ctx, projectID, actor); err != nil {
		return domain.AppDeployment{}, err
	}
	return publicAppDeploymentByID(ctx, r.pool, projectID, appID, deploymentID)
}

func publicAppDeploymentByID(ctx context.Context, query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, projectID, appID, deploymentID uuid.UUID) (domain.AppDeployment, error) {
	item, _, _, _, _, _, _, err := appDeploymentByID(ctx, query, projectID, appID, deploymentID, false, false)
	return item, err
}

func (r *Repository) ListAppBuildLogs(ctx context.Context, projectID, appID, deploymentID uuid.UUID, actor AppActor, limit int, after int64) ([]domain.AppBuildLog, error) {
	if limit < 1 || limit > 1000 || after < 0 {
		return nil, ErrInvalidAppDeployment
	}
	if _, err := r.requireAppRead(ctx, projectID, actor); err != nil {
		return nil, err
	}
	if _, err := appByID(ctx, r.pool, projectID, appID, false); err != nil {
		return nil, err
	}
	if _, err := publicAppDeploymentByID(ctx, r.pool, projectID, appID, deploymentID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+appBuildLogProjection+` FROM app_build_logs WHERE project_id=$1 AND app_id=$2 AND deployment_id=$3 AND sequence>$4 ORDER BY sequence LIMIT $5`, projectID, appID, deploymentID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.AppBuildLog, 0, limit)
	for rows.Next() {
		var item domain.AppBuildLog
		if err := rows.Scan(&item.ID, &item.DeploymentID, &item.AppID, &item.ProjectID, &item.Sequence, &item.Level, &item.Message, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SelectAppDeployment(ctx context.Context, projectID, appID, deploymentID uuid.UUID, actor AppActor) (domain.AppDeployment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireAppWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.AppDeployment{}, err
	}
	app, err := appByID(ctx, tx, projectID, appID, true)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	item, _, imagePath, _, _, _, _, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if item.BuildStatus != "succeeded" || item.Status != "ready" || item.ImageDigest == nil || item.ImageArchiveSHA256 == nil || item.ImageSizeBytes == nil || imagePath == nil {
		return domain.AppDeployment{}, ErrAppDeploymentNotReady
	}
	if app.DesiredDeploymentID == nil || *app.DesiredDeploymentID != deploymentID.String() {
		if app.DesiredGeneration == math.MaxInt64 {
			return domain.AppDeployment{}, ErrInvalidAppDeployment
		}
		if _, err := tx.Exec(ctx, `UPDATE project_apps SET desired_deployment_id=$3,desired_generation=desired_generation+1,runtime_status='pending',runtime_error=NULL,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, appID, deploymentID); err != nil {
			return domain.AppDeployment{}, err
		}
		if err := resetAppRuntimeRetryTx(ctx, tx, appID); err != nil {
			return domain.AppDeployment{}, err
		}
		metadata := appDeploymentAuditMetadata(item)
		metadata["desired_generation"] = app.DesiredGeneration + 1
		if err := r.auditAppDeploymentTx(ctx, tx, projectID, &actor, "app_deployment.select", deploymentID, metadata); err != nil {
			return domain.AppDeployment{}, err
		}
	}
	item, _, _, _, _, _, _, err = appDeploymentByID(ctx, tx, projectID, appID, deploymentID, false, false)
	if err != nil {
		return domain.AppDeployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AppDeployment{}, err
	}
	return item, nil
}

func (r *Repository) DeleteAppDeployment(ctx context.Context, projectID, appID, deploymentID uuid.UUID, actor AppActor) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.requireAppWriteTx(ctx, tx, projectID, actor); err != nil {
		return err
	}
	app, err := appByID(ctx, tx, projectID, appID, true)
	if err != nil {
		return err
	}
	item, sourcePath, imagePath, _, _, _, reserved, err := appDeploymentByID(ctx, tx, projectID, appID, deploymentID, true, true)
	if err != nil {
		return err
	}
	if app.DesiredDeploymentID != nil && *app.DesiredDeploymentID == deploymentID.String() {
		return ErrAppDeploymentSelected
	}
	if item.Status == "building" || item.BuildStatus == "running" {
		return ErrAppDeploymentRunning
	}
	quotaUpdate, err := tx.Exec(ctx, `UPDATE project_apps SET artifact_used_bytes=artifact_used_bytes-$3,artifact_reserved_bytes=artifact_reserved_bytes-$4,updated_at=now() WHERE project_id=$1 AND id=$2 AND artifact_used_bytes >= $3 AND artifact_reserved_bytes >= $4`, projectID, appID, item.SourceSizeBytes+valueInt64(item.ImageSizeBytes), reserved)
	if err != nil {
		return err
	}
	if quotaUpdate.RowsAffected() != 1 {
		return ErrInvalidAppDeployment
	}
	if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{ProjectID: projectID, StoreKind: ArtifactCleanupAppSources, Operation: ArtifactCleanupRelative, RelativePath: sourcePath}); err != nil {
		return err
	}
	if imagePath != nil {
		if err := queueArtifactCleanupTx(ctx, tx, ArtifactCleanupInput{ProjectID: projectID, StoreKind: ArtifactCleanupAppImages, Operation: ArtifactCleanupRelative, RelativePath: *imagePath}); err != nil {
			return err
		}
	}
	if err := r.auditAppDeploymentTx(ctx, tx, projectID, &actor, "app_deployment.delete", deploymentID, appDeploymentAuditMetadata(item)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM app_deployments WHERE project_id=$1 AND app_id=$2 AND id=$3`, projectID, appID, deploymentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
