package ingress

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/domainname"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
)

type dynamicConfig struct {
	HTTP dynamicHTTP `yaml:"http"`
}

type dynamicHTTP struct {
	Routers  map[string]dynamicRouter  `yaml:"routers"`
	Services map[string]dynamicService `yaml:"services"`
}

type dynamicRouter struct {
	EntryPoints []string `yaml:"entryPoints"`
	Rule        string   `yaml:"rule"`
	Priority    int      `yaml:"priority"`
	Service     string   `yaml:"service"`
}

type dynamicService struct {
	LoadBalancer dynamicLoadBalancer `yaml:"loadBalancer"`
}

type dynamicLoadBalancer struct {
	Servers        []dynamicServer `yaml:"servers"`
	PassHostHeader bool            `yaml:"passHostHeader"`
}

type dynamicServer struct {
	URL string `yaml:"url"`
}

// Render produces the complete route snapshot. The only user-derived value
// in a Traefik rule is a canonical hostname; all YAML structure is generated
// from typed values rather than string concatenation.
func Render(routes []domain.PlatformRoute, backendURL string) ([]byte, error) {
	if err := validBackendURL(backendURL); err != nil {
		return nil, err
	}
	ordered := append([]domain.PlatformRoute(nil), routes...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Hostname != ordered[j].Hostname {
			return ordered[i].Hostname < ordered[j].Hostname
		}
		return ordered[i].SiteID < ordered[j].SiteID
	})
	// Traefik's file provider rejects empty map sections such as
	// "routers: {}" as standalone elements. A comment-only document is a
	// valid no-op configuration and keeps the provider healthy while no Site
	// has an eligible platform hostname.
	if len(ordered) == 0 {
		contents := []byte("# Stealth platform route snapshot: no eligible Sites\n")
		if err := validateRendered(contents, 0, backendURL); err != nil {
			return nil, err
		}
		return contents, nil
	}
	config := dynamicConfig{HTTP: dynamicHTTP{
		Routers:  make(map[string]dynamicRouter, len(ordered)),
		Services: make(map[string]dynamicService),
	}}
	seenHosts := make(map[string]struct{}, len(ordered))
	seenRouters := make(map[string]struct{}, len(ordered))
	for _, route := range ordered {
		id, err := uuid.Parse(route.SiteID)
		if err != nil || id == uuid.Nil {
			return nil, fmt.Errorf("route Site ID %q is invalid", route.SiteID)
		}
		hostname, err := domainname.NormalizeHostname(route.Hostname)
		if err != nil {
			return nil, fmt.Errorf("route hostname %q is invalid: %w", route.Hostname, err)
		}
		if _, exists := seenHosts[hostname]; exists {
			return nil, fmt.Errorf("duplicate platform route hostname %q", hostname)
		}
		seenHosts[hostname] = struct{}{}
		routerID := "stealth-site-" + strings.ReplaceAll(id.String(), "-", "")
		if _, exists := seenRouters[routerID]; exists {
			return nil, fmt.Errorf("duplicate platform route router %q", routerID)
		}
		seenRouters[routerID] = struct{}{}
		config.HTTP.Routers[routerID] = dynamicRouter{
			EntryPoints: []string{"web"},
			Rule:        "Host(`" + hostname + "`)",
			Priority:    100,
			Service:     "stealth-platform-sites",
		}
	}
	if len(ordered) > 0 {
		config.HTTP.Services["stealth-platform-sites"] = dynamicService{LoadBalancer: dynamicLoadBalancer{
			Servers:        []dynamicServer{{URL: backendURL}},
			PassHostHeader: true,
		}}
	}
	contents, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	if err := validateRendered(contents, len(ordered), backendURL); err != nil {
		return nil, err
	}
	return contents, nil
}

// RenderApps creates a deterministic App-only file-provider snapshot. Invalid
// rows are excluded individually so one bad App cannot block Site routing or
// retain an obsolete App target in the next complete snapshot.
func RenderApps(routes []domain.AppPlatformRoute) ([]byte, error) {
	ordered := append([]domain.AppPlatformRoute(nil), routes...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Hostname != ordered[j].Hostname {
			return ordered[i].Hostname < ordered[j].Hostname
		}
		return ordered[i].AppID < ordered[j].AppID
	})
	if len(ordered) == 0 {
		return []byte("# Stealth App route snapshot: no eligible Apps\n"), nil
	}
	identityCounts := make(map[string]int, len(ordered))
	for _, route := range ordered {
		if identity, err := uuid.Parse(route.RouteIdentity); err == nil && identity != uuid.Nil && identity.String() == route.RouteIdentity {
			identityCounts[route.RouteIdentity]++
		}
	}
	config := dynamicConfig{HTTP: dynamicHTTP{
		Routers:  make(map[string]dynamicRouter, len(ordered)),
		Services: make(map[string]dynamicService, len(ordered)),
	}}
	seenHosts := make(map[string]struct{}, len(ordered))
	seenApps := make(map[string]struct{}, len(ordered))
	expectedTargets := make(map[string]string, len(ordered))
	for _, route := range ordered {
		id, err := uuid.Parse(route.AppID)
		if err != nil || id == uuid.Nil || route.Port < 1 || route.Port > 65535 {
			continue
		}
		backendHost, validIdentity := repository.AppRuntimeContainerNameForRouteIdentity(id, route.RouteIdentity)
		if !validIdentity || identityCounts[route.RouteIdentity] != 1 {
			continue
		}
		hostname, err := domainname.NormalizeHostname(route.Hostname)
		if err != nil {
			continue
		}
		idText := strings.ReplaceAll(id.String(), "-", "")
		routerID := "stealth-app-" + idText
		serviceID := "stealth-app-service-" + idText
		if _, exists := seenHosts[hostname]; exists {
			continue
		}
		if _, exists := seenApps[routerID]; exists {
			continue
		}
		seenHosts[hostname] = struct{}{}
		seenApps[routerID] = struct{}{}
		backend := "http://" + net.JoinHostPort(backendHost, fmt.Sprint(route.Port))
		expectedTargets[routerID] = backendHost
		config.HTTP.Routers[routerID] = dynamicRouter{
			EntryPoints: []string{"web"}, Rule: "Host(`" + hostname + "`)",
			Priority: 100, Service: serviceID,
		}
		config.HTTP.Services[serviceID] = dynamicService{LoadBalancer: dynamicLoadBalancer{
			Servers: []dynamicServer{{URL: backend}}, PassHostHeader: true,
		}}
	}
	if len(config.HTTP.Routers) == 0 {
		return []byte("# Stealth App route snapshot: no eligible Apps\n"), nil
	}
	contents, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	if err := validateAppRendered(contents, len(config.HTTP.Routers), len(config.HTTP.Services), expectedTargets); err != nil {
		return nil, err
	}
	return contents, nil
}

func validateAppRendered(contents []byte, routerCount, serviceCount int, expectedTargets map[string]string) error {
	var parsed dynamicConfig
	if err := yaml.Unmarshal(contents, &parsed); err != nil {
		return fmt.Errorf("generated App Traefik YAML is invalid: %w", err)
	}
	if len(parsed.HTTP.Routers) != routerCount || len(parsed.HTTP.Services) != serviceCount || routerCount != serviceCount {
		return errors.New("generated App route and service counts do not match")
	}
	for routerID, router := range parsed.HTTP.Routers {
		appID, idErr := uuid.Parse(strings.TrimPrefix(routerID, "stealth-app-"))
		if !strings.HasPrefix(routerID, "stealth-app-") || len(router.EntryPoints) != 1 || router.EntryPoints[0] != "web" ||
			router.Service != "stealth-app-service-"+strings.TrimPrefix(routerID, "stealth-app-") ||
			idErr != nil || appID == uuid.Nil || routerID != "stealth-app-"+strings.ReplaceAll(appID.String(), "-", "") ||
			!strings.HasPrefix(router.Rule, "Host(`") || !strings.HasSuffix(router.Rule, "`)") {
			return fmt.Errorf("generated App router %q is invalid", routerID)
		}
		expectedHost := expectedTargets[routerID]
		service, ok := parsed.HTTP.Services[router.Service]
		if expectedHost == "" || !ok || !service.LoadBalancer.PassHostHeader || len(service.LoadBalancer.Servers) != 1 || !validAppBackendURL(service.LoadBalancer.Servers[0].URL, expectedHost) {
			return fmt.Errorf("generated App backend for %q is invalid", routerID)
		}
	}
	return nil
}

func validAppBackendURL(raw, expectedHost string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portText)
	return err == nil && host == expectedHost && port >= 1 && port <= 65535
}

func validBackendURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("platform Site backend URL must be an HTTP(S) origin without credentials or path")
	}
	return nil
}

func validateRendered(contents []byte, routeCount int, backendURL string) error {
	var parsed dynamicConfig
	if err := yaml.Unmarshal(contents, &parsed); err != nil {
		return fmt.Errorf("generated Traefik YAML is invalid: %w", err)
	}
	if len(parsed.HTTP.Routers) != routeCount {
		return fmt.Errorf("generated Traefik router count = %d, want %d", len(parsed.HTTP.Routers), routeCount)
	}
	if routeCount == 0 {
		if len(parsed.HTTP.Services) != 0 {
			return errors.New("empty platform snapshot contains a service")
		}
		return nil
	}
	service, ok := parsed.HTTP.Services["stealth-platform-sites"]
	if !ok || !service.LoadBalancer.PassHostHeader || len(service.LoadBalancer.Servers) != 1 || service.LoadBalancer.Servers[0].URL != backendURL {
		return errors.New("generated platform service is invalid")
	}
	for routerID, router := range parsed.HTTP.Routers {
		if !strings.HasPrefix(routerID, "stealth-site-") || len(router.EntryPoints) != 1 || router.EntryPoints[0] != "web" || router.Service != "stealth-platform-sites" || !strings.HasPrefix(router.Rule, "Host(`") || !strings.HasSuffix(router.Rule, "`)") {
			return fmt.Errorf("generated platform router %q is invalid", routerID)
		}
	}
	return nil
}
