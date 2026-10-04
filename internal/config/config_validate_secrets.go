package config

import (
	"fmt"
	"net/url"
	"strings"
)

// placeholderSecretPatterns are the documented example values shipped in
// .env.production.example. They exist so an operator notices a required
// replacement, but nothing previously stopped them from reaching a running
// production process. Matching is intentionally limited to distinctive
// CHANGE_ME-style fragments to avoid rejecting legitimate random secrets.
var placeholderSecretPatterns = []string{
	"change_me",
	"changeme",
	"change-me",
	"replace_me",
	"replaceme",
	"placeholder",
	"your_",
	"your-",
	"example.com",
}

func looksLikePlaceholder(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, pattern := range placeholderSecretPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func urlPassword(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil {
		return ""
	}
	password, _ := parsed.User.Password()
	return password
}

// ValidateProductionSecrets fails closed when a production deployment still
// contains a documented placeholder for a security-sensitive value. Setup mode
// is exempt because the browser wizard intentionally starts from incomplete
// values. Invalid base64 encryption keys are already rejected elsewhere; this
// gate covers the remaining string secrets that previously failed open.
func (c Config) ValidateProductionSecrets() error {
	if c.SetupMode {
		return nil
	}
	checks := []struct {
		name  string
		value string
	}{
		{"POSTGRES_PASSWORD", urlPassword(c.DatabaseURL)},
		{"REDIS_PASSWORD", urlPassword(c.RedisURL)},
		{"CLICKHOUSE_PASSWORD", c.TelemetryClickHousePassword},
		{"METRICS_TOKEN", c.MetricsToken},
		{"GITHUB_APP_CLIENT_ID", c.GitHubAppClientID},
		{"PUBLIC_APP_URL", c.PublicAppURL},
	}
	for _, check := range checks {
		if looksLikePlaceholder(check.value) {
			return fmt.Errorf("%s still contains a placeholder value; replace it before starting production", check.name)
		}
	}
	return nil
}
