package httpapi_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Stealth-deplover/stealth/internal/config"
	"github.com/Stealth-deplover/stealth/internal/httpapi"
)

// TestOAuthStartPKCEChallengeMatchesCookieVerifier is the regression guard for
// a double-hash bug: the handler once passed the already-derived challenge into
// AuthorizationURL, which re-hashed it, so Google received sha256(challenge)
// while the token exchange sent the verifier. The authorize URL and the state
// cookie must describe the same PKCE pair: S256(cookie verifier) must equal the
// code_challenge Google is asked to bind the code to.
func TestOAuthStartPKCEChallengeMatchesCookieVerifier(t *testing.T) {
	handler := httpapi.NewWithDependencies(
		config.Config{
			PublicAppURL:            "https://console.example.test",
			OAuthGoogleClientID:     "client-id",
			OAuthGoogleClientSecret: "client-secret",
			CookieSecure:            true,
		},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		httpapi.Dependencies{},
	)
	server := httptest.NewTLSServer(handler)
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/v1/oauth/google/start")
	if err != nil {
		t.Fatalf("start request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("start status = %d body=%s", response.StatusCode, body)
	}

	var payload struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode start response: %v", err)
	}

	authorizeURL, err := url.Parse(payload.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	urlChallenge := authorizeURL.Query().Get("code_challenge")
	if urlChallenge == "" || authorizeURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL is missing an S256 challenge: %s", payload.AuthorizationURL)
	}

	// The state cookie carries the verifier; recompute its S256 challenge.
	var stateCookie *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "stealth_oauth_state" {
			stateCookie = cookie
		}
	}
	if stateCookie == nil {
		t.Fatal("start response did not set the oauth state cookie")
	}
	raw, err := base64.RawURLEncoding.DecodeString(stateCookie.Value)
	if err != nil {
		t.Fatalf("decode state cookie: %v", err)
	}
	var state struct {
		Verifier string `json:"verifier"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("unmarshal state cookie: %v", err)
	}
	if state.Verifier == "" {
		t.Fatal("state cookie is missing the PKCE verifier")
	}

	sum := sha256.Sum256([]byte(state.Verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	if urlChallenge != expected {
		t.Fatalf(
			"authorize challenge does not match the cookie verifier (double-hash bug): url=%s expected=%s",
			urlChallenge, expected,
		)
	}
	if strings.Contains(payload.AuthorizationURL, state.Verifier) {
		t.Fatal("authorize URL leaked the raw verifier")
	}
}
