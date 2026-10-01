package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Stealth-deplover/stealth/internal/installengine"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

func (a *App) runSetup(args []string) int {
	fs := flag.NewFlagSet("stealth setup", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	adoptOwner := fs.Bool("adopt-owner", false, "assign an existing account as Instance Owner on a legacy installation")
	fs.Usage = func() {
		fmt.Fprintln(a.errOut, "Usage: stealth setup [--adopt-owner]")
		fmt.Fprintln(a.errOut)
		fmt.Fprintln(a.errOut, "Resume first-run Instance Owner setup for an installed Stealth instance.")
		fmt.Fprintln(a.errOut, "The setup code expires after 15 minutes and the temporary onboarding tunnel is closed after use.")
		fmt.Fprintln(a.errOut, "Use --adopt-owner only from the local operator terminal to migrate an existing account.")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(a.errOut, "setup does not accept positional arguments")
		return 2
	}
	if *adoptOwner && !a.hasInteractiveTerminal() {
		fmt.Fprintln(a.errOut, "Owner adoption requires an interactive TTY.")
		fmt.Fprintln(a.errOut, "Run `stealth setup --adopt-owner` from a terminal; the installation was left unchanged.")
		return 1
	}
	ctx, stop := signalContext()
	defer stop()
	if *adoptOwner {
		return a.runAdoptOwnerWithContext(ctx)
	}
	return a.runSetupWithContext(ctx)
}

func (a *App) runAdoptOwnerWithContext(ctx context.Context) int {
	layout, err := a.layout()
	if err != nil {
		fmt.Fprintf(a.errOut, "cannot determine installation directory: %v\n", err)
		return 1
	}
	if !installationExists(layout) {
		fmt.Fprintln(a.errOut, "No complete Stealth installation was found. The installation was left unchanged.")
		return 1
	}
	values, err := readEnvFile(layout.EnvFile)
	if err != nil {
		fmt.Fprintf(a.errOut, "could not read installation configuration: %v\n", err)
		return 1
	}
	key, err := bootstrapCLIKey(values)
	if err != nil {
		fmt.Fprintln(a.errOut, err)
		return 1
	}
	ports := portsFromConfig(values)
	accounts, err := a.bootstrapAdoptionAccounts(ctx, "http://127.0.0.1:"+ports.API+"/v1/bootstrap/adoption/accounts", key)
	if err != nil {
		fmt.Fprintf(a.errOut, "could not list accounts eligible for adoption: %v\n", err)
		fmt.Fprintln(a.errOut, "The installation and data were left unchanged.")
		return 1
	}
	if len(accounts) == 0 {
		fmt.Fprintln(a.out, "No existing account is available for Instance Owner adoption.")
		return 1
	}
	fmt.Fprintln(a.out, "Existing installation: choose the account to become the Instance Owner")
	fmt.Fprintln(a.out)
	for index, account := range accounts {
		identity := account.Email
		if identity == "" && account.ProviderLogin != "" {
			identity = "GitHub @" + account.ProviderLogin
		}
		if identity == "" {
			identity = "account without email"
		}
		fmt.Fprintf(a.out, "  %d. %s  (%s)\n", index+1, identity, account.ID)
	}
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "This assigns a privileged instance-level role; it does not change organization membership.")
	fmt.Fprintln(a.out, "Type the exact confirmation below to continue. Anything else cancels safely.")

	reader := bufio.NewReader(a.in)
	fmt.Fprint(a.out, "Confirm with: ADOPT <account-id>\n> ")
	line, readErr := reader.ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		fmt.Fprintf(a.errOut, "could not read confirmation: %v\n", readErr)
		return 1
	}
	confirmation := strings.TrimSpace(line)
	selectedID := ""
	for _, account := range accounts {
		if confirmation == "ADOPT "+account.ID {
			selectedID = account.ID
			break
		}
	}
	if selectedID == "" {
		fmt.Fprintln(a.out, "Adoption cancelled. The installation and data were left unchanged.")
		return 1
	}
	if err := a.adoptBootstrapOwner(ctx, "http://127.0.0.1:"+ports.API+"/v1/bootstrap/adoption/owner", key, selectedID); err != nil {
		fmt.Fprintf(a.errOut, "Instance Owner adoption failed: %v\n", err)
		fmt.Fprintln(a.errOut, "The installation and data were left unchanged.")
		return 1
	}
	fmt.Fprintf(a.out, "Instance Owner assigned to account %s.\n", selectedID)
	return 0
}

func (a *App) runSetupWithContext(ctx context.Context) int {
	layout, err := a.layout()
	if err != nil {
		fmt.Fprintf(a.errOut, "cannot determine installation directory: %v\n", err)
		return 1
	}
	if !installationExists(layout) {
		if partialInstallationExists(layout) {
			fmt.Fprintf(a.errOut, "A partial Stealth installation was found at %s, but config.env is missing.\n", layout.Root)
			fmt.Fprintln(a.errOut, "The installation was left unchanged. Run `stealth install --repair` or `stealth doctor` to recover it.")
			return 1
		}
		fmt.Fprintln(a.errOut, "No Stealth installation was found. Run `stealth install` first.")
		return 1
	}
	values, err := readEnvFile(layout.EnvFile)
	if err != nil {
		fmt.Fprintf(a.errOut, "could not read installation configuration: %v\n", err)
		return 1
	}
	if strings.EqualFold(strings.TrimSpace(values["SETUP_MODE"]), "true") {
		version := readVersion(layout)
		if version == "" {
			version, err = imageVersion(values["STEALTH_API_IMAGE"])
			if err != nil {
				fmt.Fprintf(a.errOut, "could not determine setup release version: %v\n", err)
				return 1
			}
		}
		return a.runWebBootstrap(ctx, a.systemChecks(ctx, layout.Root), layout, version, true)
	}
	if statePath := strings.TrimSpace(values["STEALTH_SETUP_STATE_FILE"]); statePath != "" && installengine.FileExists(statePath) {
		if state, stateErr := a.loadSetupState(values); stateErr == nil && state.InstallRunID != "" && state.Phase != setupstate.PhaseComplete {
			version := readVersion(layout)
			if version == "" {
				version, err = imageVersion(values["STEALTH_API_IMAGE"])
				if err != nil {
					fmt.Fprintf(a.errOut, "could not determine setup release version: %v\n", err)
					return 1
				}
			}
			return a.runWebBootstrap(ctx, a.systemChecks(ctx, layout.Root), layout, version, true)
		}
	}
	ports := portsFromConfig(values)
	apiURL := "http://127.0.0.1:" + ports.API
	status, err := a.bootstrapStatus(ctx, apiURL+"/v1/bootstrap/status")
	if err != nil {
		fmt.Fprintf(a.errOut, "could not read first-run setup state: %v\n", err)
		fmt.Fprintln(a.errOut, "The installation was left unchanged. Run `stealth doctor` for diagnostics.")
		return 1
	}
	if !status.SetupRequired {
		fmt.Fprintln(a.out, "Instance setup has already been completed.")
		return 0
	}
	if !a.hasInteractiveTerminal() {
		fmt.Fprintln(a.errOut, "First-run owner setup requires an interactive TTY.")
		return 1
	}
	if err := a.waitForInstallation(ctx, InstallPlan{Layout: layout}); err != nil {
		fmt.Fprintf(a.errOut, "the local Stealth services are not healthy: %v\n", err)
		fmt.Fprintln(a.errOut, "No temporary onboarding tunnel was started and the installation was left unchanged.")
		fmt.Fprintln(a.errOut, "Run `stealth doctor` and retry `stealth setup` after the stack is healthy.")
		return 1
	}
	return a.runSetupTUI(ctx, layout, values, apiURL, "http://127.0.0.1:"+ports.Proxy+"/setup")
}

// signalContext mirrors the install command's signal handling while keeping
// the setup flow independently resumable after a user cancellation.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
func (a *App) prepareSetup(ctx context.Context, layout InstallLayout, values map[string]string, apiURL, localURL, containerName, existingTunnelURL string, tunnelState *setupTunnelState) setupPreparedMessage {
	status, err := a.bootstrapStatus(ctx, apiURL+"/v1/bootstrap/status")
	if err != nil {
		if isBootstrapComplete(err) {
			return setupPreparedMessage{complete: true}
		}
		return setupPreparedMessage{err: err}
	}
	if !status.SetupRequired {
		return setupPreparedMessage{complete: true}
	}
	key, err := bootstrapCLIKey(values)
	if err != nil {
		return setupPreparedMessage{err: err}
	}
	session, err := a.createBootstrapSession(ctx, apiURL+"/v1/bootstrap/sessions", key)
	if err != nil {
		if isBootstrapComplete(err) {
			return setupPreparedMessage{complete: true}
		}
		return setupPreparedMessage{err: err}
	}
	message := setupPreparedMessage{session: session, localURL: localURL, containerName: containerName}
	if existingTunnelURL != "" {
		message.tunnelURL = existingTunnelURL
		return message
	}
	network := strings.TrimSpace(values["STEALTH_NETWORK_NAME"])
	if network == "" {
		network = quickTunnelNetwork
	}
	if !safeDockerName(network) {
		message.warning = "the temporary tunnel could not use the configured Docker network; continue with the local setup URL"
		message.containerName = ""
		return message
	}
	// Record the generated name before Docker is invoked. If Ctrl+C arrives
	// while docker run or log discovery is still in flight, the caller can
	// still find and remove this exact temporary container.
	if tunnelState != nil {
		tunnelState.markStarted(containerName)
	}
	tunnelName, tunnelErr := a.startQuickTunnel(ctx, layout, network, containerName)
	if tunnelErr != nil {
		if tunnelName != "" {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			cleanupErr := a.closeQuickTunnel(cleanupContext, layout, tunnelName)
			cancel()
			if cleanupErr != nil {
				return setupPreparedMessage{err: fmt.Errorf("temporary onboarding tunnel failed and could not be cleaned up: %w", cleanupErr)}
			}
		}
		message.warning = "the temporary tunnel was unavailable; continue with the local setup URL"
		message.containerName = ""
		return message
	}
	message.tunnelURL = tunnelName
	return message
}
