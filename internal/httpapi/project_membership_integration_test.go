package httpapi_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/httpapi"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A console session is not authority over a project on its own: the account has
// to belong to the project's organization. Every project route sits behind one
// middleware, so this is the layer where that has to be true — a repository that
// forgot its own role check must not become a cross-tenant read.
func TestProjectRoutesRequireOrganizationMembership(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run project authorization integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.New(config.Config{
		SessionCookieName:  "stealth_session",
		SessionTTL:         time.Hour,
		StorageRoot:        t.TempDir(),
		StorageMaxFileSize: 1 << 20,
		FunctionsSecretKey: bytes.Repeat([]byte("k"), 32),
	}, repository.New(pool), logger))
	defer server.Close()

	ownerClient := newIntegrationClient(t)
	owner := registerAdminTestAccount(t, ownerClient, server.URL, "project-owner")

	strangerClient := newIntegrationClient(t)
	stranger := registerAdminTestAccount(t, strangerClient, server.URL, "project-stranger")

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
	requestJSON(t, ownerClient, http.MethodPost, server.URL+"/v1/organizations/"+owner.organizationID+"/projects", map[string]string{
		"name": "membership-" + uuid.Must(uuid.NewV7()).String()[:8],
	}, http.StatusCreated, &created)
	if created.Project.ID == "" {
		t.Fatal("the project was not created")
	}
	projectURL := server.URL + "/v1/projects/" + created.Project.ID + "/sites"

	// The owner reads their own project.
	requestJSON(t, ownerClient, http.MethodGet, projectURL, nil, http.StatusOK, nil)

	// A signed-in account from another organization gets the same answer as for
	// a project that does not exist, so the route reveals nothing.
	requestJSON(t, strangerClient, http.MethodGet, projectURL, nil, http.StatusNotFound, nil)

	// No session at all is still an authentication failure, not a 404.
	requestJSON(t, newIntegrationClient(t), http.MethodGet, projectURL, nil, http.StatusUnauthorized, nil)
}
