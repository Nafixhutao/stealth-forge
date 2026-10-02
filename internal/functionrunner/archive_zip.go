package functionrunner

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
)

func extractZip(ctx context.Context, compressed []byte, destination string, root *os.Root, limits ArchiveLimits, allowSymlinks bool) (ArchiveStats, error) {
	archive, err := zip.NewReader(bytes.NewReader(compressed), int64(len(compressed)))
	if err != nil {
		return ArchiveStats{}, fmt.Errorf("open zip source archive: %w", err)
	}
	stats := ArchiveStats{}
	seen := make(map[string]struct{}, len(archive.File))
	rootPrefix := ""
	rootSeen := false
	for _, entry := range archive.File {
		if err := contextErr(ctx); err != nil {
			return ArchiveStats{}, err
		}
		relative, isDirectory, err := safeArchiveEntryPath(entry.Name)
		if err != nil {
			return ArchiveStats{}, err
		}
		isDirectory = isDirectory || entry.Mode().IsDir()
		if limits.StripTopLevel {
			relative, isDirectory, err = stripTopLevelEntry(relative, isDirectory, &rootPrefix, &rootSeen)
			if err != nil {
				return ArchiveStats{}, err
			}
			if relative == "" && isDirectory {
				continue
			}
		}
		if _, exists := seen[relative]; exists {
			return ArchiveStats{}, fmt.Errorf("%w: duplicate path %q", ErrArchiveEntry, entry.Name)
		}
		seen[relative] = struct{}{}
		if entry.Mode()&os.ModeSymlink != 0 {
			if !allowSymlinks {
				return ArchiveStats{}, fmt.Errorf("%w: links and special files are not allowed", ErrArchiveEntry)
			}
			if isDirectory || stats.Files >= limits.MaxFiles {
				return ArchiveStats{}, ErrArchiveTooLarge
			}
			reader, err := entry.Open()
			if err != nil {
				return ArchiveStats{}, fmt.Errorf("open zip symlink: %w", err)
			}
			target, readErr := io.ReadAll(io.LimitReader(reader, maxSymlinkTargetBytes+1))
			closeErr := reader.Close()
			if readErr != nil {
				return ArchiveStats{}, readErr
			}
			if closeErr != nil {
				return ArchiveStats{}, closeErr
			}
			if int64(len(target)) > maxSymlinkTargetBytes {
				return ArchiveStats{}, ErrArchiveTooLarge
			}
			if err := writeSymlink(root, destination, relative, string(target)); err != nil {
				return ArchiveStats{}, err
			}
			stats.Files++
			continue
		}
		if entry.Mode()&os.ModeType != 0 && !entry.Mode().IsDir() {
			return ArchiveStats{}, fmt.Errorf("%w: links and special files are not allowed", ErrArchiveEntry)
		}
		if isDirectory {
			if stats.Files+stats.Directories >= limits.MaxFiles {
				return ArchiveStats{}, ErrArchiveTooLarge
			}
			if err := makeDirectory(root, destination, relative); err != nil {
				return ArchiveStats{}, err
			}
			stats.Directories++
			continue
		}
		if stats.Files >= limits.MaxFiles {
			return ArchiveStats{}, ErrArchiveTooLarge
		}
		size := int64(entry.UncompressedSize64)
		if size < 0 || size > limits.MaxEntry || size > limits.MaxBytes-stats.Bytes {
			return ArchiveStats{}, ErrArchiveTooLarge
		}
		reader, err := entry.Open()
		if err != nil {
			return ArchiveStats{}, fmt.Errorf("open zip entry: %w", err)
		}
		written, writeErr := writeEntry(ctx, root, reader, destination, relative, limits.MaxEntry, limits.MaxBytes-stats.Bytes)
		closeErr := reader.Close()
		if writeErr != nil {
			return ArchiveStats{}, writeErr
		}
		if closeErr != nil {
			return ArchiveStats{}, closeErr
		}
		stats.Files++
		stats.Bytes += written
	}
	return stats, nil
}
