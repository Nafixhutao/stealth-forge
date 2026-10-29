package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/httpapi"
	"github.com/Nafixhutao/stealth-forge/internal/ratelimit"
)

// countingLimiter records every Allow call so tests can assert that a route
// actually consults the limiter instead of silently skipping it.
type countingLimiter struct {
	mu    sync.Mutex
	calls []string
	deny  bool
}

func (l *countingLimiter) Allow(_ context.Context, key string, _ int, _ time.Duration) (ratelimit.Decision, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, key)
	if l.deny {
		return ratelimit.Decision{Allowed: false, RetryAfter: time.Minute}, nil
	}
	return ratelimit.Decision{Allowed: true}, nil
}

func (l *countingLimiter) Ping(context.Context) error { return nil }

func (l *countingLimiter) callsFor(operation string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, key := range l.calls {
		if strings.Contains(key, operation) {
			n++
		}
	}
	return n
}

func newOAuthRateLimitServer(t *testing.T, limiter *countingLimiter) (*httptest.Server, *http.Client) {
	t.Helper()
	handler := httpapi.NewWithDependencies(
		config.Config{
			PublicAppURL:            "https://console.example.test",
			OAuthGoogleClientID:     "client-id",
			OAuthGoogleClientSecret: "client-secret",
			CookieSecure:            true,
		},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		httpapi.Dependencies{AuthLimiter: limiter},
	)
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	// The callback answers with a redirect to the console origin, which does
	// not resolve in the test environment; do not follow redirects. Keep the
	// test server's TLS trust so the request itself succeeds.
	client := &http.Client{
		Transport: server.Client().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return server, client
}

// TestOAuthEndpointsRateLimited is the regression guard for the missing gate:
// the start and callback endpoints perform crypto work, cookie writes, provider
// round trips, and session inserts per request, so both must consult the auth
// limiter exactly like the password credential endpoints.
func TestOAuthEndpointsRateLimited(t *testing.T) {
	limiter := &countingLimiter{}
	server, client := newOAuthRateLimitServer(t, limiter)

	start, err := client.Get(server.URL + "/v1/oauth/google/start")
	if err != nil {
		t.Fatalf("start request: %v", err)
	}
	start.Body.Close()
	if start.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d, want 200", start.StatusCode)
	}
	if got := limiter.callsFor("oauth_start"); got < 1 {
		t.Fatalf("oauth start made %d limiter calls, want at least 1", got)
	}

	callback, err := client.Get(server.URL + "/v1/oauth/google/callback?code=x&state=y")
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	callback.Body.Close()
	if callback.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", callback.StatusCode)
	}
	if got := limiter.callsFor("oauth_callback"); got < 1 {
		t.Fatalf("oauth callback made %d limiter calls, want at least 1", got)
	}
}

// TestOAuthRateLimitDenies verifies the gate fails closed: an exhausted bucket
// answers 429 with Retry-After instead of running the handler work.
func TestOAuthRateLimitDenies(t *testing.T) {
	limiter := &countingLimiter{deny: true}
	server, client := newOAuthRateLimitServer(t, limiter)

	start, err := server.Client().Get(server.URL + "/v1/oauth/google/start")
	if err != nil {
		t.Fatalf("start request: %v", err)
	}
	start.Body.Close()
	if start.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("start status = %d, want 429", start.StatusCode)
	}
	if start.Header.Get("Retry-After") == "" {
		t.Fatal("start response is missing Retry-After")
	}

	callback, err := client.Get(server.URL + "/v1/oauth/google/callback?code=x&state=y")
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	callback.Body.Close()
	if callback.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("callback status = %d, want 429", callback.StatusCode)
	}
}
