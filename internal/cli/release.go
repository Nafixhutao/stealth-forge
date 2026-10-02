package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/buildinfo"
	"github.com/Stealth-deplover/stealth/internal/installengine"
)

func validateReleaseVersion(version string) error {
	return installengine.ValidateReleaseVersion(version)
}

func validateStableReleaseVersion(version string) error {
	return installengine.ValidateStableReleaseVersion(version)
}

func (a *App) resolveReleaseVersion(override string) (string, error) {
	version := strings.TrimSpace(override)
	if version == "" {
		version = strings.TrimSpace(os.Getenv("STEALTH_VERSION"))
	}
	if version == "" {
		version = strings.TrimSpace(buildinfo.Version)
	}
	if version == "" || version == "dev" {
		return "", fmt.Errorf("this development build has no release version; set STEALTH_VERSION or use a release CLI")
	}
	if err := validateReleaseVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

func releaseAsset(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "stealth_Linux_x86_64.tar.gz", nil
	case goos == "linux" && goarch == "arm64":
		return "stealth_Linux_arm64.tar.gz", nil
	default:
		return "", fmt.Errorf(
			"unsupported platform %s/%s; release installers support Linux amd64 and arm64",
			goos,
			goarch,
		)
	}
}

func verifySHA256(contents []byte, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("checksum is not a SHA-256 digest")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return fmt.Errorf("checksum is not a SHA-256 digest")
	}
	actual := sha256.Sum256(contents)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return fmt.Errorf("checksum mismatch")
	}
	return nil
}

func checksumForAsset(checksums, asset string) (string, error) {
	var found string
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == asset {
			if len(fields) != 2 {
				return "", fmt.Errorf("checksum entry for %s is malformed", asset)
			}
			if found != "" {
				return "", fmt.Errorf("checksum for %s is duplicated", asset)
			}
			found = fields[0]
		}
	}
	if found != "" {
		return found, nil
	}
	return "", fmt.Errorf("checksum for %s was not found", asset)
}
