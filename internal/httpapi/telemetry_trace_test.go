package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/observability"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/go-chi/chi/v5"
)

func TestRecordHTTPTraceSkipsAnonymousInstanceRoutes(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "/v1/status-page", nil)
	router := chi.NewRouter()
	router.Get("/v1/status-page", func(http.ResponseWriter, *http.Request) {})
	router.ServeHTTP(httptest.NewRecorder(), request)

	// The route context is installed by the router on the request it serves.
	// A fresh request with no authenticated account must not attempt a write.
	server.recordHTTPTrace(request.Context(), observability.HTTPTraceRecord{
		TraceID: "trace-anonymous-instance", Method: http.MethodGet,
		Route: "/v1/status-page", Status: http.StatusOK,
	})
}

func TestRecordHTTPTraceNeedsRepositoryForTenantTraffic(t *testing.T) {
	server := &Server{}
	server.recordHTTPTrace(context.Background(), observability.HTTPTraceRecord{
		TraceID: "trace-no-repository", Method: http.MethodGet,
		Route: "/v1/projects/{projectID}", Status: http.StatusOK,
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	})
}

func TestRecordHTTPTraceInputNoLongerRequiresTenantScope(t *testing.T) {
	input := repository.HTTPTraceInput{
		TraceID: "trace-instance-scope", Method: http.MethodGet,
		Route: "/v1/admin/overview", Status: http.StatusOK,
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	}
	if input.OrganizationID != nil || input.ProjectID != nil {
		t.Fatal("instance telemetry test fixture unexpectedly has a tenant")
	}
}
