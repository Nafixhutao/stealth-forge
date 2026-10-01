package installengine

import (
	"errors"
	"os"
	"path/filepath"
)

func renameAndSync(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(source)); err != nil {
		return err
	}
	if filepath.Dir(source) != filepath.Dir(target) {
		if err := syncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
	}
	return nil
}

func removeAndSync(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func removeAllAndSync(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// The small wrappers below are intentionally exported so the CLI can delete
// its duplicate file/configuration implementation while preserving its
// existing tests and operator behavior.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func FileIsPrivate(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0
}

func WritePrivateFile(path, contents string) error {
	return WriteAtomic(path, []byte(contents), 0o600)
}

func WriteAtomic(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".stealth-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	if err := syncFile(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
