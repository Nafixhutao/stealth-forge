// Package oauthlogin implements the browser OAuth login flow for external
// identity providers (GitHub, Google) on top of golang.org/x/oauth2.
//
// It is deliberately separate from the first-owner bootstrap flow in
// internal/githubauth: login only resolves an already-linked
// account_identities row and never creates an account.
package oauthlogin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	// requestTimeout bounds a single provider round trip.
	requestTimeout = 15 * time.Second
	// maxBodyBytes bounds provider user-info responses so a hostile endpoint
	// cannot stream unbounded data into memory.
	maxBodyBytes = 1 << 20
	// gitHubEmailsURL lists the account's addresses with verification flags.
	// It requires the user:email scope the GitHub provider requests.
	gitHubEmailsURL = "https://api.github.com/user/emails"
)

// kind selects the user-info parsing shape. The authorization and token
// endpoints come from the oauth2.Config, so both providers share one flow.
type kind int

const (
	kindGitHub kind = iota
	kindGoogle
)

// Provider is one configured identity provider. Endpoint values match the
// documented providers; they are inlined rather than imported from
// oauth2/google so the API does not take the compute-metadata dependency.
type Provider struct {
	Name         string
	Endpoint     oauth2.Endpoint
	UserInfoURL  string
	Scopes       []string
	ClientID     string
	ClientSecret string
	kind         kind
}

// GitHub returns the GitHub provider definition. AuthStyleInParams matches the
// documented body-parameter exchange, avoiding the library's header-first probe.
func GitHub(clientID, clientSecret string) Provider {
	return Provider{
		Name: "github",
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://github.com/login/oauth/authorize",
			TokenURL:  "https://github.com/login/oauth/access_token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
		UserInfoURL:  "https://api.github.com/user",
		Scopes:       []string{"read:user", "user:email"},
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		kind:         kindGitHub,
	}
}

// Google returns the Google provider definition.
func Google(clientID, clientSecret string) Provider {
	return Provider{
		Name: "google",
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:  "https://oauth2.googleapis.com/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
		UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		Scopes:       []string{"openid", "email", "profile"},
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		kind:         kindGoogle,
	}
}

// Configured reports whether the operator supplied credentials. An
// unconfigured provider is hidden from the login surface.
func (p Provider) Configured() bool {
	return p.ClientID != "" && p.ClientSecret != ""
}

// oauthConfig builds the library config for one request. The redirect URL is
// request-scoped because the Console host is resolved per request.
func (p Provider) oauthConfig(redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       p.Scopes,
		Endpoint:     p.Endpoint,
	}
}

// AuthorizationURL builds the provider consent URL with a PKCE S256 challenge.
func (p Provider) AuthorizationURL(redirectURI, state, codeVerifier string) string {
	return p.oauthConfig(redirectURI).AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
}

// Identity is the provider account resolved from an access token. It is
// matched against account_identities by (provider, provider_user_id).
type Identity struct {
	ProviderUserID string
	Login          string
	Email          string
	// EmailVerified reports whether the provider asserts the email address is
	// confirmed. Auto-provisioning only links to an existing account by email
	// when the provider vouches for it.
	EmailVerified bool
	DisplayName   string
	AvatarURL     string
}

// Client performs the provider round trips. It is an interface so handlers can
// be tested against a fake without network access.
type Client interface {
	Exchange(ctx context.Context, p Provider, code, redirectURI, codeVerifier string) (*oauth2.Token, error)
	UserInfo(ctx context.Context, p Provider, token *oauth2.Token) (Identity, error)
}

// HTTPClient is the production Client.
type HTTPClient struct {
	http *http.Client
}

// NewClient builds a Client with a bounded timeout. When OAUTH_DEBUG is set,
// the transport logs token-exchange request forms with secrets redacted.
func NewClient(httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	if strings.TrimSpace(os.Getenv("OAUTH_DEBUG")) != "" {
		clone := *httpClient
		clone.Transport = DebugRoundTripper{Base: httpClient.Transport}
		httpClient = &clone
	}
	return &HTTPClient{http: httpClient}
}

// DebugRoundTripper logs token-exchange request forms (with client_secret
// redacted) when OAUTH_DEBUG is enabled. It exists to diagnose PKCE
// mismatches against a real provider without logging secrets.
type DebugRoundTripper struct {
	Base http.RoundTripper
}

func (d DebugRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	base := d.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if request.Body != nil && request.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		raw, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err == nil {
			_ = request.Body.Close()
			request.Body = io.NopCloser(bytes.NewReader(raw))
			values, _ := url.ParseQuery(string(raw))
			redacted := url.Values{}
			for key, items := range values {
				if key == "client_secret" || key == "code" {
					redacted.Set(key, "<redacted:"+strconv.Itoa(len(items[0]))+">")
					continue
				}
				for _, item := range items {
					redacted.Add(key, item)
				}
			}
			log.Printf("oauth debug request host=%s form=%s", request.URL.Host, redacted.Encode())
		}
	}
	return base.RoundTrip(request)
}

// NewState returns an unguessable CSRF state value.
func NewState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// NewPKCE returns an RFC 7636 S256 verifier/challenge pair. The verifier is
// generated here rather than with oauth2.GenerateVerifier so a random-source
// failure surfaces as an error instead of panicking inside a request handler.
func NewPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// S256Challenge computes the RFC 7636 S256 challenge for a verifier. The
// callback uses it to prove the cookie's verifier matches the challenge the
// authorize URL advertised before blaming the provider.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Exchange swaps an authorization code for a token using the library flow.
func (c *HTTPClient) Exchange(ctx context.Context, p Provider, code, redirectURI, codeVerifier string) (*oauth2.Token, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.http)
	return p.oauthConfig(redirectURI).Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
}

// UserInfo reads the provider identity using the token. The request goes
// through oauth2.NewClient so the bearer token and the injected HTTP client
// both apply.
func (c *HTTPClient) UserInfo(ctx context.Context, p Provider, token *oauth2.Token) (Identity, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.http)
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return Identity{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Identity{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return Identity{}, err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return Identity{}, fmt.Errorf("provider user-info returned HTTP %d", response.StatusCode)
	}
	switch p.kind {
	case kindGoogle:
		return parseGoogleUser(body)
	default:
		identity, err := parseGitHubUser(body)
		if err != nil {
			return Identity{}, err
		}
		// GitHub's /user email is the public profile address and may be empty
		// or unverified. /user/emails (allowed by the user:email scope) is the
		// authoritative source; a failure here degrades to "email unknown"
		// rather than failing the whole sign-in.
		if email, verified, ok := c.githubPrimaryEmail(ctx, client); ok {
			identity.Email = email
			identity.EmailVerified = verified
		}
		return identity, nil
	}
}

// githubPrimaryEmail reads the account's primary verified email. It returns
// ok=false when the list is unavailable, so callers never treat an unknown
// address as verified.
func (c *HTTPClient) githubPrimaryEmail(ctx context.Context, client *http.Client) (email string, verified bool, ok bool) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gitHubEmailsURL, nil)
	if err != nil {
		return "", false, false
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", false, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", false, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return "", false, false
	}
	var payload []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false, false
	}
	// Prefer the primary verified address; fall back to any verified address
	// so an account without a primary flag still resolves.
	for _, entry := range payload {
		if entry.Primary && entry.Verified && strings.TrimSpace(entry.Email) != "" {
			return entry.Email, true, true
		}
	}
	for _, entry := range payload {
		if entry.Verified && strings.TrimSpace(entry.Email) != "" {
			return entry.Email, true, true
		}
	}
	return "", false, false
}

func parseGitHubUser(body []byte) (Identity, error) {
	var payload struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, fmt.Errorf("decode GitHub user: %w", err)
	}
	if payload.ID == 0 {
		return Identity{}, errors.New("GitHub user response did not include an id")
	}
	return Identity{
		ProviderUserID: strconv.FormatInt(payload.ID, 10),
		Login:          payload.Login,
		Email:          payload.Email,
		DisplayName:    payload.Name,
		AvatarURL:      payload.AvatarURL,
	}, nil
}

func parseGoogleUser(body []byte) (Identity, error) {
	var payload struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, fmt.Errorf("decode Google user: %w", err)
	}
	if strings.TrimSpace(payload.Sub) == "" {
		return Identity{}, errors.New("Google user response did not include a subject")
	}
	return Identity{
		ProviderUserID: payload.Sub,
		Login:          payload.Email,
		Email:          payload.Email,
		EmailVerified:  payload.EmailVerified,
		DisplayName:    payload.Name,
		AvatarURL:      payload.Picture,
	}, nil
}
