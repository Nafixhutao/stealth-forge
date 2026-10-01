package installengine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Wait checks the host-visible service endpoints for CLI plans and the
// Compose-network endpoints for browser handoff plans. A public URL is only
// probed when the plan explicitly requests the final ingress check.
func (e *Engine) Wait(ctx context.Context, plan Plan) error {
	values, err := ReadEnvFile(plan.Layout.EnvFile)
	if err != nil {
		return fmt.Errorf("read installation configuration: %w", err)
	}
	apiURL := strings.TrimRight(plan.InternalAPIURL, "/")
	consoleURL := strings.TrimRight(plan.InternalConsoleURL, "/")
	proxyURL := strings.TrimRight(plan.InternalProxyURL, "/")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:" + PortOrDefault(values["API_HOST_PORT"], "18080")
	}
	if consoleURL == "" {
		consoleURL = "http://127.0.0.1:" + PortOrDefault(values["CONSOLE_HOST_PORT"], "13000")
	}
	if proxyURL == "" {
		proxyURL = "http://127.0.0.1:" + PortOrDefault(values["PROXY_HTTP_PORT"], "8080")
	}
	endpoints := []string{apiURL + "/healthz", apiURL + "/readyz", apiURL + "/version", consoleURL + "/", proxyURL + "/"}
	if plan.VerifyPublicURL && strings.TrimRight(plan.PublicURL, "/") != "" {
		endpoints = append(endpoints, strings.TrimRight(plan.PublicURL, "/")+"/")
	}
	maxWait := time.Duration(e.pollAttempts)*e.pollInterval + 30*time.Second
	waitContext, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()
	for attempt := 0; attempt < e.pollAttempts; attempt++ {
		if err := waitContext.Err(); err != nil {
			return err
		}
		allReady := true
		for _, endpoint := range endpoints {
			status, requestErr := e.httpStatus(waitContext, endpoint)
			if requestErr != nil || status < 200 || status >= 300 {
				allReady = false
				break
			}
		}
		if allReady {
			return nil
		}
		if attempt+1 < e.pollAttempts {
			if err := waitInterval(waitContext, e.pollInterval); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("services did not become ready after %d checks", e.pollAttempts)
}
