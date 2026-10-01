package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (c Config) ValidateBootstrap() error {
	if len(c.BootstrapCLIKey) != 32 {
		return fmt.Errorf("BOOTSTRAP_CLI_KEY must be configured as base64-encoded 32 bytes")
	}
	if !c.SetupMode && !validGitHubAppClientID(c.GitHubAppClientID) {
		return fmt.Errorf("GITHUB_APP_CLIENT_ID must be configured")
	}
	return nil
}

func (c Config) ValidateSetup() error {
	if !c.SetupMode {
		return nil
	}
	if strings.TrimSpace(c.SetupStateFile) == "" || !filepath.IsAbs(c.SetupStateFile) || filepath.Clean(c.SetupStateFile) == string(filepath.Separator) {
		return fmt.Errorf("STEALTH_SETUP_STATE_FILE must be a valid non-root absolute path in setup mode")
	}
	return nil
}

func validGitHubAppClientID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 160 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

// ValidateSites keeps production deployments from silently accepting a
// malformed static publication configuration. NewWithLimiter still supplies
// safe defaults for hand-built test Config values.
func (c Config) ValidateSites() error {
	if c.SitesMaxArtifactSize <= 0 || c.SitesMaxExpandedBytes <= 0 || c.SitesDefaultQuotaBytes <= 0 || c.SitesMaxFiles < 1 || c.SitesMaxFiles > 100000 || c.SitesGitFetchConcurrency < 0 || c.SitesGitFetchConcurrency > 32 {
		return fmt.Errorf("site artifact, expanded-size, file-count, and quota settings are invalid")
	}
	if c.SitesMaxExpandedBytes > c.SitesDefaultQuotaBytes {
		return fmt.Errorf("SITES_MAX_EXPANDED_BYTES cannot exceed SITES_DEFAULT_QUOTA_BYTES")
	}
	if c.ACMEEnabled {
		if !isACMEEmail(strings.TrimSpace(c.ACMEEmail)) {
			return fmt.Errorf("ACME_EMAIL must be a valid email address when ACME_ENABLED is true")
		}
		if !isACMEDirectoryURL(c.ACMEDirectoryURL) {
			return fmt.Errorf("ACME_DIRECTORY_URL must be an absolute HTTPS URL without credentials, query, or fragment")
		}
		if !isListenAddress(c.ACMETLSAddress) || !isListenAddress(c.ACMEHTTPChallengeAddress) {
			return fmt.Errorf("ACME listener addresses are invalid")
		}
		if strings.TrimSpace(c.ACMETLSAddress) == strings.TrimSpace(c.ACMEHTTPChallengeAddress) {
			return fmt.Errorf("ACME_TLS_ADDR and ACME_HTTP_CHALLENGE_ADDR must be different listeners")
		}
		if sameListenPort(c.ACMETLSAddress, c.HTTPAddress) || sameListenPort(c.ACMEHTTPChallengeAddress, c.HTTPAddress) {
			return fmt.Errorf("ACME listeners must not reuse the HTTP_ADDR port")
		}
		if strings.TrimSpace(c.ACMECertCacheDir) == "" || !filepath.IsAbs(c.ACMECertCacheDir) || filepath.Clean(c.ACMECertCacheDir) == string(filepath.Separator) {
			return fmt.Errorf("ACME_CERT_CACHE_DIR must be a valid non-root filesystem path")
		}
	}
	return nil
}

func (c Config) ValidateApps() error {
	if c.AppsMaxSourceArchiveBytes <= 0 || c.AppsMaxSourceArchiveBytes > 2<<30 ||
		c.AppsMaxExpandedSourceBytes <= 0 || c.AppsMaxExpandedSourceBytes > 16<<30 ||
		c.AppsMaxSourceFiles < 1 || c.AppsMaxSourceFiles > 1000000 ||
		c.AppsMaxImageArchiveBytes <= 0 || c.AppsMaxImageArchiveBytes > 16<<30 ||
		c.AppsDefaultArtifactQuotaBytes < c.AppsMaxSourceArchiveBytes || c.AppsDefaultArtifactQuotaBytes > 1<<40 {
		return fmt.Errorf("App source, image, file-count, and artifact quota settings are invalid")
	}
	if !validBuildkitAddress(c.AppsBuildkitAddress) {
		return fmt.Errorf("APPS_BUILDKIT_ADDRESS must be a private TCP host:port address")
	}
	if !validBuildKitPath(c.AppsBuildkitCACert) || !validBuildKitPath(c.AppsBuildkitClientCert) || !validBuildKitPath(c.AppsBuildkitClientKey) {
		return fmt.Errorf("App BuildKit TLS certificate paths must be absolute, clean, non-root paths")
	}
	if c.AppsBuildTimeout < time.Minute || c.AppsBuildTimeout > 24*time.Hour || c.AppsBuildLeaseAge < c.AppsBuildTimeout || c.AppsBuildLeaseAge > 48*time.Hour || c.AppsBuildPollInterval < 100*time.Millisecond || c.AppsBuildPollInterval > time.Minute {
		return fmt.Errorf("App build timeout, lease, or polling settings are invalid")
	}
	if strings.TrimSpace(c.AppsBuildStagingRoot) == "" || !filepath.IsAbs(c.AppsBuildStagingRoot) || filepath.Clean(c.AppsBuildStagingRoot) == string(filepath.Separator) {
		return fmt.Errorf("APPS_BUILD_STAGING_ROOT must be an absolute non-root path")
	}
	if !isDockerName(c.AppsBuildStagingVolume) || !isDockerName(c.AppsBuildkitStateVolume) {
		return fmt.Errorf("App build volume names are invalid")
	}
	if len(c.AppsRuntimeNetworkName) > 63 || !isDockerName(c.AppsRuntimeNetworkName) ||
		c.AppsRuntimePollInterval < 100*time.Millisecond || c.AppsRuntimePollInterval > time.Minute ||
		c.AppsRuntimeLeaseAge < 30*time.Second || c.AppsRuntimeLeaseAge > 10*time.Minute ||
		c.AppsRuntimeActionTimeout < 5*time.Second || c.AppsRuntimeActionTimeout > 2*time.Minute ||
		c.AppsRuntimeImageImportTimeout < time.Minute || c.AppsRuntimeImageImportTimeout > 30*time.Minute ||
		c.AppsRuntimeImageCacheMaxBytes < 1<<20 || c.AppsRuntimeImageCacheMaxBytes > 1<<40 ||
		c.AppsRuntimeImageCacheTargetBytes < 1<<20 || c.AppsRuntimeImageCacheTargetBytes >= c.AppsRuntimeImageCacheMaxBytes ||
		c.AppsRuntimeImageGCSweepInterval < time.Minute || c.AppsRuntimeImageGCSweepInterval > 24*time.Hour {
		return fmt.Errorf("App runtime network, poll, lease, or Docker timeout settings are invalid")
	}
	return nil
}

// ValidateAppSecrets is a production startup gate for the dedicated key used
// to encrypt and decrypt persisted App environment values.
func (c Config) ValidateAppSecrets() error {
	if len(c.AppsSecretKey) != 32 {
		return fmt.Errorf("APPS_SECRET_KEY must be configured as base64-encoded 32 bytes")
	}
	return nil
}
