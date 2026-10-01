package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/bootstrap"
)

func (a *App) startQuickTunnel(ctx context.Context, layout InstallLayout, network, containerName string) (string, error) {
	return a.startQuickTunnelTo(ctx, layout, network, containerName, "http://proxy:80")
}

func (a *App) startQuickTunnelTo(ctx context.Context, layout InstallLayout, network, containerName, target string) (string, error) {
	if !isQuickTunnelContainerName(containerName) {
		return "", fmt.Errorf("temporary tunnel container name is invalid")
	}
	if !safeDockerName(network) || target != "http://proxy:80" && target != "http://setup-proxy:80" {
		return "", fmt.Errorf("temporary tunnel target is invalid")
	}
	if _, err := a.runner.Output(ctx, layout.Root, "docker", "run", "--detach", "--name", containerName, "--network", network, "--pull=missing", quickTunnelCloudflaredImage, "tunnel", "--no-autoupdate", "--url", target); err != nil {
		return containerName, fmt.Errorf("start temporary onboarding tunnel: %w", err)
	}
	var lastLogs []byte
	for attempt := 0; attempt < positiveAttempts(a.pollAttempts); attempt++ {
		if err := ctx.Err(); err != nil {
			return containerName, err
		}
		// cloudflared writes its Quick Tunnel URL to stderr, which
		// exec.Cmd.Output() would discard. Read the combined stream so the URL
		// is visible whether cloudflared logs to stdout or stderr.
		logs, logsErr := a.runner.CombinedOutput(ctx, layout.Root, "docker", "logs", containerName)
		if logsErr == nil {
			lastLogs = logs
			if tunnelURL, found := findQuickTunnelURL(logs); found {
				return tunnelURL, nil
			}
		}
		// Do not burn the whole deadline if the temporary container already
		// exited without publishing a URL.
		if stopped, statusErr := a.quickTunnelContainerStopped(ctx, layout, containerName); statusErr == nil && stopped {
			return containerName, quickTunnelExitedError(lastLogs)
		}
		if attempt+1 < positiveAttempts(a.pollAttempts) {
			if err := waitSetupPoll(ctx, a.pollInterval); err != nil {
				return containerName, err
			}
		}
	}
	return containerName, fmt.Errorf("temporary onboarding tunnel did not publish a TryCloudflare URL in time")
}

// quickTunnelContainerStopped reports whether the temporary cloudflared
// container has already reached a terminal state. A missing or unreadable
// status is treated as "unknown" so a transient Docker error never aborts a
// tunnel that could still publish a URL.
func (a *App) quickTunnelContainerStopped(ctx context.Context, layout InstallLayout, containerName string) (bool, error) {
	output, err := a.runner.Output(ctx, layout.Root, "docker", "inspect", "--format", "{{.State.Status}}", containerName)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(string(output))) {
	case "exited", "dead":
		return true, nil
	case "created", "running", "restarting", "paused":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected temporary tunnel container state")
	}
}

func quickTunnelExitedError(logs []byte) error {
	excerpt := sanitizeTunnelLogs(logs)
	if excerpt == "" {
		return fmt.Errorf("temporary Quick Tunnel exited before publishing a setup URL")
	}
	return fmt.Errorf("temporary Quick Tunnel exited before publishing a setup URL\n\ncloudflared:\n%s", excerpt)
}

// printQuickTunnelFallback explains how to reach the setup service when the
// temporary tunnel cannot start. The setup service stays bound to the loopback
// address on the host; it is never exposed publicly. The CLI does not know the
// operator's SSH username or server name, so it prints a generic template.
func (a *App) printQuickTunnelFallback(tunnelErr error) {
	fmt.Fprintln(a.errOut, "Quick Tunnel could not be started.")
	fmt.Fprintf(a.errOut, "%v\n", tunnelErr)
	fmt.Fprintln(a.errOut)
	fmt.Fprintln(a.errOut, "The setup service is still running securely on the VPS.")
	fmt.Fprintln(a.errOut)
	fmt.Fprintln(a.errOut, "From your local computer, run:")
	fmt.Fprintln(a.errOut)
	fmt.Fprintln(a.errOut, "  ssh -L 8081:127.0.0.1:8081 <user>@<server>")
	fmt.Fprintln(a.errOut)
	fmt.Fprintln(a.errOut, "Then open:")
	fmt.Fprintln(a.errOut)
	fmt.Fprintln(a.errOut, "  http://localhost:8081/setup")
}

func (a *App) registerQuickTunnel(ctx context.Context, endpoint string, key []byte, containerName, tunnelURL string) error {
	body, err := json.Marshal(setupQuickTunnelPayload{ContainerName: containerName, URL: tunnelURL})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(bootstrap.CLIProofHeader, bootstrap.CLIProof(key))
	response, err := a.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &bootstrapHTTPError{status: response.StatusCode}
	}
	return nil
}

func positiveAttempts(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

func waitSetupPoll(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return nil
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseQuickTunnelURL(output []byte) string {
	found, _ := findQuickTunnelURL(output)
	return found
}

// findQuickTunnelURL returns the first safe TryCloudflare Quick Tunnel URL in
// cloudflared logs. It accepts only HTTPS URLs whose host is directly under
// trycloudflare.com, ignores unrelated or lookalike URLs, and tolerates the
// surrounding informational and warning noise cloudflared emits.
func findQuickTunnelURL(logs []byte) (string, bool) {
	text := string(logs)
	for _, indexes := range quickTunnelURLPattern.FindAllStringIndex(text, -1) {
		if end := indexes[1]; end < len(text) && isURLHostContinuation(text, end) {
			continue
		}
		match := text[indexes[0]:indexes[1]]
		parsed, err := url.Parse(match)
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if strings.HasSuffix(host, ".trycloudflare.com") && strings.TrimSuffix(host, ".trycloudflare.com") != "" {
			return "https://" + host, true
		}
	}
	return "", false
}

const (
	maxTunnelLogLines = 20
	maxTunnelLogBytes = 4 << 10
)

var (
	tunnelSecretPattern = regexp.MustCompile(`(?i)\b(token|secret|password|passwd|credential|credentials|api[_-]?key|access[_-]?key|client[_-]?secret)\b\s*[:=]\s*("[^"]*"|'[^']*'|\S+)`)
	tunnelBearerPattern = regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/=-]+`)
)

// sanitizeTunnelLogs produces a short log excerpt for an error message while
// redacting values that could carry credentials. The tunnel token, setup code,
// and provider secrets must never reach the operator's terminal or logs.
func sanitizeTunnelLogs(logs []byte) string {
	text := strings.ReplaceAll(string(logs), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > maxTunnelLogLines {
		lines = lines[len(lines)-maxTunnelLogLines:]
	}
	for index, line := range lines {
		line = tunnelSecretPattern.ReplaceAllString(line, "$1=[redacted]")
		line = tunnelBearerPattern.ReplaceAllString(line, "$1 [redacted]")
		lines[index] = line
	}
	excerpt := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(excerpt) > maxTunnelLogBytes {
		excerpt = excerpt[len(excerpt)-maxTunnelLogBytes:]
	}
	return excerpt
}

func isURLHostContinuation(text string, start int) bool {
	character := text[start]
	if character != '.' && character != '-' && (character < '0' || character > '9') && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') {
		return false
	}
	// A period followed by whitespace or punctuation can be ordinary sentence
	// punctuation. A period followed by another hostname character is a
	// lookalike suffix such as .evil and must not be silently truncated.
	if character == '.' && (start+1 == len(text) || !isURLHostContinuationCharacter(text[start+1])) {
		return false
	}
	return true
}

func isURLHostContinuationCharacter(character byte) bool {
	return character == '.' || character == '-' || character >= '0' && character <= '9' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}
