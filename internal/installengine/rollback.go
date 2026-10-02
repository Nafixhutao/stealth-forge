package installengine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errRollbackSchemaChanged = errors.New("database migration ledger has advanced")

// Rollback restores the last managed platform release only when the current
// database migration ledger exactly matches the snapshot taken before that
// release's migration. It never runs down migrations or changes database data.
func (e *Engine) Rollback(ctx context.Context, plan Plan) error {
	if e == nil {
		return errors.New("install engine is nil")
	}
	lock, err := acquireLock(plan.Layout.StateDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	return e.RollbackLocked(ctx, plan)
}

// RollbackLocked is the rollback counterpart to InstallLocked. The caller
// must own the installation lock for plan.Layout.StateDir.
func (e *Engine) RollbackLocked(ctx context.Context, plan Plan) error {
	if e == nil {
		return errors.New("install engine is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Layout.Root == "" || plan.Layout.EnvFile == "" || plan.Layout.ComposeFile == "" {
		return errors.New("installation layout is incomplete")
	}
	if !plan.Existing || plan.Setup {
		return errors.New("platform rollback requires an existing production installation")
	}
	specs := e.managedAssetSpecs(plan)
	allowedPaths := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if !validManagedAssetPath(spec.Path) {
			return fmt.Errorf("managed asset path %q is invalid", spec.Path)
		}
		allowedPaths[spec.Path] = struct{}{}
	}
	if err := recoverInterruptedManagedAssetMigration(plan.Layout, allowedPaths); err != nil {
		return fmt.Errorf("recover interrupted managed asset operation before rollback: %w", err)
	}
	currentVersion, err := readManagedVersion(plan.Layout.VersionFile)
	if err != nil {
		return fmt.Errorf("read installed platform version: %w", err)
	}

	pending, err := readPlatformRollbackRecord(plan.Layout)
	if err != nil {
		return err
	}
	var rollback platformRollbackRecord
	if pending != nil {
		rollback = *pending
		if currentVersion == rollback.ToVersion {
			// The file transaction was committed before the process stopped.
			// Re-run image activation and health checks from the durable intent.
			return e.finishPlatformRollback(ctx, plan, rollback)
		}
		if currentVersion != rollback.FromVersion {
			return fmt.Errorf(
				"interrupted platform rollback expects %s or %s, but VERSION records %s; inspect the installation before retrying",
				rollback.FromVersion,
				rollback.ToVersion,
				currentVersion,
			)
		}
		if err := e.verifyRollbackEligibility(ctx, plan, currentVersion, rollback.ToVersion, rollback.SchemaFingerprint, allowedPaths); err != nil {
			schemaChanged := errors.Is(err, errRollbackSchemaChanged)
			outcome := "refused_after_interruption"
			if schemaChanged {
				outcome = "refused_schema_change"
			}
			return errors.Join(
				err,
				appendRollbackAudit(plan.Layout, outcome, currentVersion, rollback.ToVersion, schemaChanged),
			)
		}
	} else {
		metadata, metadataErr := readManagedReleaseMetadata(plan.Layout, currentVersion, allowedPaths)
		if metadataErr != nil {
			return errors.Join(
				fmt.Errorf("platform rollback is unavailable: %w; use a verified backup restore when the older release is required", metadataErr),
				appendRollbackAudit(plan.Layout, "refused_metadata", currentVersion, "", false),
			)
		}
		comparison, compareErr := compareReleaseVersionOrder(metadata.PreviousVersion, currentVersion)
		if compareErr != nil || comparison >= 0 {
			return errors.Join(
				fmt.Errorf("platform rollback requires a distinct older release; current version is %s and the saved release is %s", currentVersion, metadata.PreviousVersion),
				appendRollbackAudit(plan.Layout, "refused_not_older", currentVersion, metadata.PreviousVersion, false),
			)
		}
		if metadata.SchemaFingerprint == "" {
			return errors.Join(
				fmt.Errorf("platform rollback to %s is unsafe because its pre-migration schema snapshot is missing; restore PostgreSQL and object storage from verified backups if needed", metadata.PreviousVersion),
				appendRollbackAudit(plan.Layout, "refused_missing_schema_snapshot", currentVersion, metadata.PreviousVersion, false),
			)
		}
		if err := e.verifyRollbackEligibility(ctx, plan, currentVersion, metadata.PreviousVersion, metadata.SchemaFingerprint, allowedPaths); err != nil {
			schemaChanged := errors.Is(err, errRollbackSchemaChanged)
			outcome := "refused_eligibility"
			if schemaChanged {
				outcome = "refused_schema_change"
			}
			return errors.Join(err, appendRollbackAudit(plan.Layout, outcome, currentVersion, metadata.PreviousVersion, schemaChanged))
		}
		rollback = platformRollbackRecord{
			FormatVersion: 1, FromVersion: currentVersion, ToVersion: metadata.PreviousVersion,
			SchemaFingerprint: metadata.SchemaFingerprint, CLIVersion: strings.TrimSpace(plan.CLIVersion),
			OperatorUID: os.Geteuid(), StartedAt: time.Now().UTC(),
		}
		if err := appendRollbackAudit(plan.Layout, "started", rollback.FromVersion, rollback.ToVersion, false); err != nil {
			return fmt.Errorf("record platform rollback attempt: %w", err)
		}
		if err := writePlatformRollbackRecord(plan.Layout, rollback); err != nil {
			return fmt.Errorf("record platform rollback recovery state: %w", err)
		}
	}

	if err := e.applyPlatformRollback(ctx, plan, rollback, allowedPaths); err != nil {
		_ = appendRollbackAudit(plan.Layout, "retry_required", rollback.FromVersion, rollback.ToVersion, false)
		return err
	}
	return e.finishPlatformRollback(ctx, plan, rollback)
}

func (e *Engine) verifyRollbackEligibility(
	ctx context.Context,
	plan Plan,
	currentVersion, targetVersion, expectedFingerprint string,
	allowedPaths map[string]struct{},
) error {
	metadata, err := readManagedReleaseMetadata(plan.Layout, currentVersion, allowedPaths)
	if err != nil {
		return fmt.Errorf("platform rollback is unsafe: %w; use the backup/restore runbook", err)
	}
	if metadata.PreviousVersion != targetVersion || metadata.TargetVersion != currentVersion {
		return errors.New("platform rollback metadata does not match the active and previous releases")
	}
	if metadata.SchemaFingerprint == "" || !schemaFingerprintPattern.MatchString(metadata.SchemaFingerprint) ||
		metadata.SchemaFingerprint != expectedFingerprint {
		return errors.New("platform rollback metadata has no valid matching pre-migration schema snapshot")
	}
	currentFingerprint, err := e.querySchemaFingerprint(ctx, plan, plan.ExternalDatabase)
	if err != nil {
		return fmt.Errorf("cannot determine database schema compatibility for rollback: %w", err)
	}
	if currentFingerprint != expectedFingerprint {
		return fmt.Errorf(
			"%w: refusing platform rollback from %s to %s; restore PostgreSQL and object storage from verified backups before starting the older release",
			errRollbackSchemaChanged,
			currentVersion,
			targetVersion,
		)
	}
	return nil
}

func (e *Engine) applyPlatformRollback(
	ctx context.Context,
	currentPlan Plan,
	rollback platformRollbackRecord,
	allowedPaths map[string]struct{},
) error {
	targetPlan := currentPlan
	targetPlan.Version = rollback.ToVersion
	targetPlan.InstalledVersion = rollback.FromVersion
	targetPlan.Existing = true
	prepared, err := e.prepareRollbackInstallation(ctx, targetPlan, allowedPaths)
	if err != nil {
		return err
	}
	if err := e.validateStagedCompose(ctx, targetPlan, prepared); err != nil {
		_ = removeAllAndSync(prepared.assets.stageDir)
		return fmt.Errorf("previous release Compose validation failed: %w", err)
	}
	composePath, ok := prepared.assets.stagedAssetPath("compose.production.yaml")
	if !ok {
		_ = removeAllAndSync(prepared.assets.stageDir)
		return errors.New("previous production Compose asset is missing")
	}
	stageEnvFile := filepath.Join(prepared.assets.stageDir, "config.env")
	if err := WritePrivateFile(stageEnvFile, string(prepared.newEnv)); err != nil {
		_ = removeAllAndSync(prepared.assets.stageDir)
		return fmt.Errorf("stage previous configuration for state initialization: %w", err)
	}
	if err := e.validateRollbackServiceTopology(ctx, targetPlan, composePath, stageEnvFile); err != nil {
		_ = removeAllAndSync(prepared.assets.stageDir)
		return err
	}
	if err := e.runComposeFileWithEnv(ctx, targetPlan, composePath, targetPlan.Layout.Root, stageEnvFile,
		"run", "--rm", "--no-deps", "-e", fmt.Sprintf("STEALTH_TRAEFIK_HOST_UID=%d", os.Geteuid()), "traefik-state-init"); err != nil {
		_ = removeAllAndSync(prepared.assets.stageDir)
		return fmt.Errorf("prepare previous Traefik state: %w", err)
	}
	if err := prepared.commit(targetPlan); err != nil {
		return fmt.Errorf("activate previous platform release: %w", err)
	}
	metadata, err := readManagedReleaseMetadataFromDirectory(
		prepared.assets.backupDir,
		rollback.ToVersion,
		allowedPaths,
	)
	if err != nil {
		return fmt.Errorf("verify saved current release metadata: %w", err)
	}
	metadata.SchemaFingerprint = rollback.SchemaFingerprint
	if err := writeManagedReleaseMetadata(prepared.assets.backupDir, metadata); err != nil {
		return fmt.Errorf("record schema snapshot for retry rollback: %w", err)
	}
	if err := e.runCompose(ctx, targetPlan, "config", "--quiet"); err != nil {
		return errors.Join(fmt.Errorf("restored platform Compose validation failed: %w", err), prepared.rollback())
	}
	if err := prepared.composeValidated(); err != nil {
		if errors.Is(err, errMigrationProcessInterrupted) {
			return err
		}
		return errors.Join(err, prepared.rollback())
	}
	if err := prepared.finalize(); err != nil {
		return fmt.Errorf("finalize platform rollback files: %w", err)
	}
	if err := e.ensureBuildKitAppArmorProfile(ctx, targetPlan); err != nil {
		return fmt.Errorf("restore previous BuildKit AppArmor profile: %w", err)
	}
	return nil
}

func (e *Engine) prepareRollbackInstallation(
	ctx context.Context,
	plan Plan,
	allowedPaths map[string]struct{},
) (*preparedInstallation, error) {
	previousDir := filepath.Join(plan.Layout.StateDir, "managed-assets.previous")
	metadata, err := readManagedReleaseMetadata(plan.Layout, plan.InstalledVersion, allowedPaths)
	if err != nil {
		return nil, fmt.Errorf("read previous release metadata: %w", err)
	}
	if metadata.PreviousVersion != plan.Version || metadata.TargetVersion != plan.InstalledVersion {
		return nil, errors.New("previous release metadata does not match requested rollback")
	}
	currentEnv, err := os.ReadFile(plan.Layout.EnvFile)
	if err != nil {
		return nil, fmt.Errorf("read active configuration: %w", err)
	}
	values, err := ParseEnvContents(string(currentEnv))
	if err != nil {
		return nil, fmt.Errorf("parse active configuration: %w", err)
	}
	newEnv, err := migrateReleaseConfig(values, plan.Version, plan.InstalledVersion)
	if err != nil {
		return nil, fmt.Errorf("retarget configuration while preserving operator values: %w", err)
	}
	originalVersion, err := os.ReadFile(plan.Layout.VersionFile)
	if err != nil {
		return nil, fmt.Errorf("read active platform version: %w", err)
	}
	versionInfo, err := os.Lstat(plan.Layout.VersionFile)
	if err != nil || !versionInfo.Mode().IsRegular() || versionInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(errors.New("active VERSION is not a regular file"), err)
	}
	stageDir, err := os.MkdirTemp(plan.Layout.StateDir, ".stealth-managed-assets-")
	if err != nil {
		return nil, fmt.Errorf("create rollback staging directory: %w", err)
	}
	if err := os.Chmod(stageDir, 0o700); err != nil {
		_ = os.RemoveAll(stageDir)
		return nil, fmt.Errorf("protect rollback staging directory: %w", err)
	}
	byPath := make(map[string]managedAssetRecoveryEntry, len(metadata.Assets))
	for _, entry := range metadata.Assets {
		byPath[entry.RelativePath] = entry
	}
	migration := &managedAssetMigration{
		layout:       plan.Layout,
		stageDir:     stageDir,
		allowedPaths: allowedPaths,
		hook:         e.migrationHook,
	}
	for _, spec := range e.managedAssetSpecs(plan) {
		entry, found := byPath[spec.Path]
		if !found {
			_ = removeAllAndSync(stageDir)
			return nil, fmt.Errorf("previous release metadata is missing managed asset %q", spec.Path)
		}
		asset := stagedManagedAsset{
			spec:       spec,
			targetPath: filepath.Join(plan.Layout.Root, filepath.FromSlash(spec.Path)),
			remove:     !entry.Existed,
		}
		if entry.Existed {
			source := filepath.Join(previousDir, filepath.FromSlash(spec.Path))
			contents, readErr := os.ReadFile(source)
			if readErr != nil {
				_ = removeAllAndSync(stageDir)
				return nil, fmt.Errorf("read previous managed asset %q: %w", spec.Path, readErr)
			}
			digest := sha256.Sum256(contents)
			if hex.EncodeToString(digest[:]) != entry.SHA256 {
				_ = removeAllAndSync(stageDir)
				return nil, fmt.Errorf("previous managed asset %q failed its recorded checksum", spec.Path)
			}
			asset.stagePath = filepath.Join(stageDir, filepath.FromSlash(spec.Path))
			if err := WriteAtomic(asset.stagePath, contents, 0o644); err != nil {
				_ = removeAllAndSync(stageDir)
				return nil, fmt.Errorf("stage previous managed asset %q: %w", spec.Path, err)
			}
		}
		migration.assets = append(migration.assets, asset)
	}
	if _, ok := migration.stagedAssetPath("compose.production.yaml"); !ok {
		_ = removeAllAndSync(stageDir)
		return nil, errors.New("previous release does not contain production Compose")
	}
	return &preparedInstallation{
		layout: plan.Layout, assets: migration, originalEnv: currentEnv, originalEnvExists: true,
		newEnv: []byte(newEnv), originalVersion: originalVersion, originalVersionSet: true,
		originalVersionMode: versionInfo.Mode().Perm(),
	}, nil
}

func (e *Engine) finishPlatformRollback(ctx context.Context, plan Plan, rollback platformRollbackRecord) error {
	activeVersion, err := readManagedVersion(plan.Layout.VersionFile)
	if err != nil {
		return fmt.Errorf("read active platform version while resuming rollback: %w", err)
	}
	if activeVersion != rollback.ToVersion {
		return fmt.Errorf(
			"platform rollback is not active: VERSION records %s, expected %s",
			activeVersion,
			rollback.ToVersion,
		)
	}
	targetPlan := plan
	targetPlan.Version = rollback.ToVersion
	targetPlan.InstalledVersion = rollback.ToVersion
	targetPlan.Existing = true
	if targetPlan.CLIVersion == "" {
		targetPlan.CLIVersion = rollback.CLIVersion
	}
	if err := e.runCompose(ctx, targetPlan, "pull"); err != nil {
		return fmt.Errorf("pull previous release images: %w", err)
	}
	if err := e.runRollbackServices(ctx, targetPlan); err != nil {
		return err
	}
	if err := e.Wait(ctx, targetPlan); err != nil {
		return fmt.Errorf("previous release health verification failed: %w", err)
	}
	if err := e.verifyRollbackAPIVersion(ctx, targetPlan); err != nil {
		return err
	}
	if err := e.verifyRollbackServices(ctx, targetPlan); err != nil {
		return err
	}
	if err := writeCurrentPlatformRollbackState(plan.Layout, rollback); err != nil {
		return fmt.Errorf("record active platform rollback state: %w", err)
	}
	if err := appendRollbackAudit(plan.Layout, "completed", rollback.FromVersion, rollback.ToVersion, false); err != nil {
		return fmt.Errorf("record completed platform rollback: %w", err)
	}
	if err := removeAndSync(filepath.Join(plan.Layout.StateDir, platformRollbackPending)); err != nil {
		return fmt.Errorf("clear completed platform rollback recovery state: %w", err)
	}
	return nil
}

func (e *Engine) verifyRollbackAPIVersion(ctx context.Context, plan Plan) error {
	apiURL := strings.TrimRight(strings.TrimSpace(plan.InternalAPIURL), "/")
	if apiURL == "" {
		values, err := ReadEnvFile(plan.Layout.EnvFile)
		if err != nil {
			return fmt.Errorf("read API endpoint configuration after rollback: %w", err)
		}
		apiURL = "http://127.0.0.1:" + PortOrDefault(values["API_HOST_PORT"], "18080")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/version", nil)
	if err != nil {
		return fmt.Errorf("create API version request after rollback: %w", err)
	}
	client := e.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("read API version after rollback: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("API version endpoint returned HTTP %d after rollback", response.StatusCode)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&payload); err != nil {
		return fmt.Errorf("decode API version after rollback: %w", err)
	}
	if strings.TrimSpace(payload.Version) != strings.TrimSpace(plan.Version) {
		return fmt.Errorf(
			"API reports version %q after rollback, expected %s",
			strings.TrimSpace(payload.Version),
			plan.Version,
		)
	}
	return nil
}

func (e *Engine) runRollbackServices(ctx context.Context, plan Plan) error {
	services, err := e.composeServices(ctx, plan, plan.Layout.ComposeFile, plan.Layout.EnvFile)
	if err != nil {
		return fmt.Errorf("inspect previous release service topology: %w", err)
	}
	if err := checkRequiredRollbackServices(services, plan); err != nil {
		return err
	}
	dependencies := make([]string, 0, 3)
	if !plan.ExternalDatabase {
		dependencies = append(dependencies, "postgres")
	}
	if !plan.ExternalRedis {
		dependencies = append(dependencies, "redis")
	}
	dependencies = append(dependencies, "clickhouse")
	if err := e.runCompose(ctx, plan, append([]string{"up", "-d"}, dependencies...)...); err != nil {
		return fmt.Errorf("start rollback dependencies: %w", err)
	}
	for _, initializer := range []string{
		"otelcol-state-init", "telemetry-docker-logs-state-init", "cloudflare-setup-state-init",
		"cloudflare-state-init", "buildkit-worker-credentials-init", "buildkit-server-credentials-init",
	} {
		if services[initializer] {
			if err := e.runCompose(ctx, plan, "run", "--rm", "--no-deps", initializer); err != nil {
				return fmt.Errorf("prepare previous release state with %s: %w", initializer, err)
			}
		}
	}
	if services["traefik-state-init"] {
		if err := e.runTraefikStateInit(ctx, plan, plan.Layout.ComposeFile, ""); err != nil {
			return fmt.Errorf("prepare previous Traefik state: %w", err)
		}
	}
	longRunning := []string{
		"api", "worker", "buildkit", "console", "proxy", "traefik", "otel-collector",
		"telemetry-host", "telemetry-docker-logs", "telemetry-docker-proxy", "telemetry-docker",
	}
	if plan.Cloudflare {
		longRunning = append(longRunning, "cloudflared")
	}
	args := []string{"up", "-d", "--no-deps", "--force-recreate"}
	args = append(args, longRunning...)
	if err := e.runCompose(ctx, plan, args...); err != nil {
		return fmt.Errorf("start coherent previous release services: %w", err)
	}
	return nil
}

func (e *Engine) verifyRollbackServices(ctx context.Context, plan Plan) error {
	args := []string{"compose"}
	if plan.Cloudflare {
		args = append(args, "--profile", "cloudflare")
	}
	args = append(
		args,
		"--env-file",
		plan.Layout.EnvFile,
		"-f",
		plan.Layout.ComposeFile,
		"ps",
		"--all",
		"--format",
		"json",
	)
	output, err := e.runner.Output(ctx, plan.Layout.Root, "docker", args...)
	if err != nil {
		return fmt.Errorf("read previous release service status: %w", err)
	}
	statuses, err := parseRollbackServiceStatuses(output)
	if err != nil {
		return err
	}
	required := []string{
		"api",
		"worker",
		"buildkit",
		"console",
		"proxy",
		"traefik",
		"clickhouse",
		"otel-collector",
		"telemetry-host",
		"telemetry-docker-logs",
		"telemetry-docker-proxy",
		"telemetry-docker",
	}
	if !plan.ExternalDatabase {
		required = append(required, "postgres")
	}
	if !plan.ExternalRedis {
		required = append(required, "redis")
	}
	if plan.Cloudflare {
		required = append(required, "cloudflared")
	}
	for _, service := range required {
		status, ok := statuses[service]
		if !ok || !strings.EqualFold(status.State, "running") {
			return fmt.Errorf("previous release service %q is not running after rollback", service)
		}
		if status.Health != "" && !strings.EqualFold(status.Health, "healthy") {
			return fmt.Errorf("previous release service %q is %s after rollback", service, status.Health)
		}
	}
	return nil
}

type rollbackComposeStatus struct {
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
}

func parseRollbackServiceStatuses(contents []byte) (map[string]rollbackComposeStatus, error) {
	statuses := make(map[string]rollbackComposeStatus)
	trimmed := strings.TrimSpace(string(contents))
	if trimmed == "" {
		return statuses, errors.New("previous release returned no service status")
	}
	if strings.HasPrefix(trimmed, "[") {
		var entries []rollbackComposeStatus
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil, fmt.Errorf("parse previous release service status: %w", err)
		}
		for _, entry := range entries {
			if entry.Service != "" {
				statuses[entry.Service] = entry
			}
		}
		return statuses, nil
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		var entry rollbackComposeStatus
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("parse previous release service status: %w", err)
		}
		if entry.Service != "" {
			statuses[entry.Service] = entry
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read previous release service status: %w", err)
	}
	return statuses, nil
}

func (e *Engine) validateRollbackServiceTopology(ctx context.Context, plan Plan, composeFile, envFile string) error {
	services, err := e.composeServices(ctx, plan, composeFile, envFile)
	if err != nil {
		return fmt.Errorf("inspect previous release service topology: %w", err)
	}
	return checkRequiredRollbackServices(services, plan)
}

func checkRequiredRollbackServices(services map[string]bool, plan Plan) error {
	required := []string{
		"api",
		"worker",
		"buildkit",
		"console",
		"proxy",
		"traefik",
		"clickhouse",
		"otel-collector",
		"telemetry-host",
		"telemetry-docker-logs",
		"telemetry-docker-proxy",
		"telemetry-docker",
	}
	if !plan.ExternalDatabase {
		required = append(required, "postgres")
	}
	if !plan.ExternalRedis {
		required = append(required, "redis")
	}
	for _, service := range required {
		if !services[service] {
			return fmt.Errorf("refusing rollback because the previous release is missing required service %q", service)
		}
	}
	if plan.Cloudflare && !services["cloudflared"] {
		return errors.New(
			"refusing rollback because Cloudflare ingress is configured but absent from the previous release",
		)
	}
	return nil
}

func (e *Engine) composeServices(ctx context.Context, plan Plan, composeFile, envFile string) (map[string]bool, error) {
	args := []string{"compose"}
	if plan.Cloudflare {
		args = append(args, "--profile", "cloudflare")
	}
	args = append(
		args,
		"--project-directory",
		plan.Layout.Root,
		"--env-file",
		envFile,
		"-f",
		composeFile,
		"config",
		"--services",
	)
	output, err := e.runner.Output(ctx, plan.Layout.Root, "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker compose config --services: %w", err)
	}
	services := make(map[string]bool)
	for _, name := range strings.Fields(string(output)) {
		if name == "" || strings.ContainsAny(name, "\x00\r\n") {
			return nil, errors.New("previous release returned an invalid Compose service name")
		}
		services[name] = true
	}
	return services, nil
}
