package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/apikey"
	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/httpapi"
	"github.com/Nafixhutao/stealth-forge/internal/migrate"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// authenticateProjectAPIKey is the one place that validates X-Stealth-Key, but
// four middlewares call it and each builds a different actor from the result.
// A change that works for one of them can therefore still break another, so
// this test drives the same credential set through all four and requires the
// same answer from each.
//
// Representative route per middleware, taken from the route tables:
//
//	requireProjectManagement   GET  /projects/{id}/users                                  (routes_projects.go)
//	requireFunctionExecutionActor POST /projects/{id}/functions/{id}/executions           (routes_functions.go)
//	requireProjectDataActor    GET  /projects/{id}/databases/{id}/tables/{id}/rows        (routes_databases.go)
//	requireProjectStorageActor GET  /projects/{id}/storage/buckets/{id}/files             (routes_storage.go)
func TestProjectAPIKeyAuthSharedAcrossMiddlewaresIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	fixture := newAPIKeyAuthFixture(t, ctx, pool)
	fixture.runCredentialMatrix(t)
	fixture.runRateLimit(t)
}

// apiKeyAuthFixture owns everything the matrix needs: two projects, one
// resource per middleware, and the keys in each credential state.
type apiKeyAuthFixture struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	server *httptest.Server

	ownerClient *http.Client
	projectURL  string
	projectID   string

	// routes holds one URL per middleware, plus the status a fully scoped key
	// receives. The status is what the handler returns, not what the
	// middleware decides: 200 for reads, 202 for a queued execution.
	usersURL      string
	executionsURL string
	rowsURL       string
	filesURL      string

	fullKey      string // every scope the four routes need
	unrelatedKey string // a valid key with none of them
	unknownKey   string // well-formed secret that was never stored
	revokedKey   string // correct scopes, revoked
	expiredKey   string // correct scopes, expired
	foreignKey   string // correct scopes, but stored against another project
}

func newAPIKeyAuthFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *apiKeyAuthFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewWithDependencies(config.Config{
		StorageRoot:                t.TempDir(),
		StorageMaxFileSize:         1 << 20,
		StorageDefaultQuotaBytes:   1 << 20,
		FunctionsMaxArtifactSize:   1 << 20,
		FunctionsDefaultQuotaBytes: 1 << 20,
		FunctionsSecretKey:         make([]byte, 32),
		SessionCookieName:          "stealth_session",
		SessionTTL:                 time.Hour,
		AppSessionTTL:              time.Hour,
		AuthRateLimit:              100,
		AuthRateWindow:             time.Minute,
	}, repository.New(pool), logger, httpapi.Dependencies{AuthLimiter: integrationLimiter(t, ctx)}))
	t.Cleanup(server.Close)

	ownerClient := newIntegrationClient(t)
	ownerID := uuid.Must(uuid.NewV7())
	var registration struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	requestJSON(t, ownerClient, http.MethodPost, server.URL+"/v1/account/registrations", map[string]string{
		"email":    fmt.Sprintf("api-key-auth-owner-%s@example.test", ownerID),
		"password": "correct-horse-battery-staple",
	}, http.StatusCreated, &registration)

	projectID := createAPIKeyAuthProject(t, ownerClient, server.URL, registration.Organization.ID, "primary")
	projectURL := server.URL + "/v1/projects/" + projectID

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM audit_events WHERE organization_id=$1 OR actor_account_id=$2`, registration.Organization.ID, registration.Account.ID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=$1`, registration.Organization.ID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM accounts WHERE id=$1`, registration.Account.ID)
	})

	fixture := &apiKeyAuthFixture{
		t:           t,
		ctx:         ctx,
		pool:        pool,
		server:      server,
		ownerClient: ownerClient,
		projectURL:  projectURL,
		projectID:   projectID,
	}

	// The scopes each route needs: users.read, functions.write, databases.read,
	// storage.read. One key carries all four; another carries none of them.
	fixture.fullKey = fixture.createKey("Full access key", []string{
		"users.read", "functions.write", "databases.read", "storage.read",
	})
	fixture.unrelatedKey = fixture.createKey("Unrelated scope key", []string{"storage.write"})
	fixture.unknownKey = unusedAPIKeySecret(t)

	fixture.revokedKey = fixture.insertKeyState("Revoked key", "revoked")
	fixture.expiredKey = fixture.insertKeyState("Expired key", "expired")

	foreignProjectID := createAPIKeyAuthProject(t, ownerClient, server.URL, registration.Organization.ID, "secondary")
	foreignURL := server.URL + "/v1/projects/" + foreignProjectID
	fixture.foreignKey = createAPIKeyFor(t, ownerClient, foreignURL, "Foreign project key", []string{
		"users.read", "functions.write", "databases.read", "storage.read",
	})

	fixture.usersURL = projectURL + "/users"
	fixture.executionsURL = fixture.createExecutableFunction()
	fixture.rowsURL = fixture.createDatabaseTable()
	fixture.filesURL = fixture.createStorageBucket()
	return fixture
}

func createAPIKeyAuthProject(t *testing.T, client *http.Client, baseURL, organizationID, label string) string {
	t.Helper()
	var created struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	requestJSON(t, client, http.MethodPost, baseURL+"/v1/organizations/"+organizationID+"/projects", map[string]string{
		"name": fmt.Sprintf("%s-%s", label, uuid.Must(uuid.NewV7()).String()[:8]),
	}, http.StatusCreated, &created)
	if created.Project.ID == "" {
		t.Fatal("project was not created")
	}
	return created.Project.ID
}

func createAPIKeyFor(t *testing.T, client *http.Client, projectURL, name string, scopes []string) string {
	t.Helper()
	var created struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
		Secret string `json:"secret"`
	}
	requestJSON(t, client, http.MethodPost, projectURL+"/api-keys", map[string]any{
		"name":   name,
		"scopes": scopes,
	}, http.StatusCreated, &created)
	if created.Secret == "" {
		t.Fatalf("API key %q has no secret", name)
	}
	return created.Secret
}

func (f *apiKeyAuthFixture) createKey(name string, scopes []string) string {
	f.t.Helper()
	return createAPIKeyFor(f.t, f.ownerClient, f.projectURL, name, scopes)
}

// unusedAPIKeySecret returns a secret that passes apikey.ValidateSecret but was
// never written to the database, which is the "unknown key" case.
func unusedAPIKeySecret(t *testing.T) string {
	t.Helper()
	secret, _, _, err := apikey.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

// insertKeyState stores a key directly so its revoked_at or expires_at can be
// set to a state the create endpoint cannot produce.
func (f *apiKeyAuthFixture) insertKeyState(name, state string) string {
	f.t.Helper()
	secret, prefix, hash, err := apikey.NewSecret()
	if err != nil {
		f.t.Fatal(err)
	}
	var revokedAt, expiresAt any
	switch state {
	case "revoked":
		revokedAt = time.Now().UTC()
	case "expired":
		expiresAt = time.Now().UTC().Add(-time.Minute)
	default:
		f.t.Fatalf("unknown key state %q", state)
	}
	// Stored scopes must be canonical (sorted); the database enforces that with
	// `stealth_api_key_scopes_canonical`, which the API's own parser guarantees.
	scopes := []string{"databases.read", "functions.write", "storage.read", "users.read"}
	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO project_api_keys (id,project_id,name,prefix,secret_hash,scopes,revoked_at,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		uuid.Must(uuid.NewV7()), f.projectID, name, prefix, hash, scopes, revokedAt, expiresAt); err != nil {
		f.t.Fatal(err)
	}
	return secret
}

// createExecutableFunction builds the minimum a function execution needs: a
// function with an activated deployment. Without one the route answers 409
// before it ever reaches the scope check.
func (f *apiKeyAuthFixture) createExecutableFunction() string {
	f.t.Helper()
	var created struct {
		Function struct {
			ID string `json:"id"`
		} `json:"function"`
	}
	requestJSON(f.t, f.ownerClient, http.MethodPost, f.projectURL+"/functions", map[string]any{
		"name":       "api-key-auth-function",
		"runtime":    "node-22",
		"entrypoint": "src/main.js",
	}, http.StatusCreated, &created)
	functionsURL := f.projectURL + "/functions/" + created.Function.ID

	deploymentBody := uploadFunctionMultipart(
		f.t,
		f.ownerClient,
		functionsURL+"/deployments",
		"source.zip",
		[]byte("api-key-auth-source"),
		nil,
		http.StatusCreated,
	)
	var deployment struct {
		Deployment struct {
			ID string `json:"id"`
		} `json:"deployment"`
	}
	if err := json.Unmarshal(deploymentBody, &deployment); err != nil {
		f.t.Fatal(err)
	}
	requestJSON(f.t, f.ownerClient, http.MethodPost, functionsURL+"/deployments/"+deployment.Deployment.ID+"/activate", nil, http.StatusOK, nil)

	return functionsURL + "/executions"
}

func (f *apiKeyAuthFixture) createDatabaseTable() string {
	f.t.Helper()
	var database struct {
		Database struct {
			ID string `json:"id"`
		} `json:"database"`
	}
	requestJSON(f.t, f.ownerClient, http.MethodPost, f.projectURL+"/databases", map[string]string{
		"name": "API key auth data",
	}, http.StatusCreated, &database)
	databaseURL := f.projectURL + "/databases/" + database.Database.ID

	var table struct {
		Table struct {
			ID string `json:"id"`
		} `json:"table"`
	}
	requestJSON(f.t, f.ownerClient, http.MethodPost, databaseURL+"/tables", map[string]any{
		"name":               "Events",
		"row_security":       true,
		"create_permissions": []string{},
		"read_permissions":   []string{},
		"update_permissions": []string{},
		"delete_permissions": []string{},
	}, http.StatusCreated, &table)
	tableURL := databaseURL + "/tables/" + table.Table.ID
	requestJSON(f.t, f.ownerClient, http.MethodPost, tableURL+"/columns", map[string]any{
		"key": "title", "type": "text", "required": true,
	}, http.StatusCreated, nil)

	return tableURL + "/rows"
}

func (f *apiKeyAuthFixture) createStorageBucket() string {
	f.t.Helper()
	return f.projectURL + "/storage/buckets/" + createStorageBucketFor(f.t, f.ownerClient, f.projectURL, "api-key-auth-bucket") + "/files"
}

// createStorageBucketFor creates one bucket and returns its id. Callers build
// the file URL themselves because the rate-limit case needs the bucket on a
// separate project.
func createStorageBucketFor(t *testing.T, client *http.Client, projectURL, name string) string {
	t.Helper()
	var bucket struct {
		Bucket struct {
			ID string `json:"id"`
		} `json:"bucket"`
	}
	requestJSON(t, client, http.MethodPost, projectURL+"/storage/buckets", map[string]any{
		"name":                name,
		"file_security":       true,
		"create_permissions":  []string{},
		"read_permissions":    []string{},
		"update_permissions":  []string{},
		"delete_permissions":  []string{},
		"max_file_size_bytes": 1024,
		"quota_bytes":         4096,
	}, http.StatusCreated, &bucket)
	return bucket.Bucket.ID
}

type apiKeyAuthRoute struct {
	middleware string
	method     string
	url        string
	body       any
	// allowedStatus is what the handler returns for a correctly scoped key.
	allowedStatus int
}

func (f *apiKeyAuthFixture) routes() []apiKeyAuthRoute {
	return []apiKeyAuthRoute{
		{middleware: "requireProjectManagement", method: http.MethodGet, url: f.usersURL, allowedStatus: http.StatusOK},
		{middleware: "requireFunctionExecutionActor", method: http.MethodPost, url: f.executionsURL, body: map[string]any{}, allowedStatus: http.StatusAccepted},
		{middleware: "requireProjectDataActor", method: http.MethodGet, url: f.rowsURL, allowedStatus: http.StatusOK},
		{middleware: "requireProjectStorageActor", method: http.MethodGet, url: f.filesURL, allowedStatus: http.StatusOK},
	}
}

// runCredentialMatrix sends every credential state to every route. The four
// middlewares must agree, so the expectations are stated once and applied to
// all of them rather than written per middleware.
func (f *apiKeyAuthFixture) runCredentialMatrix(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		status int
		code   string
	}{
		{name: "valid key with the needed scope", secret: f.fullKey, status: 0},
		{name: "valid key without the needed scope", secret: f.unrelatedKey, status: http.StatusForbidden, code: "forbidden"},
		{name: "malformed key", secret: "not-a-key", status: http.StatusUnauthorized, code: "unauthorized"},
		{name: "well-formed unknown key", secret: f.unknownKey, status: http.StatusUnauthorized, code: "unauthorized"},
		{name: "revoked key", secret: f.revokedKey, status: http.StatusUnauthorized, code: "unauthorized"},
		{name: "expired key", secret: f.expiredKey, status: http.StatusUnauthorized, code: "unauthorized"},
		{name: "key for another project", secret: f.foreignKey, status: http.StatusUnauthorized, code: "unauthorized"},
	}

	for _, route := range f.routes() {
		route := route
		t.Run(route.middleware, func(t *testing.T) {
			for _, testCase := range cases {
				testCase := testCase
				want := testCase.status
				if want == 0 {
					want = route.allowedStatus
				}
				body := requestJSONRawWithHeaders(
					t,
					newIntegrationClient(t),
					route.method,
					route.url,
					route.body,
					want,
					map[string]string{"X-Stealth-Key": testCase.secret},
				)
				if testCase.code == "" {
					continue
				}
				if got := errorCodeOf(t, body); got != testCase.code {
					t.Errorf("%s: error code = %q, want %q (%s)", testCase.name, got, testCase.code, body)
				}
			}
		})
	}
}

func errorCodeOf(t *testing.T, body []byte) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode error envelope from %s: %v", body, err)
	}
	return envelope.Error.Code
}

// runRateLimit proves the failed-auth limiter is shared by the four
// middlewares: the bucket is keyed by project and client IP, so failures on one
// route count towards the next. A valid key never consults the limiter, so it
// keeps working while bad attempts are still under the threshold.
func (f *apiKeyAuthFixture) runRateLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewWithDependencies(config.Config{
		StorageRoot:       t.TempDir(),
		SessionCookieName: "stealth_session",
		SessionTTL:        time.Hour,
		AppSessionTTL:     time.Hour,
		AuthRateLimit:     2,
		AuthRateWindow:    time.Minute,
	}, repository.New(f.pool), logger, httpapi.Dependencies{AuthLimiter: integrationLimiter(t, f.ctx)}))
	defer server.Close()

	ownerClient := newIntegrationClient(t)
	ownerID := uuid.Must(uuid.NewV7())
	var registration struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	requestJSON(t, ownerClient, http.MethodPost, server.URL+"/v1/account/registrations", map[string]string{
		"email":    fmt.Sprintf("api-key-auth-limit-%s@example.test", ownerID),
		"password": "correct-horse-battery-staple",
	}, http.StatusCreated, &registration)
	projectID := createAPIKeyAuthProject(t, ownerClient, server.URL, registration.Organization.ID, "limited")
	projectURL := server.URL + "/v1/projects/" + projectID
	fullKey := createAPIKeyFor(t, ownerClient, projectURL, "Limited full key", []string{"users.read"})
	usersURL := projectURL + "/users"

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		_, _ = f.pool.Exec(cleanupCtx, `DELETE FROM audit_events WHERE organization_id=$1 OR actor_account_id=$2`, registration.Organization.ID, registration.Account.ID)
		_, _ = f.pool.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=$1`, registration.Organization.ID)
		_, _ = f.pool.Exec(cleanupCtx, `DELETE FROM accounts WHERE id=$1`, registration.Account.ID)
	})

	badKey := map[string]string{"X-Stealth-Key": "not-a-key"}
	goodKey := map[string]string{"X-Stealth-Key": fullKey}
	// A second route behind a different middleware, so the shared bucket is
	// observable rather than assumed.
	bucket := createStorageBucketFor(t, ownerClient, projectURL, "limited-bucket")
	storageFilesURL := projectURL + "/storage/buckets/" + bucket + "/files"

	// A valid key never reaches the failed-auth limiter, so it keeps working
	// past the limit. Two successes here, with AuthRateLimit=2, would already
	// exhaust the bucket if they counted.
	t.Run("valid keys do not consume the failed-auth budget", func(t *testing.T) {
		for attempt := 1; attempt <= 3; attempt++ {
			requestJSONRawWithHeaders(t, newIntegrationClient(t), http.MethodGet, usersURL, nil, http.StatusOK, goodKey)
		}
	})

	// Failures on one middleware must count against the next: the bucket is
	// ProjectIPKey("api_key_auth", project, clientIP), not per route.
	t.Run("failed attempts share one bucket across middlewares", func(t *testing.T) {
		requestJSONRawWithHeaders(t, newIntegrationClient(t), http.MethodGet, usersURL, nil, http.StatusUnauthorized, badKey)
		requestJSONRawWithHeaders(t, newIntegrationClient(t), http.MethodGet, storageFilesURL, nil, http.StatusUnauthorized, badKey)

		// The third failure exceeds AuthRateLimit=2 regardless of which
		// middleware it hits.
		response := rawRequest(t, http.MethodGet, usersURL, badKey)
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("third failed attempt: status = %d, want 429: %s", response.StatusCode, body)
		}
		if response.Header.Get("Retry-After") == "" {
			t.Fatal("429 response has no Retry-After header")
		}
		if got := errorCodeOf(t, body); got != "rate_limited" {
			t.Fatalf("429 error code = %q, want rate_limited: %s", got, body)
		}
	})
}

// rawRequest sends one request without asserting a status, for cases where the
// status is what the test is deciding.
func rawRequest(t *testing.T, method, url string, headers map[string]string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := newIntegrationClient(t).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
