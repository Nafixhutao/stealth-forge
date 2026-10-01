package installengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// recoverInterruptedManagedAssetMigration is deliberately rollback-first until
// Compose validation has been durably recorded. That gives a restarted CLI
// exactly two externally visible states: the complete old release, or a
// complete, Compose-validated new release whose final cleanup can be finished.
func recoverInterruptedManagedAssetMigration(layout Layout, allowedPaths map[string]struct{}) error {
	pendingPath := filepath.Join(layout.StateDir, managedAssetPendingFile)
	info, err := os.Lstat(pendingPath)
	if errors.Is(err, os.ErrNotExist) {
		return removeStaleManagedAssetTransactions(layout)
	}
	if err != nil {
		return fmt.Errorf("inspect managed asset recovery journal: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed asset recovery journal is not a regular file")
	}
	contents, err := os.ReadFile(pendingPath)
	if err != nil {
		return fmt.Errorf("read managed asset recovery journal: %w", err)
	}
	var manifest managedAssetRecoveryManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return fmt.Errorf("parse managed asset recovery journal: %w", err)
	}
	if err := validateManagedAssetRecoveryManifest(manifest, allowedPaths); err != nil {
		return err
	}
	if manifest.Phase == migrationPhaseComposeValidated || manifest.Phase == migrationPhaseFinalized {
		if err := finalizeRecoveredManagedAssetMigration(layout, manifest); err != nil {
			return err
		}
	} else if err := rollbackManagedAssetMigration(layout, manifest); err != nil {
		return err
	}
	return removeStaleManagedAssetTransactions(layout)
}
func validateManagedAssetRecoveryManifest(manifest managedAssetRecoveryManifest, allowedPaths map[string]struct{}) error {
	if strings.TrimSpace(manifest.TransactionID) == "" || strings.ContainsAny(manifest.TransactionID, `/\\`) {
		return errors.New("managed asset recovery journal contains an invalid transaction id")
	}
	if err := ValidateReleaseVersion(manifest.TargetVersion); err != nil {
		return fmt.Errorf("managed asset recovery journal contains an invalid target version: %w", err)
	}
	if manifest.OriginalVersion != "" {
		if err := ValidateReleaseVersion(manifest.OriginalVersion); err != nil {
			return fmt.Errorf("managed asset recovery journal contains an invalid original version: %w", err)
		}
	}
	if !validManagedTransactionDirectory(manifest.BackupDir, ".stealth-managed-backup-") || !validManagedTransactionDirectory(manifest.StageDir, ".stealth-managed-assets-") {
		return errors.New("managed asset recovery journal contains an invalid transaction path")
	}
	switch manifest.Phase {
	case migrationPhasePrepared, migrationPhaseBackedUp, migrationPhaseAssetsActivated, migrationPhaseConfigActivated, migrationPhaseVersionActivated, migrationPhaseComposeValidated, migrationPhaseFinalized:
	default:
		return fmt.Errorf("managed asset recovery journal contains an unknown phase %q", manifest.Phase)
	}
	seen := make(map[string]struct{}, len(manifest.Assets))
	for _, entry := range manifest.Assets {
		if !validManagedAssetPath(entry.RelativePath) {
			return fmt.Errorf("managed asset recovery journal contains an invalid path %q", entry.RelativePath)
		}
		if _, ok := allowedPaths[entry.RelativePath]; !ok {
			return fmt.Errorf("managed asset recovery journal contains a path unknown to this release %q", entry.RelativePath)
		}
		if _, ok := seen[entry.RelativePath]; ok {
			return fmt.Errorf("managed asset recovery journal contains duplicate path %q", entry.RelativePath)
		}
		seen[entry.RelativePath] = struct{}{}
	}
	for _, file := range []managedAssetRecoveryFile{manifest.Config, manifest.Version} {
		if file.Existed && (file.Backup == "" || filepath.Base(file.Backup) != file.Backup) {
			return errors.New("managed asset recovery journal contains an invalid state backup path")
		}
	}
	return nil
}
func validManagedTransactionDirectory(value, prefix string) bool {
	return filepath.Base(value) == value && strings.HasPrefix(value, prefix) && !strings.ContainsAny(value, `/\\`)
}

func validManagedAssetPath(relativePath string) bool {
	if relativePath == "" || filepath.ToSlash(filepath.Clean(relativePath)) != relativePath {
		return false
	}
	return !strings.HasPrefix(relativePath, "../") && !strings.Contains(relativePath, "//")
}
func rollbackManagedAssetMigration(layout Layout, manifest managedAssetRecoveryManifest) error {
	backupDir := filepath.Join(layout.StateDir, manifest.BackupDir)
	if err := ensurePrivateManagedDirectory(backupDir); err != nil {
		return fmt.Errorf("inspect interrupted managed asset recovery set: %w", err)
	}
	for index := len(manifest.Assets) - 1; index >= 0; index-- {
		entry := manifest.Assets[index]
		targetPath := filepath.Join(layout.Root, filepath.FromSlash(entry.RelativePath))
		if !entry.Existed {
			if err := removeManagedAssetTarget(targetPath); err != nil {
				return fmt.Errorf("remove partially activated managed asset %q: %w", entry.RelativePath, err)
			}
			continue
		}
		sourcePath := filepath.Join(backupDir, filepath.FromSlash(entry.RelativePath))
		if sourceInfo, err := os.Lstat(sourcePath); errors.Is(err, os.ErrNotExist) {
			// PREPARED is recorded before the first source rename. Until the
			// backup phase is durable, an untouched regular target is the old
			// authoritative file.
			if manifest.Phase != migrationPhasePrepared {
				return fmt.Errorf("recover previous managed asset %q: backup is missing", entry.RelativePath)
			}
			if err := ensureRegularManagedAsset(targetPath); err != nil {
				return fmt.Errorf("recover previous managed asset %q: %w", entry.RelativePath, err)
			}
			continue
		} else if err != nil {
			return fmt.Errorf("recover previous managed asset %q: %w", entry.RelativePath, err)
		} else if !sourceInfo.Mode().IsRegular() || sourceInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("recover previous managed asset %q: backup is not a regular file", entry.RelativePath)
		}
		if entry.SHA256 != "" {
			contents, err := os.ReadFile(sourcePath)
			if err != nil {
				return fmt.Errorf("read recovery backup for managed asset %q: %w", entry.RelativePath, err)
			}
			digest := sha256.Sum256(contents)
			if fmt.Sprintf("%x", digest) != entry.SHA256 {
				return fmt.Errorf("recover previous managed asset %q: backup checksum mismatch", entry.RelativePath)
			}
		}
		if err := removeManagedAssetTarget(targetPath); err != nil {
			return fmt.Errorf("clear interrupted managed asset %q: %w", entry.RelativePath, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			return fmt.Errorf("prepare managed asset recovery path %q: %w", entry.RelativePath, err)
		}
		if err := renameAndSync(sourcePath, targetPath); err != nil {
			return fmt.Errorf("restore managed asset %q: %w", entry.RelativePath, err)
		}
	}
	if err := restoreManagedStateFile(layout.EnvFile, backupDir, manifest.Config, 0o600); err != nil {
		return fmt.Errorf("restore config.env: %w", err)
	}
	if err := restoreManagedStateFile(layout.VersionFile, backupDir, manifest.Version, 0o644); err != nil {
		return fmt.Errorf("restore VERSION: %w", err)
	}
	if err := removeAllAndSync(backupDir); err != nil {
		return fmt.Errorf("remove recovered managed asset set: %w", err)
	}
	if err := removeAllAndSync(filepath.Join(layout.StateDir, manifest.StageDir)); err != nil {
		return fmt.Errorf("remove recovered managed asset staging set: %w", err)
	}
	if err := removeAndSync(filepath.Join(layout.StateDir, managedAssetPendingFile)); err != nil {
		return fmt.Errorf("remove managed asset recovery journal: %w", err)
	}
	return nil
}
func publishManagedAssetRecoverySet(layout Layout, manifest managedAssetRecoveryManifest) error {
	backupDir := filepath.Join(layout.StateDir, manifest.BackupDir)
	previous := filepath.Join(layout.StateDir, "managed-assets.previous")
	if info, err := os.Lstat(backupDir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("completed managed asset recovery set is not a directory")
		}
		if manifest.OriginalVersion != "" && manifest.OriginalVersion == manifest.TargetVersion {
			if previousInfo, previousErr := os.Lstat(previous); previousErr == nil {
				if !previousInfo.IsDir() || previousInfo.Mode()&os.ModeSymlink != 0 {
					return errors.New("existing previous release recovery set is not a directory")
				}
				// A same-version repair is not a new release boundary. Keep the
				// last distinct release snapshot available for explicit rollback.
				if err := removeAllAndSync(backupDir); err != nil {
					return fmt.Errorf("remove same-version repair recovery set: %w", err)
				}
			} else if errors.Is(previousErr, os.ErrNotExist) {
				if err := renameAndSync(backupDir, previous); err != nil {
					return fmt.Errorf("publish initial managed asset recovery set: %w", err)
				}
			} else {
				return fmt.Errorf("inspect existing previous release recovery set: %w", previousErr)
			}
		} else {
			if err := removeAllAndSync(previous); err != nil {
				return fmt.Errorf("replace previous managed asset recovery set: %w", err)
			}
			if err := renameAndSync(backupDir, previous); err != nil {
				return fmt.Errorf("publish managed asset recovery set: %w", err)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect completed managed asset recovery set: %w", err)
	} else if info, previousErr := os.Lstat(previous); previousErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("completed managed asset recovery set is missing")
	}
	if err := removeAllAndSync(filepath.Join(layout.StateDir, manifest.StageDir)); err != nil {
		return fmt.Errorf("remove completed managed asset staging set: %w", err)
	}
	return nil
}
func finalizeRecoveredManagedAssetMigration(layout Layout, manifest managedAssetRecoveryManifest) error {
	if err := publishManagedAssetRecoverySet(layout, manifest); err != nil {
		return err
	}
	if manifest.Phase != migrationPhaseFinalized {
		manifest.Phase = migrationPhaseFinalized
		contents, err := json.Marshal(manifest)
		if err != nil {
			return fmt.Errorf("encode completed managed asset recovery journal: %w", err)
		}
		if err := WriteAtomic(filepath.Join(layout.StateDir, managedAssetPendingFile), contents, 0o600); err != nil {
			return fmt.Errorf("record completed managed asset recovery journal: %w", err)
		}
	}
	if err := removeAndSync(filepath.Join(layout.StateDir, managedAssetPendingFile)); err != nil {
		return fmt.Errorf("remove completed managed asset recovery journal: %w", err)
	}
	return nil
}
func restoreManagedStateFile(targetPath, backupDir string, state managedAssetRecoveryFile, fallbackMode os.FileMode) error {
	if !state.Existed {
		return removeAndSync(targetPath)
	}
	backupPath := filepath.Join(backupDir, state.Backup)
	info, err := os.Lstat(backupPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("state backup is not a regular file")
	}
	contents, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	mode := os.FileMode(state.Mode)
	if mode == 0 {
		mode = fallbackMode
	}
	return WriteAtomic(targetPath, contents, mode)
}

func ensurePrivateManagedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("not a directory")
	}
	return nil
}

func ensureRegularManagedAsset(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("target is not a regular file")
	}
	return nil
}
func removeStaleManagedAssetTransactions(layout Layout) error {
	entries, err := os.ReadDir(layout.StateDir)
	if err != nil {
		return fmt.Errorf("inspect managed asset staging directory: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".stealth-managed-assets-") && !strings.HasPrefix(entry.Name(), ".stealth-managed-backup-") {
			continue
		}
		if err := removeAllAndSync(filepath.Join(layout.StateDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale managed asset transaction %q: %w", entry.Name(), err)
		}
	}
	return nil
}
func removeManagedAssetTarget(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("target is not a regular file")
	}
	return removeAndSync(path)
}
