package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/cloudflare"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

func (s *Server) startCloudflareOAuth(w http.ResponseWriter, r *http.Request) {
	// OAuth support remains in the repository for future provider validation,
	// but this setup flow deliberately stays inactive until Cloudflare provides
	// a verified redirect and scope configuration for the deployed origin. In
	// particular, never derive an OAuth redirect from a random Quick Tunnel.
	writeError(w, http.StatusGone, "cloudflare_oauth_inactive", "Cloudflare OAuth is experimental and inactive; use a scoped Cloudflare API token")
}

func (s *Server) setupCloudflareOAuthCallback(w http.ResponseWriter, r *http.Request) {
	// A stale or externally initiated callback must not exchange a code or
	// mutate encrypted setup state while OAuth is inactive.
	http.Redirect(w, r, setupRedirect("/setup?cloudflare=oauth-inactive"), http.StatusFound)
}

func (s *Server) listCloudflareAccounts(w http.ResponseWriter, r *http.Request) {
	client, err := s.cloudflareClient(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "cloudflare_not_connected", "connect Cloudflare before choosing an account")
		return
	}
	accounts, err := client.ListAccounts(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_unavailable", "Cloudflare accounts could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

func (s *Server) listCloudflareZones(w http.ResponseWriter, r *http.Request) {
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "account_id is required")
		return
	}
	client, err := s.cloudflareClient(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "cloudflare_not_connected", "connect Cloudflare before choosing a zone")
		return
	}
	zones, err := client.ListZones(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_unavailable", "Cloudflare zones could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zones": zones})
}

func (s *Server) saveCloudflareToken(w http.ResponseWriter, r *http.Request) {
	var request setupCloudflareTokenRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	token := strings.TrimSpace(request.APIToken)
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, "\x00\r\n") {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "Cloudflare API token is invalid")
		return
	}
	if s.cloudflareFactory == nil || s.setupState == nil {
		writeError(w, http.StatusServiceUnavailable, "cloudflare_unavailable", "Cloudflare setup is not available")
		return
	}
	client, err := s.cloudflareFactory(token)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "cloudflare_token_invalid", "Cloudflare API token is invalid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := client.ListAccounts(ctx); err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_token_rejected", "Cloudflare rejected the API token")
		return
	}
	state, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		state.Cloudflare.Mode = "api_token"
		state.Cloudflare.Connected = true
		state.Cloudflare.TokenValid = true
		state.Cloudflare.ExpiresAt = time.Time{}
		state.Cloudflare.OAuthStateHash = ""
		state.Cloudflare.OAuthExpiresAt = time.Time{}
		state.SetSecret("cloudflare_access_token", token)
		state.SetSecret("cloudflare_refresh_token", "")
		return nil
	})
	if err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", "Cloudflare settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, state.Public())
}

func (s *Server) createCloudflareTunnel(w http.ResponseWriter, r *http.Request) {
	var request setupCloudflareTunnelRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	client, err := s.cloudflareClient(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "cloudflare_not_connected", "connect Cloudflare before creating a tunnel")
		return
	}
	state, err := cloudflare.Provision(r.Context(), s.setupState, client, cloudflare.ProvisionRequest{
		AccountID: request.AccountID,
		ZoneID:    request.ZoneID,
		Hostname:  request.Hostname,
		Name:      request.Name,
	})
	if err != nil {
		var provisioningErr *cloudflare.ProvisionError
		_ = errors.As(err, &provisioningErr)
		switch {
		case errors.Is(err, cloudflare.ErrInvalidRequest):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "Cloudflare account, zone, and hostname are invalid")
		case errors.Is(err, cloudflare.ErrConflict):
			var bindingConflict *setupstate.CloudflareBindingConflict
			if provisioningErr != nil && errors.As(provisioningErr.Err, &bindingConflict) {
				writeError(w, http.StatusConflict, "cloudflare_tunnel_reconfiguration_required", bindingConflict.Error())
			} else {
				writeError(w, http.StatusConflict, "cloudflare_tunnel_conflict", cloudflareTunnelConflictMessage(err))
			}
		case errors.Is(err, cloudflare.ErrState):
			writeError(w, http.StatusConflict, "setup_state_conflict", "Cloudflare setup state could not be saved")
		case errors.Is(err, cloudflare.ErrProvider):
			stage := ""
			if provisioningErr != nil {
				stage = strings.ToLower(strings.TrimSpace(provisioningErr.Stage))
			}
			// The provider error carries the underlying Cloudflare status and
			// message. Log it (the client already redacts the API token) so an
			// operator can distinguish a missing token scope from a transient
			// provider failure, which the public response intentionally
			// summarizes.
			s.logger.Warn("Cloudflare provisioning provider operation failed",
				"request_id", w.Header().Get(requestIDHeader),
				"stage", stage,
				"error", err,
			)
			switch {
			case strings.HasPrefix(stage, "dns"):
				writeError(w, http.StatusBadGateway, "cloudflare_dns_failed", cloudflareDNSFailureMessage(err))
			case stage == "zones":
				writeError(w, http.StatusBadGateway, "cloudflare_unavailable", "Cloudflare zones could not be verified. Confirm the token grants Zone:Zone:Read for the selected account.")
			default:
				writeError(w, http.StatusBadGateway, "cloudflare_tunnel_failed", "Cloudflare could not provision the named tunnel. Confirm the token grants Account:Cloudflare Tunnel:Edit for the selected account.")
			}
		default:
			internalError(s, w, err)
		}
		return
	}
	if s.repo != nil {
		binding := state.EffectiveCloudflareBinding().Normalized()
		persistErr := s.repo.SaveCloudflareConnectionFromSetup(r.Context(), repository.CloudflareConnectionInput{
			AccountID: binding.AccountID, ConsoleZoneID: binding.ZoneID, ConsoleHostname: binding.Hostname,
			TunnelID: binding.TunnelID, TunnelName: binding.TunnelName, ConsoleRecordID: binding.RecordID,
			APIToken: strings.TrimSpace(state.Secret("cloudflare_access_token")),
		})
		if persistErr != nil {
			if errors.Is(persistErr, repository.ErrCloudflareConnectionConflict) {
				writeError(w, http.StatusConflict, "cloudflare_connection_conflict", "a different production Cloudflare tunnel connection is already saved")
			} else {
				writeError(w, http.StatusServiceUnavailable, "cloudflare_connection_unavailable", "Cloudflare was provisioned, but its production connection could not be persisted; retry this setup step")
			}
			return
		}
	}
	writeJSON(w, http.StatusOK, state.Public())
}

// cloudflareTunnelConflictMessage turns a generic provisioning conflict into a
// stage-specific instruction. The previous single sentence hid whether the
// hostname was already bound in DNS or the tunnel name was already taken, which
// left operators unable to tell what to clean up before retrying.
func cloudflareTunnelConflictMessage(err error) string {
	var provisioningErr *cloudflare.ProvisionError
	if errors.As(err, &provisioningErr) {
		switch provisioningErr.Stage {
		case "DNS lookup":
			return fmt.Sprintf("%v. The hostname is already bound in Cloudflare. Remove or update that DNS record, or reuse the existing tunnel, then retry this step.", provisioningErr.Err)
		case "tunnel lookup":
			return fmt.Sprintf("%v. Remove the conflicting tunnel in Cloudflare or choose a different tunnel name, then retry this step.", provisioningErr.Err)
		}
	}
	return "the saved Cloudflare tunnel conflicts with the requested account, zone, hostname, or provider resource"
}

// cloudflareDNSFailureMessage explains a failed Console DNS provisioning step.
// A rejected DNS write almost always means the scoped API token lacks
// Zone:DNS:Edit for the selected zone, which token verification cannot detect
// because account discovery needs no zone permission. Naming that scope turns an
// otherwise opaque 502 into a fixable instruction.
func cloudflareDNSFailureMessage(err error) string {
	switch {
	case errors.Is(err, cloudflare.ErrUnauthorized):
		return "Cloudflare rejected the DNS request. Grant Zone:DNS:Edit for the Console and workload zones, then retry this step."
	case errors.Is(err, cloudflare.ErrResourceNotFound):
		return "Cloudflare could not find the selected zone or DNS record. Re-select the domain and retry this step."
	default:
		return "Cloudflare could not configure the DNS record. Confirm the token grants Zone:DNS:Edit for the Console zone, then retry this step."
	}
}

func (s *Server) cloudflareTunnelStatus(w http.ResponseWriter, r *http.Request) {
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	binding := state.EffectiveCloudflareBinding()
	if err := state.Cloudflare.Binding.ValidateDraft(state.Draft); err != nil {
		var bindingConflict *setupstate.CloudflareBindingConflict
		if errors.As(err, &bindingConflict) {
			writeError(w, http.StatusConflict, "cloudflare_tunnel_reconfiguration_required", bindingConflict.Error())
		} else {
			writeError(w, http.StatusConflict, "setup_state_conflict", "Cloudflare tunnel state is inconsistent")
		}
		return
	}
	if binding.AccountID == "" || binding.TunnelID == "" {
		writeError(w, http.StatusConflict, "cloudflare_tunnel_missing", "create the named tunnel before checking status")
		return
	}
	client, err := s.cloudflareClient(r.Context())
	if err != nil {
		writeError(w, http.StatusConflict, "cloudflare_not_connected", "connect Cloudflare before checking tunnel status")
		return
	}
	status, err := client.TunnelStatus(r.Context(), binding.AccountID, binding.TunnelID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_unavailable", "Cloudflare tunnel status is unavailable")
		return
	}
	_, _ = s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return nil
		}
		state.Cloudflare.TokenValid = true
		return nil
	})
	writeJSON(w, http.StatusOK, setupCloudflareStatusResponse{TunnelID: binding.TunnelID, Status: status.Status, Healthy: cloudflare.StatusIsHealthy(status), Connections: status.Connections})
}

func (s *Server) cloudflareClient(ctx context.Context) (cloudflare.Client, error) {
	if s.cloudflareFactory == nil || s.setupState == nil {
		return nil, errors.New("Cloudflare is not configured")
	}
	state, err := s.setupState.Load(ctx)
	if err != nil {
		return nil, err
	}
	if state.Cloudflare.Mode != "api_token" || !state.Cloudflare.Connected || !state.Cloudflare.TokenValid {
		return nil, errors.New("Cloudflare is not connected with a scoped API token")
	}
	token := state.Secret("cloudflare_access_token")
	if token == "" {
		return nil, errors.New("Cloudflare is not connected")
	}
	return s.cloudflareFactory(token)
}
