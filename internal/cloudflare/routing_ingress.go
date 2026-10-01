package cloudflare

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domainname"
)

const (
	ConsoleOriginProxy   = "proxy"
	ConsoleOriginTraefik = "traefik"
)

func consoleOriginService(origin string) string {
	switch origin {
	case ConsoleOriginProxy:
		return "http://proxy:80"
	case ConsoleOriginTraefik:
		return "http://traefik:8080"
	default:
		return ""
	}
}

func desiredTunnelIngress(consoleHostname, wildcardHostname string, consoleOrigins ...string) []IngressRule {
	consoleOrigin := ConsoleOriginProxy
	if len(consoleOrigins) > 0 && consoleOriginService(consoleOrigins[0]) != "" {
		consoleOrigin = consoleOrigins[0]
	}
	result := []IngressRule{consoleIngressRule(consoleHostname, consoleOrigin)}
	if wildcardHostname != "" {
		result = append(result, IngressRule{Hostname: wildcardHostname, Service: "http://traefik:8080"})
	}
	return append(result, IngressRule{Service: "http_status:404"})
}

func consoleIngressRule(hostname, origin string) IngressRule {
	return IngressRule{Hostname: hostname, Service: consoleOriginService(origin)}
}

func patchConsoleIngress(rules []IngressRule, consoleHostname, workloadHostname, desiredOrigin string) ([]IngressRule, string, error) {
	if len(rules) == 0 || len(rules) > 64 || (desiredOrigin != ConsoleOriginProxy && desiredOrigin != ConsoleOriginTraefik) {
		return nil, "", errors.New("Cloudflare Tunnel ingress is structurally invalid for a Console-origin change")
	}
	consoleName, err := domainname.NormalizeHostname(consoleHostname)
	if err != nil {
		return nil, "", errors.New("saved Console hostname is invalid")
	}
	wildcardName := ""
	if workloadHostname != "" {
		wildcardName, err = canonicalWildcardHostname(workloadHostname)
		if err != nil {
			return nil, "", errors.New("saved workload wildcard hostname is invalid")
		}
	}
	consoleIndex, catchAllCount := -1, 0
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if strings.TrimSpace(rule.Service) == "" {
			return nil, "", errors.New("Cloudflare Tunnel ingress contains an empty service")
		}
		if rule.Hostname == "" {
			catchAllCount++
			if index != len(rules)-1 || rule.Service != "http_status:404" {
				return nil, "", errors.New("Cloudflare Tunnel catch-all is unsafe; expected a final http_status:404 rule")
			}
			continue
		}
		name, nameErr := canonicalIngressHostname(rule.Hostname)
		if nameErr != nil {
			return nil, "", errors.New("Cloudflare Tunnel ingress contains an invalid hostname")
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, "", errors.New("Cloudflare Tunnel ingress contains duplicate hostname rules; refusing an ambiguous Console-origin change")
		}
		seen[name] = struct{}{}
		if name == consoleName {
			if consoleIndex >= 0 {
				return nil, "", errors.New("Cloudflare Tunnel has multiple Console rules; refusing an ambiguous origin change")
			}
			consoleIndex = index
			if rule.Service != "http://proxy:80" && rule.Service != "http://traefik:8080" {
				return nil, "", errors.New("Cloudflare Console rule points to an unsupported origin; HIGH SEVERITY: refusing to overwrite provider configuration")
			}
		}
		if wildcardName != "" && name == wildcardName && rule.Service != "http://traefik:8080" {
			return nil, "", errors.New("Cloudflare workload rule points to an unexpected origin; HIGH SEVERITY: refusing a Console-origin change")
		}
	}
	if catchAllCount != 1 || consoleIndex < 0 {
		return nil, "", errors.New("Cloudflare Tunnel ingress is missing a unique Console rule or final 404 catch-all")
	}
	// A missing known workload rule is not ambiguous for this operation. Keep
	// whatever is present untouched and let the full workload reconciler repair
	// it after the Console rule has converged.
	observed := ConsoleOriginProxy
	if rules[consoleIndex].Service == "http://traefik:8080" {
		observed = ConsoleOriginTraefik
	}
	result := append([]IngressRule(nil), rules...)
	result[consoleIndex] = cloneIngressRule(result[consoleIndex])
	result[consoleIndex].Service = consoleIngressRule(consoleHostname, desiredOrigin).Service
	return result, observed, nil
}

func cloneIngressRule(rule IngressRule) IngressRule {
	clone := rule
	clone.Origin = append(rule.Origin[:0:0], rule.Origin...)
	if rule.Extra != nil {
		clone.Extra = make(map[string]json.RawMessage, len(rule.Extra))
		for key, value := range rule.Extra {
			clone.Extra[key] = append(json.RawMessage(nil), value...)
		}
	}
	return clone
}

func sameTunnelRules(left, right []IngressRule) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !sameIngressHostname(left[i].Hostname, right[i].Hostname) || left[i].Service != right[i].Service || !sameJSON(left[i].Origin, right[i].Origin) || !sameExtra(left[i].Extra, right[i].Extra) {
			return false
		}
	}
	return true
}

func sameIngressHostname(left, right string) bool {
	if left == "" || right == "" {
		return left == right
	}
	leftName, leftErr := canonicalIngressHostname(left)
	rightName, rightErr := canonicalIngressHostname(right)
	return leftErr == nil && rightErr == nil && leftName == rightName
}

func sameJSON(left, right []byte) bool {
	if len(left) == 0 || string(left) == "null" {
		return len(right) == 0 || string(right) == "null"
	}
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func sameExtra(left, right map[string]json.RawMessage) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if !sameJSON(value, right[key]) {
			return false
		}
	}
	return true
}

func canonicalIngressHostname(hostname string) (string, error) {
	if strings.HasPrefix(hostname, "*.") {
		wildcard, err := canonicalWildcardHostname(hostname)
		return wildcard, err
	}
	return domainname.NormalizeHostname(hostname)
}

func canonicalWildcardHostname(hostname string) (string, error) {
	if !strings.HasPrefix(hostname, "*.") || strings.Count(hostname, "*") != 1 {
		return "", errors.New("invalid wildcard hostname")
	}
	base, err := domainname.NormalizeDomain(strings.TrimPrefix(hostname, "*."))
	if err != nil {
		return "", err
	}
	return "*." + base, nil
}

func sameIngress(left, right []IngressRule) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if domainname.Canonical(left[i].Hostname) != domainname.Canonical(right[i].Hostname) || left[i].Service != right[i].Service {
			return false
		}
	}
	return true
}

func hasConsoleRouteAndCatchAll(rules []IngressRule, consoleHostname string) bool {
	hasConsole, hasCatchAll := false, false
	for _, rule := range rules {
		if domainname.Canonical(rule.Hostname) == domainname.Canonical(consoleHostname) && (rule.Service == "http://proxy:80" || rule.Service == "http://traefik:8080") {
			hasConsole = true
		}
		if rule.Hostname == "" && rule.Service == "http_status:404" {
			hasCatchAll = true
		}
	}
	return hasConsole && hasCatchAll
}
