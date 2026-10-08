package installengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type rollbackTestRunner struct {
	mu          sync.Mutex
	calls       []recordedCommand
	fingerprint string
	services    []string
}

func (r *rollbackTestRunner) Run(_ context.Context, _ string, _, _ io.Writer, name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCommand{name: name, args: append([]string(nil), args...)})
	return nil
}

func (r *rollbackTestRunner) RunWithEnv(_ context.Context, _ string, env []string, _, _ io.Writer, name string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCommand{name: name, args: append([]string(nil), args...), env: append([]string(nil), env...)})
	return nil
}

func (r *rollbackTestRunner) Output(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCommand{name: name, args: append([]string(nil), args...)})
	if contains(args, "schema-fingerprint") {
		return []byte(r.fingerprint + "\n"), nil
	}
	if contains(args, "--services") {
		return []byte(strings.Join(r.services, "\n") + "\n"), nil
	}
	if contains(args, "ps") {
		entries := make([]rollbackComposeStatus, 0, len(r.services))
		for _, service := range r.services {
			entries = append(entries, rollbackComposeStatus{Service: service, State: "running", Health: "healthy"})
		}
		return json.Marshal(entries)
	}
	return nil, fmt.Errorf("unexpected Docker output command: %v", args)
}

func (r *rollbackTestRunner) snapshot() []recordedCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedCommand(nil), r.calls...)
}

type rollbackTestFixture struct {
	layout      Layout
	plan        Plan
	activeFiles map[string][]byte
	oldFiles    map[string][]byte
	runner      *rollbackTestRunner
	server      *httptest.Server
	fingerprint string
}

func newRollbackTestFixture(t *testing.T, currentFingerprint string) rollbackTestFixture {
	t.Helper()
	const currentVersion = "v1.2.3"
	const previousVersion = "v1.2.2"
	layout := writeEngineFixture(t, false)
	if err := os.MkdirAll(layout.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config, err := GenerateConfig(ConfigOptions{
		Version: currentVersion, PublicURL: "https://stealth.example.test", GitHubAppClientID: "Iv1.rollback-test",
		InstallRoot: layout.Root, AppsBuildKitAppArmorProfile: "unconfined",
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := ParseEnvContents(config)
	if err != nil {
		t.Fatal(err)
	}
	values["POSTGRES_PASSWORD"] = "rotated-postgres-secret"
	values["FUNCTIONS_SECRET_KEY"] = "rotated-functions-secret"
	values["APPS_SECRET_KEY"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x53}, 32))
	values["BOOTSTRAP_CLI_KEY"] = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x71}, 32))
	if err := WritePrivateFile(layout.EnvFile, FormatEnvFile(values)); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(layout.VersionFile, []byte(currentVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousDir := filepath.Join(layout.StateDir, "managed-assets.previous")
	if err := os.Mkdir(previousDir, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		Layout: layout, Version: currentVersion, InstalledVersion: currentVersion,
		Existing: true, CLIVersion: "v1.2.3", DockerGID: uint32(os.Getgid()),
		InternalAPIURL: "http://127.0.0.1", InternalConsoleURL: "http://127.0.0.1", InternalProxyURL: "http://127.0.0.1",
	}
	engine := New(Options{})
	assets := engine.managedAssetSpecs(plan)
	metadata := managedReleaseMetadata{
		FormatVersion: 1, PreviousVersion: previousVersion, TargetVersion: currentVersion,
		SchemaFingerprint: currentFingerprint, Assets: make([]managedAssetRecoveryEntry, 0, len(assets)),
	}
	activeFiles := make(map[string][]byte, len(assets))
	oldFiles := make(map[string][]byte, len(assets))
	for _, spec := range assets {
		previous := []byte("previous release asset: " + spec.Path + "\n")
		active := []byte("current release asset: " + spec.Path + "\n")
		previousPath := filepath.Join(previousDir, filepath.FromSlash(spec.Path))
		activePath := filepath.Join(layout.Root, filepath.FromSlash(spec.Path))
		if err := WriteAtomic(previousPath, previous, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteAtomic(activePath, active, 0o644); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(previous)
		metadata.Assets = append(metadata.Assets, managedAssetRecoveryEntry{
			RelativePath: spec.Path, Existed: true, SHA256: fmt.Sprintf("%x", digest),
		})
		activeFiles[spec.Path] = active
		oldFiles[spec.Path] = previous
	}
	if err := writeManagedReleaseMetadata(previousDir, metadata); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/version" {
			_, _ = io.WriteString(w, `{"version":"v1.2.2"}`)
		}
	}))
	plan.InternalAPIURL = server.URL
	plan.InternalConsoleURL = server.URL
	plan.InternalProxyURL = server.URL
	services := []string{
		"api", "worker", "buildkit", "console", "proxy", "traefik", "postgres", "redis",
		"traefik-state-init",
		"cloudflare-setup-state-init", "cloudflare-state-init", "buildkit-worker-credentials-init", "buildkit-server-credentials-init",
	}
	runner := &rollbackTestRunner{fingerprint: currentFingerprint, services: services}
	return rollbackTestFixture{
		layout: layout, plan: plan, activeFiles: activeFiles, oldFiles: oldFiles,
		runner: runner, server: server, fingerprint: currentFingerprint,
	}
}

func TestPlatformRollbackRestoresPreviousReleaseAndPreservesSecrets(t *testing.T) {
	fingerprint := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixture := newRollbackTestFixture(t, fingerprint)
	engine := New(Options{Runner: fixture.runner, PollAttempts: 1})
	if err := engine.Rollback(context.Background(), fixture.plan); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if got, err := os.ReadFile(fixture.layout.VersionFile); err != nil || string(got) != "v1.2.2\n" {
		t.Fatalf("rolled-back VERSION = %q, %v", got, err)
	}
	for path, want := range fixture.oldFiles {
		got, err := os.ReadFile(filepath.Join(fixture.layout.Root, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("restored asset %q = %q, %v; want %q", path, got, err, want)
		}
	}
	values, err := ReadEnvFile(fixture.layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"POSTGRES_PASSWORD":    "rotated-postgres-secret",
		"FUNCTIONS_SECRET_KEY": "rotated-functions-secret",
		"APPS_SECRET_KEY":      base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x53}, 32)),
		"BOOTSTRAP_CLI_KEY":    base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x71}, 32)),
		"STEALTH_API_IMAGE":    ImageName("stealth-api", "v1.2.2"),
		"STEALTH_WORKER_IMAGE": ImageName("stealth-worker", "v1.2.2"),
	} {
		if values[key] != want {
			t.Errorf("rollback config %s = %q, want %q", key, values[key], want)
		}
	}
	stateContents, err := os.ReadFile(filepath.Join(fixture.layout.StateDir, platformRollbackState))
	if err != nil {
		t.Fatalf("rollback status was not recorded: %v", err)
	}
	var state struct {
		Platform string `json:"platform_version"`
		CLI      string `json:"cli_version"`
	}
	if err := json.Unmarshal(stateContents, &state); err != nil || state.Platform != "v1.2.2" || state.CLI != fixture.plan.CLIVersion {
		t.Fatalf("rollback status = %#v, %v", state, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, platformRollbackPending)); !os.IsNotExist(err) {
		t.Fatalf("successful rollback left pending state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, platformRollbackAudit)); err != nil {
		t.Fatalf("rollback audit was not written: %v", err)
	}
	calls := fixture.runner.snapshot()
	for _, call := range calls {
		if contains(call.args, "migrate") && !contains(call.args, "schema-fingerprint") {
			t.Errorf("platform rollback unexpectedly invoked a migration service: %#v", call)
		}
	}
}

func TestPlatformRollbackRefusesWhenSchemaLedgerAdvanced(t *testing.T) {
	expected := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixture := newRollbackTestFixture(t, expected)
	fixture.runner.fingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	beforeEnv, err := os.ReadFile(fixture.layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(Options{Runner: fixture.runner})
	err = engine.Rollback(context.Background(), fixture.plan)
	if err == nil || !strings.Contains(err.Error(), "migration ledger has advanced") {
		t.Fatalf("Rollback() error = %v, want schema compatibility refusal", err)
	}
	if got, err := os.ReadFile(fixture.layout.VersionFile); err != nil || string(got) != "v1.2.3\n" {
		t.Fatalf("refused rollback changed VERSION: %q, %v", got, err)
	}
	if got, err := os.ReadFile(fixture.layout.EnvFile); err != nil || !bytes.Equal(got, beforeEnv) {
		t.Fatalf("refused rollback changed config.env: %q, %v", got, err)
	}
	for path, want := range fixture.activeFiles {
		got, err := os.ReadFile(filepath.Join(fixture.layout.Root, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("refused rollback changed managed asset %q: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, platformRollbackPending)); !os.IsNotExist(err) {
		t.Fatalf("refused rollback left pending state: %v", err)
	}
	if len(fixture.runner.snapshot()) != 1 {
		t.Fatalf("schema-incompatible rollback performed additional Docker commands: %#v", fixture.runner.snapshot())
	}
}

func TestPlatformRollbackRefusesWhenNoOlderReleaseIsSaved(t *testing.T) {
	fingerprint := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixture := newRollbackTestFixture(t, fingerprint)
	engine := New(Options{Runner: fixture.runner})
	allowed := make(map[string]struct{})
	for _, spec := range engine.managedAssetSpecs(fixture.plan) {
		allowed[spec.Path] = struct{}{}
	}
	metadata, err := readManagedReleaseMetadata(fixture.layout, fixture.plan.Version, allowed)
	if err != nil {
		t.Fatal(err)
	}
	metadata.PreviousVersion = metadata.TargetVersion
	if err := writeManagedReleaseMetadata(filepath.Join(fixture.layout.StateDir, "managed-assets.previous"), metadata); err != nil {
		t.Fatal(err)
	}
	if err := engine.Rollback(context.Background(), fixture.plan); err == nil || !strings.Contains(err.Error(), "distinct older release") {
		t.Fatalf("Rollback() error = %v, want missing older release refusal", err)
	}
	if len(fixture.runner.snapshot()) != 0 {
		t.Fatalf("rollback without an older release invoked Docker: %#v", fixture.runner.snapshot())
	}
}

func TestPlatformRollbackRetriesAfterInterruptedFileTransaction(t *testing.T) {
	fingerprint := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixture := newRollbackTestFixture(t, fingerprint)
	first := New(Options{
		Runner: fixture.runner, PollAttempts: 1,
		MigrationHook: func(event MigrationEvent) error {
			if event.Phase == migrationPhaseVersionActivated {
				return errMigrationProcessInterrupted
			}
			return nil
		},
	})
	if err := first.Rollback(context.Background(), fixture.plan); !errors.Is(err, errMigrationProcessInterrupted) {
		t.Fatalf("injected rollback interruption = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, managedAssetPendingFile)); err != nil {
		t.Fatalf("interrupted file transaction did not leave a recovery journal: %v", err)
	}
	second := New(Options{Runner: fixture.runner, PollAttempts: 1})
	if err := second.Rollback(context.Background(), fixture.plan); err != nil {
		t.Fatalf("retry after file transaction recovery failed: %v", err)
	}
	if got, err := os.ReadFile(fixture.layout.VersionFile); err != nil || string(got) != "v1.2.2\n" {
		t.Fatalf("retried rollback VERSION = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, managedAssetPendingFile)); !os.IsNotExist(err) {
		t.Fatalf("retried rollback left asset journal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, platformRollbackPending)); !os.IsNotExist(err) {
		t.Fatalf("retried rollback left pending state: %v", err)
	}
}

func TestPlatformRollbackResumesForwardAfterComposeValidation(t *testing.T) {
	fingerprint := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixture := newRollbackTestFixture(t, fingerprint)
	first := New(Options{
		Runner: fixture.runner, PollAttempts: 1,
		MigrationHook: func(event MigrationEvent) error {
			if event.Phase == migrationPhaseComposeValidated {
				return errMigrationProcessInterrupted
			}
			return nil
		},
	})
	if err := first.Rollback(context.Background(), fixture.plan); !errors.Is(err, errMigrationProcessInterrupted) {
		t.Fatalf("injected post-validation interruption = %v", err)
	}
	if got, err := os.ReadFile(fixture.layout.VersionFile); err != nil || string(got) != "v1.2.2\n" {
		t.Fatalf("Compose-validated rollback version = %q, %v", got, err)
	}
	second := New(Options{Runner: fixture.runner, PollAttempts: 1})
	if err := second.Rollback(context.Background(), fixture.plan); err != nil {
		t.Fatalf("retry did not complete the validated rollback: %v", err)
	}
	for path, want := range fixture.oldFiles {
		got, err := os.ReadFile(filepath.Join(fixture.layout.Root, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("resumed managed asset %q = %q, %v; want %q", path, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, managedAssetPendingFile)); !os.IsNotExist(err) {
		t.Fatalf("resumed validated rollback left an asset journal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.layout.StateDir, platformRollbackPending)); !os.IsNotExist(err) {
		t.Fatalf("resumed validated rollback left pending state: %v", err)
	}
}

func TestSameVersionRepairKeepsTheLastDistinctReleaseSnapshot(t *testing.T) {
	layout, err := NewLayout(filepath.Join(t.TempDir(), "stealth"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(layout.StateDir, "managed-assets.previous")
	if err := os.Mkdir(previous, 0o700); err != nil {
		t.Fatal(err)
	}
	previousMarker := filepath.Join(previous, "previous-release-marker")
	if err := WritePrivateFile(previousMarker, "v1.2.2 recovery set"); err != nil {
		t.Fatal(err)
	}
	backupName := ".stealth-managed-backup-repair"
	stageName := ".stealth-managed-assets-repair"
	backup := filepath.Join(layout.StateDir, backupName)
	stage := filepath.Join(layout.StateDir, stageName)
	if err := os.Mkdir(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateFile(filepath.Join(backup, "config.env"), "current repair snapshot"); err != nil {
		t.Fatal(err)
	}
	manifest := managedAssetRecoveryManifest{
		OriginalVersion: "v1.2.3", TargetVersion: "v1.2.3", BackupDir: backupName, StageDir: stageName,
	}
	if err := publishManagedAssetRecoverySet(layout, manifest); err != nil {
		t.Fatalf("publishManagedAssetRecoverySet() error = %v", err)
	}
	if got, err := os.ReadFile(previousMarker); err != nil || string(got) != "v1.2.2 recovery set" {
		t.Fatalf("same-version repair replaced the prior release snapshot: %q, %v", got, err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("same-version repair recovery set was not discarded: %v", err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("same-version repair stage was not removed: %v", err)
	}
}

func TestSameVersionRepairDoesNotRefreshAnOlderSchemaSnapshot(t *testing.T) {
	expected := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixture := newRollbackTestFixture(t, expected)
	fixture.runner.fingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	engine := New(Options{Runner: fixture.runner})
	if err := engine.RunStep(context.Background(), fixture.plan, StepMigration); err != nil {
		t.Fatalf("same-version repair migration step failed: %v", err)
	}
	allowed := make(map[string]struct{})
	for _, spec := range engine.managedAssetSpecs(fixture.plan) {
		allowed[spec.Path] = struct{}{}
	}
	metadata, err := readManagedReleaseMetadata(fixture.layout, fixture.plan.Version, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.SchemaFingerprint != expected {
		t.Fatalf("repair changed previous schema snapshot to %q, want %q", metadata.SchemaFingerprint, expected)
	}
	for _, call := range fixture.runner.snapshot() {
		if contains(call.args, "schema-fingerprint") {
			t.Fatalf("same-version repair refreshed the previous schema snapshot: %#v", call)
		}
	}
}
