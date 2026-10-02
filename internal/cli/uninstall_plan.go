package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func unknownLayoutEntries(layout InstallLayout) []string {
	if !safeDirectory(layout.Root) {
		return nil
	}
	consoleName := filepath.Base(filepath.Dir(filepath.Dir(layout.ProxyFile)))
	telemetryName := filepath.Base(layout.TelemetryDir)
	allowed := map[string]bool{
		filepath.Base(layout.EnvFile):     true,
		filepath.Base(layout.ComposeFile): true,
		telemetryName:                     true,
		filepath.Base(layout.VersionFile): true,
		filepath.Base(layout.StateDir):    true,
		filepath.Base(layout.PrivateDir):  true,
		consoleName:                       true,
		filepath.Base(layout.TraefikDir):  true,
	}
	var unknown []string
	entries, err := os.ReadDir(layout.Root)
	if err != nil {
		return []string{layout.Root + " (cannot inspect: " + err.Error() + ")"}
	}
	for _, entry := range entries {
		name := entry.Name()
		if !allowed[name] {
			unknown = append(unknown, filepath.Join(layout.Root, name))
			continue
		}
		switch name {
		case consoleName:
			consoleDir := filepath.Join(layout.Root, name)
			if !entry.IsDir() {
				unknown = append(unknown, consoleDir)
				continue
			}
			deployDir := filepath.Dir(layout.ProxyFile)
			consoleEntries, consoleErr := os.ReadDir(consoleDir)
			if consoleErr != nil {
				unknown = append(unknown, consoleDir+" (cannot inspect: "+consoleErr.Error()+")")
				continue
			}
			for _, consoleEntry := range consoleEntries {
				if consoleEntry.Name() != filepath.Base(deployDir) {
					unknown = append(unknown, filepath.Join(consoleDir, consoleEntry.Name()))
				}
			}
			deployEntries, deployErr := os.ReadDir(deployDir)
			if deployErr != nil && !errors.Is(deployErr, os.ErrNotExist) {
				unknown = append(unknown, deployDir+" (cannot inspect: "+deployErr.Error()+")")
				continue
			}
			for _, deployEntry := range deployEntries {
				if deployEntry.Name() != filepath.Base(layout.ProxyFile) {
					unknown = append(unknown, filepath.Join(deployDir, deployEntry.Name()))
				}
			}
		case telemetryName:
			telemetryDir := filepath.Join(layout.Root, name)
			if !entry.IsDir() {
				unknown = append(unknown, telemetryDir)
				continue
			}
			telemetryEntries, telemetryErr := os.ReadDir(telemetryDir)
			if telemetryErr != nil {
				unknown = append(unknown, telemetryDir+" (cannot inspect: "+telemetryErr.Error()+")")
				continue
			}
			for _, telemetryEntry := range telemetryEntries {
				switch telemetryEntry.Name() {
				case "otel-collector.yaml", "host-metrics.yaml", "docker-logs.yaml", "docker-stats.yaml":
				default:
					unknown = append(unknown, filepath.Join(telemetryDir, telemetryEntry.Name()))
				}
			}
		case filepath.Base(layout.TraefikDir):
			traefikDir := filepath.Join(layout.Root, name)
			if !entry.IsDir() {
				unknown = append(unknown, traefikDir)
				continue
			}
			traefikEntries, traefikErr := os.ReadDir(traefikDir)
			if traefikErr != nil {
				unknown = append(unknown, traefikDir+" (cannot inspect: "+traefikErr.Error()+")")
				continue
			}
			for _, traefikEntry := range traefikEntries {
				switch traefikEntry.Name() {
				case filepath.Base(layout.TraefikStatic), filepath.Base(layout.TraefikDynamic):
				default:
					unknown = append(unknown, filepath.Join(traefikDir, traefikEntry.Name()))
				}
			}
			dynamicDir := layout.TraefikDynamic
			dynamicInfo, dynamicErr := os.Lstat(dynamicDir)
			if dynamicErr != nil {
				if !errors.Is(dynamicErr, os.ErrNotExist) {
					unknown = append(unknown, dynamicDir+" (cannot inspect: "+dynamicErr.Error()+")")
				}
				continue
			}
			if !dynamicInfo.IsDir() || dynamicInfo.Mode()&os.ModeSymlink != 0 {
				unknown = append(unknown, dynamicDir)
				continue
			}
			dynamicEntries, dynamicErr := os.ReadDir(dynamicDir)
			if dynamicErr != nil {
				unknown = append(unknown, dynamicDir+" (cannot inspect: "+dynamicErr.Error()+")")
				continue
			}
			for _, dynamicEntry := range dynamicEntries {
				switch dynamicEntry.Name() {
				case filepath.Base(layout.TraefikCore), filepath.Base(layout.TraefikGenerated), filepath.Base(layout.TraefikReloadMarker):
				default:
					unknown = append(unknown, filepath.Join(dynamicDir, dynamicEntry.Name()))
				}
			}
			generatedInfo, generatedErr := os.Lstat(layout.TraefikGenerated)
			if generatedErr == nil {
				if !generatedInfo.IsDir() || generatedInfo.Mode()&os.ModeSymlink != 0 {
					unknown = append(unknown, layout.TraefikGenerated)
					continue
				}
				generatedEntries, readErr := os.ReadDir(layout.TraefikGenerated)
				if readErr != nil {
					unknown = append(unknown, layout.TraefikGenerated+" (cannot inspect: "+readErr.Error()+")")
					continue
				}
				for _, generatedEntry := range generatedEntries {
					generatedPath := filepath.Join(layout.TraefikGenerated, generatedEntry.Name())
					generatedInfo, statErr := os.Lstat(generatedPath)
					if generatedEntry.Name() != ".gitkeep" || statErr != nil || generatedInfo.Mode()&os.ModeSymlink != 0 || !generatedInfo.Mode().IsRegular() {
						// Generated route files are host/control-plane state, not
						// release assets. Keep purge from deleting them implicitly.
						unknown = append(unknown, generatedPath)
					}
				}
			} else if !errors.Is(generatedErr, os.ErrNotExist) {
				unknown = append(unknown, layout.TraefikGenerated+" (cannot inspect: "+generatedErr.Error()+")")
			}
		}
	}
	slices.Sort(unknown)
	return unknown
}

func uninstallModeName(mode uninstallMode) string {
	switch mode {
	case uninstallServices:
		return "Preserve data (services only)"
	case uninstallConfiguration:
		return "Remove services + local runtime files"
	case uninstallPurge:
		return "PURGE: permanently delete instance data"
	default:
		return "Unknown"
	}
}

func (p uninstallPlan) removalItems() []string {
	items := make([]string, 0, 16)
	if p.composePresent {
		for _, service := range []string{"API container", "Worker container", "Console container", "Proxy container", "PostgreSQL container", "Redis container", "Migration container"} {
			items = append(items, service)
		}
		items = append(items, "Compose network and runtime state")
	} else {
		items = append(items, "Compose services (already absent; Compose file is missing)")
	}
	if p.mode == uninstallConfiguration || p.mode == uninstallPurge {
		for _, asset := range p.localAssets(p.mode == uninstallPurge) {
			if asset.present {
				items = append(items, asset.label)
			}
		}
	}
	if p.mode == uninstallPurge {
		items = append(items, "Managed persistent App containers and App runtime network ("+p.appRuntimeNetwork+")")
		for _, volume := range p.volumes {
			items = append(items, volume.label+" ("+volume.name+")")
		}
	}
	return items
}

func (p uninstallPlan) preservedItems() []string {
	items := make([]string, 0, 16)
	if p.mode != uninstallPurge {
		items = append(items, "Persistent App containers and App runtime network")
		for _, volume := range p.volumes {
			items = append(items, volume.label+" ("+volume.name+")")
		}
	}
	if p.mode == uninstallServices {
		for _, asset := range p.localAssets(false) {
			if asset.present {
				items = append(items, asset.label)
			}
		}
		if p.configPresent {
			items = append(items, "config.env (secrets and recovery configuration)")
		}
	}
	if p.mode != uninstallPurge && p.privatePresent {
		items = append(items, "private/ (BuildKit control-plane credentials)")
	}
	if p.mode == uninstallConfiguration && p.configPresent {
		items = append(items, "config.env (recovery secrets and encryption key)")
	}
	items = append(items, "Stealth CLI executable (not modified)")
	if p.externalStorage {
		items = append(items, "External S3 object storage (not managed by this CLI)")
	}
	return items
}

type uninstallAsset struct {
	label   string
	path    string
	present bool
}

func (p uninstallPlan) localAssets(includeConfig bool) []uninstallAsset {
	assets := []uninstallAsset{
		{label: "compose.production.yaml", path: p.layout.ComposeFile, present: p.composePresent},
		{label: "telemetry/", path: p.layout.TelemetryDir, present: p.telemetryPresent},
		{label: "console/deploy/nginx.conf", path: p.layout.ProxyFile, present: p.proxyPresent},
		{label: "traefik/traefik.yaml", path: p.layout.TraefikStatic, present: safeRegularFile(p.layout.TraefikStatic)},
		{label: "traefik/dynamic/core.yaml", path: p.layout.TraefikCore, present: safeRegularFile(p.layout.TraefikCore)},
		{label: "traefik/dynamic/.reload.yaml", path: p.layout.TraefikReloadMarker, present: safeRegularFile(p.layout.TraefikReloadMarker)},
		{label: "traefik/dynamic/generated/.gitkeep", path: filepath.Join(p.layout.TraefikGenerated, ".gitkeep"), present: safeRegularFile(filepath.Join(p.layout.TraefikGenerated, ".gitkeep"))},
		{label: "VERSION", path: p.layout.VersionFile, present: p.versionPresent},
		{label: "state/", path: p.layout.StateDir, present: p.statePresent},
	}
	if includeConfig {
		assets = append(assets, uninstallAsset{label: "config.env and local secrets", path: p.layout.EnvFile, present: p.configPresent})
		assets = append(assets, uninstallAsset{label: "private/ (BuildKit control-plane credentials)", path: p.layout.PrivateDir, present: p.privatePresent})
	}
	return assets
}

func (p uninstallPlan) warnings() []string {
	var warnings []string
	if p.partial {
		var missing []string
		if !p.configPresent {
			missing = append(missing, "config.env")
		} else if p.configErr != nil {
			missing = append(missing, "readable config.env")
		}
		if !p.composePresent {
			missing = append(missing, "compose.production.yaml")
		}
		if !p.proxyPresent {
			missing = append(missing, "console/deploy/nginx.conf")
		}
		if !p.versionPresent {
			missing = append(missing, "VERSION")
		}
		if len(missing) > 0 {
			warnings = append(warnings, "Partial installation detected; missing "+strings.Join(missing, ", ")+".")
		}
	}
	if p.configErr != nil {
		warnings = append(warnings, "config.env is preserved but could not be parsed: "+p.configErr.Error())
	}
	if p.mode == uninstallConfiguration && p.configPresent {
		warnings = append(warnings, "config.env is kept because FUNCTIONS_SECRET_KEY and database credentials are required to recover preserved data.")
	}
	if p.externalStorage {
		warnings = append(warnings, "External S3 object storage is not deleted; remove its owned bucket/prefix with provider tooling after verifying ownership.")
	}
	if len(p.unknownEntries) > 0 {
		warnings = append(warnings, "Unrecognized files are preserved: "+strings.Join(p.unknownEntries, ", "))
	}
	if p.mode == uninstallPurge && !p.canPurge() {
		warnings = append(warnings, "Purge is blocked until the complete, readable installation layout can be validated; no data will be deleted.")
	}
	return warnings
}

func (p uninstallPlan) canPurge() bool {
	return p.configPresent && p.configErr == nil && p.composePresent && len(p.unknownEntries) == 0 && p.unsafeReason == ""
}

func printUninstallPlan(w io.Writer, plan uninstallPlan) {
	fmt.Fprintln(w, "Removal plan")
	fmt.Fprintf(w, "Instance       %s\n", plan.layout.Root)
	fmt.Fprintf(w, "Mode           %s\n", uninstallModeName(plan.mode))
	if plan.partial {
		fmt.Fprintln(w, "Status         Partial installation")
	}
	fmt.Fprintln(w, "\nWill remove")
	for _, item := range plan.removalItems() {
		fmt.Fprintf(w, "✓ %s\n", item)
	}
	fmt.Fprintln(w, "\nWill preserve")
	for _, item := range plan.preservedItems() {
		fmt.Fprintf(w, "✓ %s\n", item)
	}
	if warnings := plan.warnings(); len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings")
		for _, warning := range warnings {
			fmt.Fprintf(w, "! %s\n", warning)
		}
	}
	if plan.mode == uninstallPurge {
		fmt.Fprintln(w, "\nThis is permanent. Back up important database and storage data before continuing.")
	}
}
