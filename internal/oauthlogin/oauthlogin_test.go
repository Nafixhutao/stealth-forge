package oauthlogin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestNewPKCEProducesS256Pair(t *testing.T) {
	verifier, challenge, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if len(verifier) != 43 {
		t.Fatalf("verifier length = %d, want 43 (RFC 7636)", len(verifier))
	}
	if len(challenge) != 43 {
		t.Fatalf("challenge length = %d, want 43", len(challenge))
	}
	if verifier == challenge {
		t.Fatal("verifier and challenge must differ")
	}
	// The challenge must be the S256 hash of the verifier.
	again, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil {
		t.Fatalf("challenge is not base64url: %v", err)
	}
	if len(again) != 32 {
		t.Fatalf("challenge decodes to %d bytes, want 32", len(again))
	}
}

func TestNewStateIsUnguessable(t *testing.T) {
	first, err := NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	second, err := NewState()
	if err != nil {
		t.Fatalf("NewState: %v", err)
	}
	if first == second {
		t.Fatal("two states must not collide")
	}
	if len(first) != 43 {
		t.Fatalf("state length = %d, want 43", len(first))
	}
}

func TestProviderConfiguredRequiresBothCredentials(t *testing.T) {
	if GitHub("", "").Configured() {
		t.Fatal("empty credentials must report unconfigured")
	}
	if GitHub("client", "").Configured() {
		t.Fatal("missing secret must report unconfigured")
	}
	if !GitHub("client", "secret").Configured() {
		t.Fatal("complete credentials must report configured")
	}
	if !Google("client", "secret").Configured() {
		t.Fatal("Google with complete credentials must report configured")
	}
}

func TestAuthorizationURLIncludesPKCEAndScopes(t *testing.T) {
	p := GitHub("client-id", "secret")
	verifier, challenge, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	raw := p.AuthorizationURL("https://console.example.test/v1/oauth/github/callback", "state-value", verifier)
	for _, want := range []string{
		"https://github.com/login/oauth/authorize?",
		"client_id=client-id",
		"code_challenge=" + challenge,
		"code_challenge_method=S256",
		"response_type=code",
		"state=state-value",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("authorization URL missing %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, "secret") {
		t.Fatal("authorization URL must never contain the client secret")
	}
	if strings.Contains(raw, verifier) {
		t.Fatal("authorization URL must carry the challenge, never the verifier")
	}
}

func TestGoogleAuthorizationURLUsesGoogleEndpoint(t *testing.T) {
	p := Google("client-id", "secret")
	raw := p.AuthorizationURL("https://console.example.test/v1/oauth/google/callback", "s", "c")
	if !strings.HasPrefix(raw, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Fatalf("unexpected Google authorize URL: %s", raw)
	}
	if !strings.Contains(raw, "scope=openid+email+profile") {
		t.Fatalf("Google scopes missing: %s", raw)
	}
}

func TestExchangeParsesAccessToken(t *testing.T) {
	var gotForm map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "token-123"})
	}))
	defer server.Close()

	p := Provider{
		Name:         "test",
		Endpoint:     oauth2.Endpoint{AuthURL: server.URL + "/auth", TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams},
		ClientID:     "cid",
		ClientSecret: "csecret",
		kind:         kindGoogle,
	}
	token, err := NewClient(server.Client()).Exchange(context.Background(), p, "code-1", "https://cb.example.test", "verifier-1")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if token.AccessToken != "token-123" {
		t.Fatalf("access token = %q, want token-123", token.AccessToken)
	}
	for key, want := range map[string]string{
		"client_id":     "cid",
		"client_secret": "csecret",
		"code":          "code-1",
		"code_verifier": "verifier-1",
		"grant_type":    "authorization_code",
		"redirect_uri":  "https://cb.example.test",
	} {
		values := gotForm[key]
		if len(values) != 1 || values[0] != want {
			t.Fatalf("form %s = %v, want %q", key, values, want)
		}
	}
}

func TestExchangeSurfacesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad_verification_code"})
	}))
	defer server.Close()

	p := Provider{
		Name:         "test",
		Endpoint:     oauth2.Endpoint{AuthURL: server.URL + "/auth", TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams},
		ClientID:     "cid",
		ClientSecret: "s",
	}
	if _, err := NewClient(server.Client()).Exchange(context.Background(), p, "code", "https://cb", ""); err == nil {
		t.Fatal("expected an error for a provider error payload")
	}
}

func TestUserInfoParsesGitHubIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-abc" {
			t.Errorf("Authorization header = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 4242, "login": "octocat", "name": "Mona", "email": "mona@example.test", "avatar_url": "https://avatars.example.test/u/4242",
		})
	}))
	defer server.Close()

	p := Provider{Name: "github", UserInfoURL: server.URL, kind: kindGitHub}
	identity, err := NewClient(server.Client()).UserInfo(context.Background(), p, &oauth2.Token{AccessToken: "token-abc"})
	if err != nil {
		t.Fatalf("UserInfo: %v", err)
	}
	if identity.ProviderUserID != "4242" || identity.Login != "octocat" || identity.DisplayName != "Mona" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestUserInfoParsesGoogleIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "1122334455", "email": "user@example.test", "name": "User", "picture": "https://lh3.example.test/p",
		})
	}))
	defer server.Close()

	p := Provider{Name: "google", UserInfoURL: server.URL, kind: kindGoogle}
	identity, err := NewClient(server.Client()).UserInfo(context.Background(), p, &oauth2.Token{AccessToken: "token"})
	if err != nil {
		t.Fatalf("UserInfo: %v", err)
	}
	if identity.ProviderUserID != "1122334455" || identity.Email != "user@example.test" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestUserInfoRejectsMissingIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
	}))
	defer server.Close()

	p := Provider{Name: "github", UserInfoURL: server.URL, kind: kindGitHub}
	if _, err := NewClient(server.Client()).UserInfo(context.Background(), p, &oauth2.Token{AccessToken: "token"}); err == nil {
		t.Fatal("expected an error when the provider omits the numeric id")
	}
}

func TestUserInfoRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p := Provider{Name: "github", UserInfoURL: server.URL, kind: kindGitHub}
	if _, err := NewClient(server.Client()).UserInfo(context.Background(), p, &oauth2.Token{AccessToken: "token"}); err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}
