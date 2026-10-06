package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/auth"
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

// oauthProvider resolves a provider name to its configured definition. A
// provider without operator credentials is reported as not configured so the
// Console can hide or explain the button instead of failing mid-redirect.
func (s *Server) oauthProvider(name string) (oauthlogin.Provider, bool) {
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
}

// oauthStartResponse is the non-secret provider redirect the browser follows.
type oauthStartResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

// startOAuthLogin begins the browser flow and returns the provider URL. The
// caller redirects the browser to it; no secret is exposed to the page.
func (s *Server) startOAuthLogin(w http.ResponseWriter, r *http.Request) {
	provider, configured := s.oauthProvider(chiURLParam(r, "provider"))
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
	if err := s.setOAuthStateCookie(w, oauthStatePayload{Provider: provider.Name, State: state, Verifier: verifier}); err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, oauthStartResponse{
		AuthorizationURL: provider.AuthorizationURL(callbackURL, state, challenge),
	})
}

// oauthCallback finishes the flow: verify state, exchange the code, resolve the
// linked account, and open a Console session. An identity that is not linked to
// an account is rejected; external login never creates accounts.
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	providerName := strings.ToLower(strings.TrimSpace(chiURLParam(r, "provider")))
	provider, configured := s.oauthProvider(providerName)
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
	accessToken, err := s.oauthLogin.Exchange(r.Context(), provider, code, callbackURL, payload.Verifier)
	if err != nil {
		s.logger.Warn("oauth token exchange failed", "provider", provider.Name, "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	identity, err := s.oauthLogin.UserInfo(r.Context(), provider, accessToken)
	if err != nil {
		s.logger.Warn("oauth identity lookup failed", "provider", provider.Name, "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	account, err := s.repo.AccountByProviderIdentity(r.Context(), provider.Name, identity.ProviderUserID)
	if errors.Is(err, repository.ErrNotFound) {
		// The identity is valid but not linked to any account. Do not create
		// one implicitly; surface the state so the operator can link it.
		http.Redirect(w, r, s.consoleURL("/login?oauth=unlinked"), http.StatusFound)
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
	if err := s.repo.CreateSession(r.Context(), uuid.Must(uuid.NewV7()), uuid.Must(uuid.Parse(account.ID)), tokenHash, time.Now().UTC().Add(s.config.SessionTTL)); err != nil {
		s.logger.Error("oauth session creation failed", "error", err)
		http.Redirect(w, r, s.consoleURL("/login?oauth=error"), http.StatusFound)
		return
	}
	s.setSessionCookie(w, token)
	// Instance owners and admins operate the Admin Console; everyone else
	// lands on their organizations.
	destination := "/organizations"
	if account.InstanceRole == "instance_owner" || account.InstanceRole == "instance_admin" {
		destination = "/admin"
	}
	http.Redirect(w, r, s.consoleURL(destination), http.StatusFound)
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
