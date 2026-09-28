package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Stealth-deplover/stealth/internal/buildkitpki"
	"github.com/Stealth-deplover/stealth/internal/installengine"
)

type doctorCommandRunner struct {
	statuses        []byte
	statusResponses [][]byte
	statusCalls     int
	composeErr      error
	volumeErr       error
	args            [][]string
}

func (r *doctorCommandRunner) Run(context.Context, string, io.Writer, io.Writer, string, ...string) error {
	return nil
}

func (r *doctorCommandRunner) Output(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	r.args = append(r.args, append([]string{name}, args...))
	joined := strings.Join(args, " ")
	switch {
	case name == "docker" && joined == "version --format {{.Server.Version}}":
		return []byte("27.4.0\n"), nil
	case name == "docker" && joined == "compose version --short":
		return []byte("2.39.0\n"), nil
	case name == "docker" && strings.Contains(joined, " ps --all --format json"):
		if len(r.statusResponses) > 0 {
			index := r.statusCalls
			if index >= len(r.statusResponses) {
				index = len(r.statusResponses) - 1
			}
			r.statusCalls++
			return r.statusResponses[index], nil
		}
		r.statusCalls++
		return r.statuses, nil
	case name == "docker" && strings.Contains(joined, " config --quiet"):
		return nil, r.composeErr
	case name == "docker" && strings.Contains(joined, "network inspect"):
		return []byte(`{"Name":"stealth_app_runtime","Driver":"bridge","Scope":"local","Internal":false,"Labels":{"stealth.managed":"true","stealth.resource_type":"app_runtime_network","stealth.runtime_schema":"v1"}}`), nil
	case name == "docker" && strings.Contains(joined, "volume inspect"):
		return []byte("local\n"), r.volumeErr
	default:
		return nil, fmt.Errorf("unexpected command: %s %s", name, joined)
	}
}

func (r *doctorCommandRunner) CombinedOutput(context.Context, string, string, ...string) ([]byte, error) {
	return nil, errors.New("unexpected command")
}

func doctorFixture(t *testing.T, setupMode bool, extra map[string]string) (InstallLayout, map[string]string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".stealth")
	layout, err := installengine.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"VERSION":                              "v1.2.3",
		"SETUP_MODE":                           fmt.Sprint(setupMode),
		"STEALTH_API_IMAGE":                    "stealth-api:v1.2.3",
		"STEALTH_WORKER_IMAGE":                 "stealth-worker:v1.2.3",
		"STEALTH_INGRESS_CONTROL_IMAGE":        "stealth-ingress-control:v1.2.3",
		"STEALTH_MIGRATE_IMAGE":                "stealth-migrate:v1.2.3",
		"STEALTH_CONSOLE_IMAGE":                "stealth-console:v1.2.3",
		"STEALTH_TELEMETRY_DOCKER_PROXY_IMAGE": "stealth-telemetry-docker-proxy:v1.2.3",
		"POSTGRES_DB":                          "stealth",
		"POSTGRES_USER":                        "stealth",
		"POSTGRES_PASSWORD":                    "db-private-sentinel",
		"REDIS_PASSWORD":                       "redis-private-sentinel",
		"FUNCTIONS_SECRET_KEY":                 "functions-private-sentinel",
		"BOOTSTRAP_CLI_KEY":                    "bootstrap-private-sentinel",
		"PUBLIC_APP_URL":                       "http://127.0.0.1:8080",
		"GITHUB_APP_CLIENT_ID":                 "smoke-client",
		"DOCKER_GID":                           "999",
		"DATABASE_URL":                         "postgres://user:password@postgres:5432/stealth",
		"REDIS_URL":                            "redis://:password@redis:6379/0",
		"STORAGE_DRIVER":                       "local",
		"STORAGE_VOLUME_NAME":                  "stealth_storage",
		"APPS_RUNTIME_NETWORK_NAME":            "stealth_app_runtime",
		"API_HOST_PORT":                        "18080",
		"CONSOLE_HOST_PORT":                    "13000",
		"PROXY_HTTP_PORT":                      "8080",
		"SETUP_API_HOST_PORT":                  "18081",
		"SETUP_CONSOLE_HOST_PORT":              "13001",
		"SETUP_PROXY_HTTP_PORT":                "8081",
	}
	if setupMode {
		values["STEALTH_SETUP_IMAGE"] = "stealth-setup:v1.2.3"
	}
	for key, value := range extra {
		values[key] = value
	}
	if err := installengine.WritePrivateFile(layout.EnvFile, installengine.FormatEnvFile(values)); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{layout.ComposeFile, layout.SetupComposeFile} {
		if err := os.WriteFile(file, []byte("services:\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(layout.VersionFile, []byte("v1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !setupMode {
		if _, err := buildkitpki.Ensure(layout.BuildKitPKIDir); err != nil {
			t.Fatal(err)
		}
	}
	return layout, values
}

func healthyDoctorStatuses(t *testing.T, config map[string]string, setupMode bool) []byte {
	t.Helper()
	entries := make([]composeStatusJSON, 0)
	for _, target := range statusServiceTargets(config, setupMode) {
		if target.external {
			continue
		}
		entry := composeStatusJSON{Service: target.name, State: "running", Health: "healthy"}
		if target.name == "migrate" {
			entry.State = "exited"
			entry.Health = ""
			entry.ExitCode = 0
		}
		if target.name == "cloudflared" {
			entry.Health = ""
		}
		entries = append(entries, entry)
	}
	contents, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func TestWaitForRequiredServicesRetriesAndReturnsSafeBoundedFailure(t *testing.T) {
	layout, config := doctorFixture(t, false, nil)
	healthy := healthyDoctorStatuses(t, config, false)
	var entries []composeStatusJSON
	if err := json.Unmarshal(healthy, &entries); err != nil {
		t.Fatal(err)
	}
	for index := range entries {
		if entries[index].Service == "worker" || entries[index].Service == "buildkit" {
			entries[index].Health = "starting"
			entries[index].Status = "Up 20 seconds private-status-sentinel"
		}
	}
	starting, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}

	runner := &doctorCommandRunner{statusResponses: [][]byte{starting, healthy}}
	app := NewApp(strings.NewReader(""), io.Discard, io.Discard)
	app.runner = runner
	app.pollAttempts = 2
	app.pollInterval = time.Millisecond
	if err := app.waitForRequiredServices(context.Background(), layout); err != nil {
		t.Fatalf("waitForRequiredServices() failed after services became healthy: %v", err)
	}
	if runner.statusCalls != 2 {
		t.Fatalf("Compose status was queried %d times, want 2", runner.statusCalls)
	}

	runner = &doctorCommandRunner{statusResponses: [][]byte{starting}}
	app.runner = runner
	app.pollAttempts = 1
	err = app.waitForRequiredServices(context.Background(), layout)
	if err == nil {
		t.Fatal("waitForRequiredServices() succeeded while required services were starting")
	}
	for _, want := range []string{"worker=starting", "buildkit=starting"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("readiness error %q does not include %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "private-status-sentinel") {
		t.Fatalf("readiness error exposed raw Compose status: %q", err)
	}
}

func doctorAppForTest(t *testing.T, layout InstallLayout, runner *doctorCommandRunner) (*App, *httptest.Server, *strings.Builder) {
	t.Helper()
	t.Setenv("STEALTH_INSTALL_DIR", layout.Root)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := parsed.Port()
	config, err := readEnvFile(layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	config["API_HOST_PORT"] = port
	config["CONSOLE_HOST_PORT"] = port
	config["PROXY_HTTP_PORT"] = port
	config["SETUP_API_HOST_PORT"] = port
	config["SETUP_CONSOLE_HOST_PORT"] = port
	config["SETUP_PROXY_HTTP_PORT"] = port
	if err := installengine.WritePrivateFile(layout.EnvFile, installengine.FormatEnvFile(config)); err != nil {
		t.Fatal(err)
	}
	output := &strings.Builder{}
	app := NewApp(strings.NewReader(""), output, io.Discard)
	app.runner = runner
	app.httpClient = server.Client()
	return app, server, output
}

func TestRunStatusIncludesControlPlaneBuildKitIngressAndTelemetry(t *testing.T) {
	layout, config := doctorFixture(t, false, nil)
	statuses := healthyDoctorStatuses(t, config, false)
	var entries []composeStatusJSON
	if err := json.Unmarshal(statuses, &entries); err != nil {
		t.Fatal(err)
	}
	for index := range entries {
		if entries[index].Service == "telemetry-docker-proxy" {
			entries[index].Health = "unhealthy"
		}
	}
	statuses, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	runner := &doctorCommandRunner{statuses: statuses}
	app, server, output := doctorAppForTest(t, layout, runner)
	defer server.Close()
	if code := app.runStatusCommand(nil); code != 1 {
		t.Fatalf("runStatusCommand() = %d, want 1", code)
	}
	for _, want := range []string{"App BuildKit", "ClickHouse", "Host Metrics", "Docker Logs", "Traefik", "Docker Metrics Proxy"} {
		if !strings.Contains(strings.ToLower(output.String()), strings.ToLower(want)) {
			t.Errorf("status output %q misses %q", output.String(), want)
		}
	}
	if !strings.Contains(output.String(), "Migration") || !strings.Contains(output.String(), "completed") {
		t.Errorf("successful migration is not reported as completed: %q", output.String())
	}
	if !hasArgs(runner.args, "--all", "--format", "json") {
		t.Errorf("status did not include stopped one-shot service results: %#v", runner.args)
	}
}

func TestRunDoctorChecksStorageBuildKitAndKeepsSecretsPrivate(t *testing.T) {
	extra := map[string]string{
		"STORAGE_DRIVER":        "s3",
		"STORAGE_S3_ENDPOINT":   "https://objects.example.test",
		"STORAGE_S3_BUCKET":     "artifacts",
		"STORAGE_S3_ACCESS_KEY": "access-secret-sentinel",
		"STORAGE_S3_SECRET_KEY": "secret-key-sentinel",
	}
	layout, config := doctorFixture(t, false, extra)
	runner := &doctorCommandRunner{statuses: healthyDoctorStatuses(t, config, false)}
	app, server, output := doctorAppForTest(t, layout, runner)
	defer server.Close()
	if code := app.runDoctorCommand(nil); code != 0 {
		t.Fatalf("runDoctorCommand() = %d; output=%s", code, output.String())
	}
	for _, want := range []string{"Compose validation", "BuildKit mTLS", "App BuildKit", "Storage config", "Storage volume", "App runtime", "API readiness", "Docker Metrics Proxy"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("doctor output %q misses %q", output.String(), want)
		}
	}
	for _, secret := range []string{"access-secret-sentinel", "secret-key-sentinel", "db-private-sentinel", "redis-private-sentinel", "functions-private-sentinel", "bootstrap-private-sentinel"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("doctor output leaked configured secret %q", secret)
		}
	}
	if hasArgs(runner.args, "--profile", "cloudflare") {
		t.Fatal("doctor enabled Cloudflare profile when it is not configured")
	}
}

func TestRunDoctorFailsForInvalidComposeBuildKitAndStorage(t *testing.T) {
	layout, config := doctorFixture(t, false, nil)
	if err := os.RemoveAll(layout.BuildKitPKIDir); err != nil {
		t.Fatal(err)
	}
	runner := &doctorCommandRunner{
		statuses:   healthyDoctorStatuses(t, config, false),
		composeErr: errors.New("compose error included a private value"),
		volumeErr:  errors.New("volume error included a private value"),
	}
	app, server, output := doctorAppForTest(t, layout, runner)
	defer server.Close()
	if code := app.runDoctorCommand(nil); code != 1 {
		t.Fatalf("runDoctorCommand() = %d, want 1", code)
	}
	for _, want := range []string{"Compose validation", "BuildKit mTLS", "Storage volume"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("doctor output %q misses %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "private value") {
		t.Fatalf("doctor leaked command error text: %q", output.String())
	}
}

func TestRunDoctorDetectsCLIPlatformVersionSkew(t *testing.T) {
	layout, config := doctorFixture(t, false, nil)
	runner := &doctorCommandRunner{statuses: healthyDoctorStatuses(t, config, false)}
	app, server, output := doctorAppForTest(t, layout, runner)
	defer server.Close()
	app.currentVersion = func() string { return "v1.2.2" }
	if code := app.runDoctorCommand(nil); code != 1 {
		t.Fatalf("runDoctorCommand() = %d, want failure on CLI/platform skew; output=%s", code, output.String())
	}
	for _, want := range []string{"CLI/platform release", "CLI is v1.2.2 and platform is v1.2.3", "stealth update"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("doctor output %q misses %q", output.String(), want)
		}
	}
}

func TestRunDoctorAcceptsCLIPlatformSkewFromVerifiedRollbackState(t *testing.T) {
	layout, config := doctorFixture(t, false, nil)
	if err := os.MkdirAll(layout.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(platformRollbackState{FormatVersion: 1, Platform: "v1.2.3", CLI: "v1.2.4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := installengine.WritePrivateFile(filepath.Join(layout.StateDir, "platform-rollback.state"), string(state)); err != nil {
		t.Fatal(err)
	}
	runner := &doctorCommandRunner{statuses: healthyDoctorStatuses(t, config, false)}
	app, server, output := doctorAppForTest(t, layout, runner)
	defer server.Close()
	app.currentVersion = func() string { return "v1.2.4" }
	if code := app.runDoctorCommand(nil); code != 0 {
		t.Fatalf("runDoctorCommand() = %d, want accepted intentional skew; output=%s", code, output.String())
	}
	if !strings.Contains(output.String(), "platform rollback to v1.2.3 completed with this CLI") {
		t.Fatalf("doctor did not explain intentional rollback skew: %q", output.String())
	}
}

func TestRunDoctorSetupModeSkipsBuildKitAndAppRuntime(t *testing.T) {
	layout, config := doctorFixture(t, true, nil)
	runner := &doctorCommandRunner{statuses: healthyDoctorStatuses(t, config, true)}
	app, server, output := doctorAppForTest(t, layout, runner)
	defer server.Close()
	if code := app.runDoctorCommand(nil); code != 0 {
		t.Fatalf("runDoctorCommand() = %d; output=%s", code, output.String())
	}
	if strings.Contains(output.String(), "BuildKit mTLS") || !strings.Contains(output.String(), "not started in setup mode") {
		t.Fatalf("setup-mode diagnostics are incorrect: %q", output.String())
	}
}

func TestStatusServiceTargetsMarkExternalDependenciesWithoutExposingURLs(t *testing.T) {
	config := map[string]string{
		"DATABASE_URL": "postgres://user:secret@db.example.test:5432/stealth",
		"REDIS_URL":    "rediss://:secret@cache.example.test:6379/0",
	}
	targets := statusServiceTargets(config, false)
	external := map[string]bool{}
	for _, target := range targets {
		if target.external {
			external[target.name] = true
		}
	}
	if !external["postgres"] || !external["redis"] {
		t.Fatalf("external services were not detected: %#v", targets)
	}
	for _, target := range targets {
		if target.name == "migrate" {
			t.Fatal("one-shot migration container should not be required with external PostgreSQL")
		}
	}
}

func TestComposeArgsAndServiceTargetsEnableConfiguredCloudflareTunnel(t *testing.T) {
	layout, config := doctorFixture(t, false, map[string]string{"CLOUDFLARE_TUNNEL_TOKEN_FILE": "./state/cloudflare-tunnel-token"})
	if err := installengine.WritePrivateFile(filepath.Join(layout.StateDir, "cloudflare-tunnel-token"), "tunnel-secret-sentinel\n"); err != nil {
		t.Fatal(err)
	}
	runner := &doctorCommandRunner{}
	app := NewApp(strings.NewReader(""), io.Discard, io.Discard)
	app.runner = runner
	args := app.composeArgs(layout, "ps")
	if !hasArgs([][]string{args}, "--profile", "cloudflare") {
		t.Fatalf("Compose arguments did not enable the configured Cloudflare profile: %#v", args)
	}
	targets := statusServiceTargets(config, false)
	var cloudflared *statusServiceTarget
	for index := range targets {
		if targets[index].name == "cloudflared" {
			cloudflared = &targets[index]
		}
	}
	if cloudflared == nil || !cloudflared.allowRunning {
		t.Fatalf("configured tunnel was not included as a running-only service: %#v", targets)
	}
	if !serviceTargetHealthy(ServiceStatus{Service: "cloudflared", State: "running"}, cloudflared.allowRunning) {
		t.Fatal("running Cloudflared service without a healthcheck was reported unhealthy")
	}
	if serviceTargetHealthy(ServiceStatus{Service: "cloudflared", State: "exited"}, cloudflared.allowRunning) {
		t.Fatal("stopped Cloudflared service was reported healthy")
	}
}

func hasArgs(calls [][]string, required ...string) bool {
	for _, call := range calls {
		joined := " " + strings.Join(call, " ") + " "
		matched := true
		for _, value := range required {
			if !strings.Contains(joined, " "+value+" ") {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
