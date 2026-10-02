package functionrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/functionsecret"
	"github.com/Stealth-deplover/stealth/internal/functionstore"
	"github.com/Stealth-deplover/stealth/internal/observability"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

const (
	defaultWorkerPoll         = 500 * time.Millisecond
	defaultLeaseAge           = 20 * time.Minute
	defaultBuildTimeout       = 15 * time.Minute
	defaultStagingRoot        = "/var/lib/stealth/runner-staging"
	defaultSourceDirMode      = 0o700
	maxFailureMessageSize     = 4000
	staleStagingSweepInterval = 5 * time.Minute
)

// RuntimeExecutor is intentionally narrower than DockerExecutor. Tests can
// provide a deterministic fake while production uses Docker with the
// restrictions in docker.go.
type RuntimeExecutor interface {
	Execute(
		context.Context,
		repository.FunctionExecutionJob,
		string,
		[]repository.FunctionRuntimeVariable,
	) (ExecutionResult, error)
}

// BuildExecutor is implemented by the production Docker executor. Keeping it
// separate from RuntimeExecutor lets tests and alternate runners continue to
// execute already-built artifacts without needing a container builder.
type BuildExecutor interface {
	Build(context.Context, repository.FunctionBuildJob, string, []repository.FunctionRuntimeVariable, io.Writer) error
}

type Worker struct {
	BuildStore     repository.FunctionBuildStore
	ExecutionStore repository.FunctionExecutionStore
	Store          *functionstore.Store
	Cipher         *functionsecret.Cipher
	Executor       RuntimeExecutor
	Builder        BuildExecutor
	WorkerID       string
	StagingRoot    string
	ArchiveLimit   ArchiveLimits
	PollInterval   time.Duration
	LeaseAge       time.Duration
	BuildTimeout   time.Duration
	Logger         *slog.Logger
	Metrics        *observability.WorkerMetrics
}

func NewWorker(
	persistence repository.FunctionWorkerStore,
	store *functionstore.Store,
	cipher *functionsecret.Cipher,
	executor RuntimeExecutor,
	workerID, stagingRoot string,
	logger *slog.Logger,
) (*Worker, error) {
	if persistence == nil || store == nil || cipher == nil || executor == nil || !validWorkerID(workerID) {
		return nil, fmt.Errorf("invalid function worker dependencies")
	}
	if strings.TrimSpace(stagingRoot) == "" {
		stagingRoot = defaultStagingRoot
	}
	stagingRoot, err := filepath.Abs(stagingRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve function worker staging root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(stagingRoot, "jobs"), defaultSourceDirMode); err != nil {
		return nil, fmt.Errorf("create function worker staging root: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	builder, _ := executor.(BuildExecutor)
	return &Worker{
		BuildStore:     persistence,
		ExecutionStore: persistence,
		Store:          store,
		Cipher:         cipher,
		Executor:       executor,
		Builder:        builder,
		WorkerID:       workerID,
		StagingRoot:    filepath.Clean(stagingRoot),
		ArchiveLimit:   ArchiveLimits{},
		PollInterval:   defaultWorkerPoll,
		LeaseAge:       defaultLeaseAge,
		BuildTimeout:   defaultBuildTimeout,
		Logger:         logger,
		Metrics:        observability.NewWorkerMetrics(),
	}, nil
}

// Run polls until ctx is cancelled. RequeueStaleFunctionExecutions is called
// before each poll so a crashed worker does not leave accepted work blocked.
func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.BuildStore == nil || w.ExecutionStore == nil {
		return errors.New("function worker is not configured")
	}
	poll := w.PollInterval
	if poll <= 0 {
		poll = defaultWorkerPoll
	}
	leaseAge := w.LeaseAge
	if leaseAge <= 0 {
		leaseAge = defaultLeaseAge
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	lastStagingSweep := time.Time{}
	for {
		if metrics := w.Metrics; metrics != nil {
			metrics.Polls.Inc()
		}
		if lastStagingSweep.IsZero() || time.Since(lastStagingSweep) >= staleStagingSweepInterval {
			if removed, err := w.cleanStaleStaging(2 * leaseAge); err != nil {
				w.Logger.Warn("stale function staging cleanup failed", "error", err)
			} else if removed > 0 {
				w.Logger.Info("removed stale function staging directories", "count", removed)
			}
			lastStagingSweep = time.Now()
		}
		if requeued, err := w.BuildStore.RequeueStaleFunctionDeployments(ctx, leaseAge); err != nil &&
			!errors.Is(err, context.Canceled) {
			if metrics := w.Metrics; metrics != nil {
				metrics.Errors.WithLabelValues("requeue_build").Inc()
			}
			w.Logger.Error("requeue stale function builds failed", "error", err)
		} else if requeued > 0 {
			if metrics := w.Metrics; metrics != nil {
				metrics.BuildRequeued.Add(float64(requeued))
			}
		}
		if requeued, err := w.ExecutionStore.RequeueStaleFunctionExecutions(ctx, leaseAge); err != nil &&
			!errors.Is(err, context.Canceled) {
			if metrics := w.Metrics; metrics != nil {
				metrics.Errors.WithLabelValues("requeue").Inc()
			}
			w.Logger.Error("requeue stale function executions failed", "error", err)
		} else if requeued > 0 {
			if metrics := w.Metrics; metrics != nil {
				metrics.Requeued.Add(float64(requeued))
			}
		}
		built, buildErr := w.RunBuildOnce(ctx)
		if buildErr != nil {
			if errors.Is(buildErr, context.Canceled) || errors.Is(buildErr, context.DeadlineExceeded) {
				return nil
			}
			w.Logger.Error("function build failed", "error", buildErr)
		}
		if built {
			continue
		}
		processed, err := w.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			w.Logger.Error("function execution failed", "error", err)
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) MetricsHandler() http.Handler {
	if w == nil || w.Metrics == nil {
		return http.NotFoundHandler()
	}
	return w.Metrics.Handler()
}

func validWorkerID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '.' ||
			character == '_' ||
			character == '-' {
			continue
		}
		return false
	}
	return true
}

// cleanStaleStaging removes private Function and Site workspaces left by a
// process that exited before its deferred cleanup ran. Only UUID-named
// directories older than maxAge are removed, and every candidate is verified
// to remain inside root before deletion.
func (w *Worker) cleanStaleStaging(maxAge time.Duration) (int, error) {
	if w == nil || w.StagingRoot == "" {
		return 0, errors.New("function staging cleanup is not configured")
	}
	total := 0
	for _, subdirectory := range []string{"jobs", "builds", "build-validation"} {
		removed, err := sweepStaleStagingRoot(filepath.Join(w.StagingRoot, subdirectory), maxAge)
		if err != nil {
			return total, err
		}
		total += removed
	}
	return total, nil
}

func sweepStaleStagingRoot(root string, maxAge time.Duration) (int, error) {
	if root == "" || maxAge <= 0 {
		return 0, errors.New("function staging cleanup is not configured")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
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
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.ModTime().After(cutoff) {
			continue
		}
		if err := ensureWithin(root, path); err != nil {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func redactFailure(message string, secrets []string) string {
	message = Redact(message, secrets)
	message, _ = executionErrorText(message)
	if strings.TrimSpace(message) == "" {
		return "function execution failed"
	}
	return message
}

func mustUUID(value string) uuid.UUID {
	parsed, _ := uuid.Parse(value)
	return parsed
}
