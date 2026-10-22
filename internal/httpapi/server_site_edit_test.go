package httpapi

import (
	"strings"
	"testing"
)

// The release's source_name column is a file name, not a path: the schema
// rejects "/" and "\". An edit of a nested file therefore cannot be recorded
// verbatim, and getting this wrong failed the insert after the artifact had
// already been published.
func TestEditSourceNameIsAFileName(t *testing.T) {
	for _, path := range []string{
		"index.html",
		"assets/app.css",
		"a/b/c/deep.js",
		"file with spaces.html",
	} {
		name := editSourceName(path)
		if strings.ContainsAny(name, `/\`) {
			t.Fatalf("editSourceName(%q) = %q, which the schema rejects", path, name)
		}
		if name == "" || name == "." || name == ".." {
			t.Fatalf("editSourceName(%q) = %q, which the schema rejects", path, name)
		}
		if name != strings.TrimSpace(name) {
			t.Fatalf("editSourceName(%q) = %q, which the schema rejects", path, name)
		}
		if len(name) > 255 {
			t.Fatalf("editSourceName(%q) is %d bytes, over the column's limit", path, len(name))
		}
	}

	if got := editSourceName("assets/app.css"); got != "edit: app.css" {
		t.Fatalf("editSourceName = %q, want %q", got, "edit: app.css")
	}

	// A path at the edge of the endpoint's own limit still fits the column.
	long := strings.Repeat("x", 1024)
	if name := editSourceName(long); len(name) > 255 {
		t.Fatalf("long path produced %d bytes", len(name))
	}
}
