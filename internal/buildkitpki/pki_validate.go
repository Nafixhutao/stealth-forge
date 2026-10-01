package buildkitpki

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func replaceBundle(current, pending, previous, parent string) error {
	if err := os.Rename(current, previous); err != nil {
		return fmt.Errorf("stage previous BuildKit mTLS identity: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return fmt.Errorf("sync staged BuildKit mTLS identity: %w", err)
	}
	if err := os.Rename(pending, current); err != nil {
		_ = os.Rename(previous, current)
		_ = syncDirectory(parent)
		return fmt.Errorf("activate renewed BuildKit mTLS identity: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return fmt.Errorf("sync renewed BuildKit mTLS identity: %w", err)
	}
	if err := os.RemoveAll(previous); err != nil {
		return fmt.Errorf("remove previous BuildKit mTLS identity: %w", err)
	}
	return syncDirectory(parent)
}

func pathsAtRoot(root string) Paths {
	return Paths{Root: root, CACert: filepath.Join(root, "ca-cert.pem"), CAKey: filepath.Join(root, "ca-key.pem"), ServerCert: filepath.Join(root, "server", "cert.pem"), ServerKey: filepath.Join(root, "server", "key.pem"), WorkerCert: filepath.Join(root, "worker", "cert.pem"), WorkerKey: filepath.Join(root, "worker", "key.pem"), HealthCert: filepath.Join(root, "health", "cert.pem"), HealthKey: filepath.Join(root, "health", "key.pem")}
}

func validateBundle(paths Paths, allowRenewal bool) (bool, bool, error) {
	if err := ensureRealDirectory(paths.Root, directoryMode, true); err != nil {
		return false, false, fmt.Errorf("%w: BuildKit mTLS directory: %v", ErrInvalidState, err)
	}
	bundleUID, err := ownerUID(paths.Root)
	if err != nil {
		return false, false, fmt.Errorf("%w: BuildKit mTLS directory owner is invalid", ErrInvalidState)
	}
	for _, dir := range []string{filepath.Dir(paths.ServerCert), filepath.Dir(paths.WorkerCert), filepath.Dir(paths.HealthCert)} {
		if err := ensureRealDirectory(dir, directoryMode, true); err != nil {
			return false, false, fmt.Errorf("%w: BuildKit mTLS identity directory is missing or unsafe", ErrInvalidState)
		}
		if err := requireOwnerUID(dir, bundleUID); err != nil {
			return false, false, fmt.Errorf("%w: BuildKit mTLS identity directory owner is inconsistent", ErrInvalidState)
		}
	}
	caPEM, err := readRegular(paths.CACert, fileMode)
	if err != nil {
		return false, false, invalidFile("BuildKit CA certificate", err)
	}
	if err := requireOwnerUID(paths.CACert, bundleUID); err != nil {
		return false, false, invalidFile("BuildKit CA certificate", err)
	}
	caKeyPEM, err := readRegular(paths.CAKey, caKeyMode)
	if err != nil {
		return false, false, invalidFile("BuildKit CA private key", err)
	}
	if err := requireOwnerUID(paths.CAKey, bundleUID); err != nil {
		return false, false, invalidFile("BuildKit CA private key", err)
	}
	caBlock, rest := pem.Decode(caPEM)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" || strings.TrimSpace(string(rest)) != "" {
		return false, false, invalidFile("BuildKit CA certificate", errors.New("invalid PEM"))
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil || !caCert.IsCA || !caCert.BasicConstraintsValid || caCert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return false, false, invalidFile("BuildKit CA certificate", errors.New("CA constraints or key usage are invalid"))
	}
	caKey, err := parsePrivateKey(caKeyPEM)
	if err != nil || !publicKeysMatch(caCert.PublicKey, caKey) {
		return false, false, invalidFile("BuildKit CA private key", errors.New("certificate and private key do not match"))
	}
	now := time.Now().UTC()
	if now.Before(caCert.NotBefore) {
		return false, false, invalidFile("BuildKit CA certificate", errors.New("certificate is not yet valid"))
	}
	rotateCA := !now.Before(caCert.NotAfter) || time.Until(caCert.NotAfter) <= caRenewBefore
	rotateLeaves := false
	for _, leaf := range []struct {
		name     string
		certPath string
		keyPath  string
		role     role
	}{
		{"server", paths.ServerCert, paths.ServerKey, serverRole},
		{"worker client", paths.WorkerCert, paths.WorkerKey, clientRole},
		{"health client", paths.HealthCert, paths.HealthKey, healthRole},
	} {
		certPEM, err := readRegular(leaf.certPath, fileMode)
		if err != nil {
			return false, false, invalidFile("BuildKit "+leaf.name+" certificate", err)
		}
		if err := requireOwnerUID(leaf.certPath, bundleUID); err != nil {
			return false, false, invalidFile("BuildKit "+leaf.name+" certificate", err)
		}
		keyPEM, err := readRegular(leaf.keyPath, keyMode)
		if err != nil {
			return false, false, invalidFile("BuildKit "+leaf.name+" private key", err)
		}
		if err := requireOwnerUID(leaf.keyPath, bundleUID); err != nil {
			return false, false, invalidFile("BuildKit "+leaf.name+" private key", err)
		}
		block, trailing := pem.Decode(certPEM)
		if block == nil || block.Type != "CERTIFICATE" || strings.TrimSpace(string(trailing)) != "" {
			return false, false, invalidFile("BuildKit "+leaf.name+" certificate", errors.New("invalid PEM"))
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return false, false, invalidFile("BuildKit "+leaf.name+" certificate", err)
		}
		key, err := parsePrivateKey(keyPEM)
		if err != nil || !publicKeysMatch(cert.PublicKey, key) {
			return false, false, invalidFile("BuildKit "+leaf.name+" private key", errors.New("certificate and private key do not match"))
		}
		if err := validateLeaf(cert, caCert, leaf.role, now, allowRenewal); err != nil {
			return false, false, invalidFile("BuildKit "+leaf.name+" certificate", err)
		}
		if !now.Before(cert.NotAfter) || time.Until(cert.NotAfter) <= RenewBefore {
			rotateLeaves = true
		}
	}
	if rotateCA {
		return true, true, nil
	}
	return false, rotateLeaves, nil
}

func validateBundleOnly(paths Paths, allowRenewal bool) error {
	_, _, err := validateBundle(paths, allowRenewal)
	return err
}

func validateLeaf(cert, ca *x509.Certificate, expected role, now time.Time, allowRenewal bool) error {
	if cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return errors.New("leaf constraints or key usage are invalid")
	}
	if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != expected.usage {
		return errors.New("extended key usage does not match the identity role")
	}
	if expected.dnsName != "" {
		if err := cert.VerifyHostname(expected.dnsName); err != nil {
			return errors.New("server certificate DNS SAN is invalid")
		}
		if len(cert.DNSNames) != 1 || cert.DNSNames[0] != expected.dnsName {
			return errors.New("server certificate contains an unexpected DNS SAN")
		}
	} else if len(cert.DNSNames) != 0 || len(cert.IPAddresses) != 0 {
		return errors.New("client identity must not carry server names")
	}
	if !now.Before(cert.NotBefore) {
		// Continue below. A leaf not yet valid may be an operator clock issue;
		// a just-expired leaf is the documented renewable condition.
	} else {
		return errors.New("certificate is not yet valid")
	}
	if now.Before(cert.NotAfter) {
		// Valid leaf.
	} else if !allowRenewal {
		return errors.New("certificate has expired")
	}
	if err := cert.CheckSignatureFrom(ca); err != nil {
		return errors.New("certificate is not signed by the Stealth BuildKit CA")
	}
	return nil
}
