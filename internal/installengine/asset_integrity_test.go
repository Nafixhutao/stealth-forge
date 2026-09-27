package installengine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseChecksumsManifestAndVerifyManagedAsset(t *testing.T) {
	contents := []byte("release-managed-asset\n")
	digest := sha256.Sum256(contents)
	manifest, err := parseChecksumsManifest([]byte(fmt.Sprintf("%x  compose.production.yaml\n", digest)))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyManagedAssetChecksum(contents, "compose.production.yaml", manifest); err != nil {
		t.Fatalf("verify managed asset: %v", err)
	}
	if err := verifyManagedAssetChecksum([]byte("changed\n"), "compose.production.yaml", manifest); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
	if err := verifyManagedAssetChecksum(contents, "missing.yaml", manifest); err == nil {
		t.Fatal("asset without a checksum was accepted")
	}
}

func TestParseChecksumsManifestRejectsMalformedEntries(t *testing.T) {
	digest := strings.Repeat("a", sha256.Size*2)
	for name, contents := range map[string]string{
		"empty":       "\n",
		"bad digest":  "xyz  file.yaml\n",
		"duplicate":   digest + "  file.yaml\n" + digest + "  file.yaml\n",
		"traversal":   digest + "  ../outside.yaml\n",
		"absolute":    digest + "  /outside.yaml\n",
		"extra field": digest + "  file.yaml unexpected\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseChecksumsManifest([]byte(contents)); err == nil {
				t.Fatalf("malformed checksum manifest was accepted: %q", contents)
			}
		})
	}
}

func TestReleaseManagedAssetManifestMatchesEngine(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate installengine source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	manifestPath := filepath.Join(repoRoot, "release-managed-assets.txt")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, line := range strings.Split(string(contents), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			listed = append(listed, line)
		}
	}
	assets := DefaultManagedAssets()
	managed := make([]string, 0, len(assets))
	for _, asset := range assets {
		managed = append(managed, asset.RemotePath)
	}
	if strings.Join(listed, "\n") != strings.Join(managed, "\n") {
		t.Fatalf("release-managed-assets.txt = %v, engine manifest = %v", listed, managed)
	}
	workflow, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"sha256sum \"$managed_asset\" >> dist/checksums.txt", "done < release-managed-assets.txt"} {
		if !strings.Contains(string(workflow), required) {
			t.Fatalf("release workflow does not generate the managed asset checksum manifest: missing %q", required)
		}
	}
}
