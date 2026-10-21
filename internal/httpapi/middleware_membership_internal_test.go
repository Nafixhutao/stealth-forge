package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The middleware is the layer that has to hold even if a repository forgets its
// own role check, so this test isolates it: the downstream handler is a probe
// that records whether it was reached, and no repository authorization runs. A
// stranger's request must be refused by the middleware itself, before the probe.
func TestRequireProjectManagementRefusesNonMembersBeforeTheHandler(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run project membership middleware tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		SessionCookieName:  "stealth_session",
		SessionTTL:         time.Hour,
		StorageRoot:        t.TempDir(),
		StorageMaxFileSize: 1 << 20,
		FunctionsSecretKey: bytes.Repeat([]byte("k"), 32),
	}
	repo := repository.New(pool)
	api := httptest.NewServer(New(cfg, repo, logger))
	defer api.Close()

	ownerClient := newCookieClient(t)
	owner := registerMember(t, ownerClient, api.URL, "middleware-owner")
	strangerClient := newCookieClient(t)
	stranger := registerMember(t, strangerClient, api.URL, "middleware-stranger")

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_account_id IN ($1,$2) OR organization_id IN ($3,$4)`, owner.accountID, stranger.accountID, owner.organizationID, stranger.organizationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id IN ($1,$2)`, owner.organizationID, stranger.organizationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id IN ($1,$2)`, owner.accountID, stranger.accountID)
	})

	var created struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	postJSON(t, ownerClient, api.URL+"/v1/organizations/"+owner.organizationID+"/projects", map[string]string{
		"name": "mw-" + uuid.Must(uuid.NewV7()).String()[:8],
	}, http.StatusCreated, &created)
	if created.Project.ID == "" {
		t.Fatal("the project was not created")
	}

	// A probe that succeeds for anyone the middleware lets through. If it is
	// reached by a stranger, the middleware failed.
	reached := false
	// A real router, because the middleware reads the project id from the route
	// pattern; a bare handler would answer 400 for a missing parameter.
	probeRouter := chi.NewRouter()
	probeRouter.With((&Server{config: cfg, repo: repo, logger: logger}).requireProjectManagement).
		Get("/projects/{projectID}/probe", func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		})
	probe := httptest.NewServer(probeRouter)
	defer probe.Close()
	target := probe.URL + "/projects/" + created.Project.ID + "/probe"

	get := func(client *http.Client) int {
		request, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	if status := get(ownerClient); status != http.StatusOK {
		t.Fatalf("the project's own member got %d, want 200", status)
	}
	if !reached {
		t.Fatal("the probe was not reached for the project's own member")
	}

	reached = false
	if status := get(strangerClient); status != http.StatusNotFound {
		t.Fatalf("a non-member got %d, want 404", status)
	}
	if reached {
		t.Fatal("the middleware let a non-member reach the handler")
	}

	if status := get(newCookieClient(t)); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous request got %d, want 401", status)
	}
	if reached {
		t.Fatal("the middleware let an anonymous request reach the handler")
	}
}

// The helpers below live in this package rather than the external test package:
// this file is white-box so it can call the middleware directly.

type memberAccount struct {
	accountID      string
	organizationID string
	email          string
}

func newCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func registerMember(t *testing.T, client *http.Client, baseURL, label string) memberAccount {
	t.Helper()
	account := memberAccount{
		email: "membership-" + label + "-" + uuid.Must(uuid.NewV7()).String() + "@example.test",
	}
	var response struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	postJSON(t, client, baseURL+"/v1/account/registrations", map[string]string{
		"email":    account.email,
		"password": "correct-horse-battery-staple",
	}, http.StatusCreated, &response)
	account.accountID = response.Account.ID
	account.organizationID = response.Organization.ID
	return account
}

func postJSON(t *testing.T, client *http.Client, url string, payload any, expectedStatus int, target any) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s: expected %d, got %d: %s", url, expectedStatus, response.StatusCode, body)
	}
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
	}
}
