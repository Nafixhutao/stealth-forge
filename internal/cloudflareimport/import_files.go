package cloudflareimport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func writeAtomic(path string, ciphertext []byte, owner *FileOwner) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return errArtifactIO
	}
	directoryInfo, err := os.Lstat(directory)
	if err != nil || directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return errArtifactIO
	}
	fileMode := os.FileMode(0o600)
	directoryMode := os.FileMode(0o700)
	if owner != nil {
		if owner.UID < 0 || owner.GID < 0 {
			return errArtifactIO
		}
		if err := os.Chown(directory, owner.UID, owner.GID); err != nil {
			return errArtifactIO
		}
		fileMode = 0o640
		directoryMode = 0o770
	}
	if err := os.Chmod(directory, directoryMode); err != nil {
		return errArtifactIO
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errUnsafeImportEntry
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errArtifactIO
	}
	temporary, err := os.CreateTemp(directory, ".cloudflare-import-*.tmp")
	if err != nil {
		return errArtifactIO
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(ciphertext); err != nil {
		_ = temporary.Close()
		return errArtifactIO
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errArtifactIO
	}
	if owner != nil {
		if err := temporary.Chown(owner.UID, owner.GID); err != nil {
			_ = temporary.Close()
			return errArtifactIO
		}
	}
	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return errArtifactIO
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errArtifactIO
	}
	if err := temporary.Close(); err != nil {
		return errArtifactIO
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return errArtifactIO
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return errArtifactIO
	}
	defer directoryFile.Close()
	if err := directoryFile.Sync(); err != nil {
		return errArtifactIO
	}
	return nil
}

func removeArtifact(path string) error {
	directory := filepath.Dir(path)
	directoryInfo, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return errArtifactIO
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errArtifactIO
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errUnsafeImportEntry
	}
	if err := os.Remove(path); err != nil {
		return errArtifactIO
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return errArtifactIO
	}
	defer directoryFile.Close()
	if err := directoryFile.Sync(); err != nil {
		return errArtifactIO
	}
	return nil
}

// cleanWorkerImportDirectory removes the two filenames used by the previous
// initializer to copy the complete setup snapshot. It also removes abandoned
// narrow-artifact temp files and fails closed if any unrecognized entry would
// remain visible in the worker's directory mount.
func cleanWorkerImportDirectory(directory, artifactName string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrWorkerBoundaryUnsafe
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ErrWorkerBoundaryUnsafe
	}
	removed := false
	for _, entry := range entries {
		name := entry.Name()
		if name == artifactName {
			entryInfo, statErr := os.Lstat(filepath.Join(directory, name))
			if statErr != nil || entryInfo.Mode()&os.ModeSymlink != 0 || !entryInfo.Mode().IsRegular() {
				return ErrWorkerBoundaryUnsafe
			}
			continue
		}
		if name == legacySetupSnapshotName || name == legacySetupTempName || (strings.HasPrefix(name, ".cloudflare-import-") && strings.HasSuffix(name, ".tmp")) {
			entryInfo, statErr := os.Lstat(filepath.Join(directory, name))
			if statErr != nil || entryInfo.IsDir() {
				return ErrWorkerBoundaryUnsafe
			}
			// Remove the exact entry without following a possible symlink. These
			// names are known initializer outputs, never source setup state.
			if err := os.Remove(filepath.Join(directory, name)); err != nil {
				return ErrWorkerBoundaryUnsafe
			}
			removed = true
			continue
		}
		return ErrWorkerBoundaryUnsafe
	}
	if removed {
		directoryFile, err := os.Open(directory)
		if err != nil {
			return ErrWorkerBoundaryUnsafe
		}
		defer directoryFile.Close()
		if err := directoryFile.Sync(); err != nil {
			return ErrWorkerBoundaryUnsafe
		}
	}
	return nil
}

// OwnerForSourceDirectory returns the source directory's host UID paired with
// the worker's fixed group. Carrying this owner through the narrow handoff
// keeps the derived host artifact manageable by the installation user while
// remaining readable by the non-root worker.
func OwnerForSourceDirectory(path string) (*FileOwner, error) {
	if !validFilePath(path) {
		return nil, errInvalidPath
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errInvalidPath
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errInvalidPath
	}
	return &FileOwner{UID: int(stat.Uid), GID: workerGroupID}, nil
}
