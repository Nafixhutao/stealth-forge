package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/bootstrap"
	"github.com/Stealth-deplover/stealth/internal/githubauth"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
	"github.com/google/uuid"
)

func (s *Server) startGitHubManifest(w http.ResponseWriter, r *http.Request) {
	if s.githubManifest == nil || s.githubOAuth == nil {
		writeError(w, http.StatusServiceUnavailable, "github_manifest_unavailable", "GitHub App Manifest setup is unavailable")
		return
	}
	callbackURL, err := s.externalURL(r, "/v1/setup/github/manifest/callback")
	if err != nil || !strings.HasPrefix(callbackURL, "https://") {
		writeError(w, http.StatusUnprocessableEntity, "https_required", "open the temporary HTTPS setup URL before connecting GitHub")
		return
	}
	authorizationCallbackURL, err := s.externalURL(r, "/v1/setup/github/authorize/callback")
	if err != nil || !strings.HasPrefix(authorizationCallbackURL, "https://") {
		writeError(w, http.StatusUnprocessableEntity, "https_required", "open the temporary HTTPS setup URL before connecting GitHub")
		return
	}
	plainState, stateHash, expiresAt, err := setupstate.NewManifestState()
	if err != nil {
		internalError(s, w, err)
		return
	}
	appName, err := githubauth.DefaultAppName()
	if err != nil {
		internalError(s, w, err)
		return
	}
	appURL, appURLErr := s.externalURL(r, "/")
	if appURLErr != nil {
		writeError(w, http.StatusUnprocessableEntity, "https_required", "the setup host is invalid")
		return
	}
	setupURL, setupURLErr := s.externalURL(r, "/setup")
	if setupURLErr != nil {
		writeError(w, http.StatusUnprocessableEntity, "https_required", "the setup host is invalid")
		return
	}
	manifestURL, manifestPayload, err := githubauth.ManifestForm(githubauth.AppManifest{
		Name:         appName,
		Description:  "Stealth developer control plane",
		URL:          appURL,
		RedirectURL:  callbackURL,
		CallbackURLs: []string{authorizationCallbackURL},
		Public:       false,
		// The setup service does not consume GitHub webhooks. Requesting an
		// installation event without a hook endpoint would make the manifest
		// inconsistent with the actual installation and needlessly expand the
		// App's configuration.
		DefaultEvents: []string{},
		DefaultPerms:  map[string]string{"contents": "read", "metadata": "read"},
		SetupURL:      setupURL,
	}, plainState)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "github_manifest_invalid", "the setup host cannot be used for GitHub App registration")
		return
	}
	if _, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		state.GitHub.Mode = "manifest"
		state.GitHub.ManifestStateHash = stateHash
		state.GitHub.ManifestExpiresAt = expiresAt
		return nil
	}); err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", "the GitHub setup session could not be started")
		return
	}
	writeJSON(w, http.StatusOK, setupGitHubManifestResponse{ManifestURL: manifestURL, Manifest: manifestPayload, ExpiresAt: expiresAt})
}

func (s *Server) setupGitHubManifestCallback(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	providedState := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || len(code) > 4096 || strings.ContainsAny(code, "\x00\r\n") || providedState == "" || s.setupState == nil || s.githubManifest == nil || s.githubOAuth == nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil || state.GitHub.ManifestExpiresAt.Before(time.Now().UTC()) || !compareStateHash(state.GitHub.ManifestStateHash, providedState) {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	if _, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		if state.GitHub.ManifestExpiresAt.Before(time.Now().UTC()) || !compareStateHash(state.GitHub.ManifestStateHash, providedState) {
			return errors.New("GitHub manifest state is expired or already used")
		}
		state.GitHub.ManifestStateHash = ""
		state.GitHub.ManifestExpiresAt = time.Time{}
		return nil
	}); err != nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	credentials, err := s.githubManifest.ConvertManifest(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	_, err = s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		state.GitHub.Mode = "manifest"
		state.GitHub.ClientID = credentials.ClientID
		state.GitHub.Connected = true
		state.SetSecret("github_client_secret", credentials.ClientSecret)
		state.SetSecret("github_private_key", credentials.PrivateKey)
		state.SetSecret("github_webhook_secret", credentials.WebhookSecret)
		return nil
	})
	if err != nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	authorizationURL, _, err := s.beginGitHubAuthorization(r.Context(), r, credentials.ClientID)
	if err != nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	http.Redirect(w, r, authorizationURL, http.StatusFound)
}

func (s *Server) startGitHubAuthorization(w http.ResponseWriter, r *http.Request) {
	if s.githubOAuth == nil || s.setupState == nil {
		writeError(w, http.StatusServiceUnavailable, "github_authorization_unavailable", "GitHub browser authorization is unavailable")
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	if !state.GitHub.Connected || !githubauth.ValidClientID(state.GitHub.ClientID) || strings.TrimSpace(state.Secret("github_client_secret")) == "" {
		writeError(w, http.StatusConflict, "github_not_connected", "connect a GitHub App before authorizing the first owner")
		return
	}
	authorizationURL, expiresAt, err := s.beginGitHubAuthorization(r.Context(), r, state.GitHub.ClientID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "github_authorization_invalid", "the setup host cannot be used for GitHub browser authorization")
		return
	}
	writeJSON(w, http.StatusOK, setupGitHubAuthorizationResponse{AuthorizationURL: authorizationURL, ExpiresAt: expiresAt})
}

func (s *Server) beginGitHubAuthorization(ctx context.Context, r *http.Request, clientID string) (string, time.Time, error) {
	if s.githubOAuth == nil || s.setupState == nil || !githubauth.ValidClientID(clientID) {
		return "", time.Time{}, errors.New("GitHub browser authorization is unavailable")
	}
	callbackURL, err := s.externalURL(r, "/v1/setup/github/authorize/callback")
	if err != nil || !strings.HasPrefix(callbackURL, "https://") {
		return "", time.Time{}, errors.New("GitHub browser authorization requires HTTPS")
	}
	plainState, stateHash, expiresAt, err := setupstate.NewOAuthState()
	if err != nil {
		return "", time.Time{}, err
	}
	codeVerifier, codeChallenge, err := githubauth.NewPKCE()
	if err != nil {
		return "", time.Time{}, err
	}
	authorizationURL, err := githubauth.AuthorizationURL(clientID, callbackURL, plainState, codeChallenge)
	if err != nil {
		return "", time.Time{}, err
	}
	if _, err := s.setupState.Update(ctx, func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		if !state.GitHub.Connected || state.GitHub.ClientID != clientID || strings.TrimSpace(state.Secret("github_client_secret")) == "" {
			return errors.New("GitHub App is not connected")
		}
		state.GitHub.AuthorizationStateHash = stateHash
		state.GitHub.AuthorizationExpiresAt = expiresAt
		state.SetSecret("github_oauth_code_verifier", codeVerifier)
		return nil
	}); err != nil {
		return "", time.Time{}, err
	}
	return authorizationURL, expiresAt, nil
}

func (s *Server) setupGitHubAuthorizationCallback(w http.ResponseWriter, r *http.Request) {
	providedState := strings.TrimSpace(r.URL.Query().Get("state"))
	if providedState == "" || len(providedState) > 512 || strings.ContainsAny(providedState, "\x00\r\n") || s.setupState == nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	var clientID, clientSecret, codeVerifier, setupSessionID, setupCodeHash string
	_, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		if state.GitHub.AuthorizationExpiresAt.Before(time.Now().UTC()) || !compareStateHash(state.GitHub.AuthorizationStateHash, providedState) {
			return errors.New("GitHub authorization state is expired or already used")
		}
		clientID = strings.TrimSpace(state.GitHub.ClientID)
		clientSecret = state.Secret("github_client_secret")
		codeVerifier = state.Secret("github_oauth_code_verifier")
		setupSessionID = state.SetupSessionID
		setupCodeHash = state.SetupCodeHash
		if !githubauth.ValidClientID(clientID) || clientSecret == "" || codeVerifier == "" || setupSessionID == "" || setupCodeHash == "" {
			return errors.New("GitHub authorization state is incomplete")
		}
		state.GitHub.AuthorizationStateHash = ""
		state.GitHub.AuthorizationExpiresAt = time.Time{}
		state.SetSecret("github_oauth_code_verifier", "")
		return nil
	})
	if err != nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if r.URL.Query().Get("error") != "" || code == "" || len(code) > 4096 || strings.ContainsAny(code, "\x00\r\n") || s.githubOAuth == nil || s.bootstrap == nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	callbackURL, err := s.externalURL(r, "/v1/setup/github/authorize/callback")
	if err != nil || !strings.HasPrefix(callbackURL, "https://") {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	token, err := s.githubOAuth.ExchangeAuthorizationCode(r.Context(), clientID, clientSecret, code, callbackURL, codeVerifier)
	if err != nil {
		s.logger.Warn("GitHub browser authorization exchange failed", "error", err)
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		s.logger.Warn("GitHub browser authorization returned an empty access token")
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	user, err := s.githubOAuth.GetUser(r.Context(), token.AccessToken)
	if err != nil {
		s.logger.Warn("GitHub browser identity lookup failed", "error", err)
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	sessionID, err := uuid.Parse(setupSessionID)
	if err != nil {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	codeHash, err := base64.RawURLEncoding.DecodeString(setupCodeHash)
	if err != nil || len(codeHash) != 32 {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	verification, err := s.bootstrap.VerifyBootstrapCode(r.Context(), codeHash)
	if err != nil || verification.ID != sessionID {
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	owner, err := s.createGitHubInstanceOwner(r.Context(), bootstrap.GitHubAuthorization{SessionID: sessionID, CodeHash: codeHash}, user)
	if err != nil {
		s.logger.Warn("GitHub browser owner creation failed", "error", err)
		http.Redirect(w, r, setupRedirect("/setup?github=error"), http.StatusFound)
		return
	}
	if !s.config.SetupMode {
		s.setSessionCookie(w, owner.SessionToken)
	}
	http.Redirect(w, r, setupRedirect("/setup?github=connected"), http.StatusFound)
}

func (s *Server) saveGitHubManual(w http.ResponseWriter, r *http.Request) {
	var request setupGitHubManualRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !githubauth.ValidClientID(request.ClientID) || strings.TrimSpace(request.ClientSecret) == "" || strings.TrimSpace(request.PrivateKey) == "" || strings.ContainsAny(request.ClientSecret+request.WebhookSecret, "\x00\r\n") || strings.ContainsRune(request.PrivateKey, '\x00') || len(request.ClientSecret) > 512 || len(request.PrivateKey) > 32768 || len(request.WebhookSecret) > 512 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "GitHub App credentials are invalid")
		return
	}
	if !strings.Contains(request.PrivateKey, "BEGIN") || !strings.Contains(request.PrivateKey, "END") {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "GitHub private key is invalid")
		return
	}
	state, err := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if setupstate.InstallationLocked(*state) {
			return errors.New("installation is already in progress or complete")
		}
		state.GitHub.Mode = "manual"
		state.GitHub.ClientID = strings.TrimSpace(request.ClientID)
		state.GitHub.Connected = true
		state.SetSecret("github_client_secret", request.ClientSecret)
		state.SetSecret("github_private_key", request.PrivateKey)
		state.SetSecret("github_webhook_secret", request.WebhookSecret)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusConflict, "setup_state_conflict", "the GitHub settings could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, state.Public())
}
