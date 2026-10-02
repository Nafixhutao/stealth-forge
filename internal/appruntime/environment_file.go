package appruntime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Stealth-deplover/stealth/internal/repository"
)

const (
	runtimeEnvironmentDirectory    = "/dev/shm"
	runtimeEnvironmentFileOverhead = repository.AppEnvironmentVariableMaxCount * (120 + 2)
	runtimeEnvironmentFileLimit    = repository.AppEnvironmentVariableMaxTotalValueBytes + runtimeEnvironmentFileOverhead
	// runtimeEnvironmentStaleAfter bounds how long a plaintext environment file
	// may outlive the worker that created it before startup cleanup removes it.
	runtimeEnvironmentStaleAfter = time.Hour
)

// runtimeEnvironmentSweepOnce ensures the stale-file sweep runs at most once
// per process, before the first new environment file is created.
var runtimeEnvironmentSweepOnce sync.Once

// writeRuntimeEnvironmentFile creates a short-lived Docker --env-file in the
// worker container's memory-backed shared-memory directory. The caller must
// invoke the returned cleanup function after Docker has consumed the file.
func writeRuntimeEnvironmentFile(values []RuntimeEnvironmentVariable) (string, func() error, error) {
	if len(values) == 0 || len(values) > repository.AppEnvironmentVariableMaxCount {
		return "", nil, ErrContainerCreate
	}
	ordered := append([]RuntimeEnvironmentVariable(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })
	seen := make(map[string]struct{}, len(ordered))
	total := 0
	totalValueBytes := 0
	for _, item := range ordered {
		if !appRuntimeEnvironmentKey.MatchString(item.Key) ||
			len(item.Value) > repository.AppEnvironmentVariableMaxValueBytes ||
			bytes.IndexAny(item.Value, "\x00\r\n") >= 0 {
			return "", nil, ErrContainerCreate
		}
		if _, duplicate := seen[item.Key]; duplicate {
			return "", nil, ErrContainerCreate
		}
		seen[item.Key] = struct{}{}
		totalValueBytes += len(item.Value)
		if totalValueBytes > repository.AppEnvironmentVariableMaxTotalValueBytes {
			return "", nil, ErrContainerCreate
		}
		total += len(item.Key) + len(item.Value) + 2
		if total > runtimeEnvironmentFileLimit {
			return "", nil, ErrContainerCreate
		}
	}
	if info, err := os.Stat(runtimeEnvironmentDirectory); err != nil || !info.IsDir() {
		return "", nil, ErrContainerCreate
	}
	// Reap plaintext files left by a worker killed before its deferred cleanup
	// ran. This process-wide sweep runs once, before the first new file is
	// created, and only removes files older than the stale threshold.
	runtimeEnvironmentSweepOnce.Do(func() { _, _ = CleanStaleRuntimeEnvironmentFiles() })
	file, err := os.CreateTemp(runtimeEnvironmentDirectory, ".stealth-app-env-*")
	if err != nil {
		return "", nil, ErrContainerCreate
	}
	path := file.Name()
	cleanup := func() error { return wipeAndRemoveRuntimeEnvironmentFile(path) }
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		_ = cleanup()
		return "", nil, ErrContainerCreate
	}
	contents := make([]byte, 0, total)
	for _, item := range ordered {
		contents = append(contents, item.Key...)
		contents = append(contents, '=')
		contents = append(contents, item.Value...)
		contents = append(contents, '\n')
	}
	_, writeErr := file.Write(contents)
	clear(contents)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = cleanup()
		return "", nil, ErrContainerCreate
	}
	return path, cleanup, nil
}

func validRuntimeEnvironmentFilePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path &&
		strings.HasPrefix(path, runtimeEnvironmentDirectory+"/.stealth-app-env-")
}

// CleanStaleRuntimeEnvironmentFiles removes plaintext environment files left
// behind when a worker process was killed before its deferred cleanup ran.
// Only regular files under the runtime environment directory whose names match
// the runtime environment prefix and that are older than the stale threshold
// are removed.
func CleanStaleRuntimeEnvironmentFiles() (int, error) {
	entries, err := os.ReadDir(runtimeEnvironmentDirectory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-runtimeEnvironmentStaleAfter)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(runtimeEnvironmentDirectory, entry.Name())
		if !validRuntimeEnvironmentFilePath(path) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func wipeAndRemoveRuntimeEnvironmentFile(path string) error {
	if !validRuntimeEnvironmentFilePath(path) {
		return ErrContainerCreate
	}
	// Best-effort removal so a wipe failure cannot leave plaintext on disk.
	defer func() { _ = os.Remove(path) }()
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrContainerCreate
	}
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return ErrContainerCreate
		}
		zeroes := make([]byte, 32*1024)
		remaining := info.Size()
		for remaining > 0 {
			chunk := int64(len(zeroes))
			if remaining < chunk {
				chunk = remaining
			}
			written, writeErr := file.Write(zeroes[:chunk])
			if writeErr != nil || written == 0 {
				clear(zeroes)
				_ = file.Close()
				return ErrContainerCreate
			}
			remaining -= int64(written)
		}
		clear(zeroes)
		if err := file.Truncate(0); err != nil {
			_ = file.Close()
			return ErrContainerCreate
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return ErrContainerCreate
		}
		if err := file.Close(); err != nil {
			return ErrContainerCreate
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove temporary App environment file: %w", ErrContainerCreate)
	}
	return nil
}
