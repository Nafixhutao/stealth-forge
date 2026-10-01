package ingress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func reloadSentinel(contents []byte) []byte {
	digest := sha256.Sum256(contents)
	return []byte("# Stealth platform route snapshot\n# sha256: " + hex.EncodeToString(digest[:]) + "\n")
}

func routeSetReloadSentinel(siteContents, appContents []byte) []byte {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(GeneratedFilename + "\x00"))
	_, _ = hasher.Write(siteContents)
	_, _ = hasher.Write([]byte("\x00" + GeneratedAppFilename + "\x00"))
	_, _ = hasher.Write(appContents)
	return []byte("# Stealth platform route set\n# sha256: " + hex.EncodeToString(hasher.Sum(nil)) + "\n")
}

func readOrEmptyManagedFile(path string, empty []byte) ([]byte, error) {
	contents, err := readManagedFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return append([]byte(nil), empty...), nil
	}
	return contents, err
}

func publishSnapshotFile(path string, contents []byte) (bool, error) {
	current, currentErr := readManagedFile(path)
	if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
		return false, currentErr
	}
	if bytes.Equal(current, contents) && currentErr == nil {
		return false, nil
	}
	if err := publishAtomic(path, contents, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func publishSnapshot(outputFile string, contents []byte, reloadFile string, reloadContents []byte) (changed, reloadChanged bool, err error) {
	current, currentErr := readManagedFile(outputFile)
	if currentErr != nil && !errors.Is(currentErr, os.ErrNotExist) {
		return false, false, currentErr
	}
	if !bytes.Equal(current, contents) || errors.Is(currentErr, os.ErrNotExist) {
		if err := publishAtomic(outputFile, contents, 0o644); err != nil {
			return false, false, err
		}
		changed = true
	}
	currentReload, reloadErr := readManagedFile(reloadFile)
	if reloadErr != nil && !errors.Is(reloadErr, os.ErrNotExist) {
		return changed, false, reloadErr
	}
	if !bytes.Equal(currentReload, reloadContents) || errors.Is(reloadErr, os.ErrNotExist) {
		if err := publishAtomic(reloadFile, reloadContents, 0o644); err != nil {
			return changed, false, err
		}
		reloadChanged = true
	}
	return changed || reloadChanged, reloadChanged, nil
}

func readManagedFile(path string) ([]byte, error) {
	if err := validateDestination(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func publishAtomic(path string, contents []byte, mode os.FileMode) error {
	if err := validateDestination(path); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".stealth-platform-routes-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("fsync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("atomically publish %q: %w", path, err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("fsync directory %q: %w", directory, err)
	}
	return nil
}

func validateDestination(path string) error {
	path = filepath.Clean(path)
	if path == "." || path == string(filepath.Separator) || filepath.Base(path) == "." || filepath.Base(path) == ".." {
		return errors.New("managed Traefik path is invalid")
	}
	directory := filepath.Dir(path)
	if err := validateDirectoryPath(directory); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect managed Traefik file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed Traefik file %q is not a normal file", path)
	}
	return nil
}

func validateDirectoryPath(path string) error {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	relative := strings.TrimPrefix(path, current)
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect managed Traefik directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("managed Traefik directory %q is not a normal directory", current)
		}
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
