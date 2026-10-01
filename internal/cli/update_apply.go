package cli

import (
	"context"
	"fmt"
	"os"
)

func (a *App) performUpdate(ctx context.Context, release githubRelease, asset string) error {
	return a.performUpdateWithTargetMigration(ctx, release, asset)
}

// performUpdateWithTargetMigration executes a target-owned migration after
// validating the downloaded release binary and before replacing the installed
// executable. The currently running binary deliberately does not interpret the
// target manifest: future releases can evolve their managed assets safely.
func (a *App) performUpdateWithTargetMigration(ctx context.Context, release githubRelease, asset string) error {
	return a.performUpdateWithTargetMigrationHook(ctx, release, asset, func(binary string) error {
		if err := a.invokeTargetMigration(ctx, binary, release.TagName); err != nil {
			return fmt.Errorf("run target release migration: %w", err)
		}
		return nil
	})
}

func (a *App) performUpdateWithTargetMigrationHook(ctx context.Context, release githubRelease, asset string, beforeReplace func(string) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update canceled: %w", err)
	}
	if !hasReleaseAsset(release, asset) {
		return fmt.Errorf("latest release %s does not contain %s", release.TagName, asset)
	}
	if !hasReleaseAsset(release, "checksums.txt") {
		return fmt.Errorf("latest release %s does not contain checksums.txt", release.TagName)
	}

	archiveURL, err := appendReleasePath(a.releaseDownloadBase, release.TagName, asset)
	if err != nil {
		return fmt.Errorf("invalid release download endpoint: %w", err)
	}
	checksumsURL, err := appendReleasePath(a.releaseDownloadBase, release.TagName, "checksums.txt")
	if err != nil {
		return fmt.Errorf("invalid checksum download endpoint: %w", err)
	}
	archive, err := a.fetchUpdateAsset(ctx, archiveURL, maxUpdateArchiveSize)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	checksums, err := a.fetchUpdateAsset(ctx, checksumsURL, 128<<10)
	if err != nil {
		return fmt.Errorf("download checksums.txt: %w", err)
	}
	expected, err := checksumForAsset(string(checksums), asset)
	if err != nil {
		return err
	}
	if err := verifySHA256(archive, expected); err != nil {
		return err
	}

	temporaryDir, temporaryBinary, err := extractUpdateBinary(archive)
	if err != nil {
		return fmt.Errorf("extract release archive: %w", err)
	}
	defer os.RemoveAll(temporaryDir)
	if err := a.validateDownloadedBinary(ctx, temporaryBinary, release.TagName); err != nil {
		return err
	}
	target, err := a.currentExecutablePath()
	if err != nil {
		return err
	}
	if beforeReplace != nil {
		if err := beforeReplace(temporaryBinary); err != nil {
			return fmt.Errorf("migrate existing installation: %w", err)
		}
	}
	if err := a.replaceExecutable(ctx, temporaryBinary, target); err != nil {
		return &platformMigratedUpdateError{
			targetVersion: release.TagName,
			err:           fmt.Errorf("target platform migration completed, but replacing the CLI executable failed: %w; the installed stack is at %s and a later `stealth update` will reconcile the CLI", err, release.TagName),
		}
	}
	return nil
}

func (a *App) invokeTargetMigration(ctx context.Context, binaryPath, targetVersion string) error {
	if a.runTargetMigration != nil {
		return a.runTargetMigration(ctx, binaryPath, targetVersion)
	}
	runner := a.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	return runner.Run(ctx, "", a.out, a.errOut, binaryPath, "internal", "migrate-installation", "--target-version", targetVersion)
}

// migrateInstalledRelease runs the trusted host-side install engine for an
// existing installation. A self-update is still possible on a host without an
// installation, but an installation that is present is never left on a
// pre-release topology merely because the CLI binary changed.
func (a *App) migrateInstalledRelease(ctx context.Context, targetVersion string) (bool, error) {
	layout, err := a.layout()
	if err != nil {
		return false, fmt.Errorf("inspect installation layout: %w", err)
	}
	if !installationExists(layout) {
		return false, nil
	}
	// A state directory alone can be left by an interrupted first install and
	// does not contain enough release metadata to select a target asset set.
	// Let the CLI self-update in that case; the explicit repair flow remains the
	// recovery owner once config.env exists.
	if !regularFile(layout.EnvFile) {
		return false, nil
	}
	plan, err := a.loadExistingPlan(layout)
	if err != nil {
		return false, fmt.Errorf("load existing installation for migration: %w", err)
	}
	installedVersion := plan.Version
	plan.Version = targetVersion
	plan.InstalledVersion = installedVersion
	plan.Existing = true
	if err := a.installEngine().Install(ctx, *plan, nil); err != nil {
		return false, err
	}
	return true, nil
}

// runInternal exposes one deliberately narrow host-only handoff used by a
// verified downloaded release during self-update. It accepts neither scripts
// nor URLs: the target binary uses its compiled-in managed-asset manifest and
// trusted release base, then takes the ordinary install.lock itself.
