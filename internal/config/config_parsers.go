package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func value(key, fallback string) string {
	if got := strings.TrimSpace(os.Getenv(key)); got != "" {
		return got
	}
	return fallback
}

func isWorkerID(value string) bool {
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return value != ""
}

func isStorageS3Endpoint(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\x00") {
		return false
	}
	endpointURL := raw
	if !strings.Contains(raw, "://") {
		endpointURL = "http://" + raw
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return false
	}
	return true
}

func isDockerName(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			if index == 0 && (character == '.' || character == '_' || character == '-') {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func isImageReference(value string) bool {
	if len(value) < 1 || len(value) > 255 || strings.ContainsRune(value, '\x00') {
		return false
	}
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || strings.ContainsRune("/._:@-", character) {
			continue
		}
		return false
	}
	return true
}

func isListenAddress(value string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil || strings.ContainsAny(host, "\r\n") {
		return false
	}
	parsed, err := strconv.Atoi(port)
	return err == nil && parsed >= 1 && parsed <= 65535
}

func sameListenPort(first, second string) bool {
	_, firstPort, firstErr := net.SplitHostPort(strings.TrimSpace(first))
	_, secondPort, secondErr := net.SplitHostPort(strings.TrimSpace(second))
	return firstErr == nil && secondErr == nil && firstPort == secondPort
}

func isACMEDirectoryURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n \t") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if port := parsed.Port(); port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return false
		}
	}
	return true
}

func isACMEEmail(value string) bool {
	if value == "" || len([]byte(value)) > 320 || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && strings.Contains(value, "@")
}

func isPublicAppURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n \t") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if port := parsed.Port(); port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return false
		}
	}
	return true
}

// parseConsoleCORSOrigins parses a comma-separated, exact-origin allowlist.
// It deliberately does not fall back to PUBLIC_APP_URL: that value may point
// at an email route (and a missing allowlist must fail closed for browsers).
func parseConsoleCORSOrigins(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 32 {
		return nil, fmt.Errorf("CONSOLE_CORS_ORIGINS must contain at most 32 origins")
	}
	seen := make(map[string]struct{}, len(parts))
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin, err := normalizeConsoleOrigin(part)
		if err != nil {
			return nil, fmt.Errorf("CONSOLE_CORS_ORIGINS contains an invalid origin")
		}
		if _, exists := seen[origin]; exists {
			return nil, fmt.Errorf("CONSOLE_CORS_ORIGINS contains a duplicate origin")
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins, nil
}

func normalizeConsoleOrigin(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n\t ") {
		return "", fmt.Errorf("invalid origin")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid origin")
	}
	hostname := parsed.Hostname()
	if hostname == "" || strings.ContainsAny(hostname, "*%/") {
		return "", fmt.Errorf("invalid origin")
	}
	if port := parsed.Port(); port != "" {
		parsedPort, parseErr := strconv.Atoi(port)
		if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", fmt.Errorf("invalid origin")
		}
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Host)
	if (scheme == "http" && parsed.Port() == "80") || (scheme == "https" && parsed.Port() == "443") {
		host = strings.TrimSuffix(host, ":"+parsed.Port())
	}
	return scheme + "://" + host, nil
}

// parseBytes accepts plain bytes and binary IEC suffixes. Keeping this parser
// in config makes deployment values explicit while retaining an integer value
// for quota accounting in PostgreSQL.
func parseBytes(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty byte quantity")
	}
	units := []struct {
		suffix string
		value  int64
	}{{"tib", 1 << 40}, {"gib", 1 << 30}, {"mib", 1 << 20}, {"kib", 1 << 10}, {"b", 1}}
	lower := strings.ToLower(raw)
	multiplier := int64(1)
	number := lower
	for _, unit := range units {
		if strings.HasSuffix(lower, unit.suffix) {
			multiplier = unit.value
			number = strings.TrimSpace(lower[:len(lower)-len(unit.suffix)])
			break
		}
	}
	parsed, err := strconv.ParseInt(number, 10, 64)
	if err != nil || parsed < 1 || parsed > (1<<63-1)/multiplier {
		return 0, fmt.Errorf("invalid byte quantity")
	}
	return parsed * multiplier, nil
}
