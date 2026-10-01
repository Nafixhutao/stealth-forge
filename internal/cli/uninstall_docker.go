package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/installengine"
)

type uninstallOperation struct {
	name   string
	action func(context.Context) error
}

func (a *App) uninstallOperations(plan uninstallPlan) []uninstallOperation {
	operations := make([]uninstallOperation, 0, 7)
	if plan.mode == uninstallPurge {
		operations = append(operations, uninstallOperation{
			name:   "Validate project-owned Docker resources",
			action: func(ctx context.Context) error { return a.validatePurgeScope(ctx, plan) },
		})
	}
	if plan.composePresent {
		operations = append(operations, uninstallOperation{
			name:   "Remove Stealth services and network",
			action: func(ctx context.Context) error { return a.removeComposeResources(ctx, plan) },
		})
	} else {
		operations = append(operations, uninstallOperation{
			name:   "Confirm Compose runtime is already absent",
			action: func(context.Context) error { return nil },
		})
	}
	if plan.mode == uninstallPurge {
		operations = append(operations, uninstallOperation{
			name:   "Remove managed App containers and runtime network",
			action: func(ctx context.Context) error { return a.removeAppRuntimeResources(ctx, plan) },
		})
	}
	if plan.mode == uninstallPurge {
		operations = append(operations, uninstallOperation{
			name:   "Remove remaining managed Docker volumes",
			action: func(ctx context.Context) error { return a.removeManagedVolumes(ctx, plan) },
		})
	}
	if plan.mode == uninstallPurge {
		operations = append(operations, uninstallOperation{
			name:   "Verify services and persistent data removal",
			action: func(ctx context.Context) error { return a.verifyDockerPurge(ctx, plan) },
		})
	} else {
		operations = append(operations, uninstallOperation{
			name:   "Verify service removal",
			action: func(ctx context.Context) error { return a.verifyServicesRemoved(ctx, plan) },
		})
	}
	if plan.mode == uninstallConfiguration || plan.mode == uninstallPurge {
		operations = append(operations, uninstallOperation{
			name: "Remove managed BuildKit AppArmor policy",
			action: func(ctx context.Context) error {
				return installengine.RemoveManagedBuildKitAppArmorProfile(ctx, a.runner, a.out, a.errOut)
			},
		})
	}
	if plan.mode == uninstallConfiguration {
		operations = append(operations, uninstallOperation{
			name:   "Remove local runtime files",
			action: func(context.Context) error { return removeLocalRuntimeFiles(plan) },
		})
	}
	if plan.mode == uninstallPurge {
		operations = append(operations, uninstallOperation{
			name:   "Remove installation state and secrets",
			action: func(context.Context) error { return removeAllLocalFiles(plan) },
		})
	}
	operations = append(operations, uninstallOperation{
		name:   "Verify uninstall",
		action: func(context.Context) error { return verifyLocalUninstall(plan) },
	})
	return operations
}

func (a *App) removeComposeResources(ctx context.Context, plan uninstallPlan) error {
	if !plan.configPresent || plan.configErr != nil {
		return fmt.Errorf("cannot remove Docker services safely because config.env is missing or unreadable")
	}
	args := a.composeArgs(plan.layout, "down", "--remove-orphans")
	if plan.mode == uninstallPurge {
		args = a.composeArgs(plan.layout, "down", "--volumes", "--remove-orphans")
	}
	return a.runCommandCaptured(ctx, plan.layout.Root, "docker", args...)
}

func (a *App) verifyServicesRemoved(ctx context.Context, plan uninstallPlan) error {
	if !plan.composePresent {
		return nil
	}
	if !plan.configPresent || plan.configErr != nil {
		return fmt.Errorf("cannot verify Docker services safely because config.env is missing or unreadable")
	}
	output, err := a.runner.Output(ctx, plan.layout.Root, "docker", a.composeArgs(plan.layout, "ps", "-aq")...)
	if err != nil {
		return fmt.Errorf("verify Docker services: %w", err)
	}
	if strings.TrimSpace(string(output)) != "" {
		return fmt.Errorf("Docker Compose still reports running or stopped containers")
	}
	return nil
}

func (a *App) validatePurgeScope(ctx context.Context, plan uninstallPlan) error {
	if !plan.canPurge() {
		return fmt.Errorf("purge requires a complete installation with readable config.env, Compose, and no unrecognized local files")
	}
	seen := make(map[string]struct{}, len(plan.volumes))
	declared := make(map[string]struct{}, len(plan.volumes))
	for _, volume := range plan.volumes {
		if !validDockerResourceName(volume.name) {
			return fmt.Errorf("refusing to purge invalid configured volume name %q", volume.name)
		}
		if _, ok := seen[volume.name]; ok {
			return fmt.Errorf("configured volume names are not unique")
		}
		seen[volume.name] = struct{}{}
		declared[volume.composeName] = struct{}{}
	}
	contents, err := os.ReadFile(plan.layout.ComposeFile)
	if err != nil {
		return fmt.Errorf("read Compose file for purge validation: %w", err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(line, "#", 2)[0]), "external: true") {
			return fmt.Errorf("refusing to purge a Compose layout with external resources")
		}
	}
	output, err := a.runner.Output(ctx, plan.layout.Root, "docker", a.composeArgs(plan.layout, "config", "--volumes")...)
	if err != nil {
		return fmt.Errorf("validate Compose volumes: %w", err)
	}
	actual := make(map[string]struct{})
	for _, raw := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(raw)
		if name != "" {
			actual[name] = struct{}{}
		}
	}
	// Older supported installations may legitimately declare only the legacy
	// volume subset. Every volume that Compose does declare must still be one
	// of the known Stealth-managed keys; unknown declarations remain fail-closed.
	for name := range actual {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("Compose declares unexpected persistent resource %q; refusing purge", name)
		}
	}
	if err := a.validateExistingVolumeOwnership(ctx, plan); err != nil {
		return err
	}
	if err := a.validateAppRuntimePurgeScope(ctx, plan); err != nil {
		return err
	}
	return nil
}

func (a *App) removeManagedVolumes(ctx context.Context, plan uninstallPlan) error {
	if err := a.validateExistingVolumeOwnership(ctx, plan); err != nil {
		return err
	}
	output, err := a.runner.Output(ctx, "", "docker", "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("inspect Docker volumes for purge: %w", err)
	}
	existing := make(map[string]struct{})
	for _, raw := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(raw)
		if name != "" {
			existing[name] = struct{}{}
		}
	}
	for _, volume := range plan.volumes {
		if _, ok := existing[volume.name]; !ok {
			continue
		}
		if err := a.runCommandCaptured(ctx, "", "docker", "volume", "rm", volume.name); err != nil {
			return fmt.Errorf("remove managed Docker volume %q: %w", volume.name, err)
		}
	}
	return nil
}

func (a *App) validateExistingVolumeOwnership(ctx context.Context, plan uninstallPlan) error {
	output, err := a.runner.Output(ctx, "", "docker", "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("inspect existing Docker volumes: %w", err)
	}
	existing := make(map[string]struct{})
	for _, raw := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(raw)
		if name != "" {
			existing[name] = struct{}{}
		}
	}
	projectName := strings.TrimSpace(plan.config["COMPOSE_PROJECT_NAME"])
	if projectName == "" {
		projectName = "stealth"
	}
	for _, volume := range plan.volumes {
		if _, ok := existing[volume.name]; !ok {
			continue
		}
		labels, inspectErr := a.runner.Output(ctx, "", "docker", "volume", "inspect", "--format", "{{ index .Labels \"com.docker.compose.project\" }}\t{{ index .Labels \"com.docker.compose.volume\" }}", volume.name)
		if inspectErr != nil {
			return fmt.Errorf("verify ownership of Docker volume %q: %w", volume.name, inspectErr)
		}
		fields := strings.SplitN(strings.TrimSpace(string(labels)), "\t", 2)
		if len(fields) != 2 || fields[0] != projectName || fields[1] != volume.composeName {
			return fmt.Errorf("Docker volume %q is not labeled as Compose project %q volume %q; refusing purge", volume.name, projectName, volume.composeName)
		}
	}
	return nil
}

func (a *App) verifyDockerPurge(ctx context.Context, plan uninstallPlan) error {
	if err := a.verifyServicesRemoved(ctx, plan); err != nil {
		return err
	}
	output, err := a.runner.Output(ctx, "", "docker", "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return fmt.Errorf("verify Docker volumes: %w", err)
	}
	remaining := make(map[string]struct{})
	for _, raw := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(raw)
		if name != "" {
			remaining[name] = struct{}{}
		}
	}
	for _, volume := range plan.volumes {
		if _, ok := remaining[volume.name]; ok {
			return fmt.Errorf("project-owned volume %q still exists", volume.name)
		}
	}
	containers, network, err := a.inspectAppRuntimeResources(ctx, plan)
	if err != nil {
		return err
	}
	if len(containers) > 0 || network != nil {
		return fmt.Errorf("managed App runtime containers or network remain after purge")
	}
	return nil
}

func validDockerResourceName(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for index, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' && char != '-' && char != '.' {
			return false
		}
		if index == 0 && char == '.' {
			return false
		}
	}
	return true
}
