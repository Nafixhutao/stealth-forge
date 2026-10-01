package cli

// Uninstall workflow owns option parsing, safety planning, Docker/filesystem
// effects, and deterministic non-interactive output. Interactive state and
// rendering live in uninstall_tui.go.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type uninstallMode int

const (
	uninstallServices uninstallMode = iota
	uninstallConfiguration
	uninstallPurge
)

type uninstallOptions struct {
	mode    uninstallMode
	modeSet bool
	yes     bool
	dryRun  bool
}

type uninstallVolume struct {
	label       string
	name        string
	composeName string
}

type uninstallPlan struct {
	layout            InstallLayout
	mode              uninstallMode
	config            map[string]string
	configPresent     bool
	configErr         error
	composePresent    bool
	proxyPresent      bool
	telemetryPresent  bool
	versionPresent    bool
	statePresent      bool
	privatePresent    bool
	partial           bool
	unsafeReason      string
	unknownEntries    []string
	volumes           []uninstallVolume
	appRuntimeNetwork string
	externalStorage   bool
}

func (a *App) runUninstall(args []string) int {
	initTerminalStyles()
	options, parseErr := parseUninstallOptions(args, a.errOut)
	if parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	layout, err := a.layout()
	if err != nil {
		fmt.Fprintf(a.errOut, "cannot determine installation directory: %v\n", err)
		return 1
	}
	if !installationExists(layout) {
		fmt.Fprintln(a.out, "No Stealth installation was found.")
		return 0
	}

	plan := buildUninstallPlan(layout, options.mode)
	if plan.unsafeReason != "" {
		fmt.Fprintf(a.errOut, "cannot safely inspect the Stealth installation: %s\n", plan.unsafeReason)
		return 1
	}

	interactive := a.hasInteractiveTerminal()
	if !options.modeSet {
		if interactive {
			// The menu supplies the mode. The dry-run flag is carried through the
			// model after the operator makes a selection.
		} else {
			fmt.Fprintln(a.errOut, "Non-interactive uninstall requires an explicit mode.")
			fmt.Fprintln(a.errOut, "Use `stealth uninstall --keep-data --yes` or `stealth uninstall --purge --yes`.")
			return 2
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if interactive {
		if options.dryRun && options.modeSet && options.yes {
			printUninstallPlan(a.out, plan)
			fmt.Fprintln(a.out, "\nDry run complete. No changes were made.")
			return 0
		}
		return a.runUninstallTUI(ctx, plan, options)
	}
	return a.runUninstallPlain(ctx, plan, options)
}

func parseUninstallOptions(args []string, errOut io.Writer) (uninstallOptions, error) {
	fs := flag.NewFlagSet("stealth uninstall", flag.ContinueOnError)
	fs.SetOutput(errOut)
	keepData := fs.Bool("keep-data", false, "remove services and preserve persistent data")
	purge := fs.Bool("purge", false, "permanently remove services, data, configuration, and secrets")
	yes := fs.Bool("yes", false, "skip confirmation; never selects purge by itself")
	dryRun := fs.Bool("dry-run", false, "show the removal plan without making changes")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "Usage: stealth uninstall [--keep-data|--purge] [--yes] [--dry-run]")
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "Interactive mode presents guided choices when run in a terminal.")
		fmt.Fprintln(errOut, "  --keep-data  remove services only; preserve all persistent data and config")
		fmt.Fprintln(errOut, "  --purge      permanently remove the instance and its project-owned data")
		fmt.Fprintln(errOut, "  --yes        skip confirmation; without a mode it means --keep-data")
		fmt.Fprintln(errOut, "  --dry-run    show what would be removed and preserved")
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "Examples:")
		fmt.Fprintln(errOut, "  stealth uninstall")
		fmt.Fprintln(errOut, "  stealth uninstall --keep-data --yes")
		fmt.Fprintln(errOut, "  stealth uninstall --purge --dry-run")
		fmt.Fprintln(errOut, "  stealth uninstall --purge --yes")
	}
	if err := fs.Parse(args); err != nil {
		return uninstallOptions{}, err
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(errOut, "uninstall does not accept positional arguments")
		return uninstallOptions{}, errors.New("positional arguments")
	}
	if *keepData && *purge {
		fmt.Fprintln(errOut, "uninstall modes --keep-data and --purge are mutually exclusive")
		return uninstallOptions{}, errors.New("conflicting modes")
	}
	options := uninstallOptions{yes: *yes, dryRun: *dryRun}
	switch {
	case *purge:
		options.mode = uninstallPurge
		options.modeSet = true
	case *keepData:
		options.mode = uninstallServices
		options.modeSet = true
	case *yes:
		// --yes must remain a safe default. It is intentionally equivalent to
		// --keep-data when no explicit mode is supplied.
		options.mode = uninstallServices
		options.modeSet = true
	}
	return options, nil
}

func buildUninstallPlan(layout InstallLayout, mode uninstallMode) uninstallPlan {
	plan := uninstallPlan{
		layout:           layout,
		mode:             mode,
		config:           make(map[string]string),
		composePresent:   safeRegularFile(layout.ComposeFile),
		proxyPresent:     safeRegularFile(layout.ProxyFile),
		telemetryPresent: safeDirectory(layout.TelemetryDir),
		versionPresent:   safeRegularFile(layout.VersionFile),
		statePresent:     safeDirectory(layout.StateDir),
		privatePresent:   safeDirectory(layout.PrivateDir),
	}

	if pathPresent(layout.Root) && !safeDirectory(layout.Root) {
		plan.unsafeReason = fmt.Sprintf("installation root %s is not a normal directory", layout.Root)
		return plan
	}
	for _, path := range []string{
		layout.EnvFile,
		layout.ComposeFile,
		layout.ProxyFile,
		layout.TraefikDir,
		layout.TraefikStatic,
		layout.TraefikDynamic,
		layout.TraefikCore,
		layout.TraefikGenerated,
		layout.TraefikReloadMarker,
		layout.TelemetryDir,
		layout.VersionFile,
		layout.StateDir,
		layout.PrivateDir,
		filepath.Dir(layout.ProxyFile),
		filepath.Dir(filepath.Dir(layout.ProxyFile)),
	} {
		if pathPresent(path) && !safePathType(path) {
			plan.unsafeReason = fmt.Sprintf("refusing to operate on symlink or special path %s", path)
			return plan
		}
	}

	plan.configPresent = safeRegularFile(layout.EnvFile)
	if plan.configPresent {
		plan.config, plan.configErr = readEnvFile(layout.EnvFile)
	}
	if plan.configErr == nil {
		storageDriver := "local"
		if driver := strings.TrimSpace(plan.config["STORAGE_DRIVER"]); driver != "" {
			storageDriver = strings.ToLower(driver)
		}
		plan.externalStorage = storageDriver == "s3"
	}
	plan.volumes = configuredUninstallVolumes(plan.config)
	plan.appRuntimeNetwork = configuredRuntimeNetwork(plan.config)
	plan.unknownEntries = unknownLayoutEntries(layout)
	plan.partial = !plan.configPresent || plan.configErr != nil || !plan.composePresent || !plan.proxyPresent || !plan.versionPresent
	return plan
}

func configuredUninstallVolumes(values map[string]string) []uninstallVolume {
	postgres := configuredVolumeName(values, "POSTGRES_VOLUME_NAME", "stealth_postgres_data")
	storage := configuredVolumeName(values, "STORAGE_VOLUME_NAME", "stealth_storage")
	staging := configuredVolumeName(values, "FUNCTIONS_RUNNER_STAGING_VOLUME", "stealth_function_runner_staging")
	appStaging := configuredVolumeName(values, "APPS_BUILD_STAGING_VOLUME", "stealth_app_build_staging")
	buildkitState := configuredVolumeName(values, "APPS_BUILDKIT_STATE_VOLUME", "stealth_app_buildkit_state")
	composeProject := configuredVolumeName(values, "COMPOSE_PROJECT_NAME", "stealth")
	clickhouse := configuredVolumeName(values, "CLICKHOUSE_VOLUME_NAME", "stealth_clickhouse_data")
	otelCollector := configuredVolumeName(values, "OTELCOL_VOLUME_NAME", "stealth_otelcol_state")
	otelDockerLogs := configuredVolumeName(values, "OTEL_DOCKER_LOGS_VOLUME_NAME", "stealth_otel_docker_logs_state")
	volumes := []uninstallVolume{
		{label: "PostgreSQL data volume", name: postgres, composeName: "postgres_data"},
	}
	if strings.EqualFold(strings.TrimSpace(values["STORAGE_DRIVER"]), "s3") {
		volumes = append(volumes, uninstallVolume{label: "Local staging/cache volume", name: storage, composeName: "stealth_storage"})
	} else {
		volumes = append(volumes, uninstallVolume{label: "Object storage and function/site artifacts", name: storage, composeName: "stealth_storage"})
	}
	volumes = append(volumes, uninstallVolume{label: "Function runner staging volume", name: staging, composeName: "function_runner_staging"})
	volumes = append(volumes,
		uninstallVolume{label: "App build staging volume", name: appStaging, composeName: "app_build_staging"},
		uninstallVolume{label: "App BuildKit cache volume", name: buildkitState, composeName: "buildkit_state"},
		uninstallVolume{label: "App BuildKit worker credential volume", name: composeProject + "_app_buildkit_worker_credentials", composeName: "buildkit_worker_credentials"},
		uninstallVolume{label: "App BuildKit server credential volume", name: composeProject + "_app_buildkit_server_credentials", composeName: "buildkit_server_credentials"},
		uninstallVolume{label: "Cloudflare legacy-state handoff volume", name: composeProject + "_cloudflare_setup_state_input", composeName: "cloudflare_setup_state_input"},
		uninstallVolume{label: "ClickHouse telemetry volume", name: clickhouse, composeName: "clickhouse_data"},
		uninstallVolume{label: "OTel Collector state volume", name: otelCollector, composeName: "otelcol_state"},
		uninstallVolume{label: "Docker log Collector state volume", name: otelDockerLogs, composeName: "otel_docker_logs_state"},
	)
	return uniqueUninstallVolumes(volumes)
}

func configuredVolumeName(values map[string]string, key, fallback string) string {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return fallback
	}
	return value
}

func uniqueUninstallVolumes(volumes []uninstallVolume) []uninstallVolume {
	seen := make(map[string]struct{}, len(volumes))
	result := make([]uninstallVolume, 0, len(volumes))
	for _, volume := range volumes {
		if _, ok := seen[volume.name]; ok {
			continue
		}
		seen[volume.name] = struct{}{}
		result = append(result, volume)
	}
	return result
}

func pathPresent(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func safeRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func safeDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func safePathType(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	return info.Mode().IsRegular() || info.IsDir()
}
