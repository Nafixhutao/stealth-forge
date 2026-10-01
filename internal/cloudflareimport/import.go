// Package cloudflareimport prepares and reads the narrow encrypted artifact
// used to migrate the legacy Cloudflare setup connection into PostgreSQL.
package cloudflareimport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/functionsecret"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

const (
	// Version identifies the serialized Cloudflare import envelope contract.
	Version               = 1
	maxSetupSnapshotBytes = 8 << 20
	maxArtifactBytes      = 16 << 10
	workerGroupID         = 10001
)

const (
	StateConnection        = "connection"
	StateReconnectRequired = "reconnect_required"
)

const (
	legacySetupSnapshotName = "setup-state.enc"
	legacySetupTempName     = ".setup-state.enc.tmp"
)

const (
	OutcomeNoImport          = "no_import"
	OutcomeConnection        = "connection"
	OutcomeReconnectRequired = "reconnect_required"
)

var (
	errInvalidPath       = errors.New("Cloudflare import path is invalid")
	errInvalidSource     = errors.New("encrypted setup snapshot could not be recovered")
	errInvalidArtifact   = errors.New("Cloudflare import artifact is invalid")
	errArtifactDecrypt   = errors.New("Cloudflare import artifact could not be decrypted")
	errArtifactEncrypt   = errors.New("Cloudflare import artifact could not be encrypted")
	errArtifactIO        = errors.New("Cloudflare import artifact could not be published")
	errUnsafeImportEntry = errors.New("Cloudflare import entry is not a regular file")
	// ErrWorkerBoundaryUnsafe means the worker-visible directory contains a
	// legacy full setup snapshot or unexpected content that cannot be safely
	// removed. The preparation service must fail so Compose does not start the
	// worker with a wider decryptable trust boundary.
	ErrWorkerBoundaryUnsafe = errors.New("worker Cloudflare import directory is unsafe")
)

// Envelope is the complete plaintext contract visible to the production
// worker. Keep this type limited to the durable Cloudflare connection fields.
// ReconnectRequired carries no credential or partial provider identity.
type Envelope struct {
	Version         int    `json:"version"`
	State           string `json:"state"`
	AccountID       string `json:"account_id,omitempty"`
	ConsoleZoneID   string `json:"console_zone_id,omitempty"`
	ConsoleHostname string `json:"console_hostname,omitempty"`
	TunnelID        string `json:"tunnel_id,omitempty"`
	TunnelName      string `json:"tunnel_name,omitempty"`
	ConsoleRecordID string `json:"console_record_id,omitempty"`
	APIToken        string `json:"api_token,omitempty"`
}

// FileOwner is used by the root-only Compose preparation container to make
// the encrypted artifact readable by the fixed non-root worker identity.
type FileOwner struct {
	UID int
	GID int
}

// Prepare reads the complete encrypted onboarding snapshot in the trusted
// one-shot preparation process, derives only Cloudflare connection fields,
// and atomically publishes an encrypted narrow artifact. A missing source or
// a valid non-Cloudflare setup removes any stale derived artifact.
func Prepare(ctx context.Context, sourcePath, destinationPath string, cipher *functionsecret.Cipher, owner *FileOwner) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !validFilePath(sourcePath) || !validFilePath(destinationPath) || sourcePath == destinationPath || cipher == nil {
		return "", errInvalidPath
	}
	importDirectory := filepath.Dir(destinationPath)
	relativeSource, err := filepath.Rel(importDirectory, sourcePath)
	if err != nil || relativeSource == "." || (relativeSource != ".." && !strings.HasPrefix(relativeSource, ".."+string(filepath.Separator))) {
		return "", errInvalidPath
	}
	if err := cleanWorkerImportDirectory(importDirectory, filepath.Base(destinationPath)); err != nil {
		return "", err
	}
	sourceInfo, err := os.Lstat(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		if removeErr := removeArtifact(destinationPath); removeErr != nil {
			return "", removeErr
		}
		return OutcomeNoImport, nil
	}
	if err != nil || sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() || sourceInfo.Size() <= 0 || sourceInfo.Size() > maxSetupSnapshotBytes {
		return "", clearArtifactAfterFailure(destinationPath, errInvalidSource)
	}
	state, err := setupstate.LoadEncryptedSnapshot(ctx, sourcePath, cipher)
	if err != nil {
		return "", clearArtifactAfterFailure(destinationPath, errInvalidSource)
	}

	envelope, outcome := envelopeFromSetupState(state)
	if outcome == OutcomeNoImport {
		if err := removeArtifact(destinationPath); err != nil {
			return "", err
		}
		return outcome, nil
	}
	ciphertext, err := Encrypt(envelope, cipher)
	if err != nil {
		return "", clearArtifactAfterFailure(destinationPath, err)
	}
	if err := writeAtomic(destinationPath, ciphertext, owner); err != nil {
		return "", clearArtifactAfterFailure(destinationPath, err)
	}
	return outcome, nil
}

// PublishLegacySetupSnapshot copies only the bounded encrypted legacy setup
// snapshot into the narrow Compose handoff directory. It never follows the
// source symlink, accepts no other file type, and leaves the input directory
// empty when the optional source is absent. When an owner is supplied, a
// root-run initializer resets a reused volume before cleanup and then assigns
// the handoff directory to the installation UID and fixed worker group.
