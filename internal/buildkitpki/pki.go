// Package buildkitpki manages the installation-local identities used to
// authenticate Stealth's worker, BuildKit daemon, and daemon health probe.
package buildkitpki

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DirectoryName = "buildkit-mtls"
	RenewBefore   = 30 * 24 * time.Hour
	caLifetime    = 10 * 365 * 24 * time.Hour
	leafLifetime  = 365 * 24 * time.Hour
	caRenewBefore = 365 * 24 * time.Hour
	serverName    = "buildkit"
	fileMode      = 0o644
	keyMode       = 0o600
	caKeyMode     = 0o600
	directoryMode = 0o700
)

var (
	ErrInvalidState = errors.New("BuildKit mTLS state is invalid")
)

type Paths struct {
	Root       string
	CACert     string
	CAKey      string
	ServerCert string
	ServerKey  string
	WorkerCert string
	WorkerKey  string
	HealthCert string
	HealthKey  string
}

func PathsAt(pkiDir string) Paths {
	return pathsAtRoot(filepath.Clean(pkiDir))
}

// Ensure creates an initial PKI, preserves a complete valid PKI, renews leaf
// identities within RenewBefore, and fails closed for inconsistent state.
// Host private keys remain mode 0600 under installation-private directories.
// One-shot Compose initializers copy only their assigned identity into
// service-specific volumes with service-owned read-only keys.
func Ensure(pkiDir string) (bool, error) {
	if pkiDir == "" || !filepath.IsAbs(pkiDir) || filepath.Clean(pkiDir) == string(filepath.Separator) {
		return false, fmt.Errorf("%w: invalid BuildKit PKI directory", ErrInvalidState)
	}
	pkiDir = filepath.Clean(pkiDir)
	parent := filepath.Dir(pkiDir)
	if err := ensureRealDirectory(parent, directoryMode, false); err != nil {
		return false, fmt.Errorf("inspect BuildKit PKI parent directory: %w", err)
	}
	if err := os.Chmod(parent, directoryMode); err != nil {
		return false, fmt.Errorf("protect BuildKit PKI parent directory: %w", err)
	}
	paths := PathsAt(pkiDir)
	pending := paths.Root + ".pending"
	previous := paths.Root + ".previous"

	if err := recoverBundle(paths, pending, previous); err != nil {
		return false, err
	}
	info, err := os.Lstat(paths.Root)
	if errors.Is(err, os.ErrNotExist) {
		if _, pendingErr := os.Lstat(pending); pendingErr == nil {
			if err := validateBundleOnly(pathsAtRoot(pending), false); err != nil {
				return false, fmt.Errorf("incomplete interrupted BuildKit mTLS issuance; repair the full PKI: %w", err)
			}
			if err := os.Rename(pending, paths.Root); err != nil {
				return false, fmt.Errorf("recover complete BuildKit mTLS issuance: %w", err)
			}
			if err := syncDirectory(parent); err != nil {
				return false, fmt.Errorf("sync recovered BuildKit PKI: %w", err)
			}
			return true, nil
		} else if !errors.Is(pendingErr, os.ErrNotExist) {
			return false, fmt.Errorf("inspect interrupted BuildKit PKI issuance: %w", pendingErr)
		}
		if err := writeBundleAtomic(paths, pending); err != nil {
			_ = os.RemoveAll(pending)
			return false, err
		}
		if err := os.Rename(pending, paths.Root); err != nil {
			return false, fmt.Errorf("publish BuildKit mTLS identity: %w", err)
		}
		if err := syncDirectory(parent); err != nil {
			return false, fmt.Errorf("sync BuildKit PKI: %w", err)
		}
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect BuildKit mTLS identity: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != directoryMode {
		return false, fmt.Errorf("%w: BuildKit mTLS directory must be a real mode-0700 directory", ErrInvalidState)
	}
	rotateCA, rotateLeaves, err := validateBundle(paths, true)
	if err != nil {
		return false, err
	}
	if !rotateCA && !rotateLeaves {
		return false, nil
	}
	if rotateCA {
		if err := writeBundleAtomic(paths, pending); err != nil {
			_ = os.RemoveAll(pending)
			return false, err
		}
	} else if err := writeLeafRenewalBundle(paths, pending); err != nil {
		_ = os.RemoveAll(pending)
		return false, err
	}
	if err := replaceBundle(paths.Root, pending, previous, parent); err != nil {
		return false, err
	}
	return true, nil
}

// ValidateExisting checks a complete installed bundle without creating or
// rotating any identity. The installer uses it before atomically relocating a
// development bundle from the legacy state directory.
