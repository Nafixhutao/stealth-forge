package monitoring

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/netip"
	"strings"
)

var monitorDeniedPrefixes = mustParseMonitorPrefixes([]string{
	"0.0.0.0/8",         // IPv4 "this network" and other unspecified uses.
	"10.0.0.0/8",        // RFC 1918 private.
	"100.64.0.0/10",     // RFC 6598 shared address space.
	"127.0.0.0/8",       // IPv4 loopback.
	"169.254.0.0/16",    // IPv4 link-local and cloud metadata endpoints.
	"172.16.0.0/12",     // RFC 1918 private.
	"192.0.0.0/24",      // IETF protocol assignments.
	"192.0.2.0/24",      // TEST-NET-1 documentation.
	"192.31.196.0/24",   // AS112-v4.
	"192.52.193.0/24",   // AMT.
	"192.88.99.0/24",    // 6to4 relay anycast (deprecated).
	"192.168.0.0/16",    // RFC 1918 private.
	"192.175.48.0/24",   // Direct Delegation AS112.
	"198.18.0.0/15",     // Benchmarking.
	"198.51.100.0/24",   // TEST-NET-2 documentation.
	"203.0.113.0/24",    // TEST-NET-3 documentation.
	"224.0.0.0/4",       // IPv4 multicast.
	"240.0.0.0/4",       // IPv4 reserved and future use.
	"::/128",            // IPv6 unspecified.
	"::1/128",           // IPv6 loopback.
	"100::/64",          // IPv6 discard-only.
	"100:0:0:1::/64",    // IPv6 dummy prefix.
	"2001::/23",         // IETF protocol assignments and special subranges.
	"2001:db8::/32",     // IPv6 documentation.
	"2002::/16",         // 6to4.
	"2620:4f:8000::/48", // Direct Delegation AS112.
	"3fff::/20",         // IPv6 documentation.
	"5f00::/16",         // Segment Routing SIDs.
	"fc00::/7",          // IPv6 unique local.
	"fe80::/10",         // IPv6 link-local.
	"ff00::/8",          // IPv6 multicast.
})

func mustParseMonitorPrefixes(values []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			panic("invalid monitor egress policy prefix: " + value)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

func validHTTPMethod(value string) bool {
	switch value {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodOptions:
		return true
	default:
		return false
	}
}

func validHeader(key, value string) bool {
	return strings.TrimSpace(key) != "" && len(key) <= 128 && len(value) <= 4096 && !strings.ContainsAny(key+value, "\x00\r\n")
}

func validDNSName(value string) bool {
	value = strings.TrimSuffix(strings.TrimSpace(value), ".")
	if len(value) < 1 || len(value) > 253 || strings.Contains(value, "..") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// expectedDNSValuesPresent implements the monitor contract: every configured
// expected value must be present, while additional DNS records are allowed.
// This is a subset check rather than an exact-set check because public DNS
// names commonly return additional healthy addresses over time.
func expectedDNSValuesPresent(actual, expected []string) bool {
	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		seen[strings.TrimSpace(strings.TrimSuffix(value, "."))] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[strings.TrimSpace(strings.TrimSuffix(value, "."))]; !ok {
			return false
		}
	}
	return true
}

func classifyNetworkError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errors.New("monitor probe timed out")
	}
	return errors.New("monitor probe could not connect")
}

func safeCheckError(err error) string {
	value := strings.TrimSpace(err.Error())
	value = strings.Map(func(r rune) rune {
		if r == '\x00' || r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, value)
	if len(value) > maxMonitorError {
		return value[:maxMonitorError]
	}
	if value == "" {
		return "monitor probe failed"
	}
	return value
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}
