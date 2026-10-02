package setupstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domainname"
	"github.com/Stealth-deplover/stealth/internal/functionsecret"
)

type Store interface {
	Load(context.Context) (State, error)
	Save(context.Context, State) error
	Update(context.Context, func(*State) error) (State, error)
}

var ErrUnavailable = errors.New("setup state is unavailable")

type FileStore struct {
	path   string
	cipher *functionsecret.Cipher
	mu     sync.Mutex
}

// legacyDraft contains the credential fields written by state version 1. It
// is deliberately separate from Draft so new state cannot accidentally gain a
// second credential source again.
type legacyDraft struct {
	DatabaseURL          string `json:"database_url,omitempty"`
	RedisURL             string `json:"redis_url,omitempty"`
	StorageS3AccessKey   string `json:"storage_s3_access_key,omitempty"`
	StorageS3SecretKey   string `json:"storage_s3_secret_key,omitempty"`
	CloudflareAccountID  string `json:"cloudflare_account_id,omitempty"`
	CloudflareZoneID     string `json:"cloudflare_zone_id,omitempty"`
	CloudflareTunnelID   string `json:"cloudflare_tunnel_id,omitempty"`
	CloudflareTunnelName string `json:"cloudflare_tunnel_name,omitempty"`
	CloudflareRecordID   string `json:"cloudflare_record_id,omitempty"`
}

type legacyState struct {
	Draft legacyDraft `json:"draft"`
}

func NewFileStore(path string, cipher *functionsecret.Cipher) (*FileStore, error) {
	path = strings.TrimSpace(path)
	if path == "" || filepath.Clean(path) == string(filepath.Separator) || cipher == nil {
		return nil, ErrUnavailable
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	clean := filepath.Clean(abs)
	if clean == string(filepath.Separator) {
		return nil, ErrUnavailable
	}
	return &FileStore{path: clean, cipher: cipher}, nil
}

// LoadEncryptedSnapshot reads a completed setup-state file without creating a
// lock file or mutating its parent directory. It is intended only for the
// post-install one-time PostgreSQL import, where the file is mounted
// read-only and atomic replacement guarantees readers see a complete version.
func LoadEncryptedSnapshot(ctx context.Context, path string, cipher *functionsecret.Cipher) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	path = strings.TrimSpace(path)
	if path == "" || filepath.Clean(path) == string(filepath.Separator) || cipher == nil {
		return State{}, ErrUnavailable
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	plaintext, err := cipher.Decrypt(contents)
	if err != nil {
		return State{}, errors.New("encrypted setup state could not be recovered")
	}
	var state State
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return State{}, errors.New("encrypted setup state could not be decoded")
	}
	var legacy legacyState
	if err := json.Unmarshal(plaintext, &legacy); err != nil {
		return State{}, errors.New("encrypted setup state could not be migrated")
	}
	if err := migrateState(&state, legacy); err != nil {
		return State{}, errors.New("encrypted setup state could not be migrated")
	}
	if err := validateState(state); err != nil {
		return State{}, errors.New("encrypted setup state is invalid")
	}
	if state.Secrets == nil {
		state.Secrets = make(map[string]string)
	}
	return state, nil
}

// PrepareShared makes the state directory usable by the host CLI and the root
// setup container when they run with different UIDs. The host process should
// call this before starting or resuming the setup Compose project so the
// directory group is the operator's group and atomic replacements inherit it.
func (s *FileStore) PrepareShared() error {
	if s == nil || s.cipher == nil {
		return ErrUnavailable
	}
	directory := filepath.Dir(s.path)
	mode := os.FileMode(0o770) | os.ModeSetgid
	if err := os.MkdirAll(directory, mode); err != nil {
		return fmt.Errorf("create shared setup state directory: %w", err)
	}
	if err := os.Chmod(directory, mode); err != nil {
		return fmt.Errorf("protect shared setup state directory: %w", err)
	}
	return nil
}

func (s *FileStore) Load(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.acquireFileLock()
	if err != nil {
		return State{}, err
	}
	defer lock.Close()
	return s.loadLocked(ctx)
}

func (s *FileStore) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.acquireFileLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	state.UpdatedAt = time.Now().UTC()
	state.Version = stateVersion
	if err := validateState(state); err != nil {
		return err
	}
	return s.saveLocked(state)
}

// Update serializes the complete read-modify-write cycle. Setup requests and
// the asynchronous installer use it so two browser tabs cannot overwrite a
// newer provider connection or install phase with an older snapshot.
func (s *FileStore) Update(ctx context.Context, mutate func(*State) error) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if mutate == nil {
		return State{}, errors.New("setup state update function is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := s.acquireFileLock()
	if err != nil {
		return State{}, err
	}
	defer lock.Close()
	state, err := s.loadLocked(ctx)
	if err != nil {
		return State{}, err
	}
	if err := mutate(&state); err != nil {
		return State{}, err
	}
	state.UpdatedAt = time.Now().UTC()
	state.Version = stateVersion
	if err := validateState(state); err != nil {
		return State{}, err
	}
	if err := s.saveLocked(state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s *FileStore) loadLocked(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	contents, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read setup state: %w", err)
	}
	plaintext, err := s.cipher.Decrypt(contents)
	if err != nil {
		return State{}, fmt.Errorf("decrypt setup state: %w", err)
	}
	var state State
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return State{}, fmt.Errorf("decode setup state: %w", err)
	}
	var legacy legacyState
	if err := json.Unmarshal(plaintext, &legacy); err != nil {
		return State{}, fmt.Errorf("decode legacy setup state: %w", err)
	}
	if err := migrateState(&state, legacy); err != nil {
		return State{}, err
	}
	if err := validateState(state); err != nil {
		return State{}, err
	}
	if state.Secrets == nil {
		state.Secrets = make(map[string]string)
	}
	return state, nil
}

// acquireFileLock serializes access between the setup API container and the
// host CLI. FileStore.mu protects callers within one process, while this
// advisory lock protects the complete read-modify-write transaction across
// processes. The lock and payload use group-private modes because the host
// CLI and root setup container may have different UIDs; the prepared state
// directory's setgid bit preserves its host operator group.
func (s *FileStore) acquireFileLock() (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, fmt.Errorf("create setup state directory: %w", err)
	}
	lockPath := s.path + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, sharedStateFileMode)
	if err != nil {
		return nil, fmt.Errorf("open setup state lock: %w", err)
	}
	if err := file.Chmod(sharedStateFileMode); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("protect setup state lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock setup state: %w", err)
	}
	return file, nil
}

func migrateState(state *State, legacy legacyState) error {
	if state.Version != 0 && state.Version != legacyStateVersion && state.Version != stateVersion {
		return fmt.Errorf("unsupported setup state version")
	}
	credentials := state.SetupCredentials()
	if credentials.DatabaseURL == "" {
		credentials.DatabaseURL = legacy.Draft.DatabaseURL
	}
	if credentials.RedisURL == "" {
		credentials.RedisURL = legacy.Draft.RedisURL
	}
	if credentials.StorageS3AccessKey == "" {
		credentials.StorageS3AccessKey = legacy.Draft.StorageS3AccessKey
	}
	if credentials.StorageS3SecretKey == "" {
		credentials.StorageS3SecretKey = legacy.Draft.StorageS3SecretKey
	}
	state.SetSetupCredentials(credentials)
	if state.Cloudflare.Binding.IsZero() {
		binding := cloudflareBindingFromLegacyDraft(legacy.Draft)
		binding.Hostname = domainname.Canonical(state.Draft.Hostname)
		if binding.HasIntent() {
			state.Cloudflare.Binding = binding
		}
	}
	state.Version = stateVersion
	return nil
}

func (s *FileStore) saveLocked(state State) error {
	if state.Secrets == nil {
		state.Secrets = make(map[string]string)
	}
	plaintext, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode setup state: %w", err)
	}
	ciphertext, err := s.cipher.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encrypt setup state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create setup state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".setup-state-*")
	if err != nil {
		return fmt.Errorf("create setup state file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(sharedStateFileMode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(ciphertext); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write setup state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync setup state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("commit setup state: %w", err)
	}
	return os.Chmod(s.path, sharedStateFileMode)
}
