package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/secretkey"
)

type Config struct {
	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseMaxConnIdleTime time.Duration
	RedisURL                string
	HTTPAddress             string
	// PlatformSiteAddress is a private listener containing only the public
	// static Site-serving surface. Traefik platform routers never target the
	// control-plane HTTP listener.
	PlatformSiteAddress string
	MetricsToken        string
	// TrustedProxyCIDRs is empty by default. Forwarded client-IP headers are
	// only accepted when the direct peer belongs to one of these networks.
	TrustedProxyCIDRs []*net.IPNet
	// ACME terminates HTTPS for verified Site custom domains when enabled. The
	// default keeps certificate issuance off for local development; production
	// deployments must opt in explicitly and persist ACMECertCacheDir.
	ACMEEnabled              bool
	ACMEEmail                string
	ACMEDirectoryURL         string
	ACMETLSAddress           string
	ACMEHTTPChallengeAddress string
	ACMECertCacheDir         string
	SessionCookieName        string
	SessionTTL               time.Duration
	AppSessionTTL            time.Duration
	AuthVerificationTTL      time.Duration
	AuthPasswordResetTTL     time.Duration
	PublicAppURL             string
	// ConsoleCORSOrigins is the explicit allowlist for the browser-hosted
	// management console. Project application origins remain tenant-scoped and
	// are handled by httpapi's project CORS policy.
	ConsoleCORSOrigins         []string
	EmailDeliveryMode          string
	SMTPHost                   string
	SMTPPort                   int
	SMTPUsername               string
	SMTPPassword               string
	SMTPFrom                   string
	CookieSecure               bool
	AuthRateLimit              int
	AuthRateWindow             time.Duration
	ProjectOperationRateLimit  int
	ProjectOperationRateWindow time.Duration
	StorageRoot                string
	StorageMaxFileSize         int64
	StorageDefaultQuotaBytes   int64
	StorageDriver              string
	StorageS3Endpoint          string
	StorageS3Region            string
	StorageS3Bucket            string
	StorageS3AccessKey         string
	StorageS3SecretKey         string
	StorageS3UseSSL            bool
	StorageS3PathStyle         bool
	StorageS3Prefix            string
	StorageS3StagingRoot       string
	// Functions source archives use a separate child store under StorageRoot.
	// The global storage values are used as fallbacks for older deployments.
	FunctionsMaxArtifactSize   int64
	FunctionsDefaultQuotaBytes int64
	FunctionsSecretKey         []byte
	// AppsSecretKey is a dedicated operator key for App environment values.
	AppsSecretKey []byte
	// BootstrapCLIKey authenticates the local CLI when it asks the API to mint
	// a first-run setup session and encrypts short-lived GitHub authorization state.
	// It is a separate security domain from FunctionsSecretKey.
	BootstrapCLIKey                  []byte
	GitHubAppClientID                string
	FunctionsRunnerEnabled           bool
	FunctionsWorkerID                string
	FunctionsRunnerPoll              time.Duration
	FunctionsRunnerLeaseAge          time.Duration
	FunctionsRunnerBuildTimeout      time.Duration
	FunctionsRunnerStagingRoot       string
	FunctionsRunnerStagingVolume     string
	FunctionsRunnerMetricsAddress    string
	FunctionsRunnerHelperImage       string
	FunctionsRunnerNodeImage         string
	FunctionsRunnerPythonImage       string
	FunctionsRunnerGoImage           string
	AppsMaxSourceArchiveBytes        int64
	AppsMaxExpandedSourceBytes       int64
	AppsMaxSourceFiles               int
	AppsMaxImageArchiveBytes         int64
	AppsDefaultArtifactQuotaBytes    int64
	AppsBuildkitAddress              string
	AppsBuildkitCACert               string
	AppsBuildkitClientCert           string
	AppsBuildkitClientKey            string
	AppsBuildTimeout                 time.Duration
	AppsBuildLeaseAge                time.Duration
	AppsBuildPollInterval            time.Duration
	AppsBuildStagingRoot             string
	AppsBuildStagingVolume           string
	AppsBuildkitStateVolume          string
	AppsRuntimeNetworkName           string
	AppsRuntimePollInterval          time.Duration
	AppsRuntimeLeaseAge              time.Duration
	AppsRuntimeActionTimeout         time.Duration
	AppsRuntimeImageImportTimeout    time.Duration
	AppsRuntimeImageCacheMaxBytes    int64
	AppsRuntimeImageCacheTargetBytes int64
	AppsRuntimeImageGCSweepInterval  time.Duration
	// Agent runner settings control the trusted queue lifecycle. Provider
	// adapters remain a separate capability and an empty registry never claims
	// queued runs.
	AgentRunnerEnabled          bool
	AgentRunnerExecutionTimeout time.Duration
	// Sites accept pre-built static archives. The compressed upload limit is
	// separate from the expanded publication limit because quota accounting is
	// based on bytes that are actually served from the immutable directory.
	SitesMaxArtifactSize           int64
	SitesDefaultQuotaBytes         int64
	SitesMaxExpandedBytes          int64
	SitesMaxFiles                  int
	SitesGitFetchConcurrency       int
	TraefikGeneratedDir            string
	TraefikReloadFile              string
	PlatformRouteReconcileInterval time.Duration
	CloudflareReconcileInterval    time.Duration
	// OpenTelemetry tracing is disabled when the OTLP endpoint is empty. The
	// API and worker still create no-op spans in that mode, so instrumentation
	// does not need feature flags or test-only branches.
	TelemetryOTLPEndpoint string
	TelemetryServiceName  string
	TelemetrySampleRatio  float64
	// ClickHouse is an optional control-plane dependency. An empty address
	// keeps local and legacy installations fully functional while the admin
	// telemetry surface reports the backend as unavailable. When configured,
	// all query limits are enforced by the API and the ClickHouse session.
	TelemetryClickHouseAddr     string
	TelemetryClickHouseDatabase string
	TelemetryClickHouseUser     string
	TelemetryClickHousePassword string
	TelemetryCollectorHealthURL string
	TelemetryMaxQueryDuration   time.Duration
	TelemetryMaxQueryRange      time.Duration
	TelemetryMaxQueryRows       int
	TelemetryRetention          time.Duration
	// AgentProviderCatalog contains non-secret provider/model metadata for the
	// Console. Agent execution remains queue-only until a trusted provider
	// worker is deployed.
	AgentProviderCatalog []AgentProviderCatalogItem
	// SetupMode exposes only the short-lived browser installer routes. The host
	// CLI owns Docker and production installation; setup-mode API containers do
	// not receive Docker authority.
	SetupMode                   bool
	InstallRoot                 string
	SetupStateFile              string
	CloudflareImportFile        string
	SetupHandoffFile            string
	ProductionComposeFile       string
	SetupComposeFile            string
	CloudflareOAuthClientID     string
	CloudflareOAuthClientSecret string
	CloudflareAPIBaseURL        string
}

func Load() (Config, error) {
	databaseSettings, err := loadDatabaseSettings()
	if err != nil {
		return Config{}, err
	}
	transportSettings, err := loadTransportSettings()
	if err != nil {
		return Config{}, err
	}
	authSettings, err := loadAuthSettings()
	if err != nil {
		return Config{}, err
	}
	siteSettings, err := loadSiteSettings()
	if err != nil {
		return Config{}, err
	}
	ingressSettings, err := loadIngressSettings()
	if err != nil {
		return Config{}, err
	}
	executionSettings, err := loadExecutionSettings()
	if err != nil {
		return Config{}, err
	}
	appBuildSettings, err := loadAppBuildSettings()
	if err != nil {
		return Config{}, err
	}
	telemetrySettings, err := loadTelemetrySettings()
	if err != nil {
		return Config{}, err
	}
	telemetryStoreSettings, err := loadTelemetryStoreSettings()
	if err != nil {
		return Config{}, err
	}
	agentSettings, err := loadAgentSettings()
	if err != nil {
		return Config{}, err
	}
	secretSettings, err := loadSecretSettings()
	if err != nil {
		return Config{}, err
	}
	storageSettings, err := loadStorageSettings()
	if err != nil {
		return Config{}, err
	}
	tlsSettings, err := loadTLSSettings(storageSettings.root, transportSettings.httpAddress)
	if err != nil {
		return Config{}, err
	}
	setupSettings, err := loadSetupSettings()
	if err != nil {
		return Config{}, err
	}
	config := Config{}
	databaseSettings.apply(&config)
	authSettings.apply(&config)
	siteSettings.apply(&config)
	ingressSettings.apply(&config)
	executionSettings.apply(&config)
	appBuildSettings.apply(&config)
	telemetrySettings.apply(&config)
	telemetryStoreSettings.apply(&config)
	agentSettings.apply(&config)
	secretSettings.apply(&config)
	transportSettings.apply(&config)
	storageSettings.apply(&config)
	tlsSettings.apply(&config)
	setupSettings.apply(&config)
	config.FunctionsRunnerStagingRoot, err = filepath.Abs(config.FunctionsRunnerStagingRoot)
	if err != nil || strings.TrimSpace(config.FunctionsRunnerStagingRoot) == "" ||
		filepath.Clean(config.FunctionsRunnerStagingRoot) == string(filepath.Separator) {
		return Config{}, fmt.Errorf("FUNCTIONS_RUNNER_STAGING_ROOT must be a valid non-root filesystem path")
	}
	config.AppsBuildStagingRoot, err = filepath.Abs(config.AppsBuildStagingRoot)
	if err != nil || strings.TrimSpace(config.AppsBuildStagingRoot) == "" {
		return Config{}, fmt.Errorf("APPS_BUILD_STAGING_ROOT must be a valid filesystem path")
	}
	return config, nil
}

// boundedInt32 parses an operator configuration value with the exact width
// used by the downstream database driver. Parsing at 32 bits and checking the
// bounds before returning makes the narrowing conversion explicit and safe.
func boundedInt32(name, fallback string, minimum, maximum int32) (int32, error) {
	if minimum > maximum {
		return 0, fmt.Errorf("%s has invalid bounds", name)
	}
	parsed, err := strconv.ParseInt(value(name, fallback), 10, 32)
	if err != nil || parsed < int64(minimum) || parsed > int64(maximum) {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return int32(parsed), nil
}

func decodeSecretKey(raw, name string) ([]byte, error) {
	key, err := secretkey.Decode32ByteKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be base64-encoded 32 bytes", name)
	}
	return key, nil
}

// ValidateBootstrap enforces the production credential gate for the
// first-owner flow. Tests and embedded handlers may construct a Config with
// zero values, but the API composition root must not serve a GitHub bootstrap
// without both dedicated pieces of configuration.
