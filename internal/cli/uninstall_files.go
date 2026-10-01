package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func removeLocalRuntimeFiles(plan uninstallPlan) error {
	for _, asset := range plan.localAssets(false) {
		if !asset.present {
			continue
		}
		if err := removeSafePath(asset.path); err != nil {
			return fmt.Errorf("remove %s: %w", asset.label, err)
		}
	}
	return nil
}

func removeAllLocalFiles(plan uninstallPlan) error {
	if err := removeLocalRuntimeFiles(plan); err != nil {
		return err
	}
	if plan.configPresent {
		if err := removeSafePath(plan.layout.EnvFile); err != nil {
			return fmt.Errorf("remove config.env and local secrets: %w", err)
		}
	}
	if plan.privatePresent {
		if err := removeSafePath(plan.layout.PrivateDir); err != nil {
			return fmt.Errorf("remove private BuildKit control-plane credentials: %w", err)
		}
	}
	for _, path := range []string{filepath.Dir(plan.layout.ProxyFile), filepath.Dir(filepath.Dir(plan.layout.ProxyFile)), plan.layout.TraefikGenerated, plan.layout.TraefikDynamic, plan.layout.TraefikDir} {
		if err := removeEmptyDirectory(path); err != nil {
			return err
		}
	}
	return removeEmptyDirectory(plan.layout.Root)
}

func removeSafePath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return fmt.Errorf("refusing to remove unsafe path %s", path)
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func removeEmptyDirectory(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTEMPTY) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove empty directory %s: %w", path, err)
	}
	return nil
}

func verifyLocalUninstall(plan uninstallPlan) error {
	switch plan.mode {
	case uninstallServices:
		return nil
	case uninstallConfiguration:
		for _, asset := range plan.localAssets(false) {
			if pathPresent(asset.path) {
				return fmt.Errorf("local runtime file %s remains", asset.label)
			}
		}
		if plan.configPresent && !safeRegularFile(plan.layout.EnvFile) {
			return fmt.Errorf("config.env was not preserved")
		}
		if plan.privatePresent && !safeDirectory(plan.layout.PrivateDir) {
			return fmt.Errorf("private BuildKit control-plane credentials were not preserved")
		}
		return nil
	case uninstallPurge:
		for _, asset := range plan.localAssets(true) {
			if pathPresent(asset.path) {
				return fmt.Errorf("local installation state %s remains", asset.label)
			}
		}
		if pathPresent(plan.layout.Root) {
			return fmt.Errorf("installation directory %s remains", plan.layout.Root)
		}
		return nil
	default:
		return fmt.Errorf("unknown uninstall mode")
	}
}
