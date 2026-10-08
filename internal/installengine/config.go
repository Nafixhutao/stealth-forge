package installengine

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	releaseVersionPattern       = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-rc\.[0-9]+)?$`)
	stableReleaseVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
)

// Traefik is a release-managed third-party runtime dependency. Keep the
// version and multi-architecture manifest digest in one place so fresh
// installs and upgrades converge on the same immutable image.
const defaultTraefikImage = "traefik:v3.7.13@sha256:1c32e7c368204fd72812152ebdd2ac0425993df6fd982317deb02e48f2d5423c"

const defaultBuildKitImage = "moby/buildkit:v0.33.0-rootless@sha256:80b15f0735e87bab7bf59ec4d695dfb4a7cfb25521cf56dc75d6f256285b63ef"

const (
	BuildKitAppArmorProfileName = "stealth-buildkit-rootless"
	BuildKitAppArmorProfilePath = "/etc/apparmor.d/stealth-buildkit-rootless"
	appArmorProfileManagedMark  = "# Managed by Stealth. Changes will be replaced by the installer."
)

// ConfigOptions describes the non-secret choices made before the production
// stack is started. The engine creates all initial credentials in one place so
// a retry never needs to invent a second secret set.
type ConfigOptions struct {
	Version                     string
	PublicURL                   string
	GitHubAppClientID           string
	DockerGID                   uint32
	Setup                       bool
	InstallRoot                 string
	ComposeProject              string
	NetworkSubnet               string
	TrustedProxyCIDRs           string
	IngressNetworkName          string
	IngressNetworkSubnet        string
	IngressIPRange              string
	TraefikIngressIP            string
	CloudflaredIngressIP        string
	APIImage                    string
	SetupImage                  string
	StorageDriver               string
	DatabaseURL                 string
	RedisURL                    string
	AppsBuildKitAppArmorProfile string
}

func GenerateConfig(options ConfigOptions) (string, error) {
	if err := validateVersion(options.Version); err != nil {
		return "", err
	}
	publicURL, err := validatePublicURL(options.PublicURL)
	if err != nil {
		return "", err
	}
	if !options.Setup && !validClientID(options.GitHubAppClientID) {
		return "", errorsf("GitHub App Client ID is required for production configuration")
	}
	if options.Setup && strings.TrimSpace(options.InstallRoot) == "" {
		return "", errorsf("installation root is required for setup configuration")
	}
	if strings.ContainsAny(options.InstallRoot, "\x00\r\n") {
		return "", errorsf("installation root is invalid")
	}
	project := firstNonEmpty(options.ComposeProject, "stealth")
	subnet := firstNonEmpty(options.NetworkSubnet, "172.30.0.0/24")
	ingress, err := ingressNetworkConfigFromOptions(options)
	if err != nil {
		return "", err
	}
	trustedProxy := ensureTraefikTrustedProxyCIDR(options.TrustedProxyCIDRs, subnet, ingress.trustedProxyCIDR())
	storageDriver := strings.ToLower(firstNonEmpty(options.StorageDriver, "local"))
	if storageDriver != "local" && storageDriver != "s3" {
		return "", errorsf("storage driver must be local or s3")
	}
	appArmorProfile := strings.TrimSpace(options.AppsBuildKitAppArmorProfile)
	if appArmorProfile == "" {
		appArmorProfile = "unconfined"
	}
	if !validBuildKitAppArmorProfile(appArmorProfile) {
		return "", errorsf("App BuildKit AppArmor profile must be unconfined or " + BuildKitAppArmorProfileName)
	}
	postgresPassword, err := randomHex(24)
	if err != nil {
		return "", err
	}
	redisPassword, err := randomHex(24)
	if err != nil {
		return "", err
	}
	functionsKey, err := randomBase64(32)
	if err != nil {
		return "", err
	}
	appsKey, err := randomBase64(32)
	if err != nil {
		return "", err
	}
	bootstrapKey, err := randomBase64(32)
	if err != nil {
		return "", err
	}
	metricsToken, err := randomHex(32)
	if err != nil {
		return "", err
	}
	apiImage := options.APIImage
	if apiImage == "" {
		apiImage = ImageName("stealth-api", options.Version)
	}
	setupImage := options.SetupImage
	if setupImage == "" {
		setupImage = ImageName("stealth-setup", options.Version)
	}
	databaseURL := options.DatabaseURL
	if databaseURL == "" {
		databaseURL = "postgres://stealth:" + postgresPassword + "@postgres:5432/stealth?sslmode=disable"
	}
	redisURL := options.RedisURL
	if redisURL == "" {
		redisURL = "redis://:" + redisPassword + "@redis:6379/0"
	}
	cookieSecure := "false"
	if strings.EqualFold(mustURLScheme(publicURL), "https") {
		cookieSecure = "true"
	}
	values := map[string]string{
		"COMPOSE_PROJECT_NAME":                  project,
		"STEALTH_API_IMAGE":                     apiImage,
		"STEALTH_SETUP_IMAGE":                   setupImage,
		"STEALTH_WORKER_IMAGE":                  ImageName("stealth-worker", options.Version),
		"STEALTH_INGRESS_CONTROL_IMAGE":         ImageName("stealth-ingress-control", options.Version),
		"STEALTH_MIGRATE_IMAGE":                 ImageName("stealth-migrate", options.Version),
		"STEALTH_CONSOLE_IMAGE":                 ImageName("stealth-console", options.Version),
		"TRAEFIK_IMAGE":                         defaultTraefikImage,
		"POSTGRES_DB":                           "stealth",
		"POSTGRES_USER":                         "stealth",
		"POSTGRES_PASSWORD":                     postgresPassword,
		"DATABASE_URL":                          databaseURL,
		"REDIS_PASSWORD":                        redisPassword,
		"REDIS_URL":                             redisURL,
		"FUNCTIONS_SECRET_KEY":                  functionsKey,
		"APPS_SECRET_KEY":                       appsKey,
		"BOOTSTRAP_CLI_KEY":                     bootstrapKey,
		"GITHUB_APP_CLIENT_ID":                  strings.TrimSpace(options.GitHubAppClientID),
		"PUBLIC_APP_URL":                        publicURL,
		"COOKIE_SECURE":                         cookieSecure,
		"TRUSTED_PROXY_CIDRS":                   trustedProxy,
		"STEALTH_NETWORK_SUBNET":                subnet,
		"STEALTH_NETWORK_NAME":                  "stealth_network",
		"STEALTH_INGRESS_NETWORK_SUBNET":        ingress.Subnet,
		"STEALTH_INGRESS_IP_RANGE":              ingress.IPRange,
		"STEALTH_TRAEFIK_INGRESS_IP":            ingress.TraefikIP,
		"STEALTH_CLOUDFLARED_INGRESS_IP":        ingress.CloudflaredIP,
		"DOCKER_GID":                            strconv.FormatUint(uint64(options.DockerGID), 10),
		"METRICS_TOKEN":                         metricsToken,
		"STEALTH_INGRESS_NETWORK_NAME":          ingress.Name,
		"FUNCTIONS_RUNNER_ENABLED":              "true",
		"FUNCTIONS_WORKER_ID":                   "stealth-worker",
		"FUNCTIONS_RUNNER_STAGING_VOLUME":       "stealth_function_runner_staging",
		"APPS_MAX_SOURCE_ARCHIVE_BYTES":         "128MiB",
		"APPS_MAX_EXPANDED_SOURCE_BYTES":        "1GiB",
		"APPS_MAX_SOURCE_FILES":                 "8192",
		"APPS_MAX_IMAGE_ARCHIVE_BYTES":          "2GiB",
		"APPS_DEFAULT_ARTIFACT_QUOTA_BYTES":     "5GiB",
		"APPS_BUILDKIT_ADDRESS":                 "tcp://buildkit:1234",
		"APPS_BUILDKIT_CA_CERT":                 "/run/secrets/stealth-buildkit/ca.pem",
		"APPS_BUILDKIT_CLIENT_CERT":             "/run/secrets/stealth-buildkit/client-cert.pem",
		"APPS_BUILDKIT_CLIENT_KEY":              "/run/secrets/stealth-buildkit/client-key.pem",
		"APPS_BUILDKIT_APPARMOR_PROFILE":        appArmorProfile,
		"APPS_BUILD_TIMEOUT":                    "20m",
		"APPS_BUILD_LEASE_AGE":                  "25m",
		"APPS_BUILD_POLL_INTERVAL":              "500ms",
		"APPS_BUILD_STAGING_VOLUME":             "stealth_app_build_staging",
		"APPS_BUILDKIT_STATE_VOLUME":            "stealth_app_buildkit_state",
		"STORAGE_DRIVER":                        storageDriver,
		"STORAGE_MAX_FILE_SIZE":                 "50MiB",
		"STORAGE_DEFAULT_QUOTA_BYTES":           "1GiB",
		"PROJECT_OPERATION_RATE_LIMIT":          "120",
		"PROJECT_OPERATION_RATE_WINDOW":         "1m",
		"AUTH_RATE_LIMIT":                       "10",
		"AUTH_RATE_WINDOW":                      "1m",
		"PROXY_HTTP_BIND":                       "127.0.0.1",
		"PROXY_HTTP_PORT":                       "8080",
		"API_HOST_PORT":                         "18080",
		"CONSOLE_HOST_PORT":                     "13000",
		"SETUP_API_HOST_PORT":                   "18081",
		"SETUP_CONSOLE_HOST_PORT":               "13001",
		"SETUP_PROXY_HTTP_PORT":                 "8081",
		"SETUP_MODE":                            strconv.FormatBool(options.Setup),
	}
	if strings.TrimSpace(options.InstallRoot) != "" {
		values["STEALTH_INSTALL_ROOT"] = options.InstallRoot
	}
	if options.Setup {
		root := strings.TrimRight(options.InstallRoot, "/")
		values["STEALTH_SETUP_STATE_FILE"] = root + "/state/setup-state.enc"
		values["STEALTH_PRODUCTION_COMPOSE_FILE"] = root + "/compose.production.yaml"
		values["STEALTH_SETUP_COMPOSE_FILE"] = root + "/compose.setup.yaml"
	}
	if err := validateConfigValues(values); err != nil {
		return "", err
	}
	return FormatEnvFile(values), nil
}

// imageRegistry is the canonical OCI registry namespace for release-managed
// images. It must match the namespace the release workflow publishes to
// (ghcr.io/<repository owner>).
const imageRegistry = "ghcr.io/nafixhutao"

// legacyImageRegistry is the previous registry namespace. Installations created
// before the repository moved still reference it, so an upgrade re-homes their
// release-managed images onto imageRegistry.
const legacyImageRegistry = "ghcr.io/stealth-deplover"

func ImageName(name, version string) string {
	return imageRegistry + "/" + name + ":" + strings.TrimSpace(version)
}

// legacyImageName returns the canonical reference an installation created
// before the registry move would hold for the installed release.
func legacyImageName(name, version string) string {
	return legacyImageRegistry + "/" + name + ":" + strings.TrimSpace(version)
}

var releaseManagedImageNames = map[string]string{
	"STEALTH_API_IMAGE":                    "stealth-api",
	"STEALTH_SETUP_IMAGE":                  "stealth-setup",
	"STEALTH_WORKER_IMAGE":                 "stealth-worker",
	"STEALTH_INGRESS_CONTROL_IMAGE":        "stealth-ingress-control",
	"STEALTH_MIGRATE_IMAGE":                "stealth-migrate",
	"STEALTH_CONSOLE_IMAGE":         "stealth-console",
}
