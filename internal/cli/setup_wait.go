package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/installengine"
	"github.com/Nafixhutao/stealth-forge/internal/setupstate"
)

// setupCoordinationLockName guards the long-lived host process that watches
// the browser wizard and executes the requested installation. It is distinct
// from the install engine's own lock so coordination and execution can recover
// independently after a process crash.
const setupCoordinationLockName = "setup-coordination.lock"

// setupProgressSource is the read side of the shared, encrypted setup state.
type setupProgressSource interface {
	Load(context.Context) (setupstate.State, error)
}

type setupObservationResult struct {
	exitCode          int
	cancelled         bool
	installRequested  bool
	lastObservedPhase string
	err               error
}

var errHostInstallerIncomplete = errors.New("host installer returned before persisting a terminal setup state")

func setupInstallationPending(state setupstate.State) bool {
	switch state.Phase {
	case setupstate.PhaseInstallRequested, setupstate.PhaseInstalling, setupstate.PhaseHandoff, setupstate.PhaseFailed:
		return true
	default:
		return false
	}
}

func (a *App) waitForSetupAPI(ctx context.Context, endpoint string) error {
	var lastErr error
	for attempt := range positiveAttempts(a.pollAttempts) {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := a.bootstrapStatus(ctx, endpoint)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt+1 < positiveAttempts(a.pollAttempts) {
			if err := waitSetupPoll(ctx, setupWaitInterval(a)); err != nil {
				return err
			}
		}
	}
	if lastErr == nil {
		return errors.New("setup service did not become available")
	}
	return lastErr
}

// shouldWaitForSetup is intentionally always true. A fresh browser request has
// no supported worker other than this host process, so allowing it to exit
// would leave install_requested with no executor.
func (a *App) shouldWaitForSetup() bool {
	return true
}

// orchestrateBrowserSetup acquires the single-orchestrator lock and then
// observes the shared setup state until the installation reaches a terminal
// phase.
func (a *App) orchestrateBrowserSetup(
	ctx context.Context,
	layout InstallLayout,
	values map[string]string,
	containerName string,
) int {
	lock, err := installengine.AcquireProcessLock(layout.StateDir, setupCoordinationLockName, "setup orchestration")
	if err != nil {
		fmt.Fprintf(a.errOut, "%v\n", err)
		fmt.Fprintln(a.errOut, "Another `stealth install` is already waiting for browser setup.")
		return 1
	}
	defer lock.Close()
	return a.orchestrateBrowserSetupLocked(ctx, layout, values, containerName)
}

func (a *App) orchestrateBrowserSetupLocked(
	ctx context.Context,
	layout InstallLayout,
	values map[string]string,
	containerName string,
) int {
	store, err := a.setupStateStore(values)
	if err != nil {
		fmt.Fprintf(a.errOut, "could not read browser setup state: %v\n", err)
		return 1
	}
	result := a.observeBrowserSetupWithInstaller(
		ctx,
		store,
		func(installContext context.Context, state setupstate.State) error {
			return a.executeHostInstallation(installContext, layout, values, store, state.InstallRunID)
		},
	)
	if !result.cancelled {
		return result.exitCode
	}
	if result.installRequested {
		fmt.Fprintln(a.errOut, "The installation request is saved and remains resumable.")
		fmt.Fprintln(a.errOut, "Run `stealth install --repair --wait` to observe or recover it.")
		return result.exitCode
	}
	cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	cleanupErr := a.cleanupUnrequestedSetup(cleanupContext, layout, store, containerName)
	cleanupCancel()
	if cleanupErr != nil {
		fmt.Fprintf(a.errOut, "temporary setup cleanup could not be completed: %v\n", cleanupErr)
		fmt.Fprintln(
			a.errOut,
			"The setup state was left unchanged; run `stealth install --repair --wait` to recover it.",
		)
		return 1
	}
	fmt.Fprintln(a.out, "Temporary setup resources cleaned up.")
	return result.exitCode
}

// observeBrowserSetup reports safe, non-secret milestones while the browser
// wizard runs. It never prints setup codes, cookies, or credentials.
func (a *App) observeBrowserSetup(ctx context.Context, source setupProgressSource) setupObservationResult {
	return a.observeBrowserSetupWithInstaller(ctx, source, nil)
}

// observeBrowserSetupWithInstaller keeps the browser-facing waiting behavior
// intact while giving the host CLI the execution hook for a durable request.
// The hook runs synchronously under the host coordination process; progress is
// persisted by the installer so a browser reconnect never depends on this
// process-local loop or an in-memory event buffer.
func (a *App) observeBrowserSetupWithInstaller(
	ctx context.Context,
	source setupProgressSource,
	installer func(context.Context, setupstate.State) error,
) setupObservationResult {
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Waiting for browser setup to complete...")
	fmt.Fprintln(a.errOut, "Complete the wizard in your browser. Press Ctrl+C to stop waiting.")

	interval := setupWaitInterval(a)
	step := setupInstallerStep{source: source, installer: installer}
	var requested bool
	var expiredNotice bool
	var lastStep string
	var lastPhase string
	var stateReadErrorShown bool
	for {
		state, err := source.Load(ctx)
		if err != nil {
			if !stateReadErrorShown {
				stateReadErrorShown = true
				fmt.Fprintln(a.errOut, "Waiting for the setup state file to become available...")
			}
			if result, stop := a.awaitSetupTick(ctx, source, interval, requested, lastPhase); stop {
				return result
			}
			continue
		}
		stateReadErrorShown = false
		lastPhase = state.Phase
		if setupstate.InstallationRequested(state) && !requested {
			requested = true
			fmt.Fprintln(a.out, "✓ Configuration received")
			fmt.Fprintln(a.out, "Preparing installation...")
		}
		if installer != nil && setupInstallationPending(state) && state.InstallRunID != "" {
			observed, result, stop := a.runInstallerStep(ctx, step, state, requested)
			if stop {
				return result
			}
			state = observed.state
			lastPhase = state.Phase
			requested = observed.requested
		}
		if state.Phase == setupstate.PhaseInstalling {
			if current := strings.TrimSpace(state.Step); current != "" && current != lastStep {
				lastStep = current
				fmt.Fprintf(a.out, "  → %s\n", current)
			}
		}
		if !requested && !expiredNotice && !state.SetupExpiresAt.IsZero() &&
			time.Now().UTC().After(state.SetupExpiresAt) {
			expiredNotice = true
			fmt.Fprintln(
				a.errOut,
				"The setup session expired. Run `stealth setup` from another terminal to print a new code.",
			)
		}
		switch state.Phase {
		case setupstate.PhaseComplete:
			fmt.Fprintln(a.out)
			fmt.Fprintln(a.out, "✓ Installation complete")
			if url := strings.TrimSpace(state.Draft.PublicURL); url != "" {
				fmt.Fprintf(a.out, "Stealth is ready at %s\n", url)
			} else {
				fmt.Fprintln(a.out, "Stealth is ready.")
			}
			return setupObservationResult{exitCode: 0, installRequested: true, lastObservedPhase: state.Phase}
		case setupstate.PhaseFailed:
			fmt.Fprintln(a.errOut)
			fmt.Fprintln(a.errOut, "Installation failed.")
			if message := strings.TrimSpace(state.ErrorMessage); message != "" {
				fmt.Fprintln(a.errOut, message)
			}
			fmt.Fprintln(a.errOut, "Run `stealth install --repair` to resume, or `stealth doctor` for diagnostics.")
			return setupObservationResult{
				exitCode:          1,
				installRequested:  requested || setupstate.InstallationRequested(state),
				lastObservedPhase: state.Phase,
			}
		}
		if result, stop := a.awaitSetupTick(ctx, source, interval, requested, lastPhase); stop {
			return result
		}
	}
}

// awaitSetupTick waits for the next poll and reports whether the caller must
// return, which happens when the operator cancels the wait.
func (a *App) awaitSetupTick(
	ctx context.Context,
	source setupProgressSource,
	interval time.Duration,
	requested bool,
	lastPhase string,
) (setupObservationResult, bool) {
	select {
	case <-ctx.Done():
		fmt.Fprintln(a.errOut)
		fmt.Fprintln(a.errOut, "Stopped waiting for browser setup.")
		latest, latestErr := reloadSetupObservationState(context.Background(), source)
		if latestErr == nil {
			lastPhase = latest.Phase
			requested = requested || setupstate.InstallationRequested(latest)
		}
		return setupObservationResult{
			exitCode:          0,
			cancelled:         true,
			installRequested:  requested,
			lastObservedPhase: lastPhase,
		}, true
	case <-time.After(interval):
		return setupObservationResult{}, false
	}
}

// setupInstallerStep bundles the two collaborators one installation attempt
// needs, so the step itself takes only the state and the requested flag.
type setupInstallerStep struct {
	source    setupProgressSource
	installer func(context.Context, setupstate.State) error
}

// setupStepOutcome is the observed state after one installation attempt plus
// whether the operator had requested installation by the time it finished.
type setupStepOutcome struct {
	state     setupstate.State
	requested bool
}

// runInstallerStep performs one installation attempt. It reports whether the
// wait loop must return immediately and, when it must not, the refreshed state
// the renderer should use.
func (a *App) runInstallerStep(
	ctx context.Context,
	step setupInstallerStep,
	state setupstate.State,
	requested bool,
) (setupStepOutcome, setupObservationResult, bool) {
	installerErr := step.installer(ctx, state)
	latest, latestErr := reloadSetupObservationState(ctx, step.source)
	if latestErr == nil {
		state = latest
		requested = requested || setupstate.InstallationRequested(state)
	}
	observed := setupStepOutcome{state: state, requested: requested}
	// The installer already persisted the authoritative terminal state, so the
	// normal renderer below reports the failure exactly once.
	installerPersistedTerminal := installerErr != nil && latestErr == nil && setupstate.InstallationTerminal(state)
	if installerPersistedTerminal {
		return observed, setupObservationResult{}, false
	}
	if installerErr != nil {
		if ctx.Err() != nil {
			return observed, a.cancelledSetupResult(requested, state.Phase), true
		}
		return observed, a.hostInstallerFailureResult(state, requested, installerErr, latestErr), true
	}
	if latestErr != nil {
		if ctx.Err() != nil {
			return observed, a.cancelledSetupResult(requested, state.Phase), true
		}
		return observed, a.hostInstallerFailureResult(state, requested, errHostInstallerIncomplete, latestErr), true
	}
	if !setupstate.InstallationTerminal(state) {
		if ctx.Err() != nil {
			return observed, a.cancelledSetupResult(requested, state.Phase), true
		}
		return observed, a.hostInstallerFailureResult(state, requested, errHostInstallerIncomplete, nil), true
	}
	return observed, setupObservationResult{}, false
}

func (a *App) cancelledSetupResult(requested bool, phase string) setupObservationResult {
	return setupObservationResult{
		exitCode:          0,
		cancelled:         true,
		installRequested:  requested,
		lastObservedPhase: phase,
	}
}

func reloadSetupObservationState(ctx context.Context, source setupProgressSource) (setupstate.State, error) {
	loadContext := ctx
	if ctx.Err() != nil {
		// The installer may have returned context.Canceled after Ctrl+C. A
		// background reload still lets us preserve the latest durable phase
		// for the normal cancellation or terminal-state path.
		loadContext = context.Background()
	}
	return source.Load(loadContext)
}

func (a *App) hostInstallerFailureResult(
	state setupstate.State,
	requested bool,
	installerErr, reloadErr error,
) setupObservationResult {
	if errors.Is(installerErr, installengine.ErrOperationInProgress) {
		fmt.Fprintln(a.errOut, "Another host installer already owns this installation run.")
		fmt.Fprintln(a.errOut, "No second installation was started; the setup state was preserved.")
		fmt.Fprintln(
			a.errOut,
			"Wait for the owning process to finish. If it stops unexpectedly, run `stealth install --repair --wait`.",
		)
	} else {
		fmt.Fprintln(a.errOut, "Host installation failed before a terminal failure state was persisted.")
		if reloadErr != nil {
			fmt.Fprintln(a.errOut, "The latest setup state could not be verified.")
		} else if installerErr != nil {
			fmt.Fprintf(a.errOut, "Reason: %s\n", safeHostInstallError(installerErr))
		}
		fmt.Fprintln(a.errOut, "The setup state was preserved.")
		fmt.Fprintln(a.errOut, "Run `stealth install --repair` to resume or recover it.")
	}
	if installerErr == nil {
		installerErr = reloadErr
	}
	return setupObservationResult{
		exitCode:          1,
		installRequested:  requested || setupstate.InstallationRequested(state),
		lastObservedPhase: state.Phase,
		err:               installerErr,
	}
}

func setupWaitInterval(a *App) time.Duration {
	if a != nil && a.pollInterval > 0 {
		return a.pollInterval
	}
	return setupPollInterval
}

// cleanupUnrequestedSetup removes only the temporary setup services and the
// exact Quick Tunnel recorded by the setup state. It re-reads state after
// cancellation so a request that raced with Ctrl+C is never rolled back.
func (a *App) cleanupUnrequestedSetup(
	ctx context.Context,
	layout InstallLayout,
	source setupProgressSource,
	fallbackTunnel string,
) error {
	if a == nil || a.runner == nil {
		return errors.New("CLI command runner is not configured")
	}
	state, err := source.Load(ctx)
	if err != nil {
		return fmt.Errorf("reload setup state before cleanup: %w", err)
	}
	if state.InstallRunID != "" || state.Phase != setupstate.PhaseCollecting {
		return nil
	}

	var cleanupErr error
	tunnelName := state.QuickTunnel
	if !isQuickTunnelContainerName(tunnelName) {
		tunnelName = fallbackTunnel
	}
	if isQuickTunnelContainerName(tunnelName) {
		cleanupErr = errors.Join(cleanupErr, a.closeQuickTunnel(ctx, layout, tunnelName))
	}
	if layout.SetupComposeFile != "" {
		cleanupErr = errors.Join(
			cleanupErr,
			a.runner.Run(
				ctx,
				layout.Root,
				io.Discard,
				io.Discard,
				"docker",
				"compose",
				"--env-file",
				layout.EnvFile,
				"-f",
				layout.SetupComposeFile,
				"rm",
				"-sf",
				"setup",
				"setup-console",
				"setup-proxy",
			),
		)
	}
	return cleanupErr
}
