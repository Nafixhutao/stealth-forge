package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Stealth-deplover/stealth/internal/installengine"
)

const installerAcceptanceVersion = "v1.2.3"

var installerAcceptanceTagPattern = regexp.MustCompile(`^smoke-[0-9]+-[0-9]+$`)

type installerAcceptanceRunner struct {
	command installengine.OSCommandRunner
}

func (r installerAcceptanceRunner) Run(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	if name == "docker" && len(args) > 0 && args[0] == "compose" && args[len(args)-1] == "pull" {
		return nil
	}
	return r.command.Run(ctx, dir, stdout, stderr, name, args...)
}

func (r installerAcceptanceRunner) Output(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return r.command.Output(ctx, dir, name, args...)
}

func (installerAcceptanceRunner) CombinedOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	return command.CombinedOutput()
}

type installerAcceptanceEvidence struct {
	Version            string   `json:"version"`
	HostKind           string   `json:"host_kind"`
	CleanInstallPassed bool     `json:"clean_install_passed"`
	RepairPassed       bool     `json:"repair_passed"`
	StatusPassed       bool     `json:"status_passed"`
	DoctorPassed       bool     `json:"doctor_passed"`
	ServiceChecks      []string `json:"service_checks"`
	ConfigMode         string   `json:"config_mode"`
	BuildKitKeyMode    string   `json:"buildkit_key_mode"`
}

// TestInstallerCleanHostAndRepairAcceptance exercises the host install engine
// against real Docker services, then repairs the same installation through
// `stealth install --repair`. CI opts in only on its disposable Linux VM.
func TestInstallerCleanHostAndRepairAcceptance(t *testing.T) {
	tag := strings.TrimSpace(os.Getenv("STEALTH_INSTALLER_ACCEPTANCE_TAG"))
	if tag == "" {
		t.Skip("installer clean-host acceptance is enabled by its disposable-VM workflow")
	}
	if !installerAcceptanceTagPattern.MatchString(tag) {
		t.Fatal("installer acceptance image tag is invalid")
	}
	if os.Geteuid() != 0 {
		t.Fatal("installer acceptance must run with host administration privileges")
	}

	dockerGID, err := dockerSocketGID("/var/run/docker.sock")
	if err != nil {
		t.Fatalf("Docker socket preflight failed: %v", err)
	}
	root := filepath.Join(t.TempDir(), "stealth-installer-acceptance")
	t.Setenv("STEALTH_INSTALL_DIR", root)
	layout, err := installengine.NewLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	project := "stealth-installer-" + strings.TrimPrefix(tag, "smoke-")
	freePorts := installerAcceptanceFreePorts(t, 3)
	ports := map[string]string{
		"API_HOST_PORT":     freePorts[0],
		"CONSOLE_HOST_PORT": freePorts[1],
		"PROXY_HTTP_PORT":   freePorts[2],
	}
	publicURL := "http://127.0.0.1:" + ports["CONSOLE_HOST_PORT"]
	configContents, err := installengine.GenerateConfig(installengine.ConfigOptions{
		Version: installerAcceptanceVersion, PublicURL: publicURL, GitHubAppClientID: "Iv1.acceptance-client",
		DockerGID: dockerGID, InstallRoot: root, ComposeProject: project,
		AppsBuildKitAppArmorProfile: installengine.BuildKitAppArmorProfileName,
	})
	if err != nil {
		t.Fatalf("generate isolated installer acceptance config: %v", err)
	}
	config, err := installengine.ParseEnvContents(configContents)
	if err != nil {
		t.Fatal(err)
	}
	for key, image := range map[string]string{
		"STEALTH_API_IMAGE":                    "stealth-api",
		"STEALTH_SETUP_IMAGE":                  "stealth-setup",
		"STEALTH_WORKER_IMAGE":                 "stealth-worker",
		"STEALTH_INGRESS_CONTROL_IMAGE":        "stealth-ingress-control",
		"STEALTH_MIGRATE_IMAGE":                "stealth-migrate",
		"STEALTH_CONSOLE_IMAGE":                "stealth-console",
		"STEALTH_TELEMETRY_DOCKER_PROXY_IMAGE": "stealth-telemetry-docker-proxy",
		"OTEL_COLLECTOR_IMAGE":                 "stealth-otel-collector",
		"OTEL_HOST_COLLECTOR_IMAGE":            "stealth-otel-collector",
		"OTEL_DOCKER_COLLECTOR_IMAGE":          "stealth-otel-collector",
		"OTEL_DOCKER_LOGS_COLLECTOR_IMAGE":     "stealth-otel-docker-logs",
	} {
		config[key] = image + ":" + tag
	}
	config["STEALTH_NETWORK_NAME"] = project + "-network"
	config["STEALTH_TELEMETRY_STORE_NETWORK_NAME"] = project + "-store"
	config["STEALTH_TELEMETRY_INGEST_NETWORK_NAME"] = project + "-ingest"
	config["STEALTH_TELEMETRY_DOCKER_NETWORK_NAME"] = project + "-docker"
	config["STEALTH_INGRESS_NETWORK_NAME"] = project + "-ingress"
	config["APPS_RUNTIME_NETWORK_NAME"] = project + "-runtime"
	config["FUNCTIONS_RUNNER_STAGING_VOLUME"] = project + "-runner-staging"
	config["APPS_BUILD_STAGING_VOLUME"] = project + "-build-staging"
	config["APPS_BUILDKIT_STATE_VOLUME"] = project + "-buildkit-state"
	config["STORAGE_VOLUME_NAME"] = project + "-storage"
	config["POSTGRES_VOLUME_NAME"] = project + "-postgres"
	config["CLICKHOUSE_VOLUME_NAME"] = project + "-clickhouse"
	config["OTELCOL_VOLUME_NAME"] = project + "-otelcol"
	config["OTEL_DOCKER_LOGS_VOLUME_NAME"] = project + "-docker-logs"
	config["API_HOST_PORT"] = ports["API_HOST_PORT"]
	config["CONSOLE_HOST_PORT"] = ports["CONSOLE_HOST_PORT"]
	config["PROXY_HTTP_PORT"] = ports["PROXY_HTTP_PORT"]
	configContents = installengine.FormatEnvFile(config)

	assetServer := installerAcceptanceAssetServer(t)
	t.Cleanup(assetServer.Close)
	runner := installerAcceptanceRunner{}
	app := NewApp(strings.NewReader(""), io.Discard, io.Discard)
	app.runner = runner
	app.assetBase = assetServer.URL
	app.currentVersion = func() string { return installerAcceptanceVersion }
	app.pollAttempts = 75
	app.pollInterval = 2 * time.Second

	plan := InstallPlan{
		Layout: layout, Version: installerAcceptanceVersion, PublicURL: publicURL,
		GitHubAppClientID: "Iv1.acceptance-client", DockerGID: dockerGID,
		ConfigContents:     configContents,
		InternalAPIURL:     "http://127.0.0.1:" + ports["API_HOST_PORT"],
		InternalConsoleURL: "http://127.0.0.1:" + ports["CONSOLE_HOST_PORT"],
		InternalProxyURL:   "http://127.0.0.1:" + ports["PROXY_HTTP_PORT"],
	}
	cleanupAcceptanceInstall(t, runner, layout)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := app.installEngine().Install(ctx, plan, nil); err != nil {
		t.Fatalf("clean host install failed: %v", err)
	}
	installed, err := installengine.ReadEnvFile(layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	secretSnapshot := map[string]string{}
	for _, key := range []string{"POSTGRES_PASSWORD", "REDIS_PASSWORD", "FUNCTIONS_SECRET_KEY", "APPS_SECRET_KEY", "BOOTSTRAP_CLI_KEY", "METRICS_TOKEN"} {
		secretSnapshot[key] = installed[key]
		if strings.TrimSpace(installed[key]) == "" {
			t.Fatalf("clean install did not generate %s", key)
		}
	}
	if !installengine.FileIsPrivate(layout.EnvFile) {
		t.Fatal("clean install did not protect config.env with mode 0600")
	}

	var repairOutput, repairErrors strings.Builder
	app.in = strings.NewReader("")
	app.out = &repairOutput
	app.errOut = &repairErrors
	if code := app.runInstall([]string{"--repair"}); code != 0 {
		t.Fatalf("stealth install --repair exited %d: stdout=%s stderr=%s", code, repairOutput.String(), repairErrors.String())
	}
	repaired, err := installengine.ReadEnvFile(layout.EnvFile)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range secretSnapshot {
		if repaired[key] != want {
			t.Fatalf("repair changed stable config value %s", key)
		}
	}
	if !installengine.FileIsPrivate(layout.EnvFile) {
		t.Fatal("repair did not retain config.env mode 0600")
	}
	workerKey := filepath.Join(layout.BuildKitPKIDir, "worker", "key.pem")
	keyInfo, err := os.Stat(workerKey)
	if err != nil || keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("BuildKit worker key permissions = %v, %v", keyInfo, err)
	}

	app.errOut = io.Discard
	var statusOutput, doctorOutput strings.Builder
	app.out = &statusOutput
	statusCode := app.runStatusCommand(nil)
	app.out = &doctorOutput
	doctorCode := app.runDoctorCommand(nil)
	if statusCode != 0 || doctorCode != 0 {
		t.Fatalf("repaired clean-host checks failed: status=%d\n%s\ndoctor=%d\n%s", statusCode, statusOutput.String(), doctorCode, doctorOutput.String())
	}

	evidencePath := strings.TrimSpace(os.Getenv("STEALTH_INSTALLER_ACCEPTANCE_EVIDENCE"))
	if evidencePath != "" {
		hostKind := strings.TrimSpace(os.Getenv("STEALTH_INSTALLER_ACCEPTANCE_HOST_KIND"))
		if hostKind == "" {
			hostKind = "linux-host"
		}
		evidence := installerAcceptanceEvidence{
			Version: installerAcceptanceVersion, HostKind: hostKind,
			CleanInstallPassed: true, RepairPassed: true, StatusPassed: true, DoctorPassed: true,
			ServiceChecks: []string{"postgres", "redis", "clickhouse", "api", "worker", "buildkit", "console", "proxy", "traefik", "telemetry"},
			ConfigMode:    "0600", BuildKitKeyMode: "0600",
		}
		contents, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(evidencePath, append(contents, '\n'), 0o600); err != nil {
			t.Fatalf("write non-secret installer acceptance evidence: %v", err)
		}
	}
}

func installerAcceptanceFreePorts(t *testing.T, count int) []string {
	t.Helper()
	listeners := make([]net.Listener, 0, count)
	ports := make([]string, 0, count)
	for range count {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			for _, open := range listeners {
				_ = open.Close()
			}
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
		ports = append(ports, fmt.Sprint(listener.Addr().(*net.TCPAddr).Port))
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return ports
}

func installerAcceptanceAssetServer(t *testing.T) *httptest.Server {
	t.Helper()
	_, source, _, ok := runtimeCaller()
	if !ok {
		t.Fatal("cannot locate installer acceptance source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	assets := make(map[string][]byte)
	for _, spec := range installengine.DefaultManagedAssets() {
		contents, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(spec.RemotePath)))
		if err != nil {
			t.Fatalf("read managed acceptance asset: %v", err)
		}
		assets[spec.RemotePath] = contents
	}
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)
	var checksums strings.Builder
	for _, name := range names {
		digest := sha256.Sum256(assets[name])
		fmt.Fprintf(&checksums, "%x  %s\n", digest, name)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		name := strings.TrimPrefix(request.URL.Path, "/"+installerAcceptanceVersion+"/")
		if name == "checksums.txt" {
			_, _ = io.WriteString(w, checksums.String())
			return
		}
		contents, found := assets[name]
		if !found {
			http.NotFound(w, request)
			return
		}
		_, _ = w.Write(contents)
	}))
}

func runtimeCaller() (uintptr, string, int, bool) {
	return runtime.Caller(0)
}

func cleanupAcceptanceInstall(t *testing.T, runner installerAcceptanceRunner, layout InstallLayout) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if regularFile(layout.EnvFile) && regularFile(layout.ComposeFile) {
			_ = runner.Run(ctx, layout.Root, io.Discard, io.Discard, "docker", "compose", "--env-file", layout.EnvFile, "-f", layout.ComposeFile, "down", "--volumes", "--remove-orphans")
			if config, err := installengine.ReadEnvFile(layout.EnvFile); err == nil {
				network := strings.TrimSpace(config["APPS_RUNTIME_NETWORK_NAME"])
				if safeVolumeName(network) {
					output, inspectErr := runner.Output(ctx, "", "docker", "network", "inspect", "--format", "{{json .}}", network)
					if inspectErr == nil && appRuntimeNetworkOwned(network, output) {
						_ = runner.Run(ctx, "", io.Discard, io.Discard, "docker", "network", "rm", network)
					}
				}
			}
		}
		_ = os.RemoveAll(layout.Root)
	})
}
