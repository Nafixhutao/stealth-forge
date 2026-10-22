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
	"github.com/Nafixhutao/stealth-forge/internal/migrate"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminPollingDoesNotIncreaseHTTPTraceCountIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(config.Config{
		SessionCookieName:  "stealth_session",
		SessionTTL:         time.Hour,
		StorageRoot:        t.TempDir(),
		StorageMaxFileSize: 1 << 20,
		FunctionsSecretKey: bytes.Repeat([]byte("t"), 32),
	}, repository.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()

	client := newIntegrationClient(t)
	owner := registerAdminTestAccount(t, client, server.URL, "trace-poll")
	if _, err := pool.Exec(ctx, `INSERT INTO instance_roles (account_id,role) VALUES ($1,'instance_admin')`, owner.accountID); err != nil {
		t.Fatal(err)
	}
	accountID := uuid.MustParse(owner.accountID)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM http_traces WHERE account_id=$1`, accountID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_account_id=$1 OR organization_id=$2`, owner.accountID, owner.organizationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM instance_roles WHERE account_id=$1`, owner.accountID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=$1`, owner.organizationID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id=$1`, owner.accountID)
	})

	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM http_traces WHERE account_id=$1`, accountID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		requestJSON(t, client, http.MethodGet, server.URL+"/v1/admin/overview", nil, http.StatusOK, &struct{}{})
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM http_traces WHERE account_id=$1`, accountID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("admin polling added %d HTTP trace rows", after-before)
	}
}

func TestCrossTenantRequestDoesNotMeterProjectIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(config.Config{
		SessionCookieName:  "stealth_session",
		SessionTTL:         time.Hour,
		StorageRoot:        t.TempDir(),
		StorageMaxFileSize: 1 << 20,
		FunctionsSecretKey: bytes.Repeat([]byte("t"), 32),
	}, repository.New(pool), slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()

	ownerClient := newIntegrationClient(t)
	owner := registerAdminTestAccount(t, ownerClient, server.URL, "meter-owner")
	var project struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	projectName := "meter-" + uuid.Must(uuid.NewV7()).String()[:8]
	requestJSON(t, ownerClient, http.MethodPost,
		server.URL+"/v1/organizations/"+owner.organizationID+"/projects",
		map[string]string{"name": projectName}, http.StatusCreated, &project)

	outsiderClient := newIntegrationClient(t)
	outsider := registerAdminTestAccount(t, outsiderClient, server.URL, "meter-outsider")
	projectID := uuid.MustParse(project.Project.ID)
	outsiderID := uuid.MustParse(outsider.accountID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM http_traces WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM project_usage_daily WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM audit_events WHERE actor_account_id IN ($1,$2) OR organization_id IN ($3,$4)`, owner.accountID, outsider.accountID, owner.organizationID, outsider.organizationID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM organizations WHERE id IN ($1,$2)`, owner.organizationID, outsider.organizationID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM accounts WHERE id IN ($1,$2)`, owner.accountID, outsider.accountID)
	})

	var beforeUsage int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(api_request_count),0) FROM project_usage_daily WHERE project_id=$1`, projectID).Scan(&beforeUsage); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, outsiderClient, http.MethodGet, server.URL+"/v1/projects/"+project.Project.ID, nil, http.StatusNotFound, nil)
	var afterUsage int64
	var outsiderTraces int
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(api_request_count),0) FROM project_usage_daily WHERE project_id=$1`, projectID).Scan(&afterUsage); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM http_traces WHERE project_id=$1 AND account_id=$2`, projectID, outsiderID).Scan(&outsiderTraces); err != nil {
		t.Fatal(err)
	}
	if afterUsage != beforeUsage || outsiderTraces != 0 {
		t.Fatalf("cross-tenant request wrote telemetry or usage: usage %d->%d traces=%d", beforeUsage, afterUsage, outsiderTraces)
	}
}
