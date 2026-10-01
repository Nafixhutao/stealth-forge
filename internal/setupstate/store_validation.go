package setupstate

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domainname"
)

func ValidateState(state State) error {
	if state.Version != 0 && state.Version != stateVersion {
		return fmt.Errorf("unsupported setup state version")
	}
	if state.Phase == "" {
		return errors.New("setup state phase is required")
	}
	switch state.Phase {
	case PhaseCollecting, PhaseInstallRequested, PhaseInstalling, PhaseComplete, PhaseFailed, PhaseHandoff:
	default:
		return fmt.Errorf("unsupported setup state phase")
	}
	if len(state.ErrorMessage) > 240 || strings.ContainsAny(state.ErrorMessage, "\x00\r\n") {
		return errors.New("setup state error is invalid")
	}
	if len(state.Step) > 120 || strings.ContainsAny(state.Step, "\x00\r\n") {
		return errors.New("setup state step is invalid")
	}
	if err := state.Cloudflare.Binding.Validate(); err != nil {
		return err
	}
	if len(state.SetupSessionID) > 64 || strings.ContainsAny(state.SetupSessionID, "\x00\r\n") || len(state.SetupCodeHash) > 128 || strings.ContainsAny(state.SetupCodeHash, "\x00\r\n") {
		return errors.New("setup bootstrap claim is invalid")
	}
	if len(state.InstallRunID) > 128 || strings.ContainsAny(state.InstallRunID, "\x00\r\n") {
		return errors.New("setup installation run identifier is invalid")
	}
	if len(state.HostPreflight) > 32 {
		return errors.New("setup state contains too many host preflight checks")
	}
	for _, check := range state.HostPreflight {
		if strings.TrimSpace(check.Name) == "" || len(check.Name) > 80 || strings.ContainsAny(check.Name, "\x00\r\n") || len(check.Detail) > 240 || strings.ContainsAny(check.Detail, "\x00\r\n") {
			return errors.New("host preflight check is invalid")
		}
	}
	switch state.Phase {
	case PhaseInstallRequested, PhaseInstalling, PhaseHandoff:
		if strings.TrimSpace(state.InstallRunID) == "" {
			return errors.New("setup installation phase has no run identifier")
		}
	}
	if len(state.Secrets) > 64 {
		return errors.New("setup state contains too many secrets")
	}
	for name, value := range state.Secrets {
		// Secret values are encrypted before persistence and are never copied
		// into env files or public responses. GitHub PEM private keys are
		// intentionally multi-line, so only NUL is forbidden here.
		if name == "" || len(name) > 120 || strings.ContainsAny(name, "\x00\r\n") || len(value) > 128<<10 || strings.ContainsRune(value, '\x00') {
			return errors.New("setup state secret is invalid")
		}
	}
	return nil
}

func ValidatePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errors.New("public URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("public URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ValidateHostname(raw string) (string, error) {
	host, err := domainname.NormalizeDomain(raw)
	if err != nil {
		return "", errors.New("hostname is invalid")
	}
	return host, nil
}

func ZoneNameForHostname(host string) string {
	zone, err := domainname.RegistrableDomain(host)
	if err != nil {
		return ""
	}
	return zone
}

func sha256Bytes(value []byte) []byte {
	// Kept in a helper so state hashing has one implementation and no caller
	// accidentally compares the raw callback state.
	digest := sha256.Sum256(value)
	return digest[:]
}
