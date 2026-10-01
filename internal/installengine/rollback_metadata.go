package installengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	managedReleaseMetadataFile = "release-metadata.json"
	platformRollbackPending    = "platform-rollback.pending"
	platformRollbackState      = "platform-rollback.state"
	platformRollbackAudit      = "platform-rollback.jsonl"
)

var schemaFingerprintPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type managedReleaseMetadata struct {
	FormatVersion     int                         `json:"format_version"`
	PreviousVersion   string                      `json:"previous_version"`
	TargetVersion     string                      `json:"target_version"`
	SchemaFingerprint string                      `json:"schema_fingerprint,omitempty"`
	Assets            []managedAssetRecoveryEntry `json:"assets"`
	RecordedAt        time.Time                   `json:"recorded_at"`
}

type platformRollbackRecord struct {
	FormatVersion     int       `json:"format_version"`
	FromVersion       string    `json:"from_version"`
	ToVersion         string    `json:"to_version"`
	SchemaFingerprint string    `json:"schema_fingerprint"`
	CLIVersion        string    `json:"cli_version,omitempty"`
	OperatorUID       int       `json:"operator_uid"`
	StartedAt         time.Time `json:"started_at"`
}

type platformRollbackAuditRecord struct {
	At            time.Time `json:"at"`
	Action        string    `json:"action"`
	Outcome       string    `json:"outcome"`
	FromVersion   string    `json:"from_version,omitempty"`
	ToVersion     string    `json:"to_version,omitempty"`
	OperatorUID   int       `json:"operator_uid"`
	SchemaChanged bool      `json:"schema_changed,omitempty"`
}

func writeManagedReleaseMetadata(directory string, metadata managedReleaseMetadata) error {
	if metadata.FormatVersion == 0 {
		metadata.FormatVersion = 1
	}
	if err := ValidateReleaseVersion(metadata.PreviousVersion); err != nil {
		return fmt.Errorf("invalid previous release version: %w", err)
	}
	if err := ValidateReleaseVersion(metadata.TargetVersion); err != nil {
		return fmt.Errorf("invalid target release version: %w", err)
	}
	if metadata.SchemaFingerprint != "" && !schemaFingerprintPattern.MatchString(metadata.SchemaFingerprint) {
		return errors.New("invalid database schema fingerprint")
	}
	if metadata.RecordedAt.IsZero() {
		metadata.RecordedAt = time.Now().UTC()
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("managed release recovery set is not a private directory")
	}
	contents, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode managed release metadata: %w", err)
	}
	return WriteAtomic(filepath.Join(directory, managedReleaseMetadataFile), contents, 0o600)
}

func readManagedReleaseMetadata(
	layout Layout,
	currentVersion string,
	allowedPaths map[string]struct{},
) (managedReleaseMetadata, error) {
	return readManagedReleaseMetadataFromDirectory(
		filepath.Join(layout.StateDir, "managed-assets.previous"),
		currentVersion,
		allowedPaths,
	)
}

func readManagedReleaseMetadataFromDirectory(
	directory, currentVersion string,
	allowedPaths map[string]struct{},
) (managedReleaseMetadata, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return managedReleaseMetadata{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return managedReleaseMetadata{}, errors.New("previous release recovery set is not a private directory")
	}
	path := filepath.Join(directory, managedReleaseMetadataFile)
	info, err = os.Lstat(path)
	if err != nil {
		return managedReleaseMetadata{}, errors.New(
			"previous release metadata is missing; a rollback-safe release snapshot is required",
		)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return managedReleaseMetadata{}, errors.New("previous release metadata is not a private regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return managedReleaseMetadata{}, err
	}
	var metadata managedReleaseMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return managedReleaseMetadata{}, fmt.Errorf("parse previous release metadata: %w", err)
	}
	if metadata.FormatVersion != 1 {
		return managedReleaseMetadata{}, errors.New("previous release metadata format is unsupported")
	}
	if err := ValidateReleaseVersion(metadata.PreviousVersion); err != nil {
		return managedReleaseMetadata{}, errors.New("previous release metadata has an invalid release version")
	}
	if err := ValidateReleaseVersion(metadata.TargetVersion); err != nil {
		return managedReleaseMetadata{}, errors.New("previous release metadata has an invalid target version")
	}
	if strings.TrimSpace(currentVersion) != "" && metadata.TargetVersion != strings.TrimSpace(currentVersion) {
		return managedReleaseMetadata{}, errors.New(
			"previous release metadata does not match the active platform version",
		)
	}
	if metadata.SchemaFingerprint != "" && !schemaFingerprintPattern.MatchString(metadata.SchemaFingerprint) {
		return managedReleaseMetadata{}, errors.New("previous release metadata contains an invalid schema fingerprint")
	}
	seen := make(map[string]struct{}, len(metadata.Assets))
	var hasCompose bool
	for _, entry := range metadata.Assets {
		if !validManagedAssetPath(entry.RelativePath) {
			return managedReleaseMetadata{}, errors.New(
				"previous release metadata contains an invalid managed asset path",
			)
		}
		if _, allowed := allowedPaths[entry.RelativePath]; !allowed {
			return managedReleaseMetadata{}, fmt.Errorf(
				"previous release metadata contains unknown asset %q",
				entry.RelativePath,
			)
		}
		if _, duplicate := seen[entry.RelativePath]; duplicate {
			return managedReleaseMetadata{}, fmt.Errorf(
				"previous release metadata duplicates asset %q",
				entry.RelativePath,
			)
		}
		seen[entry.RelativePath] = struct{}{}
		if entry.RelativePath == "compose.production.yaml" && entry.Existed {
			hasCompose = true
		}
		if !entry.Existed {
			if entry.SHA256 != "" {
				return managedReleaseMetadata{}, errors.New(
					"previous release metadata has a checksum for an absent asset",
				)
			}
			continue
		}
		if _, err := hex.DecodeString(entry.SHA256); err != nil || len(entry.SHA256) != sha256.Size*2 {
			return managedReleaseMetadata{}, fmt.Errorf(
				"previous release metadata has an invalid checksum for %q",
				entry.RelativePath,
			)
		}
		assetPath := filepath.Join(directory, filepath.FromSlash(entry.RelativePath))
		assetInfo, err := os.Lstat(assetPath)
		if err != nil || !assetInfo.Mode().IsRegular() || assetInfo.Mode()&os.ModeSymlink != 0 {
			return managedReleaseMetadata{}, fmt.Errorf(
				"previous managed asset %q is missing or unsafe",
				entry.RelativePath,
			)
		}
		assetContents, err := os.ReadFile(assetPath)
		if err != nil {
			return managedReleaseMetadata{}, fmt.Errorf("read previous managed asset %q: %w", entry.RelativePath, err)
		}
		digest := sha256.Sum256(assetContents)
		if hex.EncodeToString(digest[:]) != entry.SHA256 {
			return managedReleaseMetadata{}, fmt.Errorf(
				"previous managed asset %q failed its recorded checksum",
				entry.RelativePath,
			)
		}
	}
	if !hasCompose {
		return managedReleaseMetadata{}, errors.New("previous release metadata does not contain production Compose")
	}
	for path := range allowedPaths {
		if _, ok := seen[path]; !ok {
			return managedReleaseMetadata{}, fmt.Errorf("previous release metadata is missing managed asset %q", path)
		}
	}
	return metadata, nil
}

func readManagedVersion(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("VERSION is not a regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(contents))
	if err := ValidateReleaseVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

func writePlatformRollbackRecord(layout Layout, record platformRollbackRecord) error {
	if record.FormatVersion != 1 || record.FromVersion == record.ToVersion ||
		!schemaFingerprintPattern.MatchString(record.SchemaFingerprint) {
		return errors.New("platform rollback recovery record is invalid")
	}
	if err := ValidateReleaseVersion(record.FromVersion); err != nil {
		return err
	}
	if err := ValidateReleaseVersion(record.ToVersion); err != nil {
		return err
	}
	comparison, err := compareReleaseVersionOrder(record.ToVersion, record.FromVersion)
	if err != nil || comparison >= 0 {
		return errors.New("platform rollback target must be older than the active release")
	}
	contents, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(layout.StateDir, platformRollbackPending), contents, 0o600)
}

func readPlatformRollbackRecord(layout Layout) (*platformRollbackRecord, error) {
	path := filepath.Join(layout.StateDir, platformRollbackPending)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect platform rollback recovery state: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("platform rollback recovery state is not a private regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read platform rollback recovery state: %w", err)
	}
	var record platformRollbackRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		return nil, fmt.Errorf("parse platform rollback recovery state: %w", err)
	}
	if record.FormatVersion != 1 || record.FromVersion == record.ToVersion ||
		!schemaFingerprintPattern.MatchString(record.SchemaFingerprint) {
		return nil, errors.New("platform rollback recovery state is invalid")
	}
	if ValidateReleaseVersion(record.FromVersion) != nil || ValidateReleaseVersion(record.ToVersion) != nil {
		return nil, errors.New("platform rollback recovery state contains invalid release versions")
	}
	comparison, err := compareReleaseVersionOrder(record.ToVersion, record.FromVersion)
	if err != nil || comparison >= 0 {
		return nil, errors.New("platform rollback recovery target is not older than the active release")
	}
	return &record, nil
}

func writeCurrentPlatformRollbackState(layout Layout, record platformRollbackRecord) error {
	state := struct {
		FormatVersion int       `json:"format_version"`
		Platform      string    `json:"platform_version"`
		CLI           string    `json:"cli_version"`
		RolledBackAt  time.Time `json:"rolled_back_at"`
	}{
		FormatVersion: 1,
		Platform:      record.ToVersion,
		CLI:           record.CLIVersion,
		RolledBackAt:  time.Now().UTC(),
	}
	contents, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(layout.StateDir, platformRollbackState), contents, 0o600)
}

func appendRollbackAudit(layout Layout, outcome, fromVersion, toVersion string, schemaChanged bool) error {
	path := filepath.Join(layout.StateDir, platformRollbackAudit)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return errors.New("platform rollback audit log is not a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open platform rollback audit log: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect platform rollback audit log: %w", err)
	}
	entry := platformRollbackAuditRecord{
		At: time.Now().UTC(), Action: "platform_rollback", Outcome: outcome,
		FromVersion: fromVersion, ToVersion: toVersion, OperatorUID: os.Geteuid(), SchemaChanged: schemaChanged,
	}
	if err := json.NewEncoder(file).Encode(entry); err != nil {
		return fmt.Errorf("write platform rollback audit log: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync platform rollback audit log: %w", err)
	}
	return nil
}
