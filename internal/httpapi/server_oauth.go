package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/auth"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/oauthlogin"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// oauthStateCookieName holds the CSRF state and PKCE verifier between the
// authorize redirect and the provider callback. It is short-lived, HttpOnly,
// and SameSite=Lax so it survives the provider's top-level GET redirect back.
const oauthStateCookieName = "stealth_oauth_state"

// oauthStateTTL bounds how long a started login may sit at the provider.
const oauthStateTTL = 10 * time.Minute

// oauthProviderFromEnvironment builds a provider from the deployment
// environment. It is the fallback when the Console has no stored credentials.
func (s *Server) oauthProviderFromEnvironment(name string) (oauthlogin.Provider, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "github":
		p := oauthlogin.GitHub(s.config.OAuthGitHubClientID, s.config.OAuthGitHubClientSecret)
		return p, p.Configured()
	case "google":
		p := oauthlogin.Google(s.config.OAuthGoogleClientID, s.config.OAuthGoogleClientSecret)
		return p, p.Configured()
	default:
		return oauthlogin.Provider{}, false
	}
}

// oauthProvider resolves a provider name to its configured definition.
// Credentials saved in the Console take precedence over the deployment
// environment so an operator can change sign-in without editing files or
// restarting the API.
func (s *Server) oauthProvider(ctx context.Context, name string) (oauthlogin.Provider, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if s.repo != nil {
		if creds, err := s.repo.OAuthProviderCredentials(ctx, name); err == nil {
			switch name {
			case "github":
				p := oauthlogin.GitHub(creds.ClientID, creds.ClientSecret)
				return p, p.Configured()
			case "google":
				p := oauthlogin.Google(creds.ClientID, creds.ClientSecret)
				return p, p.Configured()
			}
		}
	}
	return s.oauthProviderFromEnvironment(name)
}

// oauthCallbackURL is the provider-facing redirect target. It must be
// registered verbatim in the provider's OAuth application settings.
func (s *Server) oauthCallbackURL(r *http.Request, provider string) (string, error) {
	return s.externalURL(r, "/v1/oauth/"+provider+"/callback")
}

// consoleURL builds a Console-origin redirect for the browser after login.
func (s *Server) consoleURL(path string) string {
	return strings.TrimRight(s.config.PublicAppURL, "/") + path
}

type oauthStatePayload struct {
	Provider string `json:"provider"`
	State    string `json:"state"`
	Verifier string `json:"verifier"`
	// Challenge is the S256(verifier) sent in the authorize URL. Kept in the
	// cookie so the callback can prove the pair it sends to the token
	// endpoint is the same pair the authorize request advertised.
	Challenge string `json:"challenge"`
	// Intent separates a sign-in round trip from linking a provider to the
	// already signed-in account. Empty means login (the historical default).
	Intent string `json:"intent,omitempty"`
	// AccountID is the account a link intent is bound to. The callback
	// re-checks the live session against it before writing anything.
	AccountID string `json:"account_id,omitempty"`
}

const (
	oauthIntentLogin = "login"
	oauthIntentLink  = "link"
)

// oauthStartResponse is the non-secret provider redirect the browser follows.
type oauthStartResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

// startOAuthLogin begins the browser flow and returns the provider URL. The
// caller redirects the browser to it; no secret is exposed to the page.
func (s *Server) startOAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.beginOAuth(w, r, oauthIntentLogin, "")
}

// startOAuthLink begins a link round trip for the signed-in account. The
// provider callback attaches the identity instead of opening a new session,
// so a link can never silently sign the browser into a different account.
func (s *Server) startOAuthLink(w http.ResponseWriter, r *http.Request) {
	account := accountFrom(r)
	if account.ID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	s.beginOAuth(w, r, oauthIntentLink, account.ID)
}

func (s *Server) beginOAuth(w http.ResponseWriter, r *http.Request, intent, accountID string) {
	provider, configured := s.oauthProvider(r.Context(), chiURLParam(r, "provider"))
	if !configured {
		writeError(w, http.StatusServiceUnavailable, "oauth_not_configured", "this sign-in provider is not configured on this instance")
		return
	}
	callbackURL, err := s.oauthCallbackURL(r, provider.Name)
	if err != nil || !strings.HasPrefix(callbackURL, "https://") {
		writeError(w, http.StatusUnprocessableEntity, "https_required", "sign-in with an external provider requires HTTPS")
		return
	}
	state, err := oauthlogin.NewState()
	if err != nil {
		internalError(s, w, err)
		return
	}
	verifier, challenge, err := oauthlogin.NewPKCE()
	if err != nil {
		internalError(s, w, err)
		return
	}
	if err := s.setOAuthStateCookie(w, oauthStatePayload{
		Provider:  provider.Name,
		State:     state,
		Verifier:  verifier,
		Challenge: challenge,
		Intent:    intent,
		AccountID: accountID,
	}); err != nil {
		internalError(s, w, err)
		return
	}
	// Correlate the authorize challenge with the callback cookie by state so a
	// stale browser URL is distinguishable from a provider rejection.
	s.logger.Info(
		"oauth start",
		"provider", provider.Name,
		"intent", intent,
		"state", state[:8],
		"challenge", challenge,
	)
	writeJSON(w, http.StatusOK, oauthStartResponse{
		AuthorizationURL: provider.AuthorizationURL(callbackURL, state, verifier),
	})
}

// oauthCallback finishes the flow: verify state, exchange the code, resolve the
// linked account, and open a Console session. An identity that is not linked to
// an account is rejected; external login never creates accounts.
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	providerName := strings.ToLower(strings.TrimSpace(chiURLParam(r, "provider")))
	provider, configured := s.oauthProvider(r.Context(), providerName)
	if !configured {
		http.Redirect(w, r, s.consoleURL("/login?oauth=unavailable"), http.StatusFound)
		return
	}
	payload, ok := s.takeOAuthStateCookie(w, r, provider.Name)
	if !ok {
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	providedState := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || len(code) > 4096 || strings.ContainsAny(code, "\x00\r\n") ||
		providedState == "" || providedState != payload.State {
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	callbackURL, err := s.oauthCallbackURL(r, provider.Name)
	if err != nil || !strings.HasPrefix(callbackURL, "https://") {
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	oauthToken, err := s.oauthLogin.Exchange(r.Context(), provider, code, callbackURL, payload.Verifier)
	if err != nil {
		// Distinguish an internal pair mismatch from a provider rejection:
		// if sha256(verifier) != the challenge advertised in the authorize
		// URL, the cookie and the URL came from different starts and the
		// fix is on this side; otherwise the provider rejected a consistent
		// pair and the cause is external (clock, credentials, redirect).
		computed := oauthlogin.S256Challenge(payload.Verifier)
		if computed != payload.Challenge {
			s.logger.Error(
				"oauth pkce pair mismatch",
				"provider", provider.Name,
				"state", payload.State[:min(8, len(payload.State))],
				"cookie_verifier_len", len(payload.Verifier),
				"cookie_challenge", payload.Challenge,
				"computed_challenge", computed,
			)
		} else {
			s.logger.Warn(
				"oauth token exchange failed with a consistent pkce pair",
				"provider", provider.Name,
				"state", payload.State[:min(8, len(payload.State))],
				"verifier_len", len(payload.Verifier),
				"challenge", payload.Challenge,
				"redirect", callbackURL,
				"client_id", provider.ClientID,
				"error", err,
			)
		}
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	identity, err := s.oauthLogin.UserInfo(r.Context(), provider, oauthToken)
	if err != nil {
		s.logger.Warn("oauth identity lookup failed", "provider", provider.Name, "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	if payload.Intent == oauthIntentLink {
		s.finishOAuthLink(w, r, provider, payload, identity)
		return
	}
	account, err := s.repo.AccountByProviderIdentity(r.Context(), provider.Name, identity.ProviderUserID)
	if errors.Is(err, repository.ErrNotFound) {
		// Standard provider sign-in: an unknown identity provisions an account
		// (or attaches to the existing account that owns the same verified
		// email), opens a session, and lands on the dashboard. The user is
		// never asked to link manually first.
		s.provisionOAuthAccount(w, r, provider, identity)
		return
	}
	if err != nil {
		s.logger.Error("oauth account lookup failed", "provider", provider.Name, "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		internalError(s, w, err)
		return
	}
	if err := s.repo.CreateSession(r.Context(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.Parse(account.ID)), tokenHash, time.Now().UTC().Add(s.config.SessionTTL), provider.Name); err != nil {
		s.logger.Error("oauth session creation failed", "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	s.setSessionCookie(w, token)
	s.redirectAfterOAuthLogin(w, r, account)
}

// redirectAfterOAuthLogin sends the browser to the right landing page for the
// account's role.
func (s *Server) redirectAfterOAuthLogin(w http.ResponseWriter, r *http.Request, account domain.Account) {
	destination := "/organizations"
	if account.InstanceRole == "instance_owner" || account.InstanceRole == "instance_admin" {
		destination = "/admin"
	}
	http.Redirect(w, r, s.consoleURL(destination), http.StatusFound)
}

// provisionOAuthAccount creates or claims the account for an unknown provider
// identity, opens a session, and redirects. Every failure redirects back to
// the sign-in page with a specific reason instead of leaving a dead end.
func (s *Server) provisionOAuthAccount(w http.ResponseWriter, r *http.Request, provider oauthlogin.Provider, identity oauthlogin.Identity) {
	if strings.TrimSpace(identity.Email) == "" {
		s.logger.Warn("oauth signup without an email", "provider", provider.Name)
		http.Redirect(w, r, s.consoleURL("/login?oauth=no_email"), http.StatusFound)
		return
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		internalError(s, w, err)
		return
	}
	accountID := uuid.Must(uuid.NewV7())
	organizationID := uuid.Must(uuid.NewV7())
	login := strings.TrimSpace(identity.Login)
	if login == "" {
		login = identity.Email
	}
	displayName := strings.TrimSpace(identity.DisplayName)
	if displayName == "" {
		displayName = strings.Split(identity.Email, "@")[0]
	}
	slug := "personal-" + strings.ReplaceAll(organizationID.String(), "-", "")[:16]
	account, _, err := s.repo.SignupWithIdentity(r.Context(), repository.IdentitySignupInput{
		AccountID:        accountID,
		OrganizationID:   organizationID,
		SessionID:        uuid.Must(uuid.NewV7()),
		Email:            identity.Email,
		EmailVerified:    identity.EmailVerified,
		OrganizationName: displayName + "'s organization",
		OrganizationSlug: slug,
		TokenHash:        tokenHash,
		SessionExpiresAt: time.Now().UTC().Add(s.config.SessionTTL),
		Provider:         provider.Name,
		ProviderUserID:   identity.ProviderUserID,
		ProviderLogin:    login,
		DisplayName:      identity.DisplayName,
		AvatarURL:        identity.AvatarURL,
	})
	switch {
	case errors.Is(err, repository.ErrBootstrapRequired):
		http.Redirect(w, r, s.consoleURL("/login?oauth=bootstrap"), http.StatusFound)
		return
	case errors.Is(err, repository.ErrIdentityEmailUnverified):
		// An account already uses this email but the provider has not verified
		// the address, so the sign-in cannot claim it.
		s.logger.Warn("oauth signup blocked by unverified email", "provider", provider.Name)
		http.Redirect(w, r, s.consoleURL("/login?oauth=unverified_email"), http.StatusFound)
		return
	case errors.Is(err, repository.ErrIdentityLinkedElsewhere):
		http.Redirect(w, r, s.consoleURL("/login?oauth=taken"), http.StatusFound)
		return
	case err != nil:
		s.logger.Error("oauth signup failed", "provider", provider.Name, "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	s.logger.Info("oauth account provisioned", "provider", provider.Name, "account_id", account.ID)
	s.setSessionCookie(w, token)
	s.redirectAfterOAuthLogin(w, r, account)
}

// finishOAuthLink attaches the provider identity to the signed-in account. It
// requires a live Console session whose account matches the one the link was
// started from, so a stolen callback URL cannot attach an identity elsewhere.
func (s *Server) finishOAuthLink(w http.ResponseWriter, r *http.Request, provider oauthlogin.Provider, payload oauthStatePayload, identity oauthlogin.Identity) {
	redirectFailure := func(reason string) {
		http.Redirect(w, r, s.consoleURL("/account?link="+reason), http.StatusFound)
	}

	cookie, err := r.Cookie(s.config.SessionCookieName)
	if err != nil || cookie.Value == "" {
		redirectFailure("session")
		return
	}
	// Resolve the session without writing a JSON error: this is a browser
	// redirect flow, so every outcome lands back on the account page.
	account, _, err := s.repo.AccountBySession(r.Context(), auth.HashSessionToken(cookie.Value))
	if err != nil {
		redirectFailure("session")
		return
	}
	if account.ID != payload.AccountID {
		redirectFailure("mismatch")
		return
	}

	login := strings.TrimSpace(identity.Login)
	if login == "" {
		login = strings.TrimSpace(identity.Email)
	}
	if login == "" {
		login = provider.Name + "-user"
	}

	err = s.repo.LinkAccountIdentity(
		r.Context(),
		uuid.Must(uuid.Parse(account.ID)),
		provider.Name,
		identity.ProviderUserID,
		login,
		identity.Email,
		identity.DisplayName,
		identity.AvatarURL,
	)
	if errors.Is(err, repository.ErrIdentityLinkedElsewhere) {
		redirectFailure("taken")
		return
	}
	if err != nil {
		s.logger.Error("oauth identity link failed", "provider", provider.Name, "error", err)
		redirectFailure("error")
		return
	}
	s.logger.Info("oauth identity linked", "provider", provider.Name, "account_id", account.ID)
	http.Redirect(w, r, s.consoleURL("/account?link=ok&provider="+provider.Name), http.StatusFound)
}

func (s *Server) setOAuthStateCookie(w http.ResponseWriter, payload oauthStatePayload) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(raw),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthStateTTL.Seconds()),
		Expires:  time.Now().UTC().Add(oauthStateTTL),
	})
	return nil
}

// takeOAuthStateCookie reads and immediately clears the state cookie so a
// callback URL cannot be replayed.
func (s *Server) takeOAuthStateCookie(w http.ResponseWriter, r *http.Request, provider string) (oauthStatePayload, bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || cookie.Value == "" {
		return oauthStatePayload{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return oauthStatePayload{}, false
	}
	var payload oauthStatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return oauthStatePayload{}, false
	}
	if payload.Provider != provider || payload.State == "" || payload.Verifier == "" {
		return oauthStatePayload{}, false
	}
	return payload, true
}

// chiURLParam reads a chi path parameter for the provider name.
func chiURLParam(r *http.Request, name string) string {
	if r == nil {
		return ""
	}
	return chi.URLParam(r, name)
}
