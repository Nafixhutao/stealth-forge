package cli

// Setup workflow owns argument parsing, installation checks, bootstrap API
// calls, and temporary tunnel lifecycle. TUI state and rendering live in
// setup_tui.go so the operational path remains readable and independently
// testable.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/installengine"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

const (
	// Keep this image pinned. Quick Tunnels are a temporary onboarding
	// transport, not part of the production Compose stack.
	quickTunnelCloudflaredImage = "cloudflare/cloudflared:2026.9.0@sha256:ff69a2225ad7c6f85ed84fbd5f3087df46202426b2388ec60214098e0adf05e9"
	quickTunnelNetwork          = "stealth_network"
	quickTunnelContainerPrefix  = "stealth-onboarding-"
	setupPollInterval           = 2 * time.Second
)

var quickTunnelURLPattern = regexp.MustCompile(`(?i)https://[a-z0-9][a-z0-9-]*\.trycloudflare\.com`)

type bootstrapHTTPError struct {
	status int
}

func (e *bootstrapHTTPError) Error() string {
	return fmt.Sprintf("bootstrap API returned HTTP %d", e.status)
}

type bootstrapStatusPayload struct {
	SetupRequired bool `json:"setup_required"`
}

type bootstrapAdoptionAccountPayload struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Provider      string    `json:"provider"`
	ProviderLogin string    `json:"provider_login"`
	CreatedAt     time.Time `json:"created_at"`
}

type bootstrapAdoptionAccountsPayload struct {
	Accounts []bootstrapAdoptionAccountPayload `json:"accounts"`
}

type bootstrapSessionPayload struct {
	SetupCode string    `json:"setup_code"`
	ExpiresAt time.Time `json:"expires_at"`
}

type setupQuickTunnelPayload struct {
	ContainerName string `json:"container_name"`
	URL           string `json:"url"`
}

type setupPreparedMessage struct {
	session       bootstrapSessionPayload
	localURL      string
	tunnelURL     string
	containerName string
	warning       string
	complete      bool
	err           error
}

// runWebBootstrap starts only the setup Compose project. It deliberately does
// not ask for provider credentials in the terminal; the browser owns the
// reviewed configuration, while the CLI remains responsible for local Docker
// capability checks and the short-lived access tunnel.
func (a *App) runWebBootstrap(ctx context.Context, checks []SystemCheck, layout InstallLayout, version string, existing bool) int {
	if !checksPass(checks) {
		fmt.Fprintf(a.errOut, "system requirements are not satisfied: %s\n", failedCheckSummary(checks))
		return 1
	}
	coordinationLock, err := installengine.AcquireProcessLock(layout.StateDir, setupCoordinationLockName, "setup orchestration")
	if err != nil {
		fmt.Fprintln(a.errOut, err)
		fmt.Fprintln(a.errOut, "Another `stealth install` is already coordinating browser setup.")
		return 1
	}
	defer coordinationLock.Close()
	var values map[string]string
	var state setupstate.State
	var stateErr error
	stateAvailable := false
	if existing {
		values, err = readEnvFile(layout.EnvFile)
		if err != nil {
			fmt.Fprintf(a.errOut, "could not read setup configuration: %v\n", err)
			return 1
		}
		stateStore, storeErr := a.setupStateStore(values)
		if storeErr != nil {
			fmt.Fprintf(a.errOut, "could not prepare browser setup state: %v\n", storeErr)
			return 1
		}
		if storeErr := stateStore.PrepareShared(); storeErr != nil {
			fmt.Fprintf(a.errOut, "could not prepare browser setup state permissions: %v\n", storeErr)
			return 1
		}
	}
	setupServiceResumed := false
	pendingInstall := false
	if existing {
		state, stateErr = a.loadSetupState(values)
		if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
			fmt.Fprintf(a.errOut, "could not read browser setup state: %v\n", stateErr)
			return 1
		}
		stateAvailable = stateErr == nil
		pendingInstall = stateErr == nil && setupInstallationPending(state)
		if pendingInstall {
			setupPorts := setupPortsFromConfig(values)
			apiURL := "http://127.0.0.1:" + setupPorts.API
			if _, statusErr := a.bootstrapStatus(ctx, apiURL+"/v1/bootstrap/status"); statusErr == nil {
				setupServiceResumed = true
			}
		}
	}

	var gid uint32
	configContents := ""
	if !setupServiceResumed {
		gid, err = dockerSocketGID("/var/run/docker.sock")
		if err != nil {
			fmt.Fprintf(a.errOut, "Docker socket is not accessible: %v\n", err)
			return 1
		}
	}
	if !existing {
		appArmorProfile, profileErr := installengine.DetectBuildKitAppArmorProfile()
		if profileErr != nil {
			fmt.Fprintf(a.errOut, "could not detect BuildKit AppArmor requirements: %v\n", profileErr)
			return 1
		}
		configContents, err = installengine.GenerateConfig(installengine.ConfigOptions{
			Version:                     version,
			PublicURL:                   "http://localhost:8081",
			DockerGID:                   gid,
			Setup:                       true,
			InstallRoot:                 layout.Root,
			AppsBuildKitAppArmorProfile: appArmorProfile,
		})
		if err != nil {
			fmt.Fprintf(a.errOut, "could not prepare setup configuration: %v\n", err)
			return 1
		}
	}
	plan := InstallPlan{
		Layout:             layout,
		Version:            version,
		PublicURL:          "http://localhost:8081",
		DockerGID:          gid,
		Setup:              true,
		ConfigContents:     configContents,
		InternalAPIURL:     "http://127.0.0.1:" + setupPortsFromConfig(values).API,
		InternalConsoleURL: "http://127.0.0.1:" + setupPortsFromConfig(values).Console,
		InternalProxyURL:   "http://127.0.0.1:" + setupPortsFromConfig(values).Proxy,
		Existing:           existing,
	}
	if !setupServiceResumed {
		if err := a.installEngine().Install(ctx, plan, nil); err != nil {
			if pendingInstall && strings.Contains(err.Error(), "another installation operation") {
				setupPorts := setupPortsFromConfig(values)
				apiURL := "http://127.0.0.1:" + setupPorts.API
				if waitErr := a.waitForSetupAPI(ctx, apiURL+"/v1/bootstrap/status"); waitErr == nil {
					setupServiceResumed = true
				} else {
					fmt.Fprintf(a.errOut, "could not resume the setup service: %v\n", waitErr)
					fmt.Fprintln(a.errOut, "Configuration was preserved; run `stealth install --repair --wait` when the setup service is available.")
					return 1
				}
			} else {
				fmt.Fprintf(a.errOut, "could not start the setup service: %v\n", err)
				fmt.Fprintln(a.errOut, "Configuration was preserved; run `stealth install --repair` or `stealth doctor` for diagnostics.")
				return 1
			}
		}
	}
	fmt.Fprintln(a.out, "✓ System requirements checked")
	if setupServiceResumed {
		fmt.Fprintln(a.out, "✓ Setup service resumed")
	} else {
		fmt.Fprintln(a.out, "✓ Setup service started")
	}
	if values == nil {
		values, err = readEnvFile(layout.EnvFile)
		if err != nil {
			fmt.Fprintf(a.errOut, "could not read setup configuration: %v\n", err)
			return 1
		}
	}
	if err := a.persistHostPreflight(values, checks); err != nil {
		fmt.Fprintf(a.errOut, "could not publish host system checks to setup: %v\n", err)
		return 1
	}
	key, err := bootstrapCLIKey(values)
	if err != nil {
		fmt.Fprintln(a.errOut, err)
		return 1
	}
	setupPorts := setupPortsFromConfig(values)
	apiURL := "http://127.0.0.1:" + setupPorts.API
	status, err := a.bootstrapStatus(ctx, apiURL+"/v1/bootstrap/status")
	if err != nil {
		fmt.Fprintf(a.errOut, "could not read first-run setup state: %v\n", err)
		return 1
	}
	if !stateAvailable {
		state, stateErr = a.loadSetupState(values)
		stateAvailable = stateErr == nil
	}
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		fmt.Fprintf(a.errOut, "could not read browser setup state: %v\n", stateErr)
		return 1
	}
	if !status.SetupRequired && stateErr == nil && state.Phase == setupstate.PhaseComplete {
		fmt.Fprintln(a.out, "Instance setup has already been completed.")
		return 0
	}
	resumeSession := pendingInstall && stateAvailable
	var session bootstrapSessionPayload
	if !resumeSession {
		if status.SetupRequired {
			session, err = a.createBootstrapSession(ctx, apiURL+"/v1/bootstrap/sessions", key)
		} else {
			session, err = a.recoverSetupSession(ctx, apiURL+"/v1/setup/recovery", key)
		}
		if err != nil {
			fmt.Fprintf(a.errOut, "could not create a setup session: %v\n", err)
			return 1
		}
	}
	containerName := newSetupContainerName()
	quickURL := ""
	tunnelResumed := false
	if resumeSession {
		if name := strings.TrimSpace(state.QuickTunnel); isQuickTunnelContainerName(name) {
			if candidate, found := findQuickTunnelURL([]byte(state.Secret("quick_tunnel_url"))); found {
				present, presentErr := a.quickTunnelRunning(ctx, layout, name)
				if presentErr != nil {
					fmt.Fprintf(a.errOut, "could not inspect the existing temporary setup tunnel: %v\n", presentErr)
					return 1
				}
				if present {
					containerName = name
					quickURL = candidate
					tunnelResumed = true
				}
			}
		}
	}
	if quickURL == "" {
		if previousName := a.previousSetupTunnel(values); previousName != "" && previousName != containerName {
			cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			cleanupErr := a.closeQuickTunnel(cleanupContext, layout, previousName)
			cleanupCancel()
			if cleanupErr != nil {
				fmt.Fprintf(a.errOut, "could not clean up the previous temporary setup tunnel: %v\n", cleanupErr)
				return 1
			}
		}
		network := strings.TrimSpace(values["STEALTH_NETWORK_NAME"])
		if network == "" {
			network = quickTunnelNetwork
		}
		var tunnelErr error
		quickURL, tunnelErr = a.startQuickTunnelTo(ctx, layout, network, containerName, "http://setup-proxy:80")
		if tunnelErr != nil {
			cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = a.closeQuickTunnel(cleanupContext, layout, containerName)
			cleanupCancel()
			a.printQuickTunnelFallback(tunnelErr)
			return a.orchestrateBrowserSetupLocked(ctx, layout, values, containerName)
		}
	}
	if err := a.registerQuickTunnel(ctx, apiURL+"/v1/setup/quick-tunnel", key, containerName, quickURL); err != nil {
		if !tunnelResumed {
			cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = a.closeQuickTunnel(cleanupContext, layout, containerName)
			cleanupCancel()
		}
		fmt.Fprintf(a.errOut, "could not register the temporary setup tunnel: %v\n", err)
		return 1
	}
	if tunnelResumed {
		fmt.Fprintln(a.out, "✓ Temporary setup tunnel resumed")
	} else {
		fmt.Fprintln(a.out, "✓ Temporary setup tunnel started")
	}
	fmt.Fprintf(a.out, "\nOpen %s/setup\n", quickURL)
	if session.SetupCode != "" {
		fmt.Fprintf(a.out, "Setup code: %s\n", session.SetupCode)
		fmt.Fprintf(a.out, "Expires: %s (15 minutes)\n", session.ExpiresAt.UTC().Format(time.RFC3339))
	} else {
		fmt.Fprintln(a.out, "Existing browser setup session resumed; reconnect to the setup URL to continue.")
	}
	fmt.Fprintln(a.out, "Complete setup in the browser. The temporary tunnel closes after the named production tunnel is verified.")
	return a.orchestrateBrowserSetupLocked(ctx, layout, values, containerName)
}
