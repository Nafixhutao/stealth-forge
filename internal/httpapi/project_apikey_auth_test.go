package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/ratelimit"
	"github.com/google/uuid"
)

// A malformed key must be refused before the repository is consulted, and must
// still count against the failed-auth limiter. The nil repository below proves
// the first half: reaching it would panic.
func TestAuthenticateProjectAPIKeyRejectsMalformedSecretBeforeLookup(t *testing.T) {
	server := &Server{
		config:  config.Config{AuthRateLimit: 1, AuthRateWindow: time.Minute},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter: ratelimit.NewMemoryLimiter(),
	}
	projectID := uuid.MustParse("018f27e3-5d1a-7c44-ae35-1db4ea12e6d2")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "198.51.100.9:41000"
	if _, ok := server.authenticateProjectAPIKey(recorder, request, projectID, "not-a-key"); ok {
		t.Fatal("malformed key was accepted")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("got status=%d, want 401", recorder.Code)
	}

	// The limit is one per window, so the same client is now refused by the
	// limiter rather than by key validation. That only holds if the malformed
	// attempt above was counted.
	limited := httptest.NewRecorder()
	second := httptest.NewRequest(http.MethodGet, "/", nil)
	second.RemoteAddr = "198.51.100.9:41001"
	if _, ok := server.authenticateProjectAPIKey(limited, second, projectID, "not-a-key"); ok {
		t.Fatal("second malformed attempt was accepted")
	}
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("got status=%d retry-after=%q, want 429 with Retry-After", limited.Code, limited.Header().Get("Retry-After"))
	}
}

// A failed-auth limiter that cannot answer must fail closed: the helper reports
// the 503 and never falls through to the repository.
func TestAuthenticateProjectAPIKeyFailsClosedWhenLimiterUnavailable(t *testing.T) {
	server := &Server{
		config:  config.Config{AuthRateLimit: 1, AuthRateWindow: time.Minute},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter: ratelimit.UnavailableLimiter{},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "198.51.100.10:41000"
	if _, ok := server.authenticateProjectAPIKey(
		recorder,
		request,
		uuid.MustParse("018f27e3-5d1a-7c44-ae35-1db4ea12e6d2"),
		"not-a-key",
	); ok {
		t.Fatal("unavailable limiter unexpectedly accepted a malformed key")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status=%d, want 503", recorder.Code)
	}
}
