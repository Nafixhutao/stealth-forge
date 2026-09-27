package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Stealth-deplover/stealth/internal/buildinfo"
)

type platformRollbackState struct {
	FormatVersion int    `json:"format_version"`
	Platform      string `json:"platform_version"`
	CLI           string `json:"cli_version"`
}

func (a *App) runRollback(args []string) int {
	fs := flag.NewFlagSet("stealth rollback", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	verbose := fs.Bool("verbose", false, "show Docker command output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(a.errOut, "rollback does not accept positional arguments")
		return 2
	}
	a.verbose = *verbose
	layout, err := a.layout()
	if err != nil {
		fmt.Fprintf(a.errOut, "cannot determine installation directory: %v\n", err)
		return 1
	}
	if !installationExists(layout) || !regularFile(layout.EnvFile) {
		fmt.Fprintln(a.errOut, "platform rollback requires an existing production installation")
		return 1
	}
	plan, err := a.loadExistingPlan(layout)
	if err != nil {
		fmt.Fprintf(a.errOut, "cannot load the installed platform for rollback: %v\n", err)
		return 1
	}
	if plan.Setup {
		fmt.Fprintln(a.errOut, "platform rollback is unavailable while browser setup is incomplete")
		return 1
	}
	cliVersion := buildinfo.Version
	if a.currentVersion != nil {
		cliVersion = a.currentVersion()
	}
	plan.CLIVersion = strings.TrimSpace(cliVersion)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := a.installEngine().Rollback(ctx, *plan); err != nil {
		fmt.Fprintf(a.errOut, "platform rollback failed: %v\n", err)
		return 1
	}
	previous, _, err := readInstalledVersion(layout)
	if err != nil {
		fmt.Fprintf(a.errOut, "platform rollback completed, but VERSION could not be read: %v\n", err)
		return 1
	}
	fmt.Fprintf(a.out, "Platform rollback completed: Stealth %s is active and passed health checks.\n", previous)
	return 0
}

func platformRollbackAllowsCLISkew(layout InstallLayout, cliVersion, platformVersion string) bool {
	path := filepath.Join(layout.StateDir, "platform-rollback.state")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var state platformRollbackState
	if json.Unmarshal(contents, &state) != nil {
		return false
	}
	return state.FormatVersion == 1 && state.Platform == platformVersion && state.CLI == cliVersion &&
		validateReleaseVersion(state.Platform) == nil && validateReleaseVersion(state.CLI) == nil
}
