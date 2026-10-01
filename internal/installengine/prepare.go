package installengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type preparedInstallation struct {
	layout              Layout
	assets              *managedAssetMigration
	originalEnv         []byte
	originalEnvExists   bool
	newEnv              []byte
	originalVersion     []byte
	originalVersionSet  bool
	originalVersionMode os.FileMode
}

// prepareInstallation downloads and validates the full managed asset set but
// does not publish it until commit. RunStep uses the returned transaction to
// roll the files back when docker compose config validation fails.
func (e *Engine) prepareInstallation(ctx context.Context, plan Plan) (*preparedInstallation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if plan.Layout.Root == "" || plan.Layout.EnvFile == "" || plan.Layout.ComposeFile == "" {
		return nil, errors.New("installation layout is incomplete")
	}
	if err := ValidateReleaseVersion(strings.TrimSpace(plan.Version)); err != nil {
		return nil, err
	}
	if err := validateInstallRootBeforeCreation(plan.Layout.Root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(plan.Layout.Root, 0o700); err != nil {
		return nil, fmt.Errorf("create installation directory: %w", err)
	}
	if err := os.Chmod(plan.Layout.Root, 0o700); err != nil {
		return nil, fmt.Errorf("protect installation directory: %w", err)
	}
	// Setup state is shared by the host CLI and the root setup container. The
	// setgid directory preserves the host operator's group on files atomically
	// replaced by the container, while the state payload remains group-private.
	stateDirectoryMode := os.FileMode(0o770) | os.ModeSetgid
	if err := os.MkdirAll(plan.Layout.StateDir, stateDirectoryMode); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(plan.Layout.StateDir, stateDirectoryMode); err != nil {
		return nil, fmt.Errorf("protect state directory: %w", err)
	}
	prepared := &preparedInstallation{
		layout: plan.Layout,
	}
	var originalValues map[string]string
	generatedConfig := false
	if contents, err := os.ReadFile(plan.Layout.VersionFile); err == nil {
		prepared.originalVersion = contents
		prepared.originalVersionSet = true
		if info, statErr := os.Stat(plan.Layout.VersionFile); statErr == nil {
			prepared.originalVersionMode = info.Mode().Perm()
		} else {
			return nil, fmt.Errorf("inspect existing version: %w", statErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read existing version: %w", err)
	}
	if plan.Existing {
		if !FileIsPrivate(plan.Layout.EnvFile) {
			return nil, errors.New("existing configuration permissions are too broad; expected mode 0600")
		}
		contents, err := os.ReadFile(plan.Layout.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("read existing configuration: %w", err)
		}
		values, err := ReadEnvFile(plan.Layout.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("parse existing configuration: %w", err)
		}
		originalValues = make(map[string]string, len(values))
		for key, value := range values {
			originalValues[key] = value
		}
		installedVersion := strings.TrimSpace(plan.InstalledVersion)
		recordedVersion := strings.TrimSpace(string(prepared.originalVersion))
		if prepared.originalVersionSet {
			if err := ValidateReleaseVersion(recordedVersion); err != nil {
				return nil, fmt.Errorf("existing VERSION is invalid: %w", err)
			}
			if installedVersion != "" && installedVersion != recordedVersion {
				return nil, fmt.Errorf("installation version skew: plan reports %s but VERSION records %s", installedVersion, recordedVersion)
			}
			installedVersion = recordedVersion
		}
		if installedVersion == "" {
			installedVersion = strings.TrimSpace(plan.Version)
		}
		if err := rejectReleaseDowngrade(plan.Version, installedVersion); err != nil {
			return nil, err
		}
		migrated, err := MigrateReleaseConfig(values, plan.Version, installedVersion)
		if err != nil {
			return nil, fmt.Errorf("prepare existing configuration migration: %w", err)
		}
		prepared.originalEnv = contents
		prepared.originalEnvExists = true
		prepared.newEnv = []byte(migrated)
	} else {
		contents := plan.ConfigContents
		if strings.TrimSpace(contents) == "" {
			generatedConfig = true
			var err error
			appArmorProfile, profileErr := DetectBuildKitAppArmorProfile()
			if profileErr != nil {
				return nil, fmt.Errorf("detect BuildKit AppArmor requirements: %w", profileErr)
			}
			contents, err = GenerateConfig(ConfigOptions{
				Version: plan.Version, PublicURL: plan.PublicURL, GitHubAppClientID: plan.GitHubAppClientID,
				DockerGID: plan.DockerGID, Setup: plan.Setup, InstallRoot: plan.Layout.Root,
				IngressNetworkName: plan.IngressNetworkName, AppsBuildKitAppArmorProfile: appArmorProfile,
			})
			if err != nil {
				return nil, fmt.Errorf("generate configuration: %w", err)
			}
		}
		prepared.newEnv = []byte(contents)
	}
	values, err := ParseEnvContents(string(prepared.newEnv))
	if err != nil {
		return nil, fmt.Errorf("parse prepared configuration: %w", err)
	}
	// Compose uses this generated absolute root to mask the installation-private
	// directory from telemetry-host's otherwise read-only host filesystem view.
	// Keep it current on both fresh installs and upgrades from older configs.
	values["STEALTH_INSTALL_ROOT"] = plan.Layout.Root
	prepared.newEnv = []byte(FormatEnvFile(values))
	if !plan.Setup {
		if generatedConfig {
			// GenerateConfig writes complete defaults so it can also be used as a
			// standalone config generator. For a fresh install, however, those
			// values are installer defaults rather than an operator's explicit
			// addressing choice; let collision-aware selection replace them.
			for _, key := range ingressNetworkEnvKeys {
				delete(values, key)
			}
		}
		if originalValues == nil {
			originalValues = values
		}
		ingress, err := e.resolveIngressNetworkConfig(ctx, plan, values, originalValues)
		if err != nil {
			return nil, fmt.Errorf("prepare ingress network configuration: %w", err)
		}
		if previous, previousErr := ingressNetworkConfigFromValues(originalValues); previousErr == nil && previous.trustedProxyCIDR() != ingress.trustedProxyCIDR() {
			// The previous peer was installer-derived whenever automatic subnet
			// selection or legacy-network adoption changes it. Remove only that
			// exact generated entry; all other operator proxy CIDRs survive.
			values["TRUSTED_PROXY_CIDRS"] = removeTrustedProxyCIDR(values["TRUSTED_PROXY_CIDRS"], previous.trustedProxyCIDR())
		}
		setIngressNetworkValues(values, ingress)
		trustedProxy := ensureTraefikTrustedProxyCIDR(values["TRUSTED_PROXY_CIDRS"], values["STEALTH_NETWORK_SUBNET"], ingress.trustedProxyCIDR())
		if trustedProxy != strings.TrimSpace(values["TRUSTED_PROXY_CIDRS"]) {
			values["TRUSTED_PROXY_CIDRS"] = trustedProxy
		}
		prepared.newEnv = []byte(FormatEnvFile(values))
	}
	assetPlan := plan
	assetPlan.ConfigContents = string(prepared.newEnv)
	assets, err := e.stageManagedAssets(ctx, assetPlan)
	if err != nil {
		return nil, err
	}
	prepared.assets = assets
	return prepared, nil
}
func (e *Engine) validateStagedCompose(ctx context.Context, plan Plan, prepared *preparedInstallation) error {
	if prepared == nil || prepared.assets == nil {
		return errors.New("prepared installation is incomplete")
	}
	composePath := "compose.production.yaml"
	if plan.Setup {
		composePath = "compose.setup.yaml"
	}
	stagedCompose, ok := prepared.assets.stagedAssetPath(composePath)
	if !ok {
		return fmt.Errorf("staged %s asset is missing", composePath)
	}
	stageEnvFile := filepath.Join(prepared.assets.stageDir, "config.env")
	if err := WritePrivateFile(stageEnvFile, string(prepared.newEnv)); err != nil {
		return fmt.Errorf("stage target configuration for Compose validation: %w", err)
	}
	if err := e.runComposeFileWithEnv(ctx, plan, stagedCompose, plan.Layout.Root, stageEnvFile, "config", "--quiet"); err != nil {
		return fmt.Errorf("validate staged Compose configuration: %w", err)
	}
	return nil
}
func validateInstallRootBeforeCreation(root string) error {
	clean := filepath.Clean(strings.TrimSpace(root))
	if clean == "" || clean == string(filepath.Separator) {
		return errors.New("refusing filesystem root as installation directory")
	}
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect installation directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("installation directory %q is not a normal directory", clean)
	}
	return nil
}
func (p *preparedInstallation) commit(plan Plan) error {
	if p == nil || p.assets == nil {
		return errors.New("prepared installation is incomplete")
	}
	if err := p.assets.commit(plan, p.originalEnv, p.originalEnvExists, p.originalVersion, p.originalVersionSet, p.originalVersionMode); err != nil {
		return err
	}
	if err := WritePrivateFile(p.layout.EnvFile, string(p.newEnv)); err != nil {
		return p.failCommit(fmt.Errorf("write configuration: %w", err))
	}
	if err := p.assets.transition(migrationPhaseConfigActivated); err != nil {
		return p.failCommit(err)
	}
	if err := WriteAtomic(p.layout.VersionFile, []byte(strings.TrimSpace(plan.Version)+"\n"), 0o644); err != nil {
		return p.failCommit(fmt.Errorf("write version file: %w", err))
	}
	if err := p.assets.transition(migrationPhaseVersionActivated); err != nil {
		return p.failCommit(err)
	}
	return nil
}

func (p *preparedInstallation) failCommit(cause error) error {
	if errors.Is(cause, errMigrationProcessInterrupted) {
		return cause
	}
	return errors.Join(cause, p.rollback())
}

func (p *preparedInstallation) rollback() error {
	if p == nil {
		return nil
	}
	if p.assets == nil {
		return nil
	}
	return p.assets.rollback()
}

func (p *preparedInstallation) composeValidated() error {
	if p == nil || p.assets == nil {
		return errors.New("prepared installation is incomplete")
	}
	return p.assets.transition(migrationPhaseComposeValidated)
}

func (p *preparedInstallation) finalize() error {
	if p == nil || p.assets == nil {
		return nil
	}
	return p.assets.finalize()
}
