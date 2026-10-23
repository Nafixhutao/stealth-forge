package httpapi

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
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A project-scoped read must answer the same way whether the project does not
// exist or the caller is simply not a member of it. Answering 404 for one and
// 403 for the other turns the endpoint into an existence oracle: an attacker
// with a valid session can walk guessed project UUIDs and learn which ones are
// real, which is the reconnaissance step before any targeted request.
//
// This test walks every project-scoped collection route a signed-in stranger
// can reach, and requires the two answers to match.
func TestProjectScopedReadsDoNotRevealWhetherAProjectExists(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run project visibility tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	owner := registerMember(t, ownerClient, api.URL, "visibility-owner")
	strangerClient := newCookieClient(t)
	stranger := registerMember(t, strangerClient, api.URL, "visibility-stranger")
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
		"name": "visibility-" + uuid.Must(uuid.NewV7()).String()[:8],
	}, http.StatusCreated, &created)
	if created.Project.ID == "" {
		t.Fatal("the project was not created")
	}

	// Every project-scoped collection a Console session can list. Detail
	// routes are excluded: they need a resource id that only exists inside a
	// real project, so a stranger cannot reach them without knowing one.
	routes := []string{
		"/v1/projects/%s",
		"/v1/projects/%s/users",
		"/v1/projects/%s/audit-events",
		"/v1/projects/%s/traces",
		"/v1/projects/%s/sites",
		"/v1/projects/%s/apps",
		"/v1/projects/%s/databases",
		"/v1/projects/%s/storage/buckets",
		"/v1/projects/%s/functions",
		"/v1/projects/%s/messaging/providers",
		"/v1/projects/%s/messaging/topics",
		"/v1/projects/%s/api-keys",
		"/v1/projects/%s/webhooks",
		"/v1/projects/%s/agents",
		"/v1/projects/%s/usage",
		"/v1/projects/%s/auth-settings",
		"/v1/projects/%s/service-layout",
	}

	missingProject := uuid.Must(uuid.NewV7()).String()
	for _, route := range routes {
		existing := strangerStatus(t, strangerClient, api.URL+projectPath(route, created.Project.ID))
		absent := strangerStatus(t, strangerClient, api.URL+projectPath(route, missingProject))
		if existing != absent {
			t.Errorf(
				"%s answers %d for a project the caller cannot see and %d for a project that does not exist; the difference reveals which UUIDs are real",
				route,
				existing,
				absent,
			)
		}
	}
}

// strangerStatus performs the request as the signed-in non-member and returns
// only the status. The body is discarded so a leaked payload cannot influence
// the comparison.
func strangerStatus(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

// projectPath substitutes the single %s placeholder without going through
// fmt, so the route table above stays readable and cannot be mistyped into a
// format string with the wrong number of verbs.
func projectPath(pattern, projectID string) string {
	for index := 0; index < len(pattern)-1; index++ {
		if pattern[index] == '%' && pattern[index+1] == 's' {
			return pattern[:index] + projectID + pattern[index+2:]
		}
	}
	return pattern
}
