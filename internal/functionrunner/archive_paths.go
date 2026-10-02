package functionrunner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxSymlinkTargetBytes = int64(4096)

func trustedEntryPath(name string) (string, bool, error) {
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	if name == "." || name == "" {
		return "", true, nil
	}
	return safeArchiveEntryPath(name)
}

// safeArchiveEntryPath returns a canonical, relative archive path. It is the
// only entry-name sanitizer used before an archive path reaches the
// filesystem boundary. The lexical checks are intentionally stricter than
// filepath.Clean so dot segments and platform-specific absolute paths cannot
// be normalized into an escape.
func safeArchiveEntryPath(name string) (string, bool, error) {
	// Archivers commonly include an explicit root directory (`.` or `./`)
	// when packaging the current working directory. It does not address a
	// user-controlled filesystem object, so accept that single root marker;
	// dot segments anywhere else remain rejected below.
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	if name == "." || name == "" {
		return "", true, nil
	}
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || hasWindowsDrivePrefix(name) {
		return "", false, fmt.Errorf("%w: %q", ErrArchiveTraversal, name)
	}
	isDirectory := strings.HasSuffix(name, "/")
	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" {
		return "", true, fmt.Errorf("%w: empty archive path", ErrArchiveEntry)
	}
	native := filepath.FromSlash(trimmed)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return "", false, fmt.Errorf("%w: %q", ErrArchiveTraversal, name)
	}
	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", false, fmt.Errorf("%w: %q", ErrArchiveTraversal, name)
		}
	}
	clean := path.Join(parts...)
	if clean != trimmed || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("%w: %q", ErrArchiveTraversal, name)
	}
	return clean, isDirectory, nil
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

func makeDirectory(root *os.Root, destination, relative string) error {
	if _, err := safeDestination(destination, relative); err != nil {
		return err
	}
	if err := ensureParentsAreDirectories(root, relative); err != nil {
		return err
	}
	native := filepath.FromSlash(relative)
	info, err := root.Lstat(native)
	if errors.Is(err, os.ErrNotExist) {
		mkdirErr := root.Mkdir(native, 0o700)
		if mkdirErr != nil {
			if !errors.Is(mkdirErr, os.ErrExist) {
				return fmt.Errorf("create archive directory: %w", mkdirErr)
			}
			info, err = root.Lstat(native)
		} else {
			return nil
		}
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: archive directory is not a directory", ErrArchiveEntry)
	}
	return nil
}

func writeEntry(ctx context.Context, root *os.Root, source io.Reader, destination, relative string, maxEntry, maxRemaining int64) (int64, error) {
	if _, err := safeDestination(destination, relative); err != nil {
		return 0, err
	}
	if err := ensureParentsAreDirectories(root, relative); err != nil {
		return 0, err
	}
	nativeRelative := filepath.FromSlash(relative)
	if existing, err := root.Lstat(nativeRelative); err == nil {
		if existing.IsDir() || existing.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("%w: destination is not a regular file", ErrArchiveEntry)
		}
		return 0, fmt.Errorf("%w: duplicate destination", ErrArchiveEntry)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	parent := filepath.ToSlash(filepath.Dir(relative))
	if parent == "." {
		parent = ""
	}
	temporaryRelative, temporary, err := createArchiveTemporaryFile(root, parent)
	if err != nil {
		return 0, err
	}
	cleanup := func() {
		_ = temporary.Close()
		_ = root.Remove(filepath.FromSlash(temporaryRelative))
	}
	limit := maxEntry
	if maxRemaining < limit {
		limit = maxRemaining
	}
	written, err := io.Copy(temporary, io.LimitReader(source, limit+1))
	if err != nil {
		cleanup()
		return 0, err
	}
	if written > limit {
		cleanup()
		return 0, ErrArchiveTooLarge
	}
	if err := contextErr(ctx); err != nil {
		cleanup()
		return 0, err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return 0, err
	}
	if err := temporary.Close(); err != nil {
		_ = root.Remove(filepath.FromSlash(temporaryRelative))
		return 0, err
	}
	if err := root.Rename(filepath.FromSlash(temporaryRelative), nativeRelative); err != nil {
		_ = root.Remove(filepath.FromSlash(temporaryRelative))
		return 0, err
	}
	return written, nil
}

func createArchiveTemporaryFile(root *os.Root, parent string) (string, *os.File, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, fmt.Errorf("generate archive temporary name: %w", err)
		}
		relative := path.Join(parent, ".extract-"+hex.EncodeToString(random[:]))
		file, err := root.OpenFile(filepath.FromSlash(relative), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return relative, file, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", nil, fmt.Errorf("create archive file: %w", err)
		}
	}
	return "", nil, fmt.Errorf("create archive file: temporary name collision")
}

func writeSymlink(root *os.Root, destination, relative, target string) error {
	if relative == "" || target == "" || strings.ContainsAny(target, "\\\x00\r\n") || strings.HasPrefix(target, "/") || hasWindowsDrivePrefix(target) || filepath.IsAbs(filepath.FromSlash(target)) || filepath.VolumeName(filepath.FromSlash(target)) != "" {
		return fmt.Errorf("%w: unsafe symlink target", ErrArchiveEntry)
	}
	if _, err := safeDestination(destination, relative); err != nil {
		return err
	}
	linkDirectory := path.Dir(relative)
	combined := path.Join(linkDirectory, target)
	if linkDirectory == "." {
		combined = path.Clean(target)
	}
	if _, _, err := safeArchiveEntryPath(combined); err != nil {
		return fmt.Errorf("%w: unsafe symlink target", ErrArchiveEntry)
	}
	if err := ensureParentsAreDirectories(root, relative); err != nil {
		return err
	}
	nativeRelative := filepath.FromSlash(relative)
	if _, err := root.Lstat(nativeRelative); err == nil {
		return fmt.Errorf("%w: duplicate destination", ErrArchiveEntry)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := root.Symlink(target, nativeRelative); err != nil {
		return fmt.Errorf("create archive symlink: %w", err)
	}
	return nil
}

func safeDestination(destination, relative string) (string, error) {
	base, err := filepath.Abs(destination)
	if err != nil {
		return "", ErrArchiveTraversal
	}
	target := filepath.Join(base, filepath.FromSlash(relative))
	resolved, err := filepath.Abs(target)
	if err != nil {
		return "", ErrArchiveTraversal
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", ErrArchiveTraversal
	}
	return resolved, nil
}

func ensureParentsAreDirectories(root *os.Root, relative string) error {
	if root == nil {
		return ErrArchiveTraversal
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 2 {
		return nil
	}
	current := ""
	for _, part := range parts[:len(parts)-1] {
		current = path.Join(current, part)
		native := filepath.FromSlash(current)
		info, err := root.Lstat(native)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if err := root.Mkdir(native, 0o700); err != nil {
					if !errors.Is(err, os.ErrExist) {
						return err
					}
					info, err = root.Lstat(native)
					if err != nil {
						return err
					}
				} else {
					continue
				}
			} else {
				return err
			}
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: archive parent is not a directory", ErrArchiveEntry)
		}
	}
	return nil
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
