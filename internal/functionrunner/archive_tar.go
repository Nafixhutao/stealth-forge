package functionrunner

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func extractTar(ctx context.Context, source io.Reader, destination string, root *os.Root, limits ArchiveLimits, allowSymlinks bool) (ArchiveStats, error) {
	reader := tar.NewReader(source)
	stats := ArchiveStats{}
	seen := map[string]struct{}{}
	rootPrefix := ""
	rootSeen := false
	for {
		if err := contextErr(ctx); err != nil {
			return ArchiveStats{}, err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ArchiveStats{}, fmt.Errorf("read tar source archive: %w", err)
		}
		if header.Typeflag == tar.TypeXHeader || header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeGNULongName || header.Typeflag == tar.TypeGNULongLink {
			// archive/tar consumes PAX/GNU metadata while iterating. These
			// records do not represent a filesystem object.
			continue
		}
		var relative string
		var isDirectory bool
		if allowSymlinks {
			relative, isDirectory, err = trustedEntryPath(header.Name)
		} else {
			relative, isDirectory, err = safeArchiveEntryPath(header.Name)
		}
		if err != nil {
			return ArchiveStats{}, err
		}
		isDirectory = isDirectory || header.Typeflag == tar.TypeDir
		if limits.StripTopLevel {
			relative, isDirectory, err = stripTopLevelEntry(relative, isDirectory, &rootPrefix, &rootSeen)
			if err != nil {
				return ArchiveStats{}, err
			}
			if relative == "" && isDirectory {
				continue
			}
		}
		if relative == "" && isDirectory {
			continue
		}
		if _, exists := seen[relative]; exists {
			return ArchiveStats{}, fmt.Errorf("%w: duplicate path %q", ErrArchiveEntry, header.Name)
		}
		seen[relative] = struct{}{}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := makeDirectory(root, destination, relative); err != nil {
				return ArchiveStats{}, err
			}
			stats.Directories++
		case tar.TypeReg, tar.TypeRegA:
			if stats.Files >= limits.MaxFiles || header.Size < 0 || header.Size > limits.MaxEntry || header.Size > limits.MaxBytes-stats.Bytes {
				return ArchiveStats{}, ErrArchiveTooLarge
			}
			written, writeErr := writeEntry(ctx, root, reader, destination, relative, limits.MaxEntry, limits.MaxBytes-stats.Bytes)
			if writeErr != nil {
				return ArchiveStats{}, writeErr
			}
			if written != header.Size {
				return ArchiveStats{}, fmt.Errorf("%w: tar entry size changed", ErrArchiveEntry)
			}
			stats.Files++
			stats.Bytes += written
		case tar.TypeSymlink:
			if !allowSymlinks || stats.Files >= limits.MaxFiles {
				return ArchiveStats{}, fmt.Errorf("%w: links and special files are not allowed", ErrArchiveEntry)
			}
			if err := writeSymlink(root, destination, relative, header.Linkname); err != nil {
				return ArchiveStats{}, err
			}
			stats.Files++
		default:
			return ArchiveStats{}, fmt.Errorf("%w: tar links and special files are not allowed", ErrArchiveEntry)
		}
	}
	return stats, nil
}

func stripTopLevelEntry(relative string, isDirectory bool, rootPrefix *string, rootSeen *bool) (string, bool, error) {
	if relative == "" {
		return "", isDirectory, fmt.Errorf("%w: empty Git archive path", ErrArchiveEntry)
	}
	if !*rootSeen {
		if !isDirectory {
			return "", false, fmt.Errorf("%w: Git archive must contain one repository root directory", ErrArchiveEntry)
		}
		parts := strings.Split(relative, "/")
		if len(parts) == 0 || parts[0] == "" {
			return "", false, fmt.Errorf("%w: Git archive root directory is invalid", ErrArchiveEntry)
		}
		*rootPrefix = parts[0]
		*rootSeen = true
	}
	if relative == *rootPrefix {
		return "", true, nil
	}
	prefix := *rootPrefix + "/"
	if !strings.HasPrefix(relative, prefix) {
		return "", false, fmt.Errorf("%w: Git archive contains multiple top-level directories", ErrArchiveEntry)
	}
	stripped := strings.TrimPrefix(relative, prefix)
	if stripped == "" {
		return "", false, fmt.Errorf("%w: Git archive path is empty", ErrArchiveEntry)
	}
	return stripped, isDirectory, nil
}
