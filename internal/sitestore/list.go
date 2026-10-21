package sitestore

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// MaxListedFiles bounds one listing response. A published Site is a built
// artifact, so this is far above what a static site normally contains; it exists
// so a pathological artifact cannot turn a listing into an unbounded walk.
const MaxListedFiles = 5000

// FileEntry is one regular file inside a published artifact. The path is
// relative to the artifact root and always uses forward slashes, because it is
// also the path the public serve endpoint accepts.
type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// ListFiles returns the regular files of one published artifact, sorted by path.
// `truncated` reports that the limit cut the walk short.
//
// The artifact directory itself is resolved through checkedArtifact, so the
// caller supplies an artifact path (three UUID components), never a filesystem
// path from a request. Symlinks are skipped rather than followed: a published
// artifact is extracted from an upload, and a link inside it must not be able to
// name a file outside the release.
func (s *Store) ListFiles(relative string, limit int) ([]FileEntry, bool, error) {
	artifact, _, err := s.checkedArtifact(relative)
	if err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > MaxListedFiles {
		limit = MaxListedFiles
	}

	files := make([]FileEntry, 0, 32)
	truncated := false
	walkErr := filepath.WalkDir(artifact, func(current string, entry fs.DirEntry, walkErr error) error {
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
		if len(files) >= limit {
			truncated = true
			return fs.SkipAll
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		relativePath, relErr := filepath.Rel(artifact, current)
		if relErr != nil {
			return relErr
		}
		files = append(files, FileEntry{
			Path: filepath.ToSlash(relativePath),
			Size: info.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, false, walkErr
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, truncated, nil
}
