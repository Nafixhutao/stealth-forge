package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/Stealth-deplover/stealth/internal/buildinfo"
)

const (
	maxReleaseMetadataSize = 1 << 20
	maxUpdateArchiveSize   = 64 << 20
	maxUpdateBinarySize    = 32 << 20
)

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type semanticVersion struct {
	major uint64
	minor uint64
	patch uint64
}

type platformMigratedUpdateError struct {
	targetVersion string
	err           error
}

func (e *platformMigratedUpdateError) Error() string { return e.err.Error() }
func (e *platformMigratedUpdateError) Unwrap() error { return e.err }

func (a *App) runUpdate(args []string) int {
	initTerminalStyles()

	fs := flag.NewFlagSet("stealth update", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	check := fs.Bool("check", false, "check for an available CLI update without downloading it")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(a.errOut, "update does not accept positional arguments")
		return 2
	}

	current := buildinfo.Version
	if a.currentVersion != nil {
		current = a.currentVersion()
	}
	currentLabel := displayCurrentVersion(current)
	a.printUpdateHeader()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	release, err := a.latestStableRelease(ctx)
	if err != nil {
		if *check {
			fmt.Fprintf(a.errOut, "stealth update check failed: %v\n", err)
		} else {
			a.printUpdateFailure(err)
		}
		return 1
	}

	latest := release.TagName
	fmt.Fprintf(a.out, "Current version   %s\nLatest version    %s\n\n", currentLabel, latest)

	comparison, comparable := compareReleaseVersions(current, latest)
	if comparable && comparison == 0 {
		if !*check {
			migrated, migrationErr := a.migrateInstalledRelease(ctx, latest)
			if migrationErr != nil {
				a.printUpdateFailure(migrationErr)
				return 1
			}
			if migrated {
				a.printUpdateStep("✓", "Managed production assets synchronized", successStyle)
			}
		}
		a.printUpdateSuccess("Stealth is already up to date")
		fmt.Fprintf(a.out, "%s\n", latest)
		return 0
	}
	if comparable && comparison > 0 {
		fmt.Fprintf(a.out, "Current version %s is newer than latest stable %s.\nNo update performed.\n", currentLabel, latest)
		return 0
	}

	if *check {
		fmt.Fprintln(a.out, "Update available.")
		return 1
	}

	asset, err := releaseAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		a.printUpdateFailure(err)
		return 1
	}
	a.printUpdateStep("⠹", "Downloading Stealth "+latest, cyanStyle)
	if err := a.performUpdateWithTargetMigration(ctx, release, asset); err != nil {
		a.printUpdateFailure(err)
		return 1
	}
	a.printUpdateStep("✓", "Downloaded Stealth "+latest, successStyle)
	a.printUpdateStep("✓", "SHA-256 checksum verified", successStyle)
	a.printUpdateStep("✓", "Target release migration completed", successStyle)
	a.printUpdateStep("✓", "CLI updated", successStyle)
	fmt.Fprintf(a.out, "\n%s → %s\n", currentLabel, latest)
	return 0
}

func displayCurrentVersion(version string) string {
	version = strings.TrimSpace(version)
	if _, err := parseSemanticVersion(version); err != nil {
		return "dev"
	}
	return version
}

func compareReleaseVersions(current, latest string) (int, bool) {
	currentVersion, currentErr := parseSemanticVersion(strings.TrimSpace(current))
	latestVersion, latestErr := parseSemanticVersion(strings.TrimSpace(latest))
	if currentErr != nil || latestErr != nil {
		return 0, false
	}
	switch {
	case currentVersion.major != latestVersion.major:
		return compareUint64(currentVersion.major, latestVersion.major), true
	case currentVersion.minor != latestVersion.minor:
		return compareUint64(currentVersion.minor, latestVersion.minor), true
	default:
		return compareUint64(currentVersion.patch, latestVersion.patch), true
	}
}

func compareUint64(left, right uint64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func parseSemanticVersion(version string) (semanticVersion, error) {
	if err := validateStableReleaseVersion(version); err != nil {
		return semanticVersion{}, err
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	values := [3]uint64{}
	for index, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, fmt.Errorf("invalid release version %q: %w", version, err)
		}
		values[index] = value
	}
	return semanticVersion{major: values[0], minor: values[1], patch: values[2]}, nil
}

func (a *App) latestStableRelease(ctx context.Context) (githubRelease, error) {
	endpoint, err := appendReleasePath(a.releaseAPIBase, "releases", "latest")
	if err != nil {
		return githubRelease{}, fmt.Errorf("invalid GitHub API endpoint: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("create GitHub release request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", a.updateUserAgent())
	response, err := a.updateHTTPClient().Do(request)
	if err != nil {
		return githubRelease{}, fmt.Errorf("resolve latest stable release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, githubHTTPError("resolve latest stable release", response)
	}
	contents, err := readBounded(response.Body, maxReleaseMetadataSize)
	if err != nil {
		return githubRelease{}, fmt.Errorf("read latest release metadata: %w", err)
	}
	var release githubRelease
	if err := json.Unmarshal(contents, &release); err != nil {
		return githubRelease{}, fmt.Errorf("parse latest release metadata: %w", err)
	}
	if release.Draft || release.Prerelease {
		return githubRelease{}, fmt.Errorf("latest GitHub release %q is not stable", release.TagName)
	}
	if strings.TrimSpace(release.TagName) != release.TagName {
		return githubRelease{}, fmt.Errorf("latest GitHub release has invalid tag %q", release.TagName)
	}
	if err := validateStableReleaseVersion(release.TagName); err != nil {
		return githubRelease{}, fmt.Errorf("latest GitHub release has invalid tag: %w", err)
	}
	return release, nil
}
