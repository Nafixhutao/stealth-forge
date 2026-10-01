package appbuilder

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Stealth-deplover/stealth/internal/appbuildspec"
	"github.com/Stealth-deplover/stealth/internal/appstore"
	"github.com/Stealth-deplover/stealth/internal/buildkitmetadata"
	"github.com/Stealth-deplover/stealth/internal/functionrunner"
	"github.com/Stealth-deplover/stealth/internal/ociartifact"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (w *Worker) build(parent context.Context, job repository.AppBuildJob) (result string, returned error) {
	projectID, err := uuid.Parse(job.Deployment.ProjectID)
	if err != nil {
		return "error", errors.New("invalid App build job")
	}
	appID, err := uuid.Parse(job.Deployment.AppID)
	if err != nil {
		return "error", errors.New("invalid App build job")
	}
	deploymentID, err := uuid.Parse(job.Deployment.ID)
	if err != nil {
		return "error", errors.New("invalid App build job")
	}
	if !safeWorkerID(job.WorkerID) || !safeRelativeArtifact(job.SourcePath) || job.Deployment.Source != "upload" {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source unavailable")
	}
	platform, err := appbuildspec.CurrentHostPlatform()
	if err != nil || job.Deployment.Platform != platform {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "build platform is unavailable")
	}
	buildCtx, cancel := context.WithTimeout(parent, positiveDuration(w.BuildTimeout, defaultBuildTimeout))
	defer cancel()
	jobRoot := filepath.Join(w.StagingRoot, deploymentID.String())
	if !insideRoot(w.StagingRoot, jobRoot) {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "build workspace is unavailable")
	}
	if err := os.RemoveAll(jobRoot); err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "build workspace is unavailable")
	}
	if err := os.Mkdir(jobRoot, 0o700); err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "build workspace is unavailable")
	}
	defer func() { _ = os.RemoveAll(jobRoot) }()
	sourceRoot := filepath.Join(jobRoot, "source")
	if err := os.Mkdir(sourceRoot, 0o700); err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "build workspace is unavailable")
	}
	if err := w.appendLog(parent, job.WorkerID, projectID, appID, deploymentID, "info", "Build started"); err != nil {
		w.Logger.Warn("could not persist App build log", "deployment_id", deploymentID, "error", err)
	}

	archive, err := w.Artifacts.Sources.OpenRelative(buildCtx, job.SourcePath)
	if err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source unavailable")
	}
	actualChecksum, checksumErr := checksumSeekable(archive)
	if checksumErr != nil {
		_ = archive.Close()
		if parent.Err() != nil {
			return "error", parent.Err()
		}
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source unavailable")
	}
	if actualChecksum != job.Deployment.SourceChecksumSHA256 {
		_ = archive.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source checksum mismatch")
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		_ = archive.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source unavailable")
	}
	limits := w.ArchiveLimit
	limits.MaxCompressed = job.Deployment.SourceSizeBytes
	if limits.MaxCompressed <= 0 {
		_ = archive.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source unavailable")
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = 1 << 30
	}
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = 8192
	}
	if limits.MaxEntry <= 0 {
		limits.MaxEntry = limits.MaxBytes
	}
	stats, extractErr := functionrunner.Extract(buildCtx, archive, valueOr(job.Deployment.SourceName, ""), sourceRoot, limits)
	_ = archive.Close()
	if extractErr != nil || stats.Files == 0 {
		if parent.Err() != nil {
			return "error", parent.Err()
		}
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "BuildKit timeout")
		}
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "source archive invalid")
	}
	if err := w.appendLog(parent, job.WorkerID, projectID, appID, deploymentID, "info", "Source archive extracted"); err != nil {
		w.Logger.Warn("could not persist App build log", "deployment_id", deploymentID, "error", err)
	}

	definition := appbuildspec.Spec{
		DockerfilePath: job.Deployment.DockerfilePath, ContextDirectory: job.Deployment.ContextDirectory,
		Target: job.Deployment.Target, Platform: job.Deployment.Platform,
	}
	contextPath, err := ValidateBuildContext(sourceRoot, definition)
	if err != nil {
		if errors.Is(err, ErrUnsafeDockerfileFrontend) {
			return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "Dockerfile frontend override is not allowed")
		}
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "Dockerfile or context is invalid")
	}
	if err := w.appendLog(parent, job.WorkerID, projectID, appID, deploymentID, "info", "BuildKit connected; build started"); err != nil {
		w.Logger.Warn("could not persist App build log", "deployment_id", deploymentID, "error", err)
	}
	outputPath := filepath.Join(jobRoot, "image.oci.tar")
	metadataPath := filepath.Join(jobRoot, "buildkit-metadata.json")
	progress := &progressWriter{
		ctx: buildCtx, store: w.Store, workerID: job.WorkerID, projectID: projectID, appID: appID,
		deploymentID: deploymentID, redactRoot: w.StagingRoot,
	}
	buildCommandCtx, cancelBuildCommand := context.WithCancel(buildCtx)
	tooLarge := make(chan struct{}, 1)
	monitorDone := make(chan struct{})
	go monitorArtifactSize(buildCommandCtx, cancelBuildCommand, outputPath, w.Artifacts.Images.MaxBytes(), tooLarge, monitorDone)
	buildErr := w.Builder.Build(buildCommandCtx, BuildRequest{Definition: definition, ContextPath: contextPath, DockerfileRoot: sourceRoot, OutputPath: outputPath, MetadataPath: metadataPath}, progress)
	cancelBuildCommand()
	<-monitorDone
	progress.Flush()
	select {
	case <-tooLarge:
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "artifact too large")
	default:
	}
	if buildErr != nil {
		if errors.Is(parent.Err(), context.Canceled) || errors.Is(parent.Err(), context.DeadlineExceeded) {
			return "error", parent.Err()
		}
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "BuildKit timeout")
		}
		if errors.Is(buildErr, ErrBuildKitUnavailable) || errors.Is(w.Builder.Ready(parent), ErrBuildKitUnavailable) {
			if err := w.Store.DeferAppDeploymentBuild(parent, projectID, appID, deploymentID, job.WorkerID, "BuildKit unavailable; build remains queued"); err != nil {
				return "error", err
			}
			return "deferred", nil
		}
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "Dockerfile build failed")
	}
	if err := w.appendLog(parent, job.WorkerID, projectID, appID, deploymentID, "info", "OCI export produced"); err != nil {
		w.Logger.Warn("could not persist App build log", "deployment_id", deploymentID, "error", err)
	}
	metadataFile, err := os.Open(metadataPath)
	if err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "BuildKit metadata invalid")
	}
	metadata, metadataErr := buildkitmetadata.Parse(metadataFile)
	closeErr := metadataFile.Close()
	if metadataErr != nil || closeErr != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "BuildKit metadata invalid")
	}
	imageFile, err := os.Open(outputPath)
	if err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI export invalid")
	}
	imageInfo, err := imageFile.Stat()
	if err != nil || !imageInfo.Mode().IsRegular() || imageInfo.Size() <= 0 {
		_ = imageFile.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI export invalid")
	}
	if imageInfo.Size() > w.Artifacts.Images.MaxBytes() {
		_ = imageFile.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "artifact too large")
	}
	if err := ociartifact.Validate(imageFile, metadata.ImageDigest, w.Artifacts.Images.MaxBytes()); err != nil {
		_ = imageFile.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI export invalid")
	}
	if _, err := imageFile.Seek(0, io.SeekStart); err != nil {
		_ = imageFile.Close()
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI export invalid")
	}
	imageArtifactID, err := uuid.NewV7()
	if err != nil {
		_ = imageFile.Close()
		return "error", err
	}
	prepared, err := w.Artifacts.Images.BeginUpload(buildCtx, projectID, appID, imageArtifactID, imageFile, w.Artifacts.Images.MaxBytes())
	_ = imageFile.Close()
	if err != nil {
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "BuildKit timeout")
		}
		if errors.Is(err, appstore.ErrTooLarge) {
			return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "artifact too large")
		}
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI export invalid")
	}
	defer w.Artifacts.Images.Cleanup(&prepared)
	if prepared.Size != imageInfo.Size() {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI export invalid")
	}
	publishCleanup := repository.ArtifactCleanupInput{ProjectID: projectID, StoreKind: repository.ArtifactCleanupAppImages, Operation: repository.ArtifactCleanupRelative, RelativePath: prepared.RelativePath}
	if err := w.Store.ReserveAppImagePublish(parent, projectID, appID, deploymentID, job.WorkerID, prepared.Size, publishCleanup); err != nil {
		if errors.Is(err, repository.ErrAppArtifactQuotaExceeded) {
			return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "artifact quota exceeded")
		}
		if errors.Is(err, repository.ErrAppBuildNotOwned) {
			return "error", err
		}
		return "error", errors.New("OCI artifact publication could not be reserved")
	}
	if err := w.Artifacts.Images.Commit(parent, &prepared); err != nil {
		return w.fail(parent, job.WorkerID, projectID, appID, deploymentID, "OCI artifact could not be published")
	}
	if err := w.appendLog(parent, job.WorkerID, projectID, appID, deploymentID, "info", "OCI digest verified and artifact persisted"); err != nil {
		w.Logger.Warn("could not persist App build log", "deployment_id", deploymentID, "error", err)
	}
	if _, err := w.Store.CompleteAppDeploymentBuildWithCleanup(parent, projectID, appID, deploymentID, job.WorkerID, metadata.ImageDigest, prepared.RelativePath, prepared.Checksum, prepared.Size, publishCleanup); err != nil {
		// The durable cleanup reservation must remain: a DB commit failure may
		// be ambiguous, so deleting the artifact here could break a committed row.
		return "error", errors.New("App deployment completion was not confirmed")
	}
	return "succeeded", nil
}

func monitorArtifactSize(ctx context.Context, cancel context.CancelFunc, path string, maximum int64, tooLarge chan<- struct{}, done chan<- struct{}) {
	defer close(done)
	if maximum <= 0 {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(path)
			if err == nil && info.Mode().IsRegular() && info.Size() > maximum {
				cancel()
				select {
				case tooLarge <- struct{}{}:
				default:
				}
				return
			}
		}
	}
}

func (w *Worker) fail(ctx context.Context, workerID string, projectID, appID, deploymentID uuid.UUID, message string) (string, error) {
	if ctx.Err() != nil {
		return "error", ctx.Err()
	}
	if _, err := w.Store.FailAppDeploymentBuild(ctx, projectID, appID, deploymentID, workerID, message); err != nil {
		return "error", err
	}
	return "failed", nil
}

func (w *Worker) appendLog(ctx context.Context, workerID string, projectID, appID, deploymentID uuid.UUID, level, message string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	_, err = w.Store.AppendAppBuildLog(ctx, projectID, appID, deploymentID, workerID, id, level, message)
	return err
}

func (w *Worker) logUnavailable(err error) {
	w.logMu.Lock()
	defer w.logMu.Unlock()
	if time.Since(w.lastUnavailableLog) < time.Minute {
		return
	}
	w.lastUnavailableLog = time.Now()
	w.Logger.Warn("dedicated BuildKit service is unavailable; App build jobs remain queued", "error", err)
}
