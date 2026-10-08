package installengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ManagedAsset identifies a release-owned runtime file. The manifest is
// compiled into the target release binary; callers cannot supply it through a
// command-line flag or a downloaded manifest.
type ManagedAsset struct {
	Path       string
	RemotePath string
	Marker     string

	setupOnly      bool
	productionOnly bool
	validate       func([]byte) error
	render         func([]byte, Plan) ([]byte, error)
}

// DefaultManagedAssets is this release's complete production runtime manifest.
// New releases extend this list in their own binary, which is why update
// invokes the verified target binary before replacing the installed CLI.
func DefaultManagedAssets() []ManagedAsset {
	return []ManagedAsset{
		{Path: "compose.production.yaml", RemotePath: "compose.production.yaml", Marker: "services:", validate: validateProductionComposeAsset},
		{Path: "buildkit/buildkitd.toml", RemotePath: "buildkit/buildkitd.toml", Marker: "rootless = true", productionOnly: true, validate: validateBuildKitConfigAsset},
		{Path: "buildkit/stealth-buildkit-rootless.apparmor", RemotePath: "buildkit/stealth-buildkit-rootless.apparmor", Marker: BuildKitAppArmorProfileName, productionOnly: true, validate: validateBuildKitAppArmorProfileAsset},
		{Path: "console/deploy/nginx.conf", RemotePath: "console/deploy/nginx.conf", Marker: "server {"},
		{Path: "traefik/traefik.yaml", RemotePath: "traefik/traefik.yaml", Marker: "entryPoints:", productionOnly: true, render: renderTraefikStaticAsset, validate: validateTraefikStaticAsset},
		{Path: "traefik/dynamic/core.yaml", RemotePath: "traefik/dynamic/core.yaml", Marker: "__STEALTH_PUBLIC_HOST__", productionOnly: true, render: renderTraefikCoreAsset, validate: validateTraefikCoreAsset},
		{Path: "traefik/dynamic/generated/.gitkeep", RemotePath: "traefik/dynamic/generated/.gitkeep", Marker: "Stealth route reconciler", productionOnly: true},
		{Path: "compose.setup.yaml", RemotePath: "compose.setup.yaml", Marker: "services:", setupOnly: true},
	}
}
func (e *Engine) managedAssetSpecs(plan Plan) []ManagedAsset {
	assets := make([]ManagedAsset, 0, len(e.managedAssets))
	for _, asset := range e.managedAssets {
		if asset.productionOnly && plan.Setup {
			continue
		}
		if asset.setupOnly && (plan.Layout.SetupComposeFile == "" || (!plan.Setup && !(plan.Existing && FileExists(plan.Layout.SetupComposeFile)))) {
			continue
		}
		assets = append(assets, asset)
	}
	return assets
}
func (e *Engine) stageManagedAssets(ctx context.Context, plan Plan) (*managedAssetMigration, error) {
	assets := e.managedAssetSpecs(plan)
	allowedPaths := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		if !validManagedAssetPath(asset.Path) {
			return nil, fmt.Errorf("managed asset path %q is invalid", asset.Path)
		}
		allowedPaths[asset.Path] = struct{}{}
	}
	if err := recoverInterruptedManagedAssetMigration(plan.Layout, allowedPaths); err != nil {
		return nil, fmt.Errorf("recover interrupted managed asset migration: %w", err)
	}
	checksumURL := e.releaseAssetBaseURL + "/" + strings.TrimSpace(plan.Version) + "/checksums.txt"
	checksumContents, err := e.fetchAsset(ctx, checksumURL)
	if err != nil {
		return nil, fmt.Errorf("download managed asset checksum manifest for %s: %w", strings.TrimSpace(plan.Version), err)
	}
	checksums, err := parseChecksumsManifest(checksumContents)
	if err != nil {
		return nil, fmt.Errorf("parse managed asset checksum manifest for %s: %w", strings.TrimSpace(plan.Version), err)
	}
	stageDir, err := os.MkdirTemp(plan.Layout.StateDir, ".stealth-managed-assets-")
	if err != nil {
		return nil, fmt.Errorf("create managed asset staging directory: %w", err)
	}
	migration := &managedAssetMigration{
		layout:       plan.Layout,
		stageDir:     stageDir,
		allowedPaths: allowedPaths,
		hook:         e.migrationHook,
	}
	cleanup := func() {
		_ = os.RemoveAll(stageDir)
	}
	for _, spec := range assets {
		contents, err := e.fetchAsset(ctx, e.assetBaseURL+"/"+strings.TrimSpace(plan.Version)+"/"+spec.RemotePath)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("download managed asset %q: %w", spec.RemotePath, err)
		}
		if err := verifyManagedAssetChecksum(contents, spec.RemotePath, checksums); err != nil {
			cleanup()
			return nil, err
		}
		if !bytes.Contains(contents, []byte(spec.Marker)) {
			cleanup()
			return nil, fmt.Errorf("downloaded asset %q is invalid", spec.RemotePath)
		}
		if spec.render != nil {
			contents, err = spec.render(contents, plan)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("render managed asset %q: %w", spec.RemotePath, err)
			}
		}
		if spec.validate != nil {
			if err := spec.validate(contents); err != nil {
				cleanup()
				return nil, fmt.Errorf("downloaded asset %q failed validation: %w", spec.RemotePath, err)
			}
		}
		stagePath := filepath.Join(stageDir, filepath.FromSlash(spec.Path))
		if err := WriteAtomic(stagePath, contents, 0o644); err != nil {
			cleanup()
			return nil, fmt.Errorf("stage managed asset %q: %w", spec.RemotePath, err)
		}
		migration.assets = append(migration.assets, stagedManagedAsset{
			spec:       spec,
			targetPath: filepath.Join(plan.Layout.Root, filepath.FromSlash(spec.Path)),
			stagePath:  stagePath,
		})
	}
	return migration, nil
}
