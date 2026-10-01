package buildkitpki

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func ValidateExisting(pkiDir string) error {
	if pkiDir == "" || !filepath.IsAbs(pkiDir) || filepath.Clean(pkiDir) == string(filepath.Separator) {
		return fmt.Errorf("%w: invalid BuildKit PKI directory", ErrInvalidState)
	}
	return validateBundleOnly(PathsAt(pkiDir), false)
}

// ValidateForRelocation validates a complete bundle while permitting expired
// leaves that Ensure can renew after the bundle has moved to its new location.
func ValidateForRelocation(pkiDir string) error {
	if pkiDir == "" || !filepath.IsAbs(pkiDir) || filepath.Clean(pkiDir) == string(filepath.Separator) {
		return fmt.Errorf("%w: invalid BuildKit PKI directory", ErrInvalidState)
	}
	_, _, err := validateBundle(PathsAt(pkiDir), true)
	return err
}

func recoverBundle(paths Paths, pending, previous string) error {
	_, rootErr := os.Lstat(paths.Root)
	previousInfo, previousErr := os.Lstat(previous)
	if previousErr == nil && (previousInfo.Mode()&os.ModeSymlink != 0 || !previousInfo.IsDir()) {
		return fmt.Errorf("%w: interrupted BuildKit mTLS backup is not a real directory", ErrInvalidState)
	}
	if rootErr == nil {
		if err := validateBundleOnly(paths, true); err != nil {
			return err
		}
		if previousErr == nil {
			if err := os.RemoveAll(previous); err != nil {
				return fmt.Errorf("remove recovered BuildKit mTLS backup: %w", err)
			}
		}
		if _, err := os.Lstat(pending); err == nil {
			pendingPaths := pathsAtRoot(pending)
			if pendingInfo, statErr := os.Lstat(pending); statErr != nil || pendingInfo.Mode()&os.ModeSymlink != 0 || !pendingInfo.IsDir() {
				return fmt.Errorf("%w: interrupted BuildKit mTLS staging path is unsafe", ErrInvalidState)
			} else if validateErr := validateBundleOnly(pendingPaths, false); validateErr == nil {
				if err := os.RemoveAll(pending); err != nil {
					return fmt.Errorf("remove completed BuildKit mTLS staging bundle: %w", err)
				}
			} else {
				return fmt.Errorf("%w: interrupted BuildKit mTLS staging bundle is incomplete", ErrInvalidState)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect interrupted BuildKit mTLS staging bundle: %w", err)
		}
		return nil
	}
	if !errors.Is(rootErr, os.ErrNotExist) {
		return fmt.Errorf("inspect BuildKit mTLS identity: %w", rootErr)
	}
	if previousErr == nil {
		previousPaths := pathsAtRoot(previous)
		if err := validateBundleOnly(previousPaths, true); err != nil {
			return fmt.Errorf("%w: interrupted BuildKit mTLS rotation has no valid active or previous bundle", err)
		}
		if pendingInfo, pendingErr := os.Lstat(pending); pendingErr == nil {
			if pendingInfo.Mode()&os.ModeSymlink != 0 || !pendingInfo.IsDir() {
				return fmt.Errorf("%w: interrupted BuildKit mTLS staging path is unsafe", ErrInvalidState)
			}
			if err := validateBundleOnly(pathsAtRoot(pending), false); err != nil {
				return fmt.Errorf("%w: interrupted BuildKit mTLS replacement is incomplete", err)
			}
			if err := os.Rename(pending, paths.Root); err != nil {
				return fmt.Errorf("finish BuildKit mTLS rotation: %w", err)
			}
		} else if errors.Is(pendingErr, os.ErrNotExist) {
			if err := os.Rename(previous, paths.Root); err != nil {
				return fmt.Errorf("restore previous BuildKit mTLS identity: %w", err)
			}
		} else {
			return fmt.Errorf("inspect interrupted BuildKit mTLS replacement: %w", pendingErr)
		}
		if err := os.RemoveAll(previous); err != nil {
			return fmt.Errorf("remove previous BuildKit mTLS identity: %w", err)
		}
		return syncDirectory(filepath.Dir(paths.Root))
	}
	if !errors.Is(previousErr, os.ErrNotExist) {
		return fmt.Errorf("inspect previous BuildKit mTLS identity: %w", previousErr)
	}
	return nil
}

func writeBundleAtomic(paths Paths, destination string) error {
	parent := filepath.Dir(paths.Root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create BuildKit mTLS parent: %w", err)
	}
	if info, err := os.Lstat(parent); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: BuildKit mTLS parent must be a real directory", ErrInvalidState)
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("%w: refusing to overwrite an existing BuildKit mTLS staging path", ErrInvalidState)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect BuildKit mTLS staging path: %w", err)
	}
	if err := prepareBundleDirectories(destination); err != nil {
		return err
	}
	paths = pathsAtRoot(destination)
	caKey, caCert, err := createCA(time.Now().UTC())
	if err != nil {
		return err
	}
	if err := writePEM(paths.CACert, "CERTIFICATE", caCert.Raw, fileMode); err != nil {
		return err
	}
	if err := writeKey(paths.CAKey, caKey, caKeyMode); err != nil {
		return err
	}
	for _, leaf := range []struct {
		cert string
		key  string
		role role
	}{
		{paths.ServerCert, paths.ServerKey, serverRole},
		{paths.WorkerCert, paths.WorkerKey, clientRole},
		{paths.HealthCert, paths.HealthKey, healthRole},
	} {
		key, cert, err := createLeaf(caKey, caCert, leaf.role, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := writePEM(leaf.cert, "CERTIFICATE", cert.Raw, fileMode); err != nil {
			return err
		}
		if err := writeKey(leaf.key, key, keyMode); err != nil {
			return err
		}
	}
	if err := syncTreeDirectories(paths); err != nil {
		return err
	}
	if err := validateBundleOnly(paths, false); err != nil {
		return fmt.Errorf("validate generated BuildKit mTLS identity: %w", err)
	}
	return nil
}

func writeLeafRenewalBundle(current Paths, destination string) error {
	if err := prepareBundleDirectories(destination); err != nil {
		return err
	}
	paths := pathsAtRoot(destination)
	caCertPEM, err := readRegular(current.CACert, fileMode)
	if err != nil {
		return invalidFile("BuildKit CA certificate", err)
	}
	caKeyPEM, err := readRegular(current.CAKey, caKeyMode)
	if err != nil {
		return invalidFile("BuildKit CA private key", err)
	}
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil {
		return invalidFile("BuildKit CA certificate", errors.New("invalid PEM"))
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return invalidFile("BuildKit CA certificate", err)
	}
	caKey, err := parsePrivateKey(caKeyPEM)
	if err != nil || !publicKeysMatch(caCert.PublicKey, caKey) {
		return invalidFile("BuildKit CA private key", errors.New("certificate and private key do not match"))
	}
	if err := writePEM(paths.CACert, "CERTIFICATE", caCert.Raw, fileMode); err != nil {
		return err
	}
	if err := writeKey(paths.CAKey, caKey, caKeyMode); err != nil {
		return err
	}
	for _, leaf := range []struct {
		cert string
		key  string
		role role
	}{
		{paths.ServerCert, paths.ServerKey, serverRole},
		{paths.WorkerCert, paths.WorkerKey, clientRole},
		{paths.HealthCert, paths.HealthKey, healthRole},
	} {
		key, cert, err := createLeaf(caKey, caCert, leaf.role, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := writePEM(leaf.cert, "CERTIFICATE", cert.Raw, fileMode); err != nil {
			return err
		}
		if err := writeKey(leaf.key, key, keyMode); err != nil {
			return err
		}
	}
	if err := syncTreeDirectories(paths); err != nil {
		return err
	}
	if err := validateBundleOnly(paths, false); err != nil {
		return fmt.Errorf("validate renewed BuildKit leaf identities: %w", err)
	}
	return nil
}

func prepareBundleDirectories(destination string) error {
	if err := os.Mkdir(destination, directoryMode); err != nil {
		return fmt.Errorf("create BuildKit mTLS staging directory: %w", err)
	}
	if err := os.Chmod(destination, directoryMode); err != nil {
		return fmt.Errorf("protect BuildKit mTLS staging directory: %w", err)
	}
	paths := pathsAtRoot(destination)
	for _, dir := range []string{filepath.Dir(paths.ServerCert), filepath.Dir(paths.WorkerCert), filepath.Dir(paths.HealthCert)} {
		if err := os.Mkdir(dir, directoryMode); err != nil {
			return fmt.Errorf("create BuildKit mTLS identity directory: %w", err)
		}
		if err := os.Chmod(dir, directoryMode); err != nil {
			return fmt.Errorf("protect BuildKit mTLS identity directory: %w", err)
		}
	}
	return nil
}
