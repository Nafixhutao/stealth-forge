package cloudflareimport

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func PublishLegacySetupSnapshot(ctx context.Context, sourcePath, inputDirectory string, owners ...*FileOwner) (published bool, resultErr error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(owners) > 1 {
		return false, errInvalidPath
	}
	var owner *FileOwner
	if len(owners) == 1 {
		owner = owners[0]
	}
	if !validFilePath(sourcePath) || filepath.Base(sourcePath) != legacySetupSnapshotName || !validFilePath(inputDirectory) ||
		filepath.Clean(filepath.Dir(sourcePath)) == filepath.Clean(inputDirectory) {
		return false, errInvalidPath
	}
	sourceDirectory := filepath.Dir(sourcePath)
	sourceDirInfo, err := os.Lstat(sourceDirectory)
	if err != nil || sourceDirInfo.Mode()&os.ModeSymlink != 0 || !sourceDirInfo.IsDir() {
		return false, errInvalidSource
	}
	if err := os.MkdirAll(inputDirectory, 0o700); err != nil {
		return false, errArtifactIO
	}
	inputInfo, err := os.Lstat(inputDirectory)
	if err != nil || inputInfo.Mode()&os.ModeSymlink != 0 || !inputInfo.IsDir() {
		return false, errArtifactIO
	}
	if owner != nil && (owner.UID < 0 || owner.GID < 0 || os.Geteuid() != 0) {
		return false, errArtifactIO
	}
	if os.Geteuid() == 0 {
		// This named volume may retain ownership from an earlier run. Return it
		// to root temporarily so stale entries can be removed, then restore the
		// trusted installation UID and fixed worker group before returning.
		if err := os.Chown(inputDirectory, 0, 0); err != nil {
			return false, errArtifactIO
		}
	}
	if owner != nil {
		defer func() {
			if err := restoreLegacySetupInputOwner(inputDirectory, owner); err != nil {
				published = false
				resultErr = err
			}
		}()
	}
	if err := os.Chmod(inputDirectory, 0o700); err != nil {
		return false, errArtifactIO
	}
	if err := cleanLegacySetupInputDirectory(inputDirectory); err != nil {
		return false, err
	}

	fd, err := syscall.Open(sourcePath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, errInvalidSource
	}
	source := os.NewFile(uintptr(fd), sourcePath)
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSetupSnapshotBytes || info.Mode().Perm()&0o007 != 0 {
		return false, errInvalidSource
	}
	contents, err := io.ReadAll(io.LimitReader(source, maxSetupSnapshotBytes+1))
	if err != nil || len(contents) == 0 || len(contents) > maxSetupSnapshotBytes || int64(len(contents)) != info.Size() {
		return false, errInvalidSource
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := writeLegacySetupInput(inputDirectory, contents); err != nil {
		return false, err
	}
	return true, nil
}

func restoreLegacySetupInputOwner(directory string, owner *FileOwner) error {
	if owner == nil {
		return nil
	}
	// Set permissions while the root-run initializer still owns the directory.
	// Its deliberately narrow capability set has CHOWN but not FOWNER, so a
	// later chmod would fail after ownership moves to the host installation UID.
	if err := os.Chmod(directory, 0o770); err != nil {
		return errArtifactIO
	}
	if err := os.Chown(directory, owner.UID, owner.GID); err != nil {
		return errArtifactIO
	}
	if err := syncDirectory(directory); err != nil {
		return errArtifactIO
	}
	return nil
}

func cleanLegacySetupInputDirectory(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return errArtifactIO
	}
	removed := false
	for _, entry := range entries {
		name := entry.Name()
		if name != legacySetupSnapshotName && name != legacySetupTempName && !strings.HasPrefix(name, ".setup-state.enc.tmp-") {
			return errInvalidSource
		}
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errInvalidSource
		}
		if err := os.Remove(path); err != nil {
			return errArtifactIO
		}
		removed = true
	}
	if removed {
		if err := syncDirectory(directory); err != nil {
			return errArtifactIO
		}
	}
	return nil
}

func writeLegacySetupInput(directory string, contents []byte) error {
	temporary, err := os.CreateTemp(directory, ".setup-state.enc.tmp-")
	if err != nil {
		return errArtifactIO
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return errArtifactIO
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errArtifactIO
	}
	if err := temporary.Chmod(0o400); err != nil {
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
	destination := filepath.Join(directory, legacySetupSnapshotName)
	if err := os.Rename(temporaryPath, destination); err != nil {
		return errArtifactIO
	}
	if err := syncDirectory(directory); err != nil {
		return errArtifactIO
	}
	return nil
}

// Encrypt returns an encrypted, validated artifact for a trusted producer.
