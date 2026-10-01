package installengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type migrationPhase string

const (
	migrationPhasePrepared         migrationPhase = "PREPARED"
	migrationPhaseBackedUp         migrationPhase = "BACKED_UP"
	migrationPhaseAssetsActivated  migrationPhase = "ASSETS_ACTIVATED"
	migrationPhaseConfigActivated  migrationPhase = "CONFIG_ACTIVATED"
	migrationPhaseVersionActivated migrationPhase = "VERSION_ACTIVATED"
	migrationPhaseComposeValidated migrationPhase = "COMPOSE_VALIDATED"
	migrationPhaseFinalized        migrationPhase = "FINALIZED"
)

const (
	migrationEventBackupRenamed  migrationPhase = "BACKUP_RENAMED"
	migrationEventAssetActivated migrationPhase = "ASSET_ACTIVATED"
)

// MigrationEvent reports a durable migration boundary to the test-only fault
// injection hook. A hook returning errMigrationProcessInterrupted models a
// hard process stop: it deliberately leaves the journal for the next process.
type MigrationEvent struct {
	Phase migrationPhase
	Asset string
	Index int
	Total int
}

var errMigrationProcessInterrupted = errors.New("managed asset migration interrupted")

type stagedManagedAsset struct {
	spec       ManagedAsset
	targetPath string
	stagePath  string
	backupPath string
	remove     bool
	existed    bool
	backedUp   bool
	installed  bool
}

const managedAssetPendingFile = "managed-assets.pending"

type managedAssetRecoveryManifest struct {
	TransactionID   string                      `json:"transaction_id"`
	TargetVersion   string                      `json:"target_version"`
	OriginalVersion string                      `json:"original_version"`
	BackupDir       string                      `json:"backup_dir"`
	StageDir        string                      `json:"stage_dir"`
	Phase           migrationPhase              `json:"phase"`
	Assets          []managedAssetRecoveryEntry `json:"assets"`
	Config          managedAssetRecoveryFile    `json:"config"`
	Version         managedAssetRecoveryFile    `json:"version"`
}

type managedAssetRecoveryEntry struct {
	RelativePath string `json:"relative_path"`
	Existed      bool   `json:"existed"`
	SHA256       string `json:"sha256,omitempty"`
}

type managedAssetRecoveryFile struct {
	Existed bool   `json:"existed"`
	Backup  string `json:"backup,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
}

// managedAssetMigration stages the complete release-managed runtime set before
// changing the installation. Existing managed files are moved into one
// bounded recovery set, never copied with secrets, and restored if a later
// commit step or Compose validation fails.
type managedAssetMigration struct {
	layout         Layout
	stageDir       string
	backupDir      string
	assets         []stagedManagedAsset
	allowedPaths   map[string]struct{}
	hook           func(MigrationEvent) error
	manifest       managedAssetRecoveryManifest
	journalWritten bool
}

func (m *managedAssetMigration) stagedAssetPath(relativePath string) (string, bool) {
	if m == nil {
		return "", false
	}
	for _, asset := range m.assets {
		if asset.spec.Path == relativePath {
			return asset.stagePath, true
		}
	}
	return "", false
}
func (m *managedAssetMigration) commit(plan Plan, originalEnv []byte, originalEnvExists bool, originalVersion []byte, originalVersionExists bool, originalVersionMode os.FileMode) error {
	if m == nil {
		return errors.New("managed asset migration is nil")
	}
	backupDir, err := os.MkdirTemp(m.layout.StateDir, ".stealth-managed-backup-")
	if err != nil {
		return fmt.Errorf("create managed asset recovery set: %w", err)
	}
	if err := os.Chmod(backupDir, 0o700); err != nil {
		_ = os.RemoveAll(backupDir)
		return fmt.Errorf("protect managed asset recovery set: %w", err)
	}
	m.backupDir = backupDir
	m.manifest = managedAssetRecoveryManifest{
		TransactionID:   filepath.Base(backupDir),
		TargetVersion:   strings.TrimSpace(plan.Version),
		OriginalVersion: strings.TrimSpace(plan.InstalledVersion),
		BackupDir:       filepath.Base(backupDir),
		StageDir:        filepath.Base(m.stageDir),
		Phase:           migrationPhasePrepared,
		Assets:          make([]managedAssetRecoveryEntry, 0, len(m.assets)),
		Config:          managedAssetRecoveryFile{Existed: originalEnvExists, Backup: "config.env", Mode: 0o600},
		Version:         managedAssetRecoveryFile{Existed: originalVersionExists, Backup: "VERSION", Mode: uint32(originalVersionMode)},
	}
	if m.manifest.OriginalVersion == "" && originalVersionExists {
		m.manifest.OriginalVersion = strings.TrimSpace(string(originalVersion))
	}
	if originalEnvExists {
		if err := WriteAtomic(filepath.Join(backupDir, m.manifest.Config.Backup), originalEnv, 0o600); err != nil {
			return m.failCommit(fmt.Errorf("back up config.env: %w", err))
		}
	}
	if originalVersionExists {
		mode := originalVersionMode
		if mode == 0 {
			mode = 0o644
			m.manifest.Version.Mode = uint32(mode)
		}
		if err := WriteAtomic(filepath.Join(backupDir, m.manifest.Version.Backup), originalVersion, mode); err != nil {
			return m.failCommit(fmt.Errorf("back up VERSION: %w", err))
		}
	}
	for index := range m.assets {
		asset := &m.assets[index]
		info, statErr := os.Lstat(asset.targetPath)
		if errors.Is(statErr, os.ErrNotExist) {
			m.manifest.Assets = append(m.manifest.Assets, managedAssetRecoveryEntry{RelativePath: asset.spec.Path})
			continue
		}
		if statErr != nil {
			return m.failCommit(fmt.Errorf("inspect managed asset %q: %w", asset.spec.Path, statErr))
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return m.failCommit(fmt.Errorf("managed asset %q is not a regular file", asset.spec.Path))
		}
		asset.existed = true
		asset.backupPath = filepath.Join(backupDir, filepath.FromSlash(asset.spec.Path))
		if err := os.MkdirAll(filepath.Dir(asset.backupPath), 0o700); err != nil {
			return m.failCommit(fmt.Errorf("prepare recovery path for %q: %w", asset.spec.Path, err))
		}
		contents, err := os.ReadFile(asset.targetPath)
		if err != nil {
			return m.failCommit(fmt.Errorf("read previous managed asset %q: %w", asset.spec.Path, err))
		}
		digest := sha256.Sum256(contents)
		m.manifest.Assets = append(m.manifest.Assets, managedAssetRecoveryEntry{
			RelativePath: asset.spec.Path, Existed: true, SHA256: fmt.Sprintf("%x", digest),
		})
	}
	if plan.Existing && !plan.Setup && m.manifest.OriginalVersion != "" {
		if err := writeManagedReleaseMetadata(backupDir, managedReleaseMetadata{
			FormatVersion:   1,
			PreviousVersion: m.manifest.OriginalVersion,
			TargetVersion:   strings.TrimSpace(plan.Version),
			Assets:          append([]managedAssetRecoveryEntry(nil), m.manifest.Assets...),
		}); err != nil {
			return m.failCommit(fmt.Errorf("record previous release metadata: %w", err))
		}
	}
	if err := m.writeJournal(); err != nil {
		return m.failCommit(err)
	}
	m.journalWritten = true
	if err := m.notify(MigrationEvent{Phase: migrationPhasePrepared}); err != nil {
		return m.failCommit(err)
	}
	backupIndex := 0
	for index := range m.assets {
		asset := &m.assets[index]
		if !asset.existed {
			continue
		}
		if err := renameAndSync(asset.targetPath, asset.backupPath); err != nil {
			return m.failCommit(fmt.Errorf("save previous managed asset %q: %w", asset.spec.Path, err))
		}
		asset.backedUp = true
		backupIndex++
		if err := m.notify(MigrationEvent{Phase: migrationEventBackupRenamed, Asset: asset.spec.Path, Index: backupIndex, Total: len(m.assets)}); err != nil {
			return m.failCommit(err)
		}
	}
	if err := m.transition(migrationPhaseBackedUp); err != nil {
		return m.failCommit(err)
	}
	for index := range m.assets {
		asset := &m.assets[index]
		if asset.remove {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(asset.targetPath), 0o700); err != nil {
			return m.failCommit(fmt.Errorf("create managed asset directory for %q: %w", asset.spec.Path, err))
		}
		if err := renameAndSync(asset.stagePath, asset.targetPath); err != nil {
			return m.failCommit(fmt.Errorf("activate managed asset %q: %w", asset.spec.Path, err))
		}
		asset.installed = true
		if err := m.notify(MigrationEvent{Phase: migrationEventAssetActivated, Asset: asset.spec.Path, Index: index + 1, Total: len(m.assets)}); err != nil {
			return m.failCommit(err)
		}
	}
	if err := m.transition(migrationPhaseAssetsActivated); err != nil {
		return m.failCommit(err)
	}
	return nil
}
func (m *managedAssetMigration) transition(phase migrationPhase) error {
	if m == nil || !m.journalWritten {
		return errors.New("managed asset recovery journal is not active")
	}
	m.manifest.Phase = phase
	if err := m.writeJournal(); err != nil {
		return err
	}
	return m.notify(MigrationEvent{Phase: phase})
}
func (m *managedAssetMigration) notify(event MigrationEvent) error {
	if m != nil && m.hook != nil {
		if err := m.hook(event); err != nil {
			return err
		}
	}
	return nil
}
func (m *managedAssetMigration) writeJournal() error {
	contents, err := json.Marshal(m.manifest)
	if err != nil {
		return fmt.Errorf("encode managed asset recovery journal: %w", err)
	}
	if err := WriteAtomic(filepath.Join(m.layout.StateDir, managedAssetPendingFile), contents, 0o600); err != nil {
		return fmt.Errorf("write managed asset recovery journal: %w", err)
	}
	return nil
}
func (m *managedAssetMigration) failCommit(cause error) error {
	if errors.Is(cause, errMigrationProcessInterrupted) {
		return cause
	}
	return errors.Join(cause, m.rollback())
}
func (m *managedAssetMigration) rollback() error {
	if m == nil {
		return nil
	}
	if !m.journalWritten {
		var cleanupErr error
		if m.backupDir != "" {
			cleanupErr = errors.Join(cleanupErr, removeAllAndSync(m.backupDir))
		}
		if m.stageDir != "" {
			cleanupErr = errors.Join(cleanupErr, removeAllAndSync(m.stageDir))
		}
		return cleanupErr
	}
	return recoverInterruptedManagedAssetMigration(m.layout, m.allowedPaths)
}
func (m *managedAssetMigration) finalize() error {
	if m == nil {
		return nil
	}
	if m.manifest.Phase != migrationPhaseComposeValidated {
		return errors.New("managed asset migration has not passed Compose validation")
	}
	// Do not discard the previous recovery set until the new complete backup
	// exists and the new platform has passed Compose validation. If this process
	// dies during publication, recovery sees COMPOSE_VALIDATED and completes
	// forward without exposing a mixed release.
	if err := publishManagedAssetRecoverySet(m.layout, m.manifest); err != nil {
		return err
	}
	m.manifest.Phase = migrationPhaseFinalized
	if err := m.writeJournal(); err != nil {
		return err
	}
	return removeAndSync(filepath.Join(m.layout.StateDir, managedAssetPendingFile))
}
