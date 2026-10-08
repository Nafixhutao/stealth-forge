package installengine

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func ensureTraefikTrustedProxyCIDR(value, fallbackSubnet, traefikPeerCIDR string) string {
	trusted := strings.TrimSpace(value)
	if trusted == "" {
		trusted = firstNonEmpty(strings.TrimSpace(fallbackSubnet), "172.30.0.0/24")
	}
	for _, entry := range strings.Split(trusted, ",") {
		if strings.TrimSpace(entry) == strings.TrimSpace(traefikPeerCIDR) {
			return trusted
		}
	}
	return trusted + "," + strings.TrimSpace(traefikPeerCIDR)
}

func validateConfigValues(values map[string]string) error {
	for key, value := range values {
		if !ValidEnvKey(key) || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid generated configuration value for %s", key)
		}
		if strings.TrimSpace(value) == "" && key != "GITHUB_APP_CLIENT_ID" {
			return fmt.Errorf("generated configuration value for %s is empty", key)
		}
	}
	if profile := strings.TrimSpace(values["APPS_BUILDKIT_APPARMOR_PROFILE"]); profile != "" && !validBuildKitAppArmorProfile(profile) {
		return errorsf("APPS_BUILDKIT_APPARMOR_PROFILE must be unconfined or " + BuildKitAppArmorProfileName)
	}
	for _, key := range []string{"APPS_BUILDKIT_CA_CERT", "APPS_BUILDKIT_CLIENT_CERT", "APPS_BUILDKIT_CLIENT_KEY"} {
		path := values[key]
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) || strings.ContainsAny(path, "\x00\r\n") {
			return fmt.Errorf("generated configuration value for %s must be an absolute clean non-root path", key)
		}
	}
	for _, key := range []string{"STEALTH_API_IMAGE", "STEALTH_SETUP_IMAGE", "STEALTH_WORKER_IMAGE", "STEALTH_INGRESS_CONTROL_IMAGE", "STEALTH_MIGRATE_IMAGE", "STEALTH_CONSOLE_IMAGE", "TRAEFIK_IMAGE"} {
		if !validImageReference(values[key]) {
			return fmt.Errorf("generated image reference for %s is invalid", key)
		}
	}
	return nil
}

func validBuildKitAppArmorProfile(value string) bool {
	return value == "unconfined" || value == BuildKitAppArmorProfileName
}

// DetectBuildKitAppArmorProfile selects the narrow profile required by Ubuntu
// hosts that restrict unprivileged user namespaces. Other hosts keep Docker's
// existing unconfined AppArmor setting for the official rootless BuildKit
// image. Failure to read an existing kernel setting is not treated as an
// unrestricted host.
func DetectBuildKitAppArmorProfile() (string, error) {
	contents, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")
	if errors.Is(err, os.ErrNotExist) {
		return "unconfined", nil
	}
	if err != nil {
		return "", fmt.Errorf("read AppArmor unprivileged user namespace setting: %w", err)
	}
	return buildKitAppArmorProfileForSetting(string(contents))
}

func buildKitAppArmorProfileForSetting(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "0":
		return "unconfined", nil
	case "1":
		return BuildKitAppArmorProfileName, nil
	default:
		return "", errorsf("AppArmor unprivileged user namespace setting has an unsupported value")
	}
}

func validImageReference(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("/._:@-", character) {
			continue
		}
		return false
	}
	return true
}

// ValidateReleaseVersion accepts stable releases and explicitly numbered RCs.
// The release workflow and explicit installer version pin use the same format.
func ValidateReleaseVersion(value string) error {
	if !releaseVersionPattern.MatchString(strings.TrimSpace(value)) {
		return errorsf("release version %q must match vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N", value)
	}
	return nil
}

// ValidateStableReleaseVersion deliberately excludes prereleases. It is used
// by the automatic update path so a normal installation never follows an RC.
func ValidateStableReleaseVersion(value string) error {
	if !stableReleaseVersionPattern.MatchString(strings.TrimSpace(value)) {
		return errorsf("stable release version %q must match vMAJOR.MINOR.PATCH", value)
	}
	return nil
}

func validateVersion(value string) error {
	return ValidateReleaseVersion(value)
}

func validatePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errorsf("URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errorsf("URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validClientID(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 160 {
		return false
	}
	for _, character := range raw {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune(".-_", character) {
			continue
		}
		return false
	}
	return true
}

func randomHex(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func randomBase64(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func mustURLScheme(raw string) string {
	parsed, _ := url.Parse(raw)
	if parsed == nil {
		return ""
	}
	return parsed.Scheme
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
