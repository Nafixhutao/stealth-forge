package installengine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func (e *Engine) querySchemaFingerprint(ctx context.Context, plan Plan, noDependencies bool) (string, error) {
	args := []string{"compose"}
	if plan.Cloudflare {
		args = append(args, "--profile", "cloudflare")
	}
	args = append(args, "--env-file", plan.Layout.EnvFile, "-f", plan.Layout.ComposeFile, "run", "--rm", "-T")
	if noDependencies {
		args = append(args, "--no-deps")
	}
	args = append(args, "migrate", "schema-fingerprint")
	output, err := e.runner.Output(ctx, plan.Layout.Root, "docker", args...)
	if err != nil {
		return "", fmt.Errorf("run migration ledger fingerprint: %w", err)
	}
	var fingerprint string
	for _, field := range strings.Fields(string(output)) {
		if schemaFingerprintPattern.MatchString(field) {
			if fingerprint != "" {
				return "", errors.New("migration runner returned multiple schema fingerprints")
			}
			fingerprint = field
		}
	}
	if fingerprint == "" {
		return "", errors.New("migration runner returned no valid schema fingerprint")
	}
	return fingerprint, nil
}

func (e *Engine) recordUpgradeSchemaFingerprint(ctx context.Context, plan Plan) error {
	allowedPaths := make(map[string]struct{})
	for _, spec := range e.managedAssetSpecs(plan) {
		allowedPaths[spec.Path] = struct{}{}
	}
	metadata, err := readManagedReleaseMetadata(plan.Layout, strings.TrimSpace(plan.Version), allowedPaths)
	if err != nil {
		return fmt.Errorf("cannot snapshot schema compatibility before migration: %w", err)
	}
	if metadata.TargetVersion != strings.TrimSpace(plan.Version) ||
		(strings.TrimSpace(plan.InstalledVersion) != "" && metadata.PreviousVersion != strings.TrimSpace(plan.InstalledVersion)) {
		return errors.New("previous release metadata does not match the upgrade plan")
	}
	fingerprint, err := e.querySchemaFingerprint(ctx, plan, plan.ExternalDatabase)
	if err != nil {
		return fmt.Errorf("snapshot database schema before migration: %w", err)
	}
	metadata.SchemaFingerprint = fingerprint
	if err := writeManagedReleaseMetadata(filepath.Join(plan.Layout.StateDir, "managed-assets.previous"), metadata); err != nil {
		return fmt.Errorf("persist previous release schema snapshot: %w", err)
	}
	return nil
}
