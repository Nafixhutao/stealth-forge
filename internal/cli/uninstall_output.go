package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func (a *App) runUninstallPlain(ctx context.Context, plan uninstallPlan, options uninstallOptions) int {
	printUninstallPlan(a.out, plan)
	if options.dryRun {
		fmt.Fprintln(a.out, "\nDry run complete. No changes were made.")
		return 0
	}
	if !options.yes {
		fmt.Fprintln(a.errOut, "Refusing to uninstall without --yes because this output is not a TTY.")
		return 2
	}
	operations := a.uninstallOperations(plan)
	fmt.Fprintln(a.out, "\nRemoving Stealth")
	for index, operation := range operations {
		fmt.Fprintf(a.out, "[%d/%d] %s... ", index+1, len(operations), operation.name)
		err := operation.action(ctx)
		if err != nil {
			fmt.Fprintln(a.out, "failed")
			a.printUninstallFailure(plan, err)
			return 1
		}
		fmt.Fprintln(a.out, "done")
	}
	a.printUninstallSuccess(plan)
	return 0
}

func (a *App) printUninstallFailure(plan uninstallPlan, err error) {
	fmt.Fprintf(a.errOut, "\nUninstall incomplete: %v\n", err)
	if plan.mode == uninstallPurge {
		fmt.Fprintln(a.errOut, "No further destructive cleanup was attempted. Inspect the instance before retrying.")
	} else {
		fmt.Fprintln(a.errOut, "Persistent data was not targeted by this mode.")
	}
	fmt.Fprintln(a.errOut, "Try `stealth doctor` for diagnostics.")
	if plan.composePresent {
		fmt.Fprintln(a.errOut, "Use `stealth logs <service>` if Docker services need inspection.")
	}
}

func (a *App) printUninstallSuccess(plan uninstallPlan) {
	switch plan.mode {
	case uninstallServices:
		fmt.Fprintln(a.out, "\nStealth services were removed. Persistent data, config.env, and recovery files were preserved.")
	case uninstallConfiguration:
		fmt.Fprintln(a.out, "\nStealth services and local runtime files were removed.")
		fmt.Fprintln(a.out, "Persistent data was preserved. config.env remains because it contains recovery secrets and the encryption key.")
	case uninstallPurge:
		fmt.Fprintln(a.out, "\nStealth instance data, services, configuration, managed App containers, the App runtime network, and project-owned Docker volumes were permanently removed.")
		if plan.externalStorage {
			fmt.Fprintln(a.out, "External S3 object storage was preserved; remove it separately after verifying ownership.")
		}
	}
	if binary := detectedCLIBinaryPath(); binary != "" {
		fmt.Fprintf(a.out, "The Stealth CLI is still installed at %s.\n", binary)
		fmt.Fprintf(a.out, "To remove it manually: rm %s\n", binary)
	}
}

func detectedCLIBinaryPath() string {
	path, err := os.Executable()
	if err != nil || filepath.Base(path) != "stealth" {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	}
	return path
}
