package installengine

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestManagedAssetUpgradeSmoke is driven by scripts/managed-asset-upgrade-smoke.sh
// in the production Compose workflow. It starts from a deliberately pre-PR-83
// installation root, activates target assets through the real install engine,
// validates the rendered target Compose file, then hands that same root to the
// end-to-end telemetry smoke. Normal unit-test runs skip it without Docker.
func TestManagedAssetUpgradeSmoke(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("STEALTH_UPGRADE_SMOKE_ROOT"))
	assetBase := strings.TrimSpace(os.Getenv("STEALTH_UPGRADE_SMOKE_ASSET_BASE"))
	targetVersion := strings.TrimSpace(os.Getenv("STEALTH_UPGRADE_SMOKE_VERSION"))
	if root == "" && assetBase == "" && targetVersion == "" {
		t.Skip("managed asset upgrade smoke is enabled by its Docker workflow")
	}
	if root == "" || assetBase == "" || targetVersion == "" {
		t.Fatal("managed asset upgrade smoke environment is incomplete")
	}
	layout, err := NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(Options{
		AssetBaseURL:        assetBase,
		ReleaseAssetBaseURL: assetBase,
		Runner:              OSCommandRunner{},
	})
	plan := Plan{Layout: layout, Version: targetVersion, InstalledVersion: "v0.2.5", Existing: true}
	if err := engine.RunStep(context.Background(), plan, StepConfiguration); err != nil {
		t.Fatalf("upgrade configuration migration: %v", err)
	}
	assertUpgradeSmokeTopology(t, layout)
	values, err := ReadEnvFile(layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	if values["UPGRADE_SECRET_MARKER"] != "upgrade-secret-must-survive" {
		t.Fatal("upgrade migration changed the operator secret marker")
	}
	for _, key := range []string{
		"STEALTH_CONSOLE_IMAGE",
		"TRUSTED_PROXY_CIDRS",
		"STEALTH_INGRESS_NETWORK_NAME",
		"STEALTH_INGRESS_NETWORK_SUBNET",
		"STEALTH_INGRESS_IP_RANGE",
		"STEALTH_TRAEFIK_INGRESS_IP",
		"STEALTH_CLOUDFLARED_INGRESS_IP",
	} {
		if strings.TrimSpace(values[key]) == "" {
			t.Fatalf("upgrade migration did not add required config key %s", key)
		}
	}
	if !strings.Contains(values["TRUSTED_PROXY_CIDRS"], "172.31.0.254/32") {
		t.Fatal("upgrade migration did not trust the fixed Traefik peer")
	}
	if !FileIsPrivate(layout.EnvFile) {
		t.Fatal("upgrade migration did not retain config.env mode 0600")
	}
	version, err := os.ReadFile(layout.VersionFile)
	if err != nil || string(version) != targetVersion+"\n" {
		t.Fatalf("target VERSION = %q, %v", version, err)
	}
}

func assertUpgradeSmokeTopology(t *testing.T, layout Layout) {
	t.Helper()
	compose, err := os.ReadFile(layout.ComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(compose)
	for _, marker := range []string{"  traefik:", "  traefik-state-init:", "  cloudflare-setup-state-init:", "  cloudflare-state-init:", "  app_build:"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("migrated Compose misses %q", marker)
		}
	}
	for _, path := range []string{
		layout.TraefikStatic,
		layout.TraefikCore,
		layout.TraefikGenerated + "/.gitkeep",
	} {
		if !FileExists(path) {
			t.Fatalf("migrated asset is missing: %s", path)
		}
	}
	values, err := ReadEnvFile(layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(values["TRAEFIK_IMAGE"]) == "" || strings.TrimSpace(values["STEALTH_INGRESS_NETWORK_NAME"]) == "" {
		t.Fatal("migrated configuration is missing Traefik ingress settings")
	}
}
