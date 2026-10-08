package installengine

import (
	"fmt"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/secretkey"
)

// migrateReleaseConfig advances release-owned defaults while preserving
// operator-selected values. Image values are updated only when they still
// equal the canonical image for the installed release; custom registries,
// digests, and custom tags are treated as operator overrides. Secret values
// are never regenerated unless a required secret key is absent.
func migrateReleaseConfig(values map[string]string, targetVersion, installedVersion string) (string, error) {
	if err := ValidateReleaseVersion(targetVersion); err != nil {
		return "", err
	}
	result := make(map[string]string, len(values)+len(releaseManagedImageNames)+16)
	for key, value := range values {
		result[key] = value
	}
	updates := make(map[string]string)
	for key, imageName := range releaseManagedImageNames {
		current := strings.TrimSpace(result[key])
		target := ImageName(imageName, targetVersion)
		switch {
		case current == "":
			updates[key] = target
		case strings.TrimSpace(installedVersion) != "" &&
			(current == ImageName(imageName, installedVersion) ||
				current == legacyImageName(imageName, installedVersion)):
			updates[key] = target
		}
	}
	for key, value := range map[string]string{
		"TRAEFIK_IMAGE":                     defaultTraefikImage,
		"STEALTH_INGRESS_NETWORK_NAME":      "stealth_ingress",
		"APPS_MAX_SOURCE_ARCHIVE_BYTES":     "128MiB",
		"APPS_MAX_EXPANDED_SOURCE_BYTES":    "1GiB",
		"APPS_MAX_SOURCE_FILES":             "8192",
		"APPS_MAX_IMAGE_ARCHIVE_BYTES":      "2GiB",
		"APPS_DEFAULT_ARTIFACT_QUOTA_BYTES": "5GiB",
		"APPS_BUILDKIT_ADDRESS":             "tcp://buildkit:1234",
		"APPS_BUILDKIT_CA_CERT":             "/run/secrets/stealth-buildkit/ca.pem",
		"APPS_BUILDKIT_CLIENT_CERT":         "/run/secrets/stealth-buildkit/client-cert.pem",
		"APPS_BUILDKIT_CLIENT_KEY":          "/run/secrets/stealth-buildkit/client-key.pem",
		"APPS_BUILD_TIMEOUT":                "20m",
		"APPS_BUILD_LEASE_AGE":              "25m",
		"APPS_BUILD_POLL_INTERVAL":          "500ms",
		"APPS_BUILD_STAGING_VOLUME":         "stealth_app_build_staging",
		"APPS_BUILDKIT_STATE_VOLUME":        "stealth_app_buildkit_state",
	} {
		if strings.TrimSpace(result[key]) == "" {
			updates[key] = value
		}
	}
	currentAppArmorProfile := strings.TrimSpace(result["APPS_BUILDKIT_APPARMOR_PROFILE"])
	if currentAppArmorProfile == "" {
		appArmorProfile, err := DetectBuildKitAppArmorProfile()
		if err != nil {
			return "", err
		}
		updates["APPS_BUILDKIT_APPARMOR_PROFILE"] = appArmorProfile
	} else if !validBuildKitAppArmorProfile(currentAppArmorProfile) {
		return "", errorsf("existing APPS_BUILDKIT_APPARMOR_PROFILE must be unconfined or " + BuildKitAppArmorProfileName)
	} else if result["APPS_BUILDKIT_APPARMOR_PROFILE"] != currentAppArmorProfile {
		updates["APPS_BUILDKIT_APPARMOR_PROFILE"] = currentAppArmorProfile
	}
	if strings.TrimSpace(result["STEALTH_INGRESS_NETWORK_SUBNET"]) == "" {
		updates["STEALTH_INGRESS_NETWORK_SUBNET"] = defaultIngressSubnet
	}
	if strings.TrimSpace(result["STEALTH_INGRESS_NETWORK_NAME"]) == "" {
		updates["STEALTH_INGRESS_NETWORK_NAME"] = defaultIngressNetworkName
	}
	merged := make(map[string]string, len(result)+len(updates))
	for key, value := range result {
		merged[key] = value
	}
	for key, value := range updates {
		merged[key] = value
	}
	ingress, err := ingressNetworkConfigFromValues(merged)
	if err != nil {
		return "", err
	}
	for key, value := range map[string]string{
		"STEALTH_INGRESS_IP_RANGE":       ingress.IPRange,
		"STEALTH_TRAEFIK_INGRESS_IP":     ingress.TraefikIP,
		"STEALTH_CLOUDFLARED_INGRESS_IP": ingress.CloudflaredIP,
	} {
		if strings.TrimSpace(result[key]) == "" {
			updates[key] = value
		}
	}
	trustedProxy := ensureTraefikTrustedProxyCIDR(result["TRUSTED_PROXY_CIDRS"], result["STEALTH_NETWORK_SUBNET"], ingress.trustedProxyCIDR())
	if trustedProxy != strings.TrimSpace(result["TRUSTED_PROXY_CIDRS"]) {
		updates["TRUSTED_PROXY_CIDRS"] = trustedProxy
	}
	if strings.TrimSpace(result["APPS_SECRET_KEY"]) == "" {
		key, err := randomBase64(32)
		if err != nil {
			return "", fmt.Errorf("generate App encryption key: %w", err)
		}
		updates["APPS_SECRET_KEY"] = key
	} else {
		key, err := secretkey.Decode32ByteKey(result["APPS_SECRET_KEY"])
		clear(key)
		if err != nil {
			return "", errorsf("existing APPS_SECRET_KEY must be base64-encoded 32 bytes; refusing to replace it")
		}
	}
	return MergeEnv(result, updates)
}

func rejectReleaseDowngrade(targetVersion, installedVersion string) error {
	comparison, err := compareReleaseVersionOrder(targetVersion, installedVersion)
	if err != nil {
		return fmt.Errorf("compare installed release versions: %w", err)
	}
	if comparison < 0 {
		return fmt.Errorf("refusing platform downgrade from %s to %s; use the matching release or restore a compatible backup", installedVersion, targetVersion)
	}
	return nil
}

func compareReleaseVersionOrder(left, right string) (int, error) {
	leftParts, leftRC, leftIsRC, err := splitReleaseVersion(left)
	if err != nil {
		return 0, err
	}
	rightParts, rightRC, rightIsRC, err := splitReleaseVersion(right)
	if err != nil {
		return 0, err
	}
	for index := range leftParts {
		if comparison := compareDecimalVersionPart(leftParts[index], rightParts[index]); comparison != 0 {
			return comparison, nil
		}
	}
	if leftIsRC != rightIsRC {
		if leftIsRC {
			return -1, nil
		}
		return 1, nil
	}
	if leftIsRC {
		return compareDecimalVersionPart(leftRC, rightRC), nil
	}
	return 0, nil
}

func splitReleaseVersion(version string) ([3]string, string, bool, error) {
	if err := ValidateReleaseVersion(version); err != nil {
		return [3]string{}, "", false, err
	}
	value := strings.TrimPrefix(strings.TrimSpace(version), "v")
	core, rc, isRC := strings.Cut(value, "-rc.")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return [3]string{}, "", false, fmt.Errorf("invalid release version %q", version)
	}
	return [3]string{parts[0], parts[1], parts[2]}, rc, isRC, nil
}

func compareDecimalVersionPart(left, right string) int {
	left = strings.TrimLeft(left, "0")
	if left == "" {
		left = "0"
	}
	right = strings.TrimLeft(right, "0")
	if right == "" {
		right = "0"
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return strings.Compare(left, right)
	}
}
