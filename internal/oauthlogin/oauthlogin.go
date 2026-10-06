// Package oauthlogin implements the browser OAuth login flow for external
// identity providers (GitHub, Google). It is deliberately separate from the
// first-owner bootstrap flow in internal/githubauth: login only resolves an
// already-linked account_identities row and never creates an account.
package oauthlogin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// requestTimeout bounds a single provider round trip.
	requestTimeout = 15 * time.Second
	// maxBodyBytes bounds provider responses so a hostile endpoint cannot
	// stream unbounded data into memory.
	maxBodyBytes = 1 << 20
)

// Provider kinds select the user-info parsing shape. The authorize/token
// endpoints are configured per provider so both flows share one code path.
type kind int

const (
	kindGitHub kind = iota
	kindGoogle
)

// Provider is one configured identity provider.
type Provider struct {
	Name         string
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
	ClientID     string
	ClientSecret string
	// grantType is sent on the token exchange. Google requires it; GitHub
	// rejects unknown values on some endpoints, so it stays empty there.
	grantType string
	kind      kind
}

// GitHub returns the GitHub provider definition.
func GitHub(clientID, clientSecret string) Provider {
	return Provider{
		Name:         "github",
		AuthorizeURL: "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
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
		Name:         "google",
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		Scopes:       []string{"openid", "email", "profile"},
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		grantType:    "authorization_code",
		kind:         kindGoogle,
	}
}

// Configured reports whether the operator supplied credentials. An
// unconfigured provider is hidden from the login surface.
func (p Provider) Configured() bool {
	return p.ClientID != "" && p.ClientSecret != ""
}

// Identity is the provider account resolved from an access token. It is
// matched against account_identities by (provider, provider_user_id).
type Identity struct {
	ProviderUserID string
	Login          string
	Email          string
	DisplayName    string
	AvatarURL      string
}

// Client performs the provider round trips. It is an interface so handlers can
// be tested against a fake without network access.
type Client interface {
	Exchange(ctx context.Context, p Provider, code, redirectURI, codeVerifier string) (string, error)
	UserInfo(ctx context.Context, p Provider, accessToken string) (Identity, error)
}

// HTTPClient is the production Client.
type HTTPClient struct {
	http *http.Client
}

// NewClient builds a Client with a bounded timeout.
func NewClient(httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	return &HTTPClient{http: httpClient}
}

// NewState returns an unguessable CSRF state value.
func NewState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// NewPKCE returns an RFC 7636 S256 verifier/challenge pair.
func NewPKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// AuthorizationURL builds the provider authorization redirect.
func (p Provider) AuthorizationURL(redirectURI, state, codeChallenge string) string {
	query := url.Values{}
	query.Set("client_id", p.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(p.Scopes, " "))
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	return p.AuthorizeURL + "?" + query.Encode()
}

// Exchange swaps an authorization code for an access token.
func (c *HTTPClient) Exchange(ctx context.Context, p Provider, code, redirectURI, codeVerifier string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	if p.grantType != "" {
		form.Set("grant_type", p.grantType)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	body, err := c.do(request)
	if err != nil {
		return "", err
	}
	var payload struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if payload.Error != "" {
		return "", fmt.Errorf("provider rejected the token exchange: %s", payload.Error)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("provider returned an empty access token")
	}
	return payload.AccessToken, nil
}

// UserInfo reads the provider identity for an access token.
func (c *HTTPClient) UserInfo(ctx context.Context, p Provider, accessToken string) (Identity, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return Identity{}, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	body, err := c.do(request)
	if err != nil {
		return Identity{}, err
	}
	switch p.kind {
	case kindGoogle:
		return parseGoogleUser(body)
	default:
		return parseGitHubUser(body)
	}
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
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
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
		DisplayName:    payload.Name,
		AvatarURL:      payload.Picture,
	}, nil
}

func (c *HTTPClient) do(request *http.Request) ([]byte, error) {
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("provider returned HTTP %d", response.StatusCode)
	}
	return body, nil
}
