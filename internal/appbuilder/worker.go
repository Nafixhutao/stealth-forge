// Package appbuilder consumes durable AppDeployment build jobs and publishes
// verified OCI archives. Build execution is delegated only to the dedicated
// BuildKit service; this package never uses the worker's Docker socket.
package appbuilder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Stealth-deplover/stealth/internal/appstore"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/functionrunner"
	"github.com/Stealth-deplover/stealth/internal/observability"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	defaultBuildTimeout = 20 * time.Minute
	defaultLeaseAge     = 25 * time.Minute
	defaultPollInterval = 500 * time.Millisecond
	maxRetainedLogBytes = 4 << 20
	maxRetainedLogLines = 2000
)

type Persistence interface {
	RequeueStaleAppDeployments(context.Context, time.Duration) (int64, error)
	ClaimNextAppDeployment(context.Context, string) (repository.AppBuildJob, error)
	DeferAppDeploymentBuild(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) error
	ReserveAppImagePublish(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, int64, repository.ArtifactCleanupInput) error
	CompleteAppDeploymentBuildWithCleanup(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string, string, string, int64, repository.ArtifactCleanupInput) (domain.AppDeployment, error)
	FailAppDeploymentBuild(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (domain.AppDeployment, error)
	AppendAppBuildLog(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, uuid.UUID, string, string) (domain.AppBuildLog, error)
}

var _ Persistence = (*repository.Repository)(nil)

const staleStagingSweepInterval = 5 * time.Minute

type Worker struct {
	Store        Persistence
	Artifacts    *appstore.Store
	Builder      *BuildKitClient
	WorkerID     string
	StagingRoot  string
	ArchiveLimit functionrunner.ArchiveLimits
	PollInterval time.Duration
	LeaseAge     time.Duration
	BuildTimeout time.Duration
	Logger       *slog.Logger
	Metrics      *observability.WorkerMetrics

	lastUnavailableLog time.Time
	logMu              sync.Mutex
}

func New(store Persistence, artifacts *appstore.Store, builder *BuildKitClient, workerID, stagingRoot string, logger *slog.Logger) (*Worker, error) {
	if store == nil || artifacts == nil || artifacts.Sources == nil || artifacts.Images == nil || builder == nil || !safeWorkerID(workerID) {
		return nil, errors.New("invalid App build worker dependencies")
	}
	if strings.TrimSpace(stagingRoot) == "" {
		return nil, errors.New("App build staging root is required")
	}
	absRoot, err := filepath.Abs(stagingRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve App build staging root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create App build staging root: %w", err)
	}
	info, err := os.Lstat(absRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("App build staging root must be a private real directory")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		Store: store, Artifacts: artifacts, Builder: builder, WorkerID: workerID,
		StagingRoot: filepath.Clean(absRoot), ArchiveLimit: functionrunner.ArchiveLimits{},
		PollInterval: defaultPollInterval, LeaseAge: defaultLeaseAge, BuildTimeout: defaultBuildTimeout,
		Logger: logger, Metrics: observability.NewWorkerMetrics(),
	}, nil
}

// Run supervises the App build queue independently. Readiness is checked
// before claiming work, so a BuildKit outage leaves the durable queue intact.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.Store == nil || w.Builder == nil || w.Artifacts == nil {
		return errors.New("App build worker is not configured")
	}
	poll := w.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	leaseAge := w.LeaseAge
	if leaseAge <= 0 {
		leaseAge = defaultLeaseAge
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	lastStagingSweep := time.Time{}
	for {
		if ctx.Err() != nil {
			return nil
		}
		if requeued, err := w.Store.RequeueStaleAppDeployments(ctx, leaseAge); err != nil && ctx.Err() == nil {
			w.Logger.Error("requeue stale App builds failed", "error", err)
		} else if requeued > 0 && w.Metrics != nil {
			w.Metrics.AppBuildRequeued.Add(float64(requeued))
		}
		if lastStagingSweep.IsZero() || time.Since(lastStagingSweep) >= staleStagingSweepInterval {
			if removed, err := w.cleanStaleStaging(2 * leaseAge); err != nil {
				w.Logger.Warn("stale App build staging cleanup failed", "error", err)
			} else if removed > 0 {
				w.Logger.Info("removed stale App build staging directories", "count", removed)
			}
			removedArtifactTemps := 0
			for _, namespace := range []*appstore.Namespace{w.Artifacts.Sources, w.Artifacts.Images} {
				removed, err := namespace.CleanupStaleUploads(ctx, 2*leaseAge)
				if err != nil {
					w.Logger.Warn("stale App artifact upload cleanup failed", "error", err)
					continue
				}
				removedArtifactTemps += removed
			}
			if removedArtifactTemps > 0 {
				w.Logger.Info("removed stale App artifact upload staging files", "count", removedArtifactTemps)
			}
			lastStagingSweep = time.Now()
		}
		processed, err := w.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			w.Logger.Error("App build worker iteration failed", "error", err)
		}
		if processed && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// cleanStaleStaging removes private workspaces left by a process that exited
// before its deferred cleanup ran. A directory older than twice the build
// lease cannot belong to a live build: build timeouts are configured no longer
// than their lease, and a reclaimed deployment gets a fresh workspace.
func (w *Worker) cleanStaleStaging(maxAge time.Duration) (int, error) {
	if w == nil || maxAge <= 0 || w.StagingRoot == "" {
		return 0, errors.New("App build staging cleanup is not configured")
	}
	entries, err := os.ReadDir(w.StagingRoot)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := uuid.Parse(entry.Name())
		if err != nil || id == uuid.Nil || id.Version() != uuid.Version(7) || id.String() != entry.Name() {
			continue
		}
		path := filepath.Join(w.StagingRoot, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.ModTime().After(cutoff) {
			continue
		}
		if !insideRoot(w.StagingRoot, path) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// RunOnce probes BuildKit before claiming a lease and processes at most one
// job. The readiness failure is nonfatal to this queue and all other workers.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	if err := w.Builder.Ready(ctx); err != nil {
		if w.Metrics != nil {
			w.Metrics.AppBuildErrors.WithLabelValues("buildkit_unavailable").Inc()
		}
		w.logUnavailable(err)
		return false, nil
	}
	leaseID, err := uuid.NewV7()
	if err != nil {
		return false, err
	}
	leaseToken := leaseID.String()
	job, err := w.Store.ClaimNextAppDeployment(ctx, leaseToken)
	if errors.Is(err, repository.ErrNoAppDeploymentJob) {
		return false, nil
	}
	if err != nil {
		if w.Metrics != nil {
			w.Metrics.AppBuildErrors.WithLabelValues("claim").Inc()
		}
		return false, err
	}
	if job.WorkerID != leaseToken {
		return true, errors.New("App build lease token did not match the claimed job")
	}
	started := time.Now()
	spanContext, span := observability.StartWorkerSpan(ctx, "apps.build",
		attribute.String("stealth.app.deployment.version", fmt.Sprint(job.Deployment.Version)))
	if w.Metrics != nil {
		w.Metrics.AppBuildsClaimed.Inc()
		w.Metrics.AppBuildInFlight.Inc()
	}
	result, buildErr := w.build(spanContext, job)
	if buildErr != nil {
		span.RecordError(errors.New("App build failed"))
		span.SetStatus(codes.Error, "App build failed")
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
	if w.Metrics != nil {
		w.Metrics.AppBuildInFlight.Dec()
		if result == "" {
			result = "error"
		}
		w.Metrics.AppBuildDuration.WithLabelValues(result).Observe(time.Since(started).Seconds())
		if result == "succeeded" || result == "failed" {
			w.Metrics.AppBuildsCompleted.WithLabelValues(result).Inc()
		}
		if buildErr != nil {
			w.Metrics.AppBuildErrors.WithLabelValues("build").Inc()
		}
	}
	return true, buildErr
}
