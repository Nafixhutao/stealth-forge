package httpapi

import (
	"net/url"
	"strings"
)

func validQuickTunnelName(value string) bool {
	return strings.HasPrefix(value, "stealth-onboarding-") && len(value) <= 128 && safeDockerName(value)
}

func validQuickTunnelURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" && strings.HasSuffix(host, ".trycloudflare.com") && strings.TrimSuffix(host, ".trycloudflare.com") != ""
}

func safeDockerName(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			if index == 0 && character != 's' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
