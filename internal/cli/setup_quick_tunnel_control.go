package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func (a *App) closeQuickTunnel(ctx context.Context, layout InstallLayout, containerName string) error {
	if !isQuickTunnelContainerName(containerName) {
		return nil
	}
	found, err := a.quickTunnelPresent(ctx, layout, containerName)
	if err != nil {
		return fmt.Errorf("check temporary onboarding tunnel: %w", err)
	}
	if !found {
		return nil
	}
	return a.runner.Run(ctx, layout.Root, io.Discard, io.Discard, "docker", "rm", "--force", containerName)
}

func (a *App) quickTunnelPresent(ctx context.Context, layout InstallLayout, containerName string) (bool, error) {
	if !isQuickTunnelContainerName(containerName) {
		return false, nil
	}
	containers, err := a.runner.Output(ctx, layout.Root, "docker", "ps", "--all", "--filter", "name=^"+containerName+"$", "--format", "{{.Names}}")
	if err != nil {
		return false, err
	}
	for _, candidate := range strings.Split(string(containers), "\n") {
		if strings.TrimSpace(candidate) == containerName {
			return true, nil
		}
	}
	return false, nil
}

func (a *App) quickTunnelRunning(ctx context.Context, layout InstallLayout, containerName string) (bool, error) {
	if !isQuickTunnelContainerName(containerName) {
		return false, nil
	}
	containers, err := a.runner.Output(ctx, layout.Root, "docker", "ps", "--filter", "name=^"+containerName+"$", "--format", "{{.Names}}")
	if err != nil {
		return false, err
	}
	for _, candidate := range strings.Split(string(containers), "\n") {
		if strings.TrimSpace(candidate) == containerName {
			return true, nil
		}
	}
	return false, nil
}

func isQuickTunnelContainerName(value string) bool {
	return strings.HasPrefix(value, quickTunnelContainerPrefix) && safeDockerName(value)
}

func safeDockerName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			if index == 0 && (character == '.' || character == '_' || character == '-') {
				return false
			}
			continue
		}
		return false
	}
	return true
}
