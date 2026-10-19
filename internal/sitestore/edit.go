package sitestore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// EditLimits bounds one copy-for-edit. It mirrors the archive limits the upload
// path enforces, so an edit cannot grow a release past what a fresh upload would
// have been allowed to contain.
type EditLimits struct {
	MaxBytes int64
	MaxFiles int
}

// EditStats describes the staged copy.
type EditStats struct {
	Bytes int64
	Files int
}

// ErrEditTargetMissing reports an edit of a path the release does not contain.
// Adding files is deliberately not part of the edit endpoint.
var ErrEditTargetMissing = errors.New("the release has no such file")

// CopyForEdit clones one published artifact into a staging directory.
//
// The copy is what keeps a release immutable: the published directory is only
// read, and the edit lands in a new staging tree that becomes a new release. Only
// regular files and directories are copied — a symlink is skipped, never
// followed, so a link inside a release cannot name a file outside it.
func (s *Store) CopyForEdit(sourceRelative, staging string, limits EditLimits) (EditStats, error) {
	artifact, _, err := s.checkedArtifact(sourceRelative)
	if err != nil {
		return EditStats{}, err
	}
	if s == nil || s.rootDir == nil || !s.insideRoot(staging) {
		return EditStats{}, ErrInvalidPath
	}
	stagingRelative, err := s.relativePath(staging)
	if err != nil || !strings.HasPrefix(filepath.Base(stagingRelative), ".site-upload-") {
		return EditStats{}, ErrInvalidPath
	}
	if limits.MaxBytes <= 0 || limits.MaxFiles <= 0 {
		return EditStats{}, ErrInvalidPath
	}
	sourceRelativePath, err := filepath.Rel(s.root, artifact)
	if err != nil {
		return EditStats{}, ErrInvalidPath
	}

	stats := EditStats{}
	err = filepath.WalkDir(artifact, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		relativePath, relErr := filepath.Rel(artifact, current)
		if relErr != nil {
			return relErr
		}
		if relativePath == "." {
			return nil
		}
		targetRelative := filepath.Join(stagingRelative, relativePath)
		if entry.IsDir() {
			return s.rootDir.Mkdir(targetRelative, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		stats.Bytes += info.Size()
		stats.Files++
		if stats.Bytes > limits.MaxBytes || stats.Files > limits.MaxFiles {
			return fmt.Errorf("%w: release exceeds the edit limits", ErrInvalidPath)
		}
		return copyStagedFile(s, filepath.Join(sourceRelativePath, relativePath), targetRelative)
	})
	if err != nil {
		return EditStats{}, err
	}
	return stats, nil
}

// copyStagedFile copies one regular file between two paths under the store root.
func copyStagedFile(s *Store, from, to string) error {
	source, err := s.rootDir.Open(from)
	if err != nil {
		return err
	}
	destination, err := s.rootDir.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		_ = source.Close()
		return err
	}
	_, copyErr := io.Copy(destination, source)
	sourceCloseErr := source.Close()
	destinationCloseErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	if sourceCloseErr != nil {
		return sourceCloseErr
	}
	return destinationCloseErr
}

// WriteStagedFile replaces the contents of one file in a staged copy.
//
// The file must already exist in the staging tree: the edit endpoint changes what
// a release contains, it does not add to it. The path is resolved with the same
// component checks the reader uses, so no request value can escape the staging
// directory.
func (s *Store) WriteStagedFile(staging, requested string, content []byte) (previous, written int64, err error) {
	if s == nil || s.rootDir == nil || !s.insideRoot(staging) {
		return 0, 0, ErrInvalidPath
	}
	if strings.TrimSpace(requested) == "" {
		return 0, 0, ErrInvalidFile
	}
	clean, segments, err := cleanRequestedPath(requested)
	if err != nil || len(segments) == 0 {
		return 0, 0, ErrInvalidFile
	}
	stagingRelative, err := s.relativePath(staging)
	if err != nil {
		return 0, 0, ErrInvalidPath
	}
	targetRelative := filepath.FromSlash(path.Join(filepath.ToSlash(stagingRelative), clean))

	// Every component must exist and be a real directory; the leaf must be a
	// regular file. This rejects traversal and symlinked intermediates alike.
	for index := range segments {
		current := filepath.Join(append([]string{stagingRelative}, segments[:index+1]...)...)
		info, statErr := s.rootDir.Lstat(current)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return 0, 0, ErrEditTargetMissing
			}
			return 0, 0, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return 0, 0, ErrInvalidPath
		}
		if index == len(segments)-1 {
			if !info.Mode().IsRegular() {
				return 0, 0, ErrEditTargetMissing
			}
			previous = info.Size()
			break
		}
		if !info.IsDir() {
			return 0, 0, ErrEditTargetMissing
		}
	}

	file, err := s.rootDir.OpenFile(targetRelative, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, ErrEditTargetMissing
		}
		return 0, 0, err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return 0, 0, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return 0, 0, err
	}
	if err := file.Close(); err != nil {
		return 0, 0, err
	}
	return previous, int64(len(content)), nil
}

// DigestArtifact returns a deterministic SHA-256 over the staged tree: paths and
// contents in sorted order. It is the release's identity, so two edits producing
// the same tree produce the same digest.
func (s *Store) DigestArtifact(staging string) (string, error) {
	if s == nil || s.rootDir == nil || !s.insideRoot(staging) {
		return "", ErrInvalidPath
	}
	stagingRelative, err := s.relativePath(staging)
	if err != nil {
		return "", ErrInvalidPath
	}
	stagingRoot := filepath.Join(s.root, filepath.FromSlash(stagingRelative))

	files := make([]string, 0, 32)
	err = filepath.WalkDir(stagingRoot, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		relativePath, relErr := filepath.Rel(stagingRoot, current)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	digest := sha256.New()
	for _, name := range files {
		body, readErr := os.ReadFile(filepath.Join(stagingRoot, filepath.FromSlash(name)))
		if readErr != nil {
			return "", readErr
		}
		fmt.Fprintf(digest, "%s\x00%d\x00", name, len(body))
		digest.Write(body)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
