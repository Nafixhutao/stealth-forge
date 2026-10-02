// Package functionrunner contains the trusted worker boundary for user
// Functions. Nothing in this package runs an uploaded archive on the API
// process; source files are copied into an isolated runtime container. Its
// strict archive extractor is also reused by Sites for pre-built static
// publication, where extracted files are served but never executed by API.
package functionrunner

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrUnsupportedArchive = errors.New("unsupported function source archive")
	ErrArchiveTraversal   = errors.New("function source archive contains an unsafe path")
	ErrArchiveEntry       = errors.New("function source archive contains an unsafe entry")
	ErrArchiveTooLarge    = errors.New("function source archive expands beyond worker limits")
)

const (
	DefaultMaxArchiveBytes = int64(256 << 20)
	DefaultMaxArchiveFiles = 4096
	DefaultMaxEntryBytes   = int64(128 << 20)
	DefaultMaxCompressed   = int64(64 << 20)
)

// ArchiveLimits protect the worker from zip/tar bombs and pathological file
// counts. The compressed upload limit is enforced by functionstore; these
// limits apply to the expanded workspace.
type ArchiveLimits struct {
	MaxBytes      int64
	MaxFiles      int
	MaxEntry      int64
	MaxCompressed int64
	// StripTopLevel is used for provider-generated Git archives, which wrap
	// the repository in one synthetic directory. Ordinary user uploads keep
	// their archive paths unchanged.
	StripTopLevel bool
}

func (l ArchiveLimits) withDefaults() ArchiveLimits {
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultMaxArchiveBytes
	}
	if l.MaxFiles <= 0 {
		l.MaxFiles = DefaultMaxArchiveFiles
	}
	if l.MaxEntry <= 0 {
		l.MaxEntry = DefaultMaxEntryBytes
	}
	if l.MaxCompressed <= 0 {
		l.MaxCompressed = DefaultMaxCompressed
	}
	return l
}

type ArchiveStats struct {
	Files       int
	Directories int
	Bytes       int64
}

// Extract accepts zip, tar, tar.gz and tgz source names. Archive entries are
// always created below destination; absolute paths, dot segments, symlinks,
// hard links and special files are rejected before any user file is written.
func Extract(ctx context.Context, source io.Reader, sourceName, destination string, limits ArchiveLimits) (ArchiveStats, error) {
	return extractArchive(ctx, source, sourceName, destination, limits, false)
}

// ExtractTrusted reads a tar artifact produced by the isolated builder. It
// permits only lexically in-workspace relative symlinks (package managers
// commonly create these), while still rejecting absolute targets, dot-segment
// escapes, hard links, and special files. Untrusted uploads must use Extract.
func ExtractTrusted(ctx context.Context, source io.Reader, sourceName, destination string, limits ArchiveLimits) (ArchiveStats, error) {
	return extractArchive(ctx, source, sourceName, destination, limits, true)
}

func extractArchive(ctx context.Context, source io.Reader, sourceName, destination string, limits ArchiveLimits, allowSymlinks bool) (ArchiveStats, error) {
	limits = limits.withDefaults()
	destination, root, err := prepareArchiveRoot(destination)
	if err != nil {
		return ArchiveStats{}, err
	}
	defer root.Close()
	// zip.Reader requires random access. The function upload ceiling is small
	// relative to worker memory, and this bounded read avoids an unbounded
	// allocation when a caller invokes Extract directly.
	readLimit := limits.MaxCompressed
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	compressed, err := io.ReadAll(io.LimitReader(source, readLimit))
	if err != nil {
		return ArchiveStats{}, err
	}
	if int64(len(compressed)) > limits.MaxCompressed {
		return ArchiveStats{}, ErrArchiveTooLarge
	}
	name := strings.ToLower(strings.TrimSpace(sourceName))
	switch {
	case strings.HasSuffix(name, ".zip"):
		return extractZip(ctx, compressed, destination, root, limits, allowSymlinks)
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		reader, closeFn, err := gzipReader(bytes.NewReader(compressed))
		if err != nil {
			return ArchiveStats{}, err
		}
		defer func() { _ = closeFn() }()
		return extractTar(ctx, reader, destination, root, limits, allowSymlinks)
	case strings.HasSuffix(name, ".tar"):
		return extractTar(ctx, bytes.NewReader(compressed), destination, root, limits, allowSymlinks)
	default:
		return ArchiveStats{}, ErrUnsupportedArchive
	}
}

func prepareArchiveRoot(destination string) (string, *os.Root, error) {
	destination, err := filepath.Abs(destination)
	if err != nil || strings.TrimSpace(destination) == "" {
		return "", nil, ErrArchiveTraversal
	}
	destination = filepath.Clean(destination)
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(destination, 0o700); err != nil {
			return "", nil, fmt.Errorf("create function workspace: %w", err)
		}
		info, err = os.Lstat(destination)
	}
	if err != nil {
		return "", nil, fmt.Errorf("inspect function workspace: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, ErrArchiveTraversal
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return "", nil, fmt.Errorf("open function workspace: %w", err)
	}
	return destination, root, nil
}

func gzipReader(source io.Reader) (io.Reader, func() error, error) {
	reader, err := gzip.NewReader(source)
	if err != nil {
		return nil, nil, fmt.Errorf("open gzip source archive: %w", err)
	}
	return reader, reader.Close, nil
}
