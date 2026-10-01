package httpapi

import (
	"time"

	"github.com/Stealth-deplover/stealth/internal/setupstate"
)

type setupStatusResponse struct {
	SetupRequired bool                   `json:"setup_required"`
	Ready         bool                   `json:"ready"`
	State         setupstate.PublicState `json:"state"`
}

type setupCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Required bool   `json:"required"`
}

type setupPreflightResponse struct {
	Checks []setupCheck `json:"checks"`
}

type setupGitHubManifestResponse struct {
	ManifestURL string    `json:"manifest_url"`
	Manifest    string    `json:"manifest"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type setupGitHubAuthorizationResponse struct {
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type setupGitHubManualRequest struct {
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	PrivateKey    string `json:"private_key"`
	WebhookSecret string `json:"webhook_secret"`
}

type setupCloudflareOAuthResponse struct {
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type setupCloudflareTokenRequest struct {
	APIToken string `json:"api_token"`
}

type setupCloudflareTunnelRequest struct {
	AccountID string `json:"account_id"`
	ZoneID    string `json:"zone_id"`
	Hostname  string `json:"hostname"`
	Name      string `json:"name"`
}

type setupCloudflareStatusResponse struct {
	TunnelID    string `json:"tunnel_id"`
	Status      string `json:"status"`
	Healthy     bool   `json:"healthy"`
	Connections int    `json:"connections"`
}

type setupTestRequest struct {
	URL       string `json:"url"`
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	UseSSL    *bool  `json:"use_ssl"`
	PathStyle *bool  `json:"path_style"`
}

type setupQuickTunnelRequest struct {
	ContainerName string `json:"container_name"`
	URL           string `json:"url"`
}

type setupInstallResponse struct {
	Status string                 `json:"status"`
	State  setupstate.PublicState `json:"state"`
}

type setupHandoffStatusResponse struct {
	Pending bool `json:"pending"`
}

type setupInstallValidationError struct {
	err error
}

func (e *setupInstallValidationError) Error() string { return e.err.Error() }

func (e *setupInstallValidationError) Unwrap() error { return e.err }
