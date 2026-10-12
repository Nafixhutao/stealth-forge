package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/bootstrap"
	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/httpapi"
	"github.com/Nafixhutao/stealth-forge/internal/migrate"
	"github.com/Nafixhutao/stealth-forge/internal/ratelimit"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInstanceBootstrapIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_BOOTSTRAP_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_BOOTSTRAP_DATABASE_URL to run isolated Instance Owner integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var accountCount, sessionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&accountCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bootstrap_sessions`).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if accountCount != 0 || sessionCount != 0 {
		t.Skip("TEST_BOOTSTRAP_DATABASE_URL must point to an empty throwaway database")
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.NewWithDependencies(
		config.Config{
			SessionCookieName:        "stealth_session",
			SessionTTL:               time.Hour,
			FunctionsSecretKey:       key,
			BootstrapCLIKey:          key,
			GitHubAppClientID:        "Iv1.test-client-id",
			StorageRoot:              t.TempDir(),
			StorageMaxFileSize:       1 << 20,
			StorageDefaultQuotaBytes: 2 << 20,
		},
		repository.New(pool),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		httpapi.Dependencies{AuthLimiter: ratelimit.NoopLimiter{}},
	))
	defer server.Close()

	client := newBootstrapClient(t)
	status := bootstrapRequest(t, client, http.MethodGet, server.URL+"/v1/bootstrap/status", nil, nil)
	if status.StatusCode != http.StatusOK || !strings.Contains(string(status.Body), `"setup_required":true`) {
		t.Fatalf("initial bootstrap status = %d %q", status.StatusCode, status.Body)
	}

	firstSession := createBootstrapSessionForTest(t, client, server.URL, key)
	assertSetupCodeNotStored(t, pool, firstSession.SetupCode)

	invalidCode, err := bootstrap.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	invalid := bootstrapRequest(t, client, http.MethodPost, server.URL+"/v1/bootstrap/verify", map[string]string{"setup_code": invalidCode}, nil)
	if invalid.StatusCode != http.StatusUnauthorized || !strings.Contains(string(invalid.Body), "invalid_bootstrap_code") {
		t.Fatalf("invalid code response = %d %q", invalid.StatusCode, invalid.Body)
	}

	expiredCode, err := bootstrap.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bootstrap_sessions (id,code_hash,expires_at) VALUES ($1,$2,$3)`, uuid.Must(uuid.NewV7()), bootstrap.HashCode(expiredCode), time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	expired := bootstrapRequest(t, client, http.MethodPost, server.URL+"/v1/bootstrap/verify", map[string]string{"setup_code": expiredCode}, nil)
	if expired.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired code status = %d, want 401", expired.StatusCode)
	}

	if _, err := pool.Exec(ctx, `UPDATE bootstrap_sessions SET used_at=now() WHERE code_hash=$1`, bootstrap.HashCode(firstSession.SetupCode)); err != nil {
		t.Fatal(err)
	}
	used := bootstrapRequest(t, client, http.MethodPost, server.URL+"/v1/bootstrap/verify", map[string]string{"setup_code": firstSession.SetupCode}, nil)
	if used.StatusCode != http.StatusUnauthorized {
		t.Fatalf("used code status = %d, want 401", used.StatusCode)
	}

	secondSession := createBootstrapSessionForTest(t, client, server.URL, key)
	assertSetupCodeNotStored(t, pool, secondSession.SetupCode)
	var verification struct {
		AuthorizationSessionID string    `json:"authorization_session_id"`
		ExpiresAt              time.Time `json:"expires_at"`
	}
	verified := bootstrapRequest(t, client, http.MethodPost, server.URL+"/v1/bootstrap/verify", map[string]string{"setup_code": secondSession.SetupCode}, nil)
	if verified.StatusCode != http.StatusOK {
		t.Fatalf("verify setup code status = %d %q", verified.StatusCode, verified.Body)
	}
	if err := json.Unmarshal(verified.Body, &verification); err != nil {
		t.Fatal(err)
	}
}

func TestExistingMigrationSealsWithoutAssigningOwner(t *testing.T) {
	databaseURL := os.Getenv("TEST_BOOTSTRAP_LEGACY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_BOOTSTRAP_LEGACY_DATABASE_URL to run the upgrade migration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var accountCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&accountCount); err != nil {
		t.Fatal(err)
	}
	if accountCount != 0 {
		t.Skip("TEST_BOOTSTRAP_LEGACY_DATABASE_URL must point to an empty throwaway database")
	}

	firstID := uuid.Must(uuid.NewV7())
	secondID := uuid.Must(uuid.NewV7())
	firstCreated := time.Now().UTC().Add(-2 * time.Hour)
	secondCreated := firstCreated.Add(time.Hour)
	for _, account := range []struct {
		id        uuid.UUID
		email     string
		createdAt time.Time
	}{
		{id: firstID, email: "legacy-first@example.test", createdAt: firstCreated},
		{id: secondID, email: "legacy-second@example.test", createdAt: secondCreated},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO accounts (id,email,password_hash,created_at,updated_at) VALUES ($1,$2,$3,$4,$4)`, account.id, account.email, "legacy-test-hash", account.createdAt); err != nil {
			t.Fatal(err)
		}
	}
	organizationID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id,name,slug) VALUES ($1,'Legacy Workspace',$2)`, organizationID, "legacy-workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_plans (organization_id) VALUES ($1)`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id,account_id,role) VALUES ($1,$2,'owner')`, organizationID, firstID); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE account_identities`,
		`DROP TABLE bootstrap_sessions`,
		`DROP TABLE instance_bootstrap`,
		`DROP TABLE instance_roles`,
		`DELETE FROM schema_migrations WHERE name IN ('000039_instance_bootstrap.up.sql','000040_github_instance_bootstrap.up.sql')`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	var ownerCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM instance_roles WHERE role='instance_owner'`).Scan(&ownerCount); err != nil {
		t.Fatal(err)
	}
	if ownerCount != 0 {
		t.Fatalf("legacy migration assigned %d owner(s); existing installations must require explicit adoption", ownerCount)
	}
	var setupRequired bool
	if err := pool.QueryRow(ctx, `SELECT sealed_at IS NULL FROM instance_bootstrap WHERE id=TRUE`).Scan(&setupRequired); err != nil {
		t.Fatal(err)
	}
	if setupRequired {
		t.Fatal("legacy migration reopened first-run setup")
	}
	var sealedReason string
	if err := pool.QueryRow(ctx, `SELECT sealed_reason FROM instance_bootstrap WHERE id=TRUE`).Scan(&sealedReason); err != nil {
		t.Fatal(err)
	}
	if sealedReason != "legacy_installation" {
		t.Fatalf("legacy bootstrap reason = %q", sealedReason)
	}
	var membershipCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND account_id=$2`, organizationID, firstID).Scan(&membershipCount); err != nil {
		t.Fatal(err)
	}
	if membershipCount != 1 {
		t.Fatalf("legacy organization membership count = %d, want 1", membershipCount)
	}
}

type bootstrapSessionTestResponse struct {
	SetupCode string    `json:"setup_code"`
	ExpiresAt time.Time `json:"expires_at"`
}

type bootstrapHTTPTestResponse struct {
	StatusCode int
	Body       []byte
}

func createBootstrapSessionForTest(t *testing.T, client *http.Client, serverURL string, key []byte) bootstrapSessionTestResponse {
	t.Helper()
	response := bootstrapRequest(t, client, http.MethodPost, serverURL+"/v1/bootstrap/sessions", nil, map[string]string{bootstrap.CLIProofHeader: bootstrap.CLIProof(key)})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create bootstrap session status = %d %q", response.StatusCode, response.Body)
	}
	var session bootstrapSessionTestResponse
	if err := json.Unmarshal(response.Body, &session); err != nil {
		t.Fatal(err)
	}
	if !bootstrap.ValidCode(session.SetupCode) || !session.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("invalid bootstrap session response = %#v", session)
	}
	return session
}

func assertSetupCodeNotStored(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT code_hash FROM bootstrap_sessions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var hash []byte
		if err := rows.Scan(&hash); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(hash, []byte(code)) {
			t.Fatalf("plaintext setup code was stored")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func newBootstrapClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func bootstrapRequest(t *testing.T, client *http.Client, method, url string, payload any, headers map[string]string) bootstrapHTTPTestResponse {
	t.Helper()
	response, err := bootstrapRequestRaw(client, method, url, payload, headers)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func bootstrapRequestRaw(client *http.Client, method, url string, payload any, headers map[string]string) (bootstrapHTTPTestResponse, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return bootstrapHTTPTestResponse{}, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return bootstrapHTTPTestResponse{}, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return bootstrapHTTPTestResponse{}, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return bootstrapHTTPTestResponse{}, err
	}
	return bootstrapHTTPTestResponse{StatusCode: response.StatusCode, Body: contents}, nil
}
