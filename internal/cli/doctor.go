package cli

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/buildkitpki"
	"github.com/Stealth-deplover/stealth/internal/preflight"
)

type statusServiceTarget struct {
	name         string
	external     bool
	allowRunning bool
}

var productionServices = []string{
	"postgres", "redis", "clickhouse", "migrate", "otel-collector", "telemetry-host",
	"telemetry-docker-logs", "telemetry-docker-proxy", "telemetry-docker", "api", "worker",
	"buildkit", "console", "proxy", "traefik",
}

var setupServices = []string{"postgres", "redis", "setup", "setup-console", "setup-proxy"}

func (a *App) runStatusCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(a.errOut, "status does not accept arguments")
		return 2
	}
	layout, err := a.layout()
	if err != nil {
		fmt.Fprintln(a.errOut, "cannot determine installation directory")
		return 1
	}
	if !installationExists(layout) {
		fmt.Fprintln(a.errOut, "Stealth is not installed")
		return 1
	}
	config, err := readEnvFile(layout.EnvFile)
	if err != nil {
		fmt.Fprintln(a.errOut, "could not read installation configuration")
		return 1
	}
	setupMode := strings.EqualFold(strings.TrimSpace(config["SETUP_MODE"]), "true")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statuses, err := a.composeStatuses(ctx, layout, true)
	if err != nil {
		fmt.Fprintln(a.errOut, "could not read Docker service status")
		return 1
	}
	version := valueOr(config["VERSION"], readVersion(layout))
	if validateReleaseVersion(strings.TrimSpace(version)) != nil {
		version = "unknown"
	}
	fmt.Fprintf(a.out, "Stealth %s\n\n", version)
	fmt.Fprintln(a.out, "SERVICE          STATUS")
	failed := false
	for _, target := range statusServiceTargets(config, setupMode) {
		status := statuses[target.name]
		if target.external {
			status = ServiceStatus{Service: target.name, State: "external"}
		} else if status.Service == "" {
			status = ServiceStatus{Service: target.name, State: "not found"}
		}
		fmt.Fprintf(a.out, "%-16s %s\n", displayServiceName(target.name), status.Display())
		if !target.external && !serviceTargetHealthy(status, target.allowRunning) {
			failed = true
		}
	}
	if publicURL, urlErr := validatePublicURL(config["PUBLIC_APP_URL"]); urlErr == nil {
		fmt.Fprintf(a.out, "\nConsole: %s\n", publicURL)
	}
	return boolExit(failed)
}

func (a *App) runDoctorCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(a.errOut, "doctor does not accept arguments")
		return 2
	}
	layout, err := a.layout()
	if err != nil {
		fmt.Fprintln(a.errOut, "cannot determine installation directory")
		return 1
	}
	fmt.Fprintln(a.out, "Stealth Doctor")
	fmt.Fprintln(a.out)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	failed := false
	check := func(name string, ok bool, detail string) {
		if !ok {
			failed = true
		}
		fmt.Fprintln(a.out, renderCheck(SystemCheck{Name: name, OK: ok, Detail: detail, Required: true}))
	}
	warning := func(name string, ok bool, detail string) {
		fmt.Fprintln(a.out, renderCheck(SystemCheck{Name: name, OK: ok, Detail: detail, Required: false}))
	}

	if output, commandErr := a.runner.Output(ctx, "", "docker", "version", "--format", "{{.Server.Version}}"); commandErr != nil {
		check("Docker", false, "Docker is unavailable")
	} else {
		check("Docker", true, strings.TrimSpace(string(output)))
	}
	if output, commandErr := a.runner.Output(ctx, "", "docker", "compose", "version", "--short"); commandErr != nil {
		check("Docker Compose", false, "Docker Compose is unavailable")
	} else {
		check("Docker Compose", true, strings.TrimSpace(string(output)))
	}
	for _, hostCheck := range preflight.ResourceChecks(ctx, preflight.NewProbes(a.runner, a.httpClient), filepath.Dir(layout.Root)) {
		warning(hostCheck.Name, hostCheck.OK, hostCheck.Detail)
	}

	if !installationExists(layout) {
		check("Installation", false, "installation directory is not initialized")
		return boolExit(failed)
	}
	configPrivate := fileIsPrivate(layout.EnvFile)
	check("Configuration", configPrivate, configCheckDetail("config.env", configPrivate))
	config, configErr := readEnvFile(layout.EnvFile)
	if configErr != nil {
		check("Configuration syntax", false, "could not parse config.env")
		return boolExit(failed)
	}
	setupMode := strings.EqualFold(strings.TrimSpace(config["SETUP_MODE"]), "true")
	platformVersion := strings.TrimSpace(readVersion(layout))
	if platformVersion == "" {
		platformVersion, _ = imageVersion(config["STEALTH_API_IMAGE"])
	}
	if validateReleaseVersion(platformVersion) != nil {
		check("Platform release", false, "installed release version could not be verified")
	} else {
		cliVersion := ""
		if a.currentVersion != nil {
			cliVersion = strings.TrimSpace(a.currentVersion())
		}
		if validateStableReleaseVersion(cliVersion) != nil {
			warning("CLI/platform release", true, "platform "+platformVersion+"; running CLI is not a stable release build")
		} else if comparison, comparable := compareReleaseVersions(cliVersion, platformVersion); !comparable {
			warning("CLI/platform release", true, "running CLI and platform release could not be compared")
		} else if comparison == 0 {
			check("CLI/platform release", true, platformVersion)
		} else {
			detail := fmt.Sprintf("CLI is %s and platform is %s; run `stealth update` to synchronize them", cliVersion, platformVersion)
			check("CLI/platform release", false, detail)
		}
	}
	composePath := layout.ComposeFile
	composeLabel := "production Compose file"
	if setupMode {
		composePath = layout.SetupComposeFile
		composeLabel = "setup Compose file"
	}
	check("Compose file", regularFile(composePath), composeLabel+" is present")
	check("Configuration syntax", hasRequiredConfig(config), "required values are present")

	composeArgs := a.composeArgs(layout, "config", "--quiet")
	if _, configErr := a.runner.Output(ctx, layout.Root, "docker", composeArgs...); configErr != nil {
		check("Compose validation", false, "configuration is invalid; run stealth install --repair after correcting config.env")
	} else {
		check("Compose validation", true, "configuration is valid")
	}

	if setupMode {
		warning("App runtime", false, "not started in setup mode")
	} else {
		runtimeNetwork := strings.TrimSpace(config["APPS_RUNTIME_NETWORK_NAME"])
		if runtimeNetwork == "" {
			runtimeNetwork = "stealth_app_runtime"
		}
		output, inspectErr := a.runner.Output(ctx, "", "docker", "network", "inspect", "--format", "{{json .}}", runtimeNetwork)
		if inspectErr == nil && appRuntimeNetworkOwned(runtimeNetwork, output) {
			check("App runtime", true, "owned bridge network is available")
		} else {
			check("App runtime", false, "Docker runtime network is missing or ownership could not be verified")
		}
		if pkiErr := buildkitpki.ValidateExisting(layout.BuildKitPKIDir); pkiErr != nil {
			check("BuildKit mTLS", false, "identity is missing or invalid; run stealth install --repair")
		} else {
			check("BuildKit mTLS", true, "installed identities are valid")
		}
	}

	driverOK, driverDetail := storageDriverCheck(config)
	check("Storage config", driverOK, driverDetail)
	volumeName := configuredVolumeName(config, "STORAGE_VOLUME_NAME", "stealth_storage")
	if !safeVolumeName(volumeName) {
		check("Storage volume", false, "configured Docker volume name is invalid")
	} else if volumeDriver, volumeErr := a.runner.Output(ctx, "", "docker", "volume", "inspect", "--format", "{{.Driver}}", volumeName); volumeErr != nil || strings.TrimSpace(string(volumeDriver)) == "" {
		check("Storage volume", false, "configured Docker volume is unavailable")
	} else {
		check("Storage volume", true, "configured Docker volume is available")
	}

	if configuredCloudflareTunnel(config) {
		tokenPath := configuredCloudflareTokenPath(layout, config)
		tokenOK := safeRegularFile(tokenPath) && fileIsPrivate(tokenPath)
		tokenDetail := "tunnel token file is private and available"
		if !tokenOK {
			tokenDetail = "tunnel token file must exist with mode 0600"
		}
		check("Cloudflare credentials", tokenOK, tokenDetail)
		networkCtx, networkCancel := context.WithTimeout(ctx, 5*time.Second)
		for _, networkCheck := range preflight.CloudflareChecks(networkCtx, preflight.NewProbes(a.runner, a.httpClient), preflight.DefaultCloudflareTunnelHosts) {
			if networkCheck.Name == "Cloudflare Tunnel" {
				check(networkCheck.Name, networkCheck.OK, networkCheck.Detail)
			} else {
				warning(networkCheck.Name, networkCheck.OK, networkCheck.Detail)
			}
		}
		networkCancel()
	} else {
		warning("Cloudflare Tunnel", false, "not configured")
	}

	statuses, statusErr := a.composeStatuses(ctx, layout, true)
	if statusErr != nil {
		check("Docker services", false, "could not query Compose")
	} else {
		for _, target := range statusServiceTargets(config, setupMode) {
			if target.external {
				check(displayServiceName(target.name), true, "configured externally; API readiness is checked below")
				continue
			}
			status := statuses[target.name]
			if status.Service == "" {
				status = ServiceStatus{Service: target.name, State: "not found"}
			}
			check(displayServiceName(target.name), serviceTargetHealthy(status, target.allowRunning), status.Display())
		}
	}

	ports := portsFromConfig(config)
	if setupMode {
		ports = setupPortsFromConfig(config)
	}
	for _, endpoint := range []struct {
		name string
		url  string
	}{
		{"API health", "http://127.0.0.1:" + ports.API + "/healthz"},
		{"API readiness", "http://127.0.0.1:" + ports.API + "/readyz"},
		{"API version", "http://127.0.0.1:" + ports.API + "/version"},
		{"Console", "http://127.0.0.1:" + ports.Console + "/"},
		{"Proxy", "http://127.0.0.1:" + ports.Proxy + "/"},
	} {
		status, requestErr := a.httpStatus(ctx, endpoint.url)
		check(endpoint.name, requestErr == nil && status >= 200 && status < 300, httpStatusDetail(status, requestErr))
	}
	return boolExit(failed)
}

func statusServiceTargets(config map[string]string, setupMode bool) []statusServiceTarget {
	targets := make([]statusServiceTarget, 0, len(productionServices)+1)
	services := productionServices
	if setupMode {
		services = setupServices
	}
	for _, service := range services {
		externalDatabase := externalDependency(config, "DATABASE_MODE", "DATABASE_URL", "postgres")
		if service == "migrate" && externalDatabase {
			continue
		}
		if service == "postgres" && externalDatabase {
			targets = append(targets, statusServiceTarget{name: service, external: true})
			continue
		}
		if service == "redis" && externalDependency(config, "REDIS_MODE", "REDIS_URL", "redis") {
			targets = append(targets, statusServiceTarget{name: service, external: true})
			continue
		}
		targets = append(targets, statusServiceTarget{name: service})
	}
	if !setupMode && configuredCloudflareTunnel(config) {
		targets = append(targets, statusServiceTarget{name: "cloudflared", allowRunning: true})
	}
	return targets
}

func serviceTargetHealthy(status ServiceStatus, allowRunning bool) bool {
	if allowRunning {
		return status.Running()
	}
	return status.Healthy()
}

// waitForRequiredServices waits for the configured production or setup stack
// to reach the same service health state reported by `stealth status`.
func (a *App) waitForRequiredServices(ctx context.Context, layout InstallLayout) error {
	config, err := readEnvFile(layout.EnvFile)
	if err != nil {
		return fmt.Errorf("could not read installation configuration")
	}
	setupMode := strings.EqualFold(strings.TrimSpace(config["SETUP_MODE"]), "true")
	targets := statusServiceTargets(config, setupMode)
	attempts := 60
	interval := 2 * time.Second
	if a != nil {
		if a.pollAttempts > 0 {
			attempts = a.pollAttempts
		}
		if a.pollInterval > 0 {
			interval = a.pollInterval
		}
	}

	var latest map[string]ServiceStatus
	var queryFailed bool
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		statuses, statusErr := a.composeStatuses(ctx, layout, true)
		if statusErr == nil {
			latest = statuses
			queryFailed = false
			allHealthy := true
			for _, target := range targets {
				if target.external {
					continue
				}
				if !serviceTargetHealthy(statuses[target.name], target.allowRunning) {
					allHealthy = false
					break
				}
			}
			if allHealthy {
				return nil
			}
		} else {
			queryFailed = true
		}

		if attempt+1 < attempts {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	if queryFailed {
		return fmt.Errorf("could not query Docker service status")
	}
	return fmt.Errorf("required services did not become healthy: %s", serviceReadinessSummary(targets, latest))
}

func serviceReadinessSummary(targets []statusServiceTarget, statuses map[string]ServiceStatus) string {
	failed := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.external || serviceTargetHealthy(statuses[target.name], target.allowRunning) {
			continue
		}
		failed = append(failed, target.name+"="+safeServiceReadinessState(statuses[target.name]))
	}
	if len(failed) == 0 {
		return "unknown service state"
	}
	return strings.Join(failed, ", ")
}

func safeServiceReadinessState(status ServiceStatus) string {
	if status.Service == "" {
		return "not-found"
	}
	state := strings.ToLower(strings.TrimSpace(status.State))
	health := strings.ToLower(strings.TrimSpace(status.Health))
	if state == "running" {
		switch health {
		case "healthy", "unhealthy", "starting":
			return health
		default:
			return "health-unknown"
		}
	}
	switch state {
	case "created", "restarting", "removing", "paused", "exited", "dead":
		return state
	}
	switch health {
	case "healthy", "unhealthy", "starting":
		return health
	default:
		return "unknown"
	}
}

func externalServiceURL(raw, internalService string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Hostname() != "" && !strings.EqualFold(parsed.Hostname(), internalService)
}

func externalDependency(config map[string]string, modeKey, urlKey, internalService string) bool {
	return strings.EqualFold(strings.TrimSpace(config[modeKey]), "external") || externalServiceURL(config[urlKey], internalService)
}

func configuredCloudflareTunnel(config map[string]string) bool {
	return strings.TrimSpace(config["CLOUDFLARE_TUNNEL_TOKEN_FILE"]) != ""
}

func configuredCloudflareTokenPath(layout InstallLayout, config map[string]string) string {
	path := strings.TrimSpace(config["CLOUDFLARE_TUNNEL_TOKEN_FILE"])
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(layout.Root, filepath.Clean(path))
}

func safeVolumeName(name string) bool {
	if !validDockerResourceName(name) || len(name) > 255 {
		return false
	}
	first := name[0]
	return first != '-' && first != '.'
}

func storageDriverCheck(config map[string]string) (bool, string) {
	driver := strings.ToLower(strings.TrimSpace(config["STORAGE_DRIVER"]))
	if driver == "" {
		driver = "local"
	}
	switch driver {
	case "local":
		return true, "local object storage is configured"
	case "s3":
		endpoint, err := url.Parse(strings.TrimSpace(config["STORAGE_S3_ENDPOINT"]))
		validEndpoint := err == nil && (endpoint.Scheme == "http" || endpoint.Scheme == "https") && endpoint.Hostname() != "" && endpoint.User == nil && endpoint.Path == "" && endpoint.RawQuery == "" && endpoint.Fragment == ""
		complete := validEndpoint && strings.TrimSpace(config["STORAGE_S3_BUCKET"]) != "" && strings.TrimSpace(config["STORAGE_S3_ACCESS_KEY"]) != "" && strings.TrimSpace(config["STORAGE_S3_SECRET_KEY"]) != ""
		if complete {
			return true, "S3 endpoint, bucket, and credentials are configured"
		}
		return false, "S3 settings are incomplete or invalid; rerun setup without exposing credentials"
	default:
		return false, "unsupported storage driver; use local or s3"
	}
}
