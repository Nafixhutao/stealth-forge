package sitestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// A published artifact is untrusted input that was extracted from an upload, so
// the listing must describe exactly the regular files inside it: no symlink
// targets, no directories, and a stable order the UI can render.
func TestListFilesReturnsRegularFilesSorted(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	projectID := uuid.Must(uuid.NewV7())
	siteID := uuid.Must(uuid.NewV7())
	deploymentID := uuid.Must(uuid.NewV7())
	relative, err := ArtifactRelativePath(projectID, siteID, deploymentID)
	if err != nil {
		t.Fatalf("ArtifactRelativePath: %v", err)
	}

	artifact := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Join(artifact, "assets", "img"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{
		"index.html":          "<!doctype html>",
		"assets/app.css":      "body{}",
		"assets/img/logo.svg": "<svg/>",
	} {
		path := filepath.Join(artifact, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	// A link must never be reported: its target is outside this release.
	if err := os.Symlink("/etc/passwd", filepath.Join(artifact, "escape.txt")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	files, truncated, err := store.ListFiles(relative, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if truncated {
		t.Fatalf("truncated = true, want false")
	}

	want := []string{"assets/app.css", "assets/img/logo.svg", "index.html"}
	if len(files) != len(want) {
		t.Fatalf("got %d files (%v), want %d", len(files), files, len(want))
	}
	for index, path := range want {
		if files[index].Path != path {
			t.Fatalf("files[%d] = %q, want %q", index, files[index].Path, path)
		}
	}
	if files[2].Size != int64(len("<!doctype html>")) {
		t.Fatalf("index.html size = %d, want %d", files[2].Size, len("<!doctype html>"))
	}
}

func TestListFilesRespectsLimit(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	projectID := uuid.Must(uuid.NewV7())
	siteID := uuid.Must(uuid.NewV7())
	deploymentID := uuid.Must(uuid.NewV7())
	relative, err := ArtifactRelativePath(projectID, siteID, deploymentID)
	if err != nil {
		t.Fatalf("ArtifactRelativePath: %v", err)
	}
	artifact := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(artifact, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"a.html", "b.html", "c.html"} {
		if err := os.WriteFile(filepath.Join(artifact, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	files, truncated, err := store.ListFiles(relative, 2)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if !truncated {
		t.Fatalf("truncated = false, want true")
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
}

func TestListFilesRejectsPathOutsideAnArtifact(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, _, err := store.ListFiles("not/an/artifact", 0); err == nil {
		t.Fatalf("ListFiles accepted a non-artifact path")
	}
}
