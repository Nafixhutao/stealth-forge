package monitoring

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func validatePublicURL(ctx context.Context, value *url.URL) error {
	return validatePublicURLWithResolver(ctx, net.DefaultResolver, value)
}

func validatePublicURLWithResolver(ctx context.Context, resolver ipResolver, value *url.URL) error {
	if value == nil || (value.Scheme != "http" && value.Scheme != "https") || value.Hostname() == "" || value.User != nil || value.Fragment != "" {
		return errors.New("monitor URL is invalid")
	}
	if _, err := resolvePublicHostWithResolver(ctx, resolver, value.Hostname()); err != nil {
		return err
	}
	return nil
}

// ValidatePublicHTTPSURL is shared by trusted outbound workers such as alert
// notifications. It preserves the monitor SSRF boundary while allowing the
// query parameters used by provider webhook endpoints.
func ValidatePublicHTTPSURL(ctx context.Context, value *url.URL) error {
	return validatePublicHTTPSURLWithResolver(ctx, net.DefaultResolver, value)
}

func validatePublicHTTPSURLWithResolver(ctx context.Context, resolver ipResolver, value *url.URL) error {
	if value == nil || value.Scheme != "https" || value.Hostname() == "" || value.User != nil || value.Fragment != "" {
		return errors.New("notification URL is invalid")
	}
	if _, err := resolvePublicHostWithResolver(ctx, resolver, value.Hostname()); err != nil {
		return err
	}
	return nil
}

// NewSafeHTTPSClient is the trusted outbound client for owner-configured
// notification endpoints. It resolves and dials public addresses only and
// validates every redirect, so a notification URL cannot become an SSRF
// primitive for the worker.
func NewSafeHTTPSClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           safeDialContext,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			MaxIdleConnsPerHost:   2,
		},
	}
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		return ValidatePublicHTTPSURL(next.Context(), next.URL)
	}
	return client
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("monitor network address is invalid")
	}
	ips, err := resolvePublicHost(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{}
	var lastErr error
	for _, ip := range ips {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = errors.New("monitor host has no public address")
	}
	return nil, lastErr
}

type ipResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

func resolvePublicHost(ctx context.Context, host string) ([]net.IP, error) {
	return resolvePublicHostWithResolver(ctx, net.DefaultResolver, host)
}

func resolvePublicHostWithResolver(ctx context.Context, resolver ipResolver, host string) ([]net.IP, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if isPublicIP(ip) {
			return []net.IP{ip}, nil
		}
		return nil, errors.New("monitor target resolves to a private address")
	}
	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, errors.New("monitor target could not be resolved")
	}
	if len(ips) == 0 {
		return nil, errors.New("monitor target could not be resolved")
	}
	public := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, errors.New("monitor target resolves to a private address")
		}
		public = append(public, ip)
	}
	return public, nil
}

func isPublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	if address.Is4In6() {
		address = address.Unmap()
	}
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range monitorDeniedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// monitorDeniedPrefixes is the explicit monitor egress policy. The standard
// library's IsPrivate and IsGlobalUnicast methods intentionally do not cover
// every special-use allocation, so the monitor policy denies the documented
// shared, documentation, benchmarking, reserved, and non-global ranges here.
// The list is maintained against the IANA IPv4/IPv6 special-purpose registries
// and deliberately errs on the side of rejecting special-purpose destinations.
