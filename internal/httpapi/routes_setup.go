package httpapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/bootstrap"
	"github.com/Stealth-deplover/stealth/internal/setupconfig"
	"github.com/Stealth-deplover/stealth/internal/setuphandoff"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) registerSetupRoutes(r chi.Router) {
	r.Get("/setup/status", s.setupStatus)
	r.Get("/setup/github/manifest/callback", s.setupGitHubManifestCallback)
	r.Get("/setup/github/authorize/callback", s.setupGitHubAuthorizationCallback)
	r.Get("/setup/cloudflare/oauth/callback", s.setupCloudflareOAuthCallback)
	r.Post("/setup/quick-tunnel", s.registerSetupQuickTunnel)
	r.Post("/setup/recovery", s.recoverSetupSession)
	r.With(s.requireSetup).Get("/setup/preflight", s.setupPreflight)
	r.With(s.requireSetupMutation).Put("/setup/config", s.saveSetupConfig)
	r.With(s.requireSetupMutation).Post("/setup/github/manifest/start", s.startGitHubManifest)
	r.With(s.requireSetupMutation).Post("/setup/github/authorize/start", s.startGitHubAuthorization)
	r.With(s.requireSetupMutation).Post("/setup/github/manual", s.saveGitHubManual)
	r.With(s.requireSetupMutation).Post("/setup/cloudflare/oauth/start", s.startCloudflareOAuth)
	r.With(s.requireSetup).Get("/setup/cloudflare/accounts", s.listCloudflareAccounts)
	r.With(s.requireSetup).Get("/setup/cloudflare/zones", s.listCloudflareZones)
	r.With(s.requireSetupMutation).Post("/setup/cloudflare/token", s.saveCloudflareToken)
	r.With(s.requireSetupMutation).Post("/setup/cloudflare/tunnel", s.createCloudflareTunnel)
	r.With(s.requireSetup).Get("/setup/cloudflare/status", s.cloudflareTunnelStatus)
	r.With(s.requireSetupMutation).Post("/setup/infrastructure/database/test", s.testSetupDatabase)
	r.With(s.requireSetupMutation).Post("/setup/infrastructure/redis/test", s.testSetupRedis)
	r.With(s.requireSetupMutation).Post("/setup/infrastructure/storage/test", s.testSetupStorage)
	r.With(s.requireSetup).Get("/setup/handoff-token", s.issueSetupHandoffToken)
	r.Get("/setup/handoff/status", s.setupHandoffStatus)
	r.With(s.requireSetupMutation).Post("/setup/install", s.startSetupInstall)
	r.With(s.requireSetup).Get("/setup/install/events", s.setupInstallEvents)
}
func (s *Server) issueSetupHandoffToken(w http.ResponseWriter, r *http.Request) {
	if s.setupHandoff == nil || s.bootstrap == nil {
		writeError(w, http.StatusServiceUnavailable, "handoff_unavailable", "production session handoff is unavailable")
		return
	}
	status, err := s.bootstrap.BootstrapStatus(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	if status.SetupRequired {
		writeError(w, http.StatusConflict, "handoff_not_ready", "finish first-owner setup before preparing the production dashboard")
		return
	}
	token, err := s.setupHandoff.Issue(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "handoff_unavailable", "the production session handoff is no longer available")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// setupHandoffStatus is a host-CLI-only observation endpoint. It is
// authenticated with the same private bootstrap proof as other CLI
// coordination calls and never exposes the handoff token or session.
func (s *Server) setupHandoffStatus(w http.ResponseWriter, r *http.Request) {
	if s.setupHandoff == nil {
		writeError(w, http.StatusServiceUnavailable, "handoff_unavailable", "production session handoff is unavailable")
		return
	}
	if !s.verifyBootstrapCLI(w, r) {
		return
	}
	pendingStore, ok := s.setupHandoff.(setuphandoff.PendingStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "handoff_unavailable", "production session handoff status is unavailable")
		return
	}
	pending, err := pendingStore.Pending(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, setupHandoffStatusResponse{Pending: pending})
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	if !s.config.SetupMode || s.bootstrap == nil || s.setupState == nil {
		writeError(w, http.StatusNotFound, "not_found", "setup is not available")
		return
	}
	status, err := s.bootstrap.BootstrapStatus(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, setupStatusResponse{SetupRequired: status.SetupRequired, Ready: s.setupStateReady(), State: state.Public()})
}

func (s *Server) saveSetupConfig(w http.ResponseWriter, r *http.Request) {
	var request setupconfig.Request
	if !decodeJSON(w, r, &request) {
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.setupState == nil {
		writeError(w, http.StatusServiceUnavailable, "setup_unavailable", "setup state is not available")
		return
	}
	state, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		return setupconfig.Apply(state, request)
	})
	if err != nil {
		var bindingConflict *setupstate.CloudflareBindingConflict
		if errors.As(err, &bindingConflict) {
			writeError(w, http.StatusConflict, "cloudflare_tunnel_reconfiguration_required", bindingConflict.Error())
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state.Public())
}

// recoverSetupSession mints a fresh short-lived setup code only after the
// local CLI proves possession of the installation's bootstrap key. This is
// the trusted recovery path for a setup API restart after the first owner has
// already sealed the bootstrap row; anonymous remote callers cannot create a
// new first-run session.
func (s *Server) recoverSetupSession(w http.ResponseWriter, r *http.Request) {
	if !s.setupStateReady() || !s.verifyBootstrapCLI(w, r) {
		return
	}
	status, err := s.bootstrap.BootstrapStatus(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	if status.SetupRequired {
		writeError(w, http.StatusConflict, "recovery_not_needed", "first-owner setup is still waiting for its original local session")
		return
	}
	code, err := bootstrap.GenerateCode()
	if err != nil {
		internalError(s, w, err)
		return
	}
	codeHash := bootstrap.HashCode(code)
	sessionID, err := uuid.NewV7()
	if err != nil {
		internalError(s, w, err)
		return
	}
	expiresAt := time.Now().UTC().Add(bootstrap.CodeLifetime)
	_, err = s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		// Recovery is CLI-proofed and host-owned. It must remain available for
		// an installing or handoff run so a browser can reconnect after the
		// setup API/container was restarted. A completed run is the only state
		// that must not receive a new setup claim.
		if state.Phase == setupstate.PhaseComplete {
			return errors.New("installation is already complete")
		}
		state.SetupSessionID = sessionID.String()
		state.SetupCodeHash = base64.RawURLEncoding.EncodeToString(codeHash)
		state.SetupExpiresAt = expiresAt
		return nil
	})
	if err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", "a setup recovery session could not be saved")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBootstrapJSON(w, http.StatusCreated, bootstrapSessionResponse{SetupCode: code, ExpiresAt: expiresAt})
}
func (s *Server) registerSetupQuickTunnel(w http.ResponseWriter, r *http.Request) {
	if !s.setupStateReady() {
		writeError(w, http.StatusServiceUnavailable, "setup_unavailable", "setup state is not available")
		return
	}
	if !s.verifyBootstrapCLI(w, r) {
		return
	}
	var request setupQuickTunnelRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !validQuickTunnelName(request.ContainerName) || !validQuickTunnelURL(request.URL) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "temporary tunnel details are invalid")
		return
	}
	status, err := s.bootstrap.BootstrapStatus(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	state, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		// This endpoint is CLI-proofed and is a host-owned coordination
		// projection. The host may need to replace a Quick Tunnel after a
		// restart or transient disconnect while production installation is
		// already locked. Browser-owned mutations remain sealed by the normal
		// setup middleware and setupconfig.Apply guard.
		if state.Phase == setupstate.PhaseComplete {
			return errors.New("installation is already complete")
		}
		state.QuickTunnel = request.ContainerName
		if status.SetupRequired {
			state.SetupSessionID = ""
			state.SetupCodeHash = ""
			state.SetupExpiresAt = time.Time{}
		}
		state.SetSecret("quick_tunnel_url", strings.TrimRight(request.URL, "/"))
		return nil
	})
	if err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", "temporary tunnel details could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, state.Public())
}
