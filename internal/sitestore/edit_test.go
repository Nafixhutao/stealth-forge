package sitestore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The edit path is the only place a release's contents can change, so its
// guarantees are worth pinning: it copies rather than mutates, it refuses to add
// files, it never follows a symlink, and its digest is deterministic.
func editFixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	projectID := uuid.Must(uuid.NewV7())
	siteID := uuid.Must(uuid.NewV7())
	deploymentID := uuid.Must(uuid.NewV7())
	source, err := ArtifactRelativePath(projectID, siteID, deploymentID)
	if err != nil {
		t.Fatalf("ArtifactRelativePath: %v", err)
	}
	artifact := filepath.Join(root, filepath.FromSlash(source))
	if err := os.MkdirAll(filepath.Join(artifact, "assets"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{
		"index.html":     "<!doctype html><p>one</p>",
		"assets/app.css": "body{}",
	} {
		if err := os.WriteFile(filepath.Join(artifact, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return store, source, artifact
}

func TestCopyForEditLeavesTheSourceUntouched(t *testing.T) {
	store, source, artifact := editFixture(t)
	projectID := uuid.Must(uuid.NewV7())
	siteID := uuid.Must(uuid.NewV7())
	deploymentID := uuid.Must(uuid.NewV7())

	staging, _, err := store.BeginStaging(projectID, siteID, deploymentID)
	if err != nil {
		t.Fatalf("BeginStaging: %v", err)
	}
	stats, err := store.CopyForEdit(source, staging, EditLimits{MaxBytes: 1 << 20, MaxFiles: 100})
	if err != nil {
		t.Fatalf("CopyForEdit: %v", err)
	}
	if stats.Files != 2 {
		t.Fatalf("copied %d files, want 2", stats.Files)
	}

	previous, written, err := store.WriteStagedFile(staging, "index.html", []byte("<!doctype html><p>two</p>"))
	if err != nil {
		t.Fatalf("WriteStagedFile: %v", err)
	}
	if previous != int64(len("<!doctype html><p>one</p>")) {
		t.Fatalf("previous size = %d", previous)
	}
	if written != int64(len("<!doctype html><p>two</p>")) {
		t.Fatalf("written size = %d", written)
	}

	// The published release must still hold its original bytes.
	published, err := os.ReadFile(filepath.Join(artifact, "index.html"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(published) != "<!doctype html><p>one</p>" {
		t.Fatalf("source release was modified: %q", published)
	}

	staged, err := os.ReadFile(filepath.Join(staging, "index.html"))
	if err != nil {
		t.Fatalf("ReadFile staged: %v", err)
	}
	if string(staged) != "<!doctype html><p>two</p>" {
		t.Fatalf("staged content = %q", staged)
	}
}

func TestWriteStagedFileRefusesToAddFiles(t *testing.T) {
	store, source, _ := editFixture(t)
	projectID := uuid.Must(uuid.NewV7())
	siteID := uuid.Must(uuid.NewV7())
	deploymentID := uuid.Must(uuid.NewV7())

	staging, _, err := store.BeginStaging(projectID, siteID, deploymentID)
	if err != nil {
		t.Fatalf("BeginStaging: %v", err)
	}
	if _, err := store.CopyForEdit(source, staging, EditLimits{MaxBytes: 1 << 20, MaxFiles: 100}); err != nil {
		t.Fatalf("CopyForEdit: %v", err)
	}

	if _, _, err := store.WriteStagedFile(staging, "new-file.html", []byte("x")); err == nil {
		t.Fatalf("WriteStagedFile created a file that was not in the release")
	}
	if _, _, err := store.WriteStagedFile(staging, "../escape.html", []byte("x")); err == nil {
		t.Fatalf("WriteStagedFile accepted a traversal path")
	}
	if _, _, err := store.WriteStagedFile(staging, "/etc/passwd", []byte("x")); err == nil {
		t.Fatalf("WriteStagedFile accepted an absolute path")
	}
	if _, _, err := store.WriteStagedFile(staging, "assets", []byte("x")); err == nil {
		t.Fatalf("WriteStagedFile accepted a directory as its target")
	}
}

func TestCopyForEditSkipsSymlinks(t *testing.T) {
	store, source, artifact := editFixture(t)
	if err := os.Symlink("/etc/passwd", filepath.Join(artifact, "escape.txt")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	staging, _, err := store.BeginStaging(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("BeginStaging: %v", err)
	}
	stats, err := store.CopyForEdit(source, staging, EditLimits{MaxBytes: 1 << 20, MaxFiles: 100})
	if err != nil {
		t.Fatalf("CopyForEdit: %v", err)
	}
	if stats.Files != 2 {
		t.Fatalf("copied %d files, want 2 (the link must not be copied)", stats.Files)
	}
	if _, err := os.Lstat(filepath.Join(staging, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("the symlink was copied into the staging tree")
	}
}

func TestCopyForEditEnforcesLimits(t *testing.T) {
	store, source, _ := editFixture(t)
	staging, _, err := store.BeginStaging(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("BeginStaging: %v", err)
	}
	if _, err := store.CopyForEdit(source, staging, EditLimits{MaxBytes: 1, MaxFiles: 100}); err == nil {
		t.Fatalf("CopyForEdit accepted a release larger than its byte limit")
	}
	if _, err := store.CopyForEdit(source, staging, EditLimits{MaxBytes: 1 << 20, MaxFiles: 1}); err == nil {
		t.Fatalf("CopyForEdit accepted a release larger than its file limit")
	}
}

func TestDigestArtifactIsDeterministicAndContentSensitive(t *testing.T) {
	store, source, _ := editFixture(t)
	staging, _, err := store.BeginStaging(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	if err != nil {
		t.Fatalf("BeginStaging: %v", err)
	}
	if _, err := store.CopyForEdit(source, staging, EditLimits{MaxBytes: 1 << 20, MaxFiles: 100}); err != nil {
		t.Fatalf("CopyForEdit: %v", err)
	}

	first, err := store.DigestArtifact(staging)
	if err != nil {
		t.Fatalf("DigestArtifact: %v", err)
	}
	again, err := store.DigestArtifact(staging)
	if err != nil {
		t.Fatalf("DigestArtifact: %v", err)
	}
	if first != again {
		t.Fatalf("digest is not deterministic: %s vs %s", first, again)
	}
	if len(first) != 64 || strings.Trim(first, "0123456789abcdef") != "" {
		t.Fatalf("digest is not a hex SHA-256: %q", first)
	}

	if _, _, err := store.WriteStagedFile(staging, "assets/app.css", []byte("body{color:red}")); err != nil {
		t.Fatalf("WriteStagedFile: %v", err)
	}
	changed, err := store.DigestArtifact(staging)
	if err != nil {
		t.Fatalf("DigestArtifact: %v", err)
	}
	if changed == first {
		t.Fatalf("digest did not change with the contents")
	}
}
