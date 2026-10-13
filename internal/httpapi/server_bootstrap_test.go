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
)

func TestBootstrapRateLimitUsesIPAndSessionBuckets(t *testing.T) {
	server := &Server{
		config:  config.Config{AuthRateLimit: 1, AuthRateWindow: time.Minute},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter: ratelimit.NewMemoryLimiter(),
	}
	first := httptest.NewRequest(http.MethodPost, "/v1/bootstrap/verify", nil)
	first.RemoteAddr = "198.51.100.20:41000"
	if !server.allowBootstrapAttempt(httptest.NewRecorder(), first, "verify") {
		t.Fatal("first bootstrap attempt was unexpectedly limited")
	}

	rotatedDimension := httptest.NewRequest(http.MethodPost, "/v1/bootstrap/github/device", nil)
	rotatedDimension.RemoteAddr = "198.51.100.20:41001"
	recorder := httptest.NewRecorder()
	if server.allowBootstrapAttempt(recorder, rotatedDimension, "another-session") {
		t.Fatal("rotating session bypassed bootstrap IP bucket")
	}
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limited response = status=%d retry-after=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
}

func TestBootstrapResponsesAreNotCacheable(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeBootstrapJSON(recorder, http.StatusOK, map[string]bool{"setup_required": true})
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("bootstrap Cache-Control = %q, want no-store", got)
	}

	recorder = httptest.NewRecorder()
	writeBootstrapError(recorder, http.StatusUnauthorized, "invalid_bootstrap_code", "invalid setup code")
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("bootstrap error Cache-Control = %q, want no-store", got)
	}
}

func TestBootstrapUsesDedicatedKeyWithoutFunctionsFallback(t *testing.T) {
	legacy := []byte("legacy-key")
	dedicated := []byte("dedicated-key")
	server := &Server{config: config.Config{FunctionsSecretKey: legacy, BootstrapCLIKey: dedicated}}
	if got := server.bootstrapCLIKey(); string(got) != string(dedicated) {
		t.Fatalf("dedicated key = %q, want dedicated key", got)
	}
	server.config.BootstrapCLIKey = nil
	if got := server.bootstrapCLIKey(); len(got) != 0 {
		t.Fatalf("missing dedicated key returned %q; FunctionsSecretKey must not be reused", got)
	}
}
