package installengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (e *Engine) runCompose(ctx context.Context, plan Plan, args ...string) error {
	composeFile := plan.Layout.ComposeFile
	if plan.Setup && plan.Layout.SetupComposeFile != "" {
		composeFile = plan.Layout.SetupComposeFile
	}
	return e.runComposeFile(ctx, plan, composeFile, "", args...)
}
func (e *Engine) runTraefikStateInit(ctx context.Context, plan Plan, composeFile, projectDirectory string) error {
	args := []string{
		"run", "--rm", "--no-deps",
		"-e", fmt.Sprintf("STEALTH_TRAEFIK_HOST_UID=%d", os.Geteuid()),
		"traefik-state-init",
	}
	return e.runComposeFile(ctx, plan, composeFile, projectDirectory, args...)
}
func (e *Engine) runComposeFile(ctx context.Context, plan Plan, composeFile, projectDirectory string, args ...string) error {
	return e.runComposeFileWithEnv(ctx, plan, composeFile, projectDirectory, plan.Layout.EnvFile, args...)
}
func (e *Engine) runComposeFileWithEnv(ctx context.Context, plan Plan, composeFile, projectDirectory, envFile string, args ...string) error {
	composeArgs := []string{"compose"}
	if plan.Cloudflare {
		composeArgs = append(composeArgs, "--profile", "cloudflare")
	}
	if projectDirectory != "" {
		composeArgs = append(composeArgs, "--project-directory", projectDirectory)
	}
	if strings.TrimSpace(composeFile) == "" {
		return errors.New("Compose file is required")
	}
	if strings.TrimSpace(envFile) == "" {
		return errors.New("Compose environment file is required")
	}
	composeArgs = append(composeArgs, "--env-file", envFile, "-f", composeFile)
	composeArgs = append(composeArgs, args...)
	var err error
	if filepath.Clean(envFile) != filepath.Clean(plan.Layout.EnvFile) {
		runner, ok := e.runner.(EnvironmentCommandRunner)
		if !ok {
			return errors.New("command runner cannot set STEALTH_ENV_FILE for staged Compose validation")
		}
		err = runner.RunWithEnv(ctx, plan.Layout.Root, []string{"STEALTH_ENV_FILE=" + envFile}, e.output, e.output, "docker", composeArgs...)
	} else {
		err = e.runner.Run(ctx, plan.Layout.Root, e.output, e.output, "docker", composeArgs...)
	}
	if err != nil {
		return fmt.Errorf("docker compose %v failed: %w", args, err)
	}
	return nil
}
func (e *Engine) fetchAsset(ctx context.Context, assetURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create asset request: %w", err)
	}
	response, err := e.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download asset returned HTTP %d", response.StatusCode)
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxAssetSize+1))
	if err != nil {
		return nil, fmt.Errorf("read downloaded asset: %w", err)
	}
	if len(contents) > maxAssetSize {
		return nil, errors.New("downloaded asset is unexpectedly large")
	}
	return contents, nil
}

// RunCommand exposes the same process boundary used by installation steps for
// narrowly scoped lifecycle operations such as removing the exact temporary
// onboarding container. Callers must validate any resource name before
// passing it here; this method is not a general shell interface.
func (e *Engine) RunCommand(ctx context.Context, dir, name string, args ...string) error {
	if e == nil || e.runner == nil {
		return errors.New("install engine is not configured")
	}
	return e.runner.Run(ctx, dir, io.Discard, io.Discard, name, args...)
}
func (e *Engine) httpStatus(ctx context.Context, endpoint string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	response, err := e.httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode, nil
}
func waitInterval(ctx context.Context, interval time.Duration) error {
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
func acquireLock(stateDir string) (*os.File, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, errors.New("installation state directory is required")
	}
	return AcquireProcessLock(stateDir, lockFileName, "installation")
}

// AcquireProcessLock takes a non-blocking exclusive advisory lock on
// stateDir/name and returns the open lock file. Closing the file releases the
// lock, so a crashed process cannot leave a stale lock behind. Callers use this
// to guarantee that only one process owns a long-running operation such as a
// host-side setup orchestration or a production install.
func AcquireProcessLock(stateDir, name, description string) (*os.File, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, errors.New("lock directory is required")
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return nil, errors.New("lock name is invalid")
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s lock directory: %w", description, err)
	}
	lockPath := filepath.Join(stateDir, name)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s lock: %w", description, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			if description == "installation" {
				return nil, ErrOperationInProgress
			}
			return nil, fmt.Errorf("another %s operation is already running", description)
		}
		return nil, fmt.Errorf("lock %s: %w", description, err)
	}
	return file, nil
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	// Compose errors are allowed to include service names and status codes, but
	// never copy a command's output into a browser event. The command runner is
	// configured with a discard writer by the HTTP composition root.
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}
