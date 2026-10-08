// Package installengine owns the idempotent installation primitives used by
// the host CLI. UI and setup API layers select or project state; this package
// owns files, Compose, and health checks without exposing a shell boundary to
// the browser.
package installengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAssetBase        = "https://raw.githubusercontent.com/Stealth-deplover/stealth"
	defaultReleaseAssetBase = "https://github.com/Stealth-deplover/stealth/releases/download"
	maxAssetSize            = 2 << 20
	lockFileName            = "install.lock"
)

// ErrOperationInProgress lets a recovering worker distinguish lock ownership
// from an installation failure. A worker that loses the race leaves the
// durable run in installing so the lock owner can continue publishing state;
// a later process can resume it after a crash releases the OS lock.
var ErrOperationInProgress = errors.New("another installation operation is already running")

// CommandRunner is the only process boundary the host engine needs. The setup
// API does not construct an engine or receive a command runner.
type CommandRunner interface {
	Run(context.Context, string, io.Writer, io.Writer, string, ...string) error
	Output(context.Context, string, string, ...string) ([]byte, error)
}

// EnvironmentCommandRunner runs a command with explicit process environment
// overrides. Staged Compose validation uses this to point env_file entries at
// the staged config.env before the final installation files are activated.
type EnvironmentCommandRunner interface {
	RunWithEnv(context.Context, string, []string, io.Writer, io.Writer, string, ...string) error
}

// InputCommandRunner extends the process boundary for fixed trusted input.
// It is used when a narrow privileged host command must consume a validated
// release asset without reopening a user-writable source path.
type InputCommandRunner interface {
	RunInput(context.Context, string, io.Reader, io.Writer, io.Writer, string, ...string) error
}

// OSCommandRunner executes the Docker Compose commands selected by a plan.
// It is intentionally small so tests can assert the exact command surface.
type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func (OSCommandRunner) RunWithEnv(ctx context.Context, dir string, env []string, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), env...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func (OSCommandRunner) RunInput(ctx context.Context, dir string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func (OSCommandRunner) Output(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	return command.Output()
}

type Options struct {
	Runner       CommandRunner
	HTTPClient   *http.Client
	AssetBaseURL string
	// ReleaseAssetBaseURL points at the versioned GitHub Release that carries
	// checksums.txt. Managed source files are accepted only when that release's
	// checksum manifest verifies their bytes.
	ReleaseAssetBaseURL string
	Output              io.Writer
	PollAttempts        int
	PollInterval        time.Duration

	// BuildKitAppArmorProfilePath is injectable for tests. Production stores
	// the profile under AppArmor's system profile directory so it is loaded on
	// host boot as well as during installation.
	BuildKitAppArmorProfilePath string

	// ManagedAssets is intentionally supplied by the release binary, never by
	// CLI input. It lets the target release own its runtime asset manifest, so
	// a future release can add an asset without teaching the previous binary
	// about it first. A nil value selects this release's built-in manifest.
	ManagedAssets []ManagedAsset

	// MigrationHook is a test-only fault-injection seam. Production callers do
	// not set it; it exists so recovery is exercised after each durable
	// transaction boundary instead of relying on timing-sensitive SIGKILL tests.
	MigrationHook func(MigrationEvent) error
}
type Engine struct {
	runner              CommandRunner
	httpClient          *http.Client
	assetBaseURL        string
	releaseAssetBaseURL string
	output              io.Writer
	pollAttempts        int
	pollInterval        time.Duration
	appArmorProfilePath string
	managedAssets       []ManagedAsset
	migrationHook       func(MigrationEvent) error
}

func New(options Options) *Engine {
	runner := options.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	assetBaseURL := strings.TrimRight(strings.TrimSpace(options.AssetBaseURL), "/")
	if assetBaseURL == "" {
		assetBaseURL = defaultAssetBase
	}
	releaseAssetBaseURL := strings.TrimRight(strings.TrimSpace(options.ReleaseAssetBaseURL), "/")
	if releaseAssetBaseURL == "" {
		if strings.TrimSpace(options.AssetBaseURL) != "" {
			// Controlled acceptance fixtures serve raw files and checksums from
			// one local origin. Production always uses the fixed release origin.
			releaseAssetBaseURL = strings.TrimRight(strings.TrimSpace(options.AssetBaseURL), "/")
		} else {
			releaseAssetBaseURL = defaultReleaseAssetBase
		}
	}
	attempts := options.PollAttempts
	if attempts < 1 {
		attempts = 60
	}
	interval := options.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	output := options.Output
	if output == nil {
		output = io.Discard
	}
	appArmorProfilePath := strings.TrimSpace(options.BuildKitAppArmorProfilePath)
	if appArmorProfilePath == "" {
		appArmorProfilePath = BuildKitAppArmorProfilePath
	}
	managedAssets := options.ManagedAssets
	if managedAssets == nil {
		managedAssets = DefaultManagedAssets()
	}
	return &Engine{
		runner: runner, httpClient: httpClient, assetBaseURL: assetBaseURL,
		releaseAssetBaseURL: releaseAssetBaseURL, output: output,
		pollAttempts: attempts, pollInterval: interval, appArmorProfilePath: appArmorProfilePath,
		managedAssets: append([]ManagedAsset(nil), managedAssets...),
		migrationHook: options.MigrationHook,
	}
}

// Install executes every step while holding an OS-level lock. The lock is
// released on process exit, so a restarted host CLI can safely resume a
// partial installation without manual lock-file cleanup.
func (e *Engine) Install(ctx context.Context, plan Plan, emit func(Event)) error {
	if e == nil {
		return errors.New("install engine is nil")
	}
	lock, err := acquireLock(plan.Layout.StateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	return e.InstallLocked(ctx, plan, emit)
}

// InstallLocked executes every step after the caller has acquired the
// installation lock. It exists for the host orchestrator, which must hold the
// same lock while it claims the durable InstallRunID and performs handoff
// cleanup. Callers must not invoke this method without owning the lock for
// plan.Layout.StateDir.
func (e *Engine) InstallLocked(ctx context.Context, plan Plan, emit func(Event)) error {
	if e == nil {
		return errors.New("install engine is nil")
	}
	for step := StepConfiguration; step <= StepVerify; step++ {
		event := Event{Step: StepNames[step], Status: "started", At: time.Now().UTC()}
		if emit != nil {
			emit(event)
		}
		if err := e.RunStep(ctx, plan, step); err != nil {
			event.Status = "failed"
			event.Error = safeError(err)
			event.At = time.Now().UTC()
			if emit != nil {
				emit(event)
			}
			return err
		}
		event.Status = "succeeded"
		event.Message = "complete"
		event.At = time.Now().UTC()
		if emit != nil {
			emit(event)
		}
	}
	return nil
}
func (e *Engine) RunStep(ctx context.Context, plan Plan, step Step) error {
	switch step {
	case StepConfiguration:
		prepared, err := e.prepareInstallation(ctx, plan)
		if err != nil {
			return err
		}
		// Validate the target Compose file and target config while both are still
		// staged. This rejects interpolation and topology errors before any
		// release-managed path, config.env, or VERSION is activated.
		if err := e.validateStagedCompose(ctx, plan, prepared); err != nil {
			return errors.Join(err, prepared.rollback())
		}
		if !plan.Setup {
			if err := ensureTraefikDirectories(plan.Layout); err != nil {
				return errors.Join(err, prepared.rollback())
			}
		}
		// Existing installations from the previous release may still have the
		// dynamic directory owned by the fixed worker UID. Run the target
		// release's narrow init service against the staged Compose file before
		// managed assets are activated, so an unprivileged host user can still
		// complete an upgrade without changing ownership itself.
		if !plan.Setup && plan.Existing {
			composePath, ok := prepared.assets.stagedAssetPath("compose.production.yaml")
			if !ok {
				return prepared.failCommit(errors.New("staged production Compose asset is missing"))
			}
			stageEnvFile := filepath.Join(prepared.assets.stageDir, "config.env")
			if err := WritePrivateFile(stageEnvFile, string(prepared.newEnv)); err != nil {
				return prepared.failCommit(fmt.Errorf("stage target configuration for Traefik state initialization: %w", err))
			}
			if err := e.runComposeFileWithEnv(ctx, plan, composePath, plan.Layout.Root, stageEnvFile,
				"run", "--rm", "--no-deps",
				"-e", fmt.Sprintf("STEALTH_TRAEFIK_HOST_UID=%d", os.Geteuid()),
				"traefik-state-init"); err != nil {
				return prepared.failCommit(err)
			}
		}
		if err := prepared.commit(plan); err != nil {
			return err
		}
		if err := e.runCompose(ctx, plan, "config", "--quiet"); err != nil {
			rollbackErr := prepared.rollback()
			return errors.Join(err, rollbackErr)
		}
		if err := prepared.composeValidated(); err != nil {
			if errors.Is(err, errMigrationProcessInterrupted) {
				return err
			}
			return errors.Join(err, prepared.rollback())
		}
		// From COMPOSE_VALIDATED onward the new release is coherent. If the
		// bounded backup publication is interrupted, the next lifecycle command
		// completes it forward rather than rolling valid target assets back.
		if err := prepared.finalize(); err != nil {
			return err
		}
		if !plan.Setup {
			if _, err := ensureBuildKitPKI(plan.Layout); err != nil {
				return fmt.Errorf("prepare BuildKit mutual TLS identity: %w", err)
			}
			return e.ensureBuildKitAppArmorProfile(ctx, plan)
		}
		return nil
	case StepPull:
		return e.runCompose(ctx, plan, "pull")
	case StepDependencies:
		if !plan.Setup {
			// StepServices may use --no-deps for external PostgreSQL/Redis.
			// Run all narrow ownership init services explicitly so that path
			// never bypasses persistent state preparation.
			if err := e.runTraefikStateInit(ctx, plan, plan.Layout.ComposeFile, ""); err != nil {
				return err
			}
			if err := e.runCompose(ctx, plan, "run", "--rm", "--no-deps", "cloudflare-setup-state-init"); err != nil {
				return fmt.Errorf("prepare Cloudflare legacy-state input: %w", err)
			}
			if err := e.runCompose(ctx, plan, "run", "--rm", "--no-deps", "cloudflare-state-init"); err != nil {
				return err
			}
			if err := e.runCompose(ctx, plan, "run", "--rm", "--no-deps", "buildkit-worker-credentials-init"); err != nil {
				return fmt.Errorf("prepare worker BuildKit client credentials: %w", err)
			}
			if err := e.runCompose(ctx, plan, "run", "--rm", "--no-deps", "buildkit-server-credentials-init"); err != nil {
				return fmt.Errorf("prepare BuildKit server credentials: %w", err)
			}
		}
		services := make([]string, 0, 2)
		if !plan.ExternalDatabase {
			services = append(services, "postgres")
		}
		if !plan.ExternalRedis {
			services = append(services, "redis")
		}
		if len(services) == 0 {
			return nil
		}
		return e.runCompose(ctx, plan, append([]string{"up", "-d"}, services...)...)
	case StepMigration:
		if plan.Existing && !plan.Setup {
			installedVersion := strings.TrimSpace(plan.InstalledVersion)
			if installedVersion == "" {
				return errors.New("existing platform migration requires the recorded installed version")
			}
			if installedVersion != strings.TrimSpace(plan.Version) {
				if err := e.recordUpgradeSchemaFingerprint(ctx, plan); err != nil {
					return err
				}
			}
		}
		if plan.ExternalDatabase {
			return e.runCompose(ctx, plan, "run", "--rm", "--no-deps", "migrate")
		}
		return e.runCompose(ctx, plan, "up", "migrate")
	case StepServices:
		services := []string{"api", "worker", "console", "proxy", "traefik"}
		if plan.Setup {
			services = []string{"setup", "setup-console", "setup-proxy"}
		} else {
			if plan.Existing {
				// Compose does not notice file content changes inside named
				// credential volumes. Recreate both consumers after the
				// one-shot initializers refresh them, including repair after an
				// interrupted rotation.
				if err := e.runCompose(ctx, plan, "up", "-d", "--no-deps", "--force-recreate", "buildkit", "worker"); err != nil {
					return fmt.Errorf("restart BuildKit and worker after refreshing mutual TLS credentials: %w", err)
				}
			}
			services = append(services, "buildkit")
		}
		if plan.Cloudflare {
			services = append(services, "cloudflared")
		}
		args := []string{"up", "-d"}
		if plan.ExternalDatabase || plan.ExternalRedis {
			args = append(args, "--no-deps")
		}
		args = append(args, services...)
		return e.runCompose(ctx, plan, args...)
	case StepVerify:
		return e.Wait(ctx, plan)
	default:
		return fmt.Errorf("unknown install step %d", step)
	}
}
func (e *Engine) Prepare(ctx context.Context, plan Plan) error {
	// Keep this exported preparation entry point subject to the same durable
	// transaction boundary as the lifecycle step. No caller can finalize a
	// managed-asset journal without successful `docker compose config --quiet`.
	return e.RunStep(ctx, plan, StepConfiguration)
}
