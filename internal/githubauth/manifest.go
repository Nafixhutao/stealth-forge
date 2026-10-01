package githubauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultAPIURL = "https://api.github.com"

type AppManifest struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	URL           string            `json:"url"`
	RedirectURL   string            `json:"redirect_url"`
	CallbackURLs  []string          `json:"callback_urls,omitempty"`
	Public        bool              `json:"public"`
	DefaultEvents []string          `json:"default_events"`
	DefaultPerms  map[string]string `json:"default_permissions"`
	SetupURL      string            `json:"setup_url,omitempty"`
}

type AppCredentials struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	PrivateKey    string `json:"pem"`
	WebhookSecret string `json:"webhook_secret"`
}

type ManifestClient interface {
	ConvertManifest(context.Context, string) (AppCredentials, error)
}

type HTTPManifestClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewManifestClient(baseURL string, httpClient *http.Client) (*HTTPManifestClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("GitHub API base URL is invalid")
	}
	schemeInvalid := parsed.Scheme != "http" && parsed.Scheme != "https"
	hostInvalid := parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != ""
	if schemeInvalid || hostInvalid {
		return nil, errors.New("GitHub API base URL is invalid")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &HTTPManifestClient{baseURL: baseURL, httpClient: httpClient}, nil
}

func (c *HTTPManifestClient) ConvertManifest(ctx context.Context, code string) (AppCredentials, error) {
	if c == nil || c.httpClient == nil {
		return AppCredentials{}, errors.New("GitHub manifest client is not configured")
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 4096 || strings.ContainsAny(code, "\x00\r\n") {
		return AppCredentials{}, errors.New("GitHub manifest code is invalid")
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/app-manifests/"+url.PathEscape(code)+"/conversions",
		bytes.NewReader(nil),
	)
	if err != nil {
		return AppCredentials{}, fmt.Errorf("create GitHub manifest request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return AppCredentials{}, fmt.Errorf("GitHub manifest request failed: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return AppCredentials{}, fmt.Errorf("read GitHub manifest response: %w", err)
	}
	if response.StatusCode != http.StatusCreated {
		return AppCredentials{}, fmt.Errorf("GitHub manifest request was rejected (HTTP %d)", response.StatusCode)
	}
	var credentials AppCredentials
	if err := json.Unmarshal(contents, &credentials); err != nil {
		return AppCredentials{}, errors.New("GitHub returned an invalid app manifest response")
	}
	invalidIdentity := credentials.ID <= 0 || !validClientID(credentials.ClientID)
	secretMissing := strings.TrimSpace(credentials.ClientSecret) == ""
	privateKeyInvalid := !strings.Contains(credentials.PrivateKey, "BEGIN")
	secretTooLong := len(credentials.ClientSecret) > 512
	privateKeyTooLong := len(credentials.PrivateKey) > 32768
	webhookSecretTooLong := len(credentials.WebhookSecret) > 512
	if invalidIdentity || secretMissing || privateKeyInvalid || secretTooLong || privateKeyTooLong ||
		webhookSecretTooLong {
		return AppCredentials{}, errors.New("GitHub returned incomplete app credentials")
	}
	return credentials, nil
}

func ManifestURL(manifest AppManifest, state string) (string, error) {
	contents, err := validateAndEncodeManifest(manifest, state)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("manifest", string(contents))
	values.Set("redirect_url", manifest.RedirectURL)
	values.Set("state", state)
	return "https://github.com/settings/apps/new?" + values.Encode(), nil
}

// ManifestForm returns the action URL and JSON payload for GitHub's App
// Manifest registration form. GitHub documents this flow as a POST with the
// JSON manifest in a form field; the payload contains no provider secret and
// is intended to be submitted by the browser.
func ManifestForm(manifest AppManifest, state string) (string, string, error) {
	contents, err := validateAndEncodeManifest(manifest, state)
	if err != nil {
		return "", "", err
	}
	values := url.Values{}
	values.Set("state", state)
	return "https://github.com/settings/apps/new?" + values.Encode(), string(contents), nil
}

func validateAndEncodeManifest(manifest AppManifest, state string) ([]byte, error) {
	redirectURLInvalid := !validRedirectURL(manifest.RedirectURL)
	appURLInvalid := !validRedirectURL(manifest.URL)
	setupURLInvalid := manifest.SetupURL != "" && !validRedirectURL(manifest.SetupURL)
	tooManyCallbacks := len(manifest.CallbackURLs) > 10
	nameMissing := strings.TrimSpace(manifest.Name) == ""
	nameTooLong := len(manifest.Name) > 34
	stateMissing := strings.TrimSpace(state) == ""
	stateTooLong := len(state) > 512
	stateMalformed := strings.ContainsAny(state, "\x00\r\n")
	if redirectURLInvalid || appURLInvalid || setupURLInvalid || tooManyCallbacks || nameMissing || nameTooLong ||
		stateMissing ||
		stateTooLong ||
		stateMalformed {
		return nil, errors.New("GitHub manifest settings are invalid")
	}
	for _, callbackURL := range manifest.CallbackURLs {
		if !validRedirectURL(callbackURL) {
			return nil, errors.New("GitHub manifest settings are invalid")
		}
	}
	if len(manifest.Description) > 512 || strings.ContainsAny(manifest.Name+manifest.Description, "\x00\r\n") {
		return nil, errors.New("GitHub manifest settings are invalid")
	}
	contents, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode GitHub app manifest: %w", err)
	}
	return contents, nil
}

// DefaultAppName returns a short, editable GitHub App name that avoids
// collisions when several installations are bootstrapped at the same time.
// GitHub presents this value in the Manifest registration form and lets the
// registering user change it before creating the App.
func DefaultAppName() (string, error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate GitHub App name: %w", err)
	}
	return "Stealth Setup " + strings.ToUpper(hex.EncodeToString(suffix)), nil
}

func validRedirectURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" &&
		parsed.Fragment == ""
}

func validClientID(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 160 {
		return false
	}
	for _, character := range raw {
		isLower := character >= 'a' && character <= 'z'
		isUpper := character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		isAllowedSymbol := strings.ContainsRune(".-_", character)
		if isLower || isUpper || isDigit || isAllowedSymbol {
			continue
		}
		return false
	}
	return true
}

// ValidClientID validates the non-secret identifier returned by GitHub App
// registration or entered through the documented manual fallback.
func ValidClientID(raw string) bool {
	return validClientID(raw)
}
