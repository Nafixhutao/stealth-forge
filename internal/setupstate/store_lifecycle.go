package setupstate

import (
	"errors"
	"strings"
	"time"
)

func NewState() State {
	return State{Version: stateVersion, Phase: PhaseCollecting, Draft: Draft{NetworkMode: "cloudflare_tunnel", DatabaseMode: "bundled", RedisMode: "bundled", StorageMode: "local"}, Secrets: make(map[string]string), UpdatedAt: time.Now().UTC()}
}

func (s State) Public() PublicState {
	binding := s.EffectiveCloudflareBinding()
	return PublicState{
		Version:      s.Version,
		Phase:        s.Phase,
		Step:         s.Step,
		ErrorCode:    s.ErrorCode,
		ErrorMessage: s.ErrorMessage,
		Draft: PublicDraft{
			InstanceName: s.Draft.InstanceName, PublicURL: s.Draft.PublicURL,
			NetworkMode: s.Draft.NetworkMode, Hostname: s.Draft.Hostname,
			CloudflareAccountID: binding.AccountID, CloudflareZoneID: binding.ZoneID,
			CloudflareTunnelID: binding.TunnelID, CloudflareTunnelName: binding.TunnelName, CloudflareRecordID: binding.RecordID,
			DatabaseMode: s.Draft.DatabaseMode, DatabaseTested: s.Draft.DatabaseTested,
			RedisMode: s.Draft.RedisMode, RedisTested: s.Draft.RedisTested,
			StorageMode: s.Draft.StorageMode, StorageTested: s.Draft.StorageTested,
			StorageS3Endpoint: s.Draft.StorageS3Endpoint, StorageS3Region: s.Draft.StorageS3Region,
			StorageS3Bucket: s.Draft.StorageS3Bucket, StorageS3UseSSL: s.Draft.StorageS3UseSSL,
			StorageS3PathStyle: s.Draft.StorageS3PathStyle, StorageS3Prefix: s.Draft.StorageS3Prefix,
		},
		GitHub:      PublicGitHub{Mode: s.GitHub.Mode, ClientID: s.GitHub.ClientID, Connected: s.GitHub.Connected, AuthorizationSession: s.GitHub.AuthorizationSession, ManifestExpiresAt: s.GitHub.ManifestExpiresAt},
		Cloudflare:  PublicCloudflare{Mode: s.Cloudflare.Mode, Connected: s.Cloudflare.Connected, ExpiresAt: s.Cloudflare.ExpiresAt, TokenValid: s.Cloudflare.TokenValid},
		UpdatedAt:   s.UpdatedAt,
		LastEventID: s.LastEventID,
	}
}

func (s State) Secret(name string) string {
	return s.Secrets[strings.TrimSpace(name)]
}

func (s *State) SetSecret(name, value string) {
	if s.Secrets == nil {
		s.Secrets = make(map[string]string)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if strings.TrimSpace(value) == "" {
		delete(s.Secrets, name)
		return
	}
	s.Secrets[name] = value
}

// InstallationLocked reports whether browser-owned setup mutations must stop.
// The request phase is persisted before the asynchronous installer starts, so
// a process restart cannot reopen the configuration window after the operator
// has committed to installation.
func InstallationLocked(state State) bool {
	switch state.Phase {
	case PhaseInstallRequested, PhaseInstalling, PhaseHandoff, PhaseComplete:
		return true
	default:
		return false
	}
}

// InstallationRequested reports whether an installation has been committed
// for this setup state. InstallRunID remains set after a failed or completed
// run so repair and recovery can distinguish it from an unrequested draft.
func InstallationRequested(state State) bool {
	if state.InstallRunID != "" {
		return true
	}
	switch state.Phase {
	case PhaseInstallRequested, PhaseInstalling, PhaseHandoff, PhaseComplete:
		return true
	default:
		return false
	}
}

// InstallationTerminal reports whether the host-owned installation lifecycle
// has reached a state that the observer can render without another installer
// attempt. A handoff is deliberately not terminal because cleanup and the
// one-time production transition may still need to be resumed.
func InstallationTerminal(state State) bool {
	switch state.Phase {
	case PhaseComplete, PhaseFailed:
		return true
	default:
		return false
	}
}

// RequestInstallation durably advances a reviewed setup draft into the
// request phase. Callers must supply a fresh run identifier and persist the
// surrounding state through Store.Update.
func RequestInstallation(state *State, runID string) error {
	if state == nil {
		return errors.New("setup state is required")
	}
	if strings.TrimSpace(runID) == "" {
		return errors.New("installation run identifier is required")
	}
	if InstallationLocked(*state) {
		return errors.New("installation is already in progress or complete")
	}
	state.Phase = PhaseInstallRequested
	state.InstallRunID = strings.TrimSpace(runID)
	state.ErrorCode = ""
	state.ErrorMessage = ""
	state.LastEventID++
	return nil
}

// BeginInstallation claims a previously requested run. It is idempotent for
// the same run identifier, which lets the setup service recover a request that
// was persisted immediately before a process restart.
func BeginInstallation(state *State, runID string) error {
	if state == nil {
		return errors.New("setup state is required")
	}
	if strings.TrimSpace(runID) == "" || state.InstallRunID != strings.TrimSpace(runID) {
		return errors.New("installation run does not own setup state")
	}
	switch state.Phase {
	case PhaseInstallRequested:
		state.Phase = PhaseInstalling
		state.ErrorCode = ""
		state.ErrorMessage = ""
		state.Step = ""
		return nil
	case PhaseFailed:
		// A failed host installation is resumable. Re-claiming the same run
		// lets the documented `stealth install --repair` path re-run the engine
		// instead of only re-displaying the previous failure.
		state.Phase = PhaseInstalling
		state.ErrorCode = ""
		state.ErrorMessage = ""
		state.Step = ""
		return nil
	case PhaseInstalling:
		return nil
	default:
		return errors.New("setup installation is no longer startable")
	}
}

// UpdateInstallationProgress records a host-owned lifecycle milestone. The
// run identifier check is intentional: a stale repair process must not be able
// to publish progress for a newer installation request.
func UpdateInstallationProgress(state *State, runID, step string) error {
	if state == nil {
		return errors.New("setup state is required")
	}
	if strings.TrimSpace(runID) == "" || state.InstallRunID != strings.TrimSpace(runID) {
		return errors.New("installation run does not own setup state")
	}
	if state.Phase != PhaseInstalling && state.Phase != PhaseHandoff {
		return errors.New("setup installation is not running")
	}
	step = strings.TrimSpace(step)
	if step == "" {
		return errors.New("installation step is required")
	}
	if len(step) > 120 || strings.ContainsAny(step, "\x00\r\n") {
		return errors.New("installation step is invalid")
	}
	state.Step = step
	state.LastEventID++
	return nil
}

// FailInstallation transitions a host-owned run to a durable failure. Error
// text is supplied by the caller only after it has been reduced to a safe,
// non-secret message.
func FailInstallation(state *State, runID, code, message string) error {
	if state == nil {
		return errors.New("setup state is required")
	}
	if strings.TrimSpace(runID) == "" || state.InstallRunID != strings.TrimSpace(runID) {
		return errors.New("installation run does not own setup state")
	}
	if state.Phase != PhaseInstallRequested && state.Phase != PhaseInstalling && state.Phase != PhaseHandoff {
		return errors.New("setup installation cannot be failed from its current phase")
	}
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" || len(code) > 80 || strings.ContainsAny(code, "\x00\r\n") {
		return errors.New("installation error code is invalid")
	}
	if len(message) > 240 || strings.ContainsAny(message, "\x00\r\n") {
		return errors.New("installation error message is invalid")
	}
	state.Phase = PhaseFailed
	state.ErrorCode = code
	state.ErrorMessage = message
	state.LastEventID++
	return nil
}

// MarkInstallationHandoff publishes production readiness before temporary
// setup resources are removed. Keeping this transition separate preserves the
// one-time handoff race window across browser reconnects and CLI restarts.
func MarkInstallationHandoff(state *State, runID string) error {
	if state == nil {
		return errors.New("setup state is required")
	}
	if strings.TrimSpace(runID) == "" || state.InstallRunID != strings.TrimSpace(runID) {
		return errors.New("installation run does not own setup state")
	}
	if state.Phase != PhaseInstalling && state.Phase != PhaseHandoff {
		return errors.New("setup installation is not ready for handoff")
	}
	state.Phase = PhaseHandoff
	state.Step = "Handoff"
	state.ErrorCode = ""
	state.ErrorMessage = ""
	state.LastEventID++
	return nil
}

// CompleteInstallation closes the setup claim only after the production API
// has consumed (or expired) the one-time handoff and temporary services have
// been safely cleaned up.
func CompleteInstallation(state *State, runID string) error {
	if state == nil {
		return errors.New("setup state is required")
	}
	if strings.TrimSpace(runID) == "" || state.InstallRunID != strings.TrimSpace(runID) {
		return errors.New("installation run does not own setup state")
	}
	if state.Phase != PhaseHandoff && state.Phase != PhaseComplete {
		return errors.New("setup installation is not in handoff")
	}
	state.Phase = PhaseComplete
	state.Step = "Complete"
	state.ErrorCode = ""
	state.ErrorMessage = ""
	state.SetupSessionID = ""
	state.SetupCodeHash = ""
	state.SetupExpiresAt = time.Time{}
	state.LastEventID++
	return nil
}
