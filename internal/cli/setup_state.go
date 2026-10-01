package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/functionsecret"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

func (a *App) loadSetupState(values map[string]string) (setupstate.State, error) {
	store, err := a.setupStateStore(values)
	if err != nil {
		return setupstate.State{}, err
	}
	return store.Load(context.Background())
}

// setupLifecycleNeedsRecovery keeps an unfinished browser installation in the
// web setup path even after the host has rendered SETUP_MODE=false into the
// production env file. The durable phase, not that mutable deployment value,
// decides whether repair must restore the temporary setup services.
func (a *App) setupLifecycleNeedsRecovery(layout InstallLayout) bool {
	values, err := readEnvFile(layout.EnvFile)
	if err != nil {
		return false
	}
	state, err := a.loadSetupState(values)
	if err != nil {
		return false
	}
	return state.InstallRunID != "" && state.Phase != setupstate.PhaseComplete
}

func (a *App) setupStateStore(values map[string]string) (*setupstate.FileStore, error) {
	statePath := strings.TrimSpace(values["STEALTH_SETUP_STATE_FILE"])
	if statePath == "" {
		statePath = filepath.Join(strings.TrimRight(a.valueOrInstallRoot(values), "/"), "state", "setup-state.enc")
	}
	key, err := decodeConfigSecret(values["FUNCTIONS_SECRET_KEY"])
	if err != nil {
		return nil, err
	}
	cipher, err := functionsecret.New(key)
	if err != nil {
		return nil, err
	}
	return setupstate.NewFileStore(statePath, cipher)
}

func (a *App) previousSetupTunnel(values map[string]string) string {
	store, err := a.setupStateStore(values)
	if err != nil {
		return ""
	}
	state, err := store.Load(context.Background())
	if err != nil || !isQuickTunnelContainerName(state.QuickTunnel) {
		return ""
	}
	return state.QuickTunnel
}

func (a *App) valueOrInstallRoot(values map[string]string) string {
	if root := strings.TrimSpace(values["STEALTH_INSTALL_ROOT"]); root != "" {
		return root
	}
	if a != nil && a.homeDir != "" {
		return filepath.Join(a.homeDir, defaultHomeName)
	}
	return ""
}

func decodeConfigSecret(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("secret is empty")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(raw)
	}
	if err != nil || len(key) != functionsecret.KeySize {
		return nil, errors.New("secret is invalid")
	}
	return key, nil
}
