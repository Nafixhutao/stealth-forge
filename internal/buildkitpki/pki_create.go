package buildkitpki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type role struct {
	usage   x509.ExtKeyUsage
	dnsName string
	name    string
}

var (
	serverRole = role{usage: x509.ExtKeyUsageServerAuth, dnsName: serverName, name: "stealth-buildkit-server"}
	clientRole = role{usage: x509.ExtKeyUsageClientAuth, name: "stealth-buildkit-worker"}
	healthRole = role{usage: x509.ExtKeyUsageClientAuth, name: "stealth-buildkit-health"}
)

func createCA(now time.Time) (*ecdsa.PrivateKey, *x509.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate BuildKit CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Stealth BuildKit CA", Organization: []string{"Stealth"}},
		NotBefore:    now.Add(-5 * time.Minute), NotAfter: now.Add(caLifetime),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		SubjectKeyId: make([]byte, 20),
	}
	if _, err := rand.Read(template.SubjectKeyId); err != nil {
		return nil, nil, fmt.Errorf("generate BuildKit CA subject key ID: %w", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, nil, fmt.Errorf("issue BuildKit CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse generated BuildKit CA certificate: %w", err)
	}
	return key, cert, nil
}

func createLeaf(caKey *ecdsa.PrivateKey, ca *x509.Certificate, identity role, now time.Time) (*ecdsa.PrivateKey, *x509.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate BuildKit %s key: %w", identity.name, err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: identity.name, Organization: []string{"Stealth"}},
		NotBefore:    now.Add(-5 * time.Minute), NotAfter: now.Add(leafLifetime),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{identity.usage},
	}
	if identity.dnsName != "" {
		template.DNSNames = []string{identity.dnsName}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, key.Public(), caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("issue BuildKit %s certificate: %w", identity.name, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse generated BuildKit %s certificate: %w", identity.name, err)
	}
	return key, cert, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 159)
	for {
		serial, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return nil, fmt.Errorf("generate BuildKit certificate serial: %w", err)
		}
		if serial.Sign() > 0 {
			return serial, nil
		}
	}
}

func writeKey(path string, key *ecdsa.PrivateKey, mode os.FileMode) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal BuildKit private key: %w", err)
	}
	return writePEM(path, "PRIVATE KEY", der, mode)
}

func writePEM(path, kind string, contents []byte, mode os.FileMode) error {
	return atomicWrite(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: contents}), mode)
}

func atomicWrite(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return fmt.Errorf("create BuildKit mTLS file directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stealth-buildkit-*")
	if err != nil {
		return fmt.Errorf("stage BuildKit mTLS file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect BuildKit mTLS file: %w", err)
	}
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write BuildKit mTLS file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync BuildKit mTLS file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close BuildKit mTLS file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish BuildKit mTLS file: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
}

func readRegular(path string, expectedMode os.FileMode) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular file")
	}
	if info.Mode().Perm() != expectedMode {
		return nil, fmt.Errorf("file mode is %04o, expected %04o", info.Mode().Perm(), expectedMode)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return contents, nil
}

func ownerUID(path string) (uint32, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("owner cannot be read from an unsafe path")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("filesystem does not report a numeric owner")
	}
	return stat.Uid, nil
}

func requireOwnerUID(path string, expected uint32) error {
	actual, err := ownerUID(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("owner does not match the private BuildKit PKI directory")
	}
	return nil
}

func ensureRealDirectory(path string, mode os.FileMode, mustExist bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && !mustExist {
		return os.MkdirAll(path, mode)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("not a real directory")
	}
	if mustExist && info.Mode().Perm() != mode {
		return fmt.Errorf("directory mode is %04o, expected %04o", info.Mode().Perm(), mode)
	}
	return nil
}

func parsePrivateKey(contents []byte) (*ecdsa.PrivateKey, error) {
	block, rest := pem.Decode(contents)
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("invalid PKCS#8 private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("private key must use ECDSA P-256")
	}
	return key, nil
}

func publicKeysMatch(certPublic any, key *ecdsa.PrivateKey) bool {
	certKey, ok := certPublic.(*ecdsa.PublicKey)
	return ok && certKey.Curve == key.Curve && certKey.X.Cmp(key.X) == 0 && certKey.Y.Cmp(key.Y) == 0
}

func invalidFile(name string, err error) error {
	return fmt.Errorf("%w: %s is missing, malformed, or inconsistent (%v)", ErrInvalidState, name, err)
}

func syncTreeDirectories(paths Paths) error {
	for _, dir := range []string{filepath.Dir(paths.ServerCert), filepath.Dir(paths.WorkerCert), filepath.Dir(paths.HealthCert), paths.Root} {
		if err := syncDirectory(dir); err != nil {
			return fmt.Errorf("sync BuildKit mTLS directory: %w", err)
		}
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
