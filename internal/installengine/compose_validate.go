package installengine

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

func validateProductionComposeAsset(contents []byte) error {
	for _, marker := range []string{
		"  traefik:",
		"  buildkit-worker-credentials-init:",
		"  buildkit-server-credentials-init:",
		"  buildkit:",
		"  traefik-state-init:",
		"  cloudflare-setup-state-init:",
		"  cloudflare-state-init:",
		"  otel-collector:",
		"  telemetry-host:",
		"  telemetry-docker-logs:",
		"  telemetry-docker:",
		"  telemetry-docker-proxy:",
		"  telemetry_ingest:",
		"  app_build:",
		"  buildkit_worker_credentials:",
		"  buildkit_server_credentials:",
		"  cloudflare_setup_state_input:",
		"  buildkit_server_credentials:\n    name: \"${COMPOSE_PROJECT_NAME:-stealth}_app_buildkit_server_credentials\"\n  cloudflare_setup_state_input:\n    name: \"${COMPOSE_PROJECT_NAME:-stealth}_cloudflare_setup_state_input\"",
	} {
		if !bytes.Contains(contents, []byte(marker)) {
			return fmt.Errorf("missing current production Compose marker %q", marker)
		}
	}
	text := string(contents)
	service := productionServiceBlock(text, "buildkit")
	if service == "" {
		return errors.New("missing dedicated App BuildKit service")
	}
	for _, required := range []string{
		"image: " + defaultBuildKitImage,
		"user: \"1000:1000\"", "read_only: true", "seccomp=unconfined",
		"apparmor=${APPS_BUILDKIT_APPARMOR_PROFILE:-unconfined}", "systempaths=unconfined", "buildkit_state:/home/user/.local/share/buildkit",
		"networks: [app_build]", "buildkit/buildkitd.toml:/etc/buildkit/buildkitd.toml:ro",
		"buildkit_server_credentials:/run/secrets/stealth-buildkit:ro",
		"tcp://0.0.0.0:1234", "--config", "/etc/buildkit/buildkitd.toml",
		"--tlscacert", "/run/secrets/stealth-buildkit/ca.pem",
		"--tlscert", "/run/secrets/stealth-buildkit/health-client-cert.pem",
		"--tlskey", "/run/secrets/stealth-buildkit/health-client-key.pem", "debug", "workers",
	} {
		if !strings.Contains(service, required) {
			return fmt.Errorf("App BuildKit service is missing required setting %q", required)
		}
	}
	for _, forbidden := range []string{
		"privileged:", "network_mode: host", "pid: host", "ipc: host", "/var/run/docker.sock",
		"ports:", "stealth:", "telemetry_store:", "ingress_control_db:", "stealth_storage:", "app_build_staging:",
		"state/buildkit-mtls", "buildkit_worker_credentials:",
	} {
		if strings.Contains(service, forbidden) {
			return fmt.Errorf("App BuildKit service contains forbidden setting %q", forbidden)
		}
	}
	worker := productionServiceBlock(text, "worker")
	if worker == "" || !strings.Contains(worker, "buildkit_worker_credentials:/run/secrets/stealth-buildkit:ro") {
		return errors.New("worker must mount only its read-only BuildKit client credentials volume")
	}
	if strings.Contains(worker, "buildkit-mtls") || strings.Contains(worker, "buildkit_server_credentials:") || strings.Contains(worker, "ca-key.pem") {
		return errors.New("worker must not mount host PKI files or BuildKit server credentials")
	}
	workerInit := productionServiceBlock(text, "buildkit-worker-credentials-init")
	serverInit := productionServiceBlock(text, "buildkit-server-credentials-init")
	for name, block := range map[string]string{"worker": workerInit, "BuildKit": serverInit} {
		if block == "" || !strings.Contains(block, "network_mode: none") || !strings.Contains(block, "restart: \"no\"") ||
			!strings.Contains(block, "cap_drop: [ALL]") || !strings.Contains(block, "cap_add: [CHOWN, DAC_OVERRIDE]") {
			return fmt.Errorf("%s BuildKit credential initializer must be a networkless one-shot with only copy and ownership capabilities", name)
		}
	}
	for _, required := range []string{
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/ca-cert.pem:/input/ca.pem:ro",
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/worker/cert.pem:/input/client-cert.pem:ro",
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/worker/key.pem:/input/client-key.pem:ro",
		"buildkit_worker_credentials:/output",
	} {
		if !strings.Contains(workerInit, required) {
			return fmt.Errorf("worker BuildKit credential initializer is missing %q", required)
		}
	}
	if !strings.Contains(workerInit, "for stale in /output/* /output/.[!.]* /output/..?*; do [ ! -e ") || !strings.Contains(workerInit, "$$stale") {
		return errors.New("worker BuildKit credential initializer must clear stale volume contents before copying its identity")
	}
	if strings.Contains(workerInit, "/server/") || strings.Contains(workerInit, "/health/") || strings.Contains(workerInit, "ca-key.pem") {
		return errors.New("worker BuildKit credential initializer has access to a non-worker private key")
	}
	for _, required := range []string{
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/ca-cert.pem:/input/ca.pem:ro",
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/server/cert.pem:/input/server-cert.pem:ro",
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/server/key.pem:/input/server-key.pem:ro",
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/health/cert.pem:/input/health-client-cert.pem:ro",
		"${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls/health/key.pem:/input/health-client-key.pem:ro",
		"buildkit_server_credentials:/output",
	} {
		if !strings.Contains(serverInit, required) {
			return fmt.Errorf("BuildKit credential initializer is missing %q", required)
		}
	}
	if !strings.Contains(serverInit, "for stale in /output/* /output/.[!.]* /output/..?*; do [ ! -e ") || !strings.Contains(serverInit, "$$stale") {
		return errors.New("BuildKit credential initializer must clear stale volume contents before copying its identities")
	}
	if strings.Contains(serverInit, "/worker/") || strings.Contains(serverInit, "ca-key.pem") {
		return errors.New("BuildKit credential initializer has access to the worker private key or CA private key")
	}

	sourceInit := productionServiceBlock(text, "cloudflare-setup-state-init")
	for _, required := range []string{
		"network_mode: none", "restart: \"no\"", "read_only: true", "cap_drop: [ALL]", "user: \"0:0\"",
		"cap_add: [CHOWN, DAC_READ_SEARCH]",
		"entrypoint: [\"/usr/local/bin/stealth-cloudflare-state-init\"]",
		"${STEALTH_INSTALL_ROOT:-.}/state:/source:ro", "cloudflare_setup_state_input:/output:rw",
	} {
		if !strings.Contains(sourceInit, required) {
			return fmt.Errorf("Cloudflare source initializer is missing required setting %q", required)
		}
	}
	for _, forbidden := range []string{
		"./state:/state", "private", "buildkit-mtls", "ca-key.pem", "server/key.pem", "worker/key.pem", "health/key.pem",
		"FUNCTIONS_SECRET_KEY", "DATABASE_URL", "REDIS_URL", "CLOUDFLARE_API_TOKEN", "/var/run/docker.sock", "networks:", "environment:",
	} {
		if strings.Contains(sourceInit, forbidden) {
			return fmt.Errorf("Cloudflare source initializer contains forbidden setting %q", forbidden)
		}
	}
	if strings.Count(sourceInit, "cap_add:") != 1 || !strings.Contains(sourceInit, "cap_add: [CHOWN, DAC_READ_SEARCH]") {
		return errors.New("Cloudflare source initializer may add only CHOWN and DAC_READ_SEARCH")
	}

	cloudflareInit := productionServiceBlock(text, "cloudflare-state-init")
	for _, required := range []string{
		"network_mode: none", "read_only: true", "entrypoint: [\"/usr/local/bin/stealth-cloudflare-import-init\"]",
		"cloudflare_setup_state_input:/input:ro", "${STEALTH_INSTALL_ROOT:-.}/state/.cloudflare-import:/output:rw",
		"condition: service_completed_successfully", "FUNCTIONS_SECRET_KEY:",
	} {
		if !strings.Contains(cloudflareInit, required) {
			return fmt.Errorf("Cloudflare state initializer is missing required setting %q", required)
		}
	}
	for _, forbidden := range []string{
		"./state:/", "${STEALTH_INSTALL_ROOT:-.}/state:/", "private", "buildkit-mtls", "ca-key.pem", "server/key.pem", "worker/key.pem", "health/key.pem",
		"/source", "setup-state.enc:ro", "/var/run/docker.sock",
	} {
		if strings.Contains(cloudflareInit, forbidden) {
			return fmt.Errorf("Cloudflare state initializer contains forbidden setting %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		"./private:", "${STEALTH_INSTALL_ROOT:-.}/private:", "./:/", ".:/", "${STEALTH_INSTALL_ROOT:-.}:/", "state/buildkit-mtls", "ca-key.pem",
	} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("production Compose contains a broad or legacy private-state bind %q", forbidden)
		}
	}
	telemetryHost := productionServiceBlock(text, "telemetry-host")
	if !strings.Contains(telemetryHost, "- /:/hostfs:ro") {
		return errors.New("telemetry-host is missing its read-only host filesystem view")
	}
	maskStart := strings.Index(telemetryHost, "      - type: tmpfs\n")
	if maskStart < 0 {
		return errors.New("telemetry-host must mask the installation private directory with a tmpfs")
	}
	maskEnd := len(telemetryHost)
	if next := strings.Index(telemetryHost[maskStart+1:], "\n      - "); next >= 0 {
		maskEnd = maskStart + 1 + next
	}
	privateMask := telemetryHost[maskStart:maskEnd]
	for _, required := range []string{
		"type: tmpfs", "target: /hostfs/${STEALTH_INSTALL_ROOT:?set STEALTH_INSTALL_ROOT}/private",
		"read_only: true", "size: 1048576",
	} {
		if !strings.Contains(privateMask, required) {
			return fmt.Errorf("telemetry-host private-directory tmpfs is missing %q", required)
		}
	}
	for _, name := range productionServiceNames(text) {
		block := productionServiceBlock(text, name)
		if name != "buildkit-worker-credentials-init" && name != "buildkit-server-credentials-init" &&
			(strings.Contains(block, "private/buildkit-mtls") || strings.Contains(block, "ca-key.pem") || strings.Contains(block, "/private:/")) {
			return fmt.Errorf("service %q can see private BuildKit PKI", name)
		}
	}
	for name, block := range map[string]string{"worker": workerInit, "BuildKit": serverInit} {
		for _, broadMount := range []string{"./private:", "${STEALTH_INSTALL_ROOT:-.}/private:", "./:/", ".:/", "${STEALTH_INSTALL_ROOT:-.}:/", "./private/buildkit-mtls:", "${STEALTH_INSTALL_ROOT:-.}/private/buildkit-mtls:"} {
			if strings.Contains(block, broadMount) {
				return fmt.Errorf("%s credential initializer has a broad BuildKit PKI mount %q", name, broadMount)
			}
		}
	}
	return nil
}
func productionServiceNames(contents string) []string {
	lines := strings.Split(contents, "\n")
	inServices := false
	var names []string
	for _, line := range lines {
		if line == "services:" {
			inServices = true
			continue
		}
		if !inServices {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(trimmed, ":") {
			names = append(names, strings.TrimSuffix(trimmed, ":"))
		}
	}
	return names
}
func productionServiceBlock(contents, wanted string) string {
	lines := strings.Split(contents, "\n")
	inServices := false
	active := false
	var block []string
	for _, line := range lines {
		if line == "services:" {
			inServices = true
			continue
		}
		if !inServices {
			continue
		}
		trimmed := strings.TrimSpace(line)
		isService := strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(trimmed, ":")
		if isService {
			name := strings.TrimSuffix(trimmed, ":")
			if active {
				return strings.Join(block, "\n")
			}
			if name == wanted {
				active = true
				block = append(block, line)
			}
			continue
		}
		if active {
			block = append(block, line)
		}
	}
	if active {
		return strings.Join(block, "\n")
	}
	return ""
}
func validateMainCollectorAsset(contents []byte) error {
	for _, marker := range [][]byte{[]byte("exporters:"), []byte("clickhouse:")} {
		if !bytes.Contains(contents, marker) {
			return fmt.Errorf("missing main Collector marker %q", marker)
		}
	}
	if bytes.Contains(contents, []byte("file_log/docker:")) {
		return errors.New("main Collector still contains the Docker file-log receiver")
	}
	return nil
}
