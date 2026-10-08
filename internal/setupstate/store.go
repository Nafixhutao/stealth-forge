// Package setupstate contains the durable, server-owned state for the
// pre-production browser setup flow. It intentionally separates public draft
// choices from encrypted provider credentials.
package setupstate

import (
	"time"
)

const (
	PhaseCollecting       = "collecting"
	PhaseInstallRequested = "install_requested"
	PhaseInstalling       = "installing"
	PhaseComplete         = "complete"
	PhaseFailed           = "failed"
	PhaseHandoff          = "handoff"
	stateVersion          = 2
	legacyStateVersion    = 1
	sharedStateFileMode   = 0o660
)

// Draft contains setup choices that can safely be represented by the public
// setup-state projection. Credential material belongs in SetupCredentials.
type Draft struct {
	InstanceName       string `json:"instance_name,omitempty"`
	PublicURL          string `json:"public_url,omitempty"`
	NetworkMode        string `json:"network_mode,omitempty"`
	Hostname           string `json:"hostname,omitempty"`
	DatabaseMode       string `json:"database_mode,omitempty"`
	DatabaseTested     bool   `json:"database_tested,omitempty"`
	RedisMode          string `json:"redis_mode,omitempty"`
	RedisTested        bool   `json:"redis_tested,omitempty"`
	StorageMode        string `json:"storage_mode,omitempty"`
	StorageTested      bool   `json:"storage_tested,omitempty"`
	StorageS3Endpoint  string `json:"storage_s3_endpoint,omitempty"`
	StorageS3Region    string `json:"storage_s3_region,omitempty"`
	StorageS3Bucket    string `json:"storage_s3_bucket,omitempty"`
	StorageS3UseSSL    bool   `json:"storage_s3_use_ssl"`
	StorageS3PathStyle bool   `json:"storage_s3_path_style"`
	StorageS3Prefix    string `json:"storage_s3_prefix,omitempty"`
	// External sign-in client IDs are public identifiers, so they live in the
	// draft; their secrets belong in SetupCredentials.
	OAuthGitHubClientID string `json:"oauth_github_client_id,omitempty"`
	OAuthGoogleClientID string `json:"oauth_google_client_id,omitempty"`
}

// SetupCredentials is the single in-memory view of setup credentials. The
// FileStore persists these values only through State.Secrets.
type SetupCredentials struct {
	DatabaseURL             string
	RedisURL                string
	StorageS3AccessKey      string
	StorageS3SecretKey      string
	OAuthGitHubClientSecret string
	OAuthGoogleClientSecret string
}

type GitHubState struct {
	Mode                   string    `json:"mode,omitempty"`
	ClientID               string    `json:"client_id,omitempty"`
	ManifestStateHash      string    `json:"manifest_state_hash,omitempty"`
	ManifestExpiresAt      time.Time `json:"manifest_expires_at,omitempty"`
	AuthorizationStateHash string    `json:"authorization_state_hash,omitempty"`
	AuthorizationExpiresAt time.Time `json:"authorization_expires_at,omitempty"`
	Connected              bool      `json:"connected,omitempty"`
	AuthorizationSession   string    `json:"authorization_session,omitempty"`
}

type CloudflareState struct {
	Mode           string            `json:"mode,omitempty"`
	Connected      bool              `json:"connected,omitempty"`
	ExpiresAt      time.Time         `json:"expires_at,omitempty"`
	TokenValid     bool              `json:"token_valid,omitempty"`
	Binding        CloudflareBinding `json:"binding,omitempty"`
	OAuthStateHash string            `json:"oauth_state_hash,omitempty"`
	OAuthExpiresAt time.Time         `json:"oauth_expires_at,omitempty"`
}

// HostPreflightCheck is a safe projection of a check performed by the host
// CLI. It deliberately contains only display data; the setup API never runs
// host Docker or resource probes itself.
type HostPreflightCheck struct {
	Name     string `json:"name"`
	Detail   string `json:"detail"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
}

// State is encrypted in its entirety when persisted. Secrets are available to
// the setup module through methods, but the HTTP adapter only serializes the
// public projection below.
type State struct {
	Version        int                  `json:"version"`
	Phase          string               `json:"phase"`
	Step           string               `json:"step,omitempty"`
	ErrorCode      string               `json:"error_code,omitempty"`
	ErrorMessage   string               `json:"error_message,omitempty"`
	Draft          Draft                `json:"draft"`
	GitHub         GitHubState          `json:"github"`
	Cloudflare     CloudflareState      `json:"cloudflare"`
	Secrets        map[string]string    `json:"secrets,omitempty"`
	UpdatedAt      time.Time            `json:"updated_at"`
	QuickTunnel    string               `json:"quick_tunnel,omitempty"`
	SetupSessionID string               `json:"setup_session_id,omitempty"`
	SetupCodeHash  string               `json:"setup_code_hash,omitempty"`
	SetupExpiresAt time.Time            `json:"setup_expires_at,omitempty"`
	InstallRunID   string               `json:"install_run_id,omitempty"`
	LastEventID    uint64               `json:"last_event_id,omitempty"`
	HostPreflight  []HostPreflightCheck `json:"host_preflight,omitempty"`
}

const (
	setupDatabaseURLSecret        = "database_url"
	setupRedisURLSecret           = "redis_url"
	setupStorageS3AccessKeySecret = "storage_s3_access_key"
	setupStorageS3SecretKeySecret = "storage_s3_secret_key"
	setupOAuthGitHubSecret        = "oauth_github_client_secret"
	setupOAuthGoogleSecret        = "oauth_google_client_secret"
)

func (s State) SetupCredentials() SetupCredentials {
	return SetupCredentials{
		DatabaseURL:             s.Secret(setupDatabaseURLSecret),
		RedisURL:                s.Secret(setupRedisURLSecret),
		StorageS3AccessKey:      s.Secret(setupStorageS3AccessKeySecret),
		StorageS3SecretKey:      s.Secret(setupStorageS3SecretKeySecret),
		OAuthGitHubClientSecret: s.Secret(setupOAuthGitHubSecret),
		OAuthGoogleClientSecret: s.Secret(setupOAuthGoogleSecret),
	}
}

func (s *State) SetSetupCredentials(credentials SetupCredentials) {
	s.SetSecret(setupDatabaseURLSecret, credentials.DatabaseURL)
	s.SetSecret(setupRedisURLSecret, credentials.RedisURL)
	s.SetSecret(setupStorageS3AccessKeySecret, credentials.StorageS3AccessKey)
	s.SetSecret(setupStorageS3SecretKeySecret, credentials.StorageS3SecretKey)
	s.SetSecret(setupOAuthGitHubSecret, credentials.OAuthGitHubClientSecret)
	s.SetSecret(setupOAuthGoogleSecret, credentials.OAuthGoogleClientSecret)
}

type PublicState struct {
	Version      int              `json:"version"`
	Phase        string           `json:"phase"`
	Step         string           `json:"step,omitempty"`
	ErrorCode    string           `json:"error_code,omitempty"`
	ErrorMessage string           `json:"error_message,omitempty"`
	Draft        PublicDraft      `json:"draft"`
	GitHub       PublicGitHub     `json:"github"`
	Cloudflare   PublicCloudflare `json:"cloudflare"`
	UpdatedAt    time.Time        `json:"updated_at"`
	LastEventID  uint64           `json:"last_event_id,omitempty"`
}

type PublicDraft struct {
	InstanceName         string `json:"instance_name,omitempty"`
	PublicURL            string `json:"public_url,omitempty"`
	NetworkMode          string `json:"network_mode,omitempty"`
	Hostname             string `json:"hostname,omitempty"`
	CloudflareAccountID  string `json:"cloudflare_account_id,omitempty"`
	CloudflareZoneID     string `json:"cloudflare_zone_id,omitempty"`
	CloudflareTunnelID   string `json:"cloudflare_tunnel_id,omitempty"`
	CloudflareTunnelName string `json:"cloudflare_tunnel_name,omitempty"`
	CloudflareRecordID   string `json:"cloudflare_record_id,omitempty"`
	DatabaseMode         string `json:"database_mode,omitempty"`
	DatabaseTested       bool   `json:"database_tested,omitempty"`
	RedisMode            string `json:"redis_mode,omitempty"`
	RedisTested          bool   `json:"redis_tested,omitempty"`
	StorageMode          string `json:"storage_mode,omitempty"`
	StorageTested        bool   `json:"storage_tested,omitempty"`
	StorageS3Endpoint    string `json:"storage_s3_endpoint,omitempty"`
	StorageS3Region      string `json:"storage_s3_region,omitempty"`
	StorageS3Bucket      string `json:"storage_s3_bucket,omitempty"`
	StorageS3UseSSL      bool   `json:"storage_s3_use_ssl"`
	StorageS3PathStyle   bool   `json:"storage_s3_path_style"`
	StorageS3Prefix      string `json:"storage_s3_prefix,omitempty"`
	OAuthGitHubClientID  string `json:"oauth_github_client_id,omitempty"`
	OAuthGoogleClientID  string `json:"oauth_google_client_id,omitempty"`
}

type PublicGitHub struct {
	Mode                 string    `json:"mode,omitempty"`
	ClientID             string    `json:"client_id,omitempty"`
	Connected            bool      `json:"connected,omitempty"`
	AuthorizationSession string    `json:"authorization_session,omitempty"`
	ManifestExpiresAt    time.Time `json:"manifest_expires_at,omitempty"`
}

type PublicCloudflare struct {
	Mode       string    `json:"mode,omitempty"`
	Connected  bool      `json:"connected,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	TokenValid bool      `json:"token_valid,omitempty"`
}
