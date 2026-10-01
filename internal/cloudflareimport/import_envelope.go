package cloudflareimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Stealth-deplover/stealth/internal/domainname"
	"github.com/Stealth-deplover/stealth/internal/functionsecret"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

func Encrypt(envelope Envelope, cipher *functionsecret.Cipher) ([]byte, error) {
	if err := validateEnvelope(envelope); err != nil || cipher == nil {
		return nil, errInvalidArtifact
	}
	plaintext, err := json.Marshal(envelope)
	if err != nil || len(plaintext) > maxArtifactBytes {
		return nil, errInvalidArtifact
	}
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil || len(ciphertext) > maxArtifactBytes {
		return nil, errArtifactEncrypt
	}
	return ciphertext, nil
}

// Read decrypts the worker-visible file and rejects unknown fields, duplicate
// keys, non-canonical JSON, trailing data, and oversized artifacts.
func Read(path string, cipher *functionsecret.Cipher) (Envelope, error) {
	if !validFilePath(path) || cipher == nil {
		return Envelope{}, errInvalidPath
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return Envelope{}, os.ErrNotExist
		}
		return Envelope{}, errInvalidArtifact
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxArtifactBytes {
		return Envelope{}, errUnsafeImportEntry
	}
	ciphertext, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	if err != nil || len(ciphertext) > maxArtifactBytes {
		return Envelope{}, errInvalidArtifact
	}
	plaintext, err := cipher.Decrypt(ciphertext)
	if err != nil {
		return Envelope{}, errArtifactDecrypt
	}
	var envelope Envelope
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, errInvalidArtifact
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Envelope{}, errInvalidArtifact
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || !bytes.Equal(canonical, plaintext) || validateEnvelope(envelope) != nil {
		return Envelope{}, errInvalidArtifact
	}
	return envelope, nil
}

func envelopeFromSetupState(state setupstate.State) (Envelope, string) {
	binding := state.EffectiveCloudflareBinding().Normalized()
	apiToken := strings.TrimSpace(state.Secret("cloudflare_access_token"))
	tunnelToken := strings.TrimSpace(state.Secret("cloudflare_tunnel_token"))
	if tunnelToken == "" {
		tunnelToken = strings.TrimSpace(state.Secret("tunnel_token"))
	}
	hasIntent := state.Cloudflare.Mode != "" || state.Cloudflare.Connected || !state.Cloudflare.Binding.IsZero() || binding.HasIntent() || apiToken != "" || tunnelToken != ""
	if !hasIntent {
		return Envelope{}, OutcomeNoImport
	}
	if binding.Validate() != nil || binding.AccountID == "" || binding.ZoneID == "" || binding.Hostname == "" || binding.TunnelID == "" || binding.TunnelName == "" || binding.RecordID == "" {
		return Envelope{Version: Version, State: StateReconnectRequired}, OutcomeReconnectRequired
	}
	if len(apiToken) > 4096 || strings.ContainsAny(apiToken, "\x00\r\n") {
		apiToken = ""
	}
	return Envelope{
		Version: Version, State: StateConnection,
		AccountID: binding.AccountID, ConsoleZoneID: binding.ZoneID, ConsoleHostname: binding.Hostname,
		TunnelID: binding.TunnelID, TunnelName: binding.TunnelName, ConsoleRecordID: binding.RecordID,
		APIToken: apiToken,
	}, OutcomeConnection
}

func validateEnvelope(envelope Envelope) error {
	if envelope.Version != Version {
		return errInvalidArtifact
	}
	switch envelope.State {
	case StateReconnectRequired:
		if envelope.AccountID != "" || envelope.ConsoleZoneID != "" || envelope.ConsoleHostname != "" || envelope.TunnelID != "" || envelope.TunnelName != "" || envelope.ConsoleRecordID != "" || envelope.APIToken != "" {
			return errInvalidArtifact
		}
		return nil
	case StateConnection:
	default:
		return errInvalidArtifact
	}
	for _, field := range []struct {
		value string
		max   int
	}{
		{envelope.AccountID, 128}, {envelope.ConsoleZoneID, 128}, {envelope.TunnelID, 128},
		{envelope.TunnelName, 120}, {envelope.ConsoleRecordID, 128}, {envelope.ConsoleHostname, 253},
	} {
		if strings.TrimSpace(field.value) != field.value || field.value == "" || len(field.value) > field.max || strings.ContainsAny(field.value, "\x00\r\n") {
			return errInvalidArtifact
		}
	}
	hostname, err := domainname.NormalizeHostname(envelope.ConsoleHostname)
	if err != nil || hostname != envelope.ConsoleHostname {
		return errInvalidArtifact
	}
	if len(envelope.APIToken) > 4096 || strings.TrimSpace(envelope.APIToken) != envelope.APIToken || strings.ContainsAny(envelope.APIToken, "\x00\r\n") {
		return errInvalidArtifact
	}
	return nil
}

func validFilePath(path string) bool {
	return strings.TrimSpace(path) != "" && filepath.IsAbs(path) && filepath.Clean(path) != string(filepath.Separator)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func clearArtifactAfterFailure(path string, cause error) error {
	if err := removeArtifact(path); err != nil {
		return err
	}
	return cause
}
