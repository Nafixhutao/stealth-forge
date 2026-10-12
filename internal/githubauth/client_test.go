package githubauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHTTPClientGetUserValidatesIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"id": 0, "login": ""})
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.UserURL = server.URL
	if _, err := client.GetUser(context.Background(), "token"); err == nil {
		t.Fatal("incomplete GitHub identity was accepted")
	}
}

func TestAuthorizationURLUsesHTTPSCallbackAndPKCE(t *testing.T) {
	verifier, challenge, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier) != 43 || len(challenge) != 43 || strings.ContainsAny(verifier+challenge, "+/=\r\n") {
		t.Fatalf("PKCE values are not URL-safe: verifier=%q challenge=%q", verifier, challenge)
	}
	location, err := AuthorizationURL("Iv1.test-client", "https://setup.example.test/v1/setup/github/authorize/callback", "callback-state", challenge)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "github.com" || parsed.Path != "/login/oauth/authorize" || query.Get("client_id") != "Iv1.test-client" || query.Get("redirect_uri") != "https://setup.example.test/v1/setup/github/authorize/callback" || query.Get("state") != "callback-state" || query.Get("code_challenge") != challenge || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL = %q", location)
	}
	if _, err := AuthorizationURL("Iv1.test-client", "http://setup.example.test/callback", "state", challenge); err == nil {
		t.Fatal("AuthorizationURL accepted an HTTP callback")
	}
}

func TestHTTPClientExchangesBrowserAuthorizationCodeWithoutReturningProviderSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/token" || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("OAuth exchange request = %s %s headers=%#v", request.Method, request.URL, request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		form, err := url.ParseQuery(string(body))
		if err != nil || form.Get("client_id") != "Iv1.test-client" || form.Get("client_secret") != "client-secret" || form.Get("code") != "one-time-code" || form.Get("redirect_uri") != "https://setup.example.test/callback" || form.Get("code_verifier") != "verifier-abcdefghijklmnopqrstuvwxyz-0123456789" {
			t.Fatalf("OAuth exchange form = %q", body)
		}
		_, _ = io.WriteString(writer, `{"access_token":"server-only-token","token_type":"bearer"}`)
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.TokenURL = server.URL + "/token"
	result, err := client.ExchangeAuthorizationCode(context.Background(), "Iv1.test-client", "client-secret", "one-time-code", "https://setup.example.test/callback", "verifier-abcdefghijklmnopqrstuvwxyz-0123456789")
	if err != nil || result.AccessToken != "server-only-token" {
		t.Fatalf("OAuth exchange = %#v, %v", result, err)
	}
}

func TestHTTPClientBrowserAuthorizationProviderFailureIsGeneric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":"bad_verification_code","error_description":"one-time-code"}`)
	}))
	defer server.Close()
	client := NewClient(server.Client())
	client.TokenURL = server.URL
	_, err := client.ExchangeAuthorizationCode(context.Background(), "Iv1.test-client", "client-secret", "one-time-code", "https://setup.example.test/callback", "verifier-abcdefghijklmnopqrstuvwxyz-0123456789")
	if err == nil || strings.Contains(err.Error(), "one-time-code") || strings.Contains(err.Error(), "client-secret") {
		t.Fatalf("OAuth provider error = %v", err)
	}
}
