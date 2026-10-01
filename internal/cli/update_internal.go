package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Stealth-deplover/stealth/internal/buildinfo"
)

func (a *App) runInternal(args []string) int {
	if len(args) == 0 || args[0] != "migrate-installation" {
		fmt.Fprintln(a.errOut, "unknown internal command")
		return 2
	}
	fs := flag.NewFlagSet("stealth internal migrate-installation", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	targetVersion := fs.String("target-version", "", "target release version")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*targetVersion) == "" {
		fmt.Fprintln(a.errOut, "internal migration requires exactly --target-version vX.Y.Z")
		return 2
	}
	if err := validateStableReleaseVersion(*targetVersion); err != nil {
		fmt.Fprintf(a.errOut, "invalid internal migration target: %v\n", err)
		return 2
	}
	current := buildinfo.Version
	if a.currentVersion != nil {
		current = a.currentVersion()
	}
	if strings.TrimSpace(current) != strings.TrimSpace(*targetVersion) {
		fmt.Fprintln(a.errOut, "internal migration target does not match this Stealth binary")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	migrated, err := a.migrateInstalledRelease(ctx, *targetVersion)
	if err != nil {
		fmt.Fprintf(a.errOut, "internal installation migration failed: %v\n", err)
		return 1
	}
	if migrated {
		fmt.Fprintln(a.out, "managed installation migration completed")
	}
	return 0
}

func hasReleaseAsset(release githubRelease, name string) bool {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return true
		}
	}
	return false
}

func (a *App) fetchUpdateAsset(ctx context.Context, assetURL string, maxSize int64) ([]byte, error) {
	if err := validateHTTPSURL(assetURL); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	request.Header.Set("User-Agent", a.updateUserAgent())
	response, err := a.updateHTTPClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, githubHTTPError("download release asset", response)
	}
	return readBounded(response.Body, maxSize)
}

func readBounded(reader io.Reader, maxSize int64) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > maxSize {
		return nil, fmt.Errorf("response exceeds the %d-byte limit", maxSize)
	}
	return contents, nil
}

func githubHTTPError(operation string, response *http.Response) error {
	if response.StatusCode == http.StatusTooManyRequests ||
		(response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0") {
		return fmt.Errorf("%s was rate limited by GitHub (HTTP %d); try again later", operation, response.StatusCode)
	}
	if response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s returned HTTP 404; the stable release or asset may be unavailable", operation)
	}
	return fmt.Errorf("%s returned HTTP %d", operation, response.StatusCode)
}

func appendReleasePath(base string, elements ...string) (string, error) {
	if err := validateHTTPSURL(base); err != nil {
		return "", err
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	for _, element := range elements {
		if element == "" || element == "." || element == ".." || strings.ContainsAny(element, "/\\") {
			return "", fmt.Errorf("invalid release path component %q", element)
		}
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + url.PathEscape(element)
	}
	return parsed.String(), nil
}

func validateHTTPSURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("release downloads must use an HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("release URL must not contain credentials, query parameters, or fragments")
	}
	return nil
}

func (a *App) updateHTTPClient() *http.Client {
	if a.httpClient != nil {
		return a.httpClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (a *App) updateUserAgent() string {
	version := buildinfo.Version
	if a.currentVersion != nil {
		version = a.currentVersion()
	}
	version = strings.TrimSpace(version)
	if version == "" || strings.ContainsAny(version, "\r\n") {
		version = "dev"
	}
	return "Stealth/" + version + " (self-update)"
}

func (a *App) currentExecutablePath() (string, error) {
	getExecutable := a.executablePath
	if getExecutable == nil {
		getExecutable = os.Executable
	}
	executable, err := getExecutable()
	if err != nil {
		return "", fmt.Errorf("cannot determine the running Stealth executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("cannot resolve the running Stealth executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("cannot resolve the running Stealth executable %s: %w", executable, err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot inspect the running Stealth executable %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("running Stealth executable %s is not a regular executable file", resolved)
	}
	parentInfo, err := os.Stat(filepath.Dir(resolved))
	if err != nil || !parentInfo.IsDir() {
		return "", fmt.Errorf("cannot access the installation directory for %s", resolved)
	}
	return resolved, nil
}
