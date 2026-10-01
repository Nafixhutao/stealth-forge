// Package ingress materializes PostgreSQL-derived platform Site and eligible
// App routes into the existing Traefik file provider. It never treats the
// generated files as authoritative and owns two snapshots plus one reload
// sentinel.
package ingress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log/slog"

	"github.com/Stealth-deplover/stealth/internal/domain"
)

const (
	GeneratedFilename          = "platform-sites.yaml"
	DefaultPlatformSiteBackend = "http://api:8082"
	GeneratedAppFilename       = "platform-apps.yaml"
)

var ErrLockNotAcquired = errors.New("platform route reconciliation lock is held by another worker")

// Store is intentionally narrower than Repository. The worker needs only a
// distributed lease and the authoritative desired-state snapshot.
type Store interface {
	TryPlatformRouteReconcileLock(context.Context) (func() error, bool, error)
	ListPlatformRoutes(context.Context) ([]domain.PlatformRoute, error)
	ListAppPlatformRoutes(context.Context) ([]domain.AppPlatformRoute, error)
}

type Reconciler struct {
	store      Store
	outputFile string
	appFile    string
	reloadFile string
	backendURL string
	interval   time.Duration
	logger     *slog.Logger
}

type Result struct {
	Routes        int
	AppRoutes     int
	LockAcquired  bool
	Changed       bool
	ReloadUpdated bool
}

func New(store Store, generatedDir, reloadFile string, interval time.Duration, logger *slog.Logger) (*Reconciler, error) {
	if store == nil {
		return nil, errors.New("platform route store is required")
	}
	generatedDir = filepath.Clean(strings.TrimSpace(generatedDir))
	reloadFile = filepath.Clean(strings.TrimSpace(reloadFile))
	if !filepath.IsAbs(generatedDir) || !filepath.IsAbs(reloadFile) || generatedDir == string(filepath.Separator) || reloadFile == string(filepath.Separator) {
		return nil, errors.New("platform route paths must be absolute non-root paths")
	}
	if pathWithin(generatedDir, reloadFile) {
		return nil, errors.New("Traefik reload sentinel must be outside the generated route directory")
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{
		store:      store,
		outputFile: filepath.Join(generatedDir, GeneratedFilename),
		appFile:    filepath.Join(generatedDir, GeneratedAppFilename),
		reloadFile: reloadFile,
		backendURL: DefaultPlatformSiteBackend,
		interval:   interval,
		logger:     logger,
	}, nil
}

func pathWithin(directory, path string) bool {
	relative, err := filepath.Rel(directory, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

// Run performs an immediate reconstruction and then retries on a bounded
// ticker. Transient database or filesystem failures preserve the last known
// good file and do not take down unrelated worker loops.
func (r *Reconciler) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.reconcileAndLog(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.reconcileAndLog(ctx)
		}
	}
}

func (r *Reconciler) reconcileAndLog(ctx context.Context) {
	result, err := r.Reconcile(ctx)
	if errors.Is(err, ErrLockNotAcquired) {
		r.logger.Info("platform route reconcile lock not acquired")
		return
	}
	if err != nil {
		r.logger.Error("platform route reconcile failed", "error", err)
		return
	}
	if result.Changed {
		r.logger.Info("platform routes reconciled", "routes", result.Routes, "reload_updated", result.ReloadUpdated)
		return
	}
	r.logger.Info("platform route reconcile no-op", "routes", result.Routes)
}

// Reconcile publishes one complete deterministic snapshot. The distributed
// lease remains held through render and publication so an older worker cannot
// overwrite a newer snapshot.
func (r *Reconciler) Reconcile(ctx context.Context) (result Result, err error) {
	release, acquired, err := r.store.TryPlatformRouteReconcileLock(ctx)
	if err != nil {
		return result, fmt.Errorf("acquire platform route lock: %w", err)
	}
	if !acquired {
		return result, ErrLockNotAcquired
	}
	result.LockAcquired = true
	defer func() {
		releaseErr := release()
		if releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release platform route lock: %w", releaseErr))
		}
	}()

	routes, err := r.store.ListPlatformRoutes(ctx)
	if err != nil {
		return result, fmt.Errorf("read platform route snapshot: %w", err)
	}
	result.Routes = len(routes)
	siteContents, err := Render(routes, r.backendURL)
	if err != nil {
		return result, fmt.Errorf("render platform routes: %w", err)
	}
	siteChanged, err := publishSnapshotFile(r.outputFile, siteContents)
	if err != nil {
		return result, fmt.Errorf("publish platform Site routes: %w", err)
	}
	appRoutes, appListErr := r.store.ListAppPlatformRoutes(ctx)
	var appContents []byte
	if appListErr == nil {
		result.AppRoutes = len(appRoutes)
		appContents, err = RenderApps(appRoutes)
		if err != nil {
			appListErr = fmt.Errorf("render App routes: %w", err)
		}
	}
	if appListErr != nil {
		// A transient App snapshot failure preserves its last-known-good file,
		// while a fresh Site snapshot can still converge independently.
		appContents, err = readOrEmptyManagedFile(r.appFile, []byte("# Stealth App route snapshot: no eligible Apps\n"))
		if err != nil {
			return result, errors.Join(appListErr, err)
		}
	} else {
		var appChanged bool
		appChanged, err = publishSnapshotFile(r.appFile, appContents)
		result.Changed = result.Changed || appChanged
		if err != nil {
			appListErr = fmt.Errorf("publish App routes: %w", err)
			appContents, err = readOrEmptyManagedFile(r.appFile, []byte("# Stealth App route snapshot: no eligible Apps\n"))
			if err != nil {
				return result, errors.Join(appListErr, err)
			}
		}
	}
	reloadContents := routeSetReloadSentinel(siteContents, appContents)
	currentReload, reloadErr := readManagedFile(r.reloadFile)
	if reloadErr != nil && !errors.Is(reloadErr, os.ErrNotExist) {
		return result, reloadErr
	}
	if !bytes.Equal(currentReload, reloadContents) || errors.Is(reloadErr, os.ErrNotExist) {
		if err := publishAtomic(r.reloadFile, reloadContents, 0o644); err != nil {
			return result, fmt.Errorf("publish route reload sentinel: %w", err)
		}
		result.ReloadUpdated = true
	}
	result.Changed = result.Changed || siteChanged || result.ReloadUpdated
	err = appListErr
	if err != nil {
		return result, err
	}
	return result, nil
}
