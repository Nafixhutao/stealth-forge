package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/Nafixhutao/stealth-forge/internal/telemetry"
)

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

type adminComponentStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

type adminTelemetryStatus struct {
	Status string `json:"status"`
}

type adminHTTPOverview struct {
	RequestRate  float64 `json:"request_rate"`
	ErrorRate    float64 `json:"error_rate"`
	P50LatencyMS float64 `json:"p50_latency_ms"`
	P95LatencyMS float64 `json:"p95_latency_ms"`
	P99LatencyMS float64 `json:"p99_latency_ms"`
	SampleCount  uint64  `json:"sample_count"`
}

type adminOverviewResponse struct {
	InstanceStatus    string                         `json:"instance_status"`
	CheckedAt         time.Time                      `json:"checked_at"`
	Components        []adminComponentStatus         `json:"components"`
	Telemetry         adminTelemetryStatus           `json:"telemetry"`
	HTTP              *adminHTTPOverview             `json:"http,omitempty"`
	Operations        *domain.AdminOperationSummary  `json:"operations,omitempty"`
	RecentDeployments []domain.AdminRecentDeployment `json:"recent_deployments,omitempty"`
}

type adminOperationsResponse struct {
	Items []domain.AdminOperation `json:"items"`
}

type adminAuditResponse struct {
	Items      []domain.AuditEvent `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type adminErrorGroupStatusRequest struct {
	Status string `json:"status"`
}

type adminErrorGroupStatusResponse struct {
	Fingerprint string    `json:"fingerprint"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Server) requireInstanceAdmin(next http.Handler) http.Handler {
	return s.requireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.repo == nil {
			internalError(s, w, errors.New("repository is unavailable"))
			return
		}
		allowed, err := s.repo.IsInstanceAdmin(r.Context(), mustUUID(accountFrom(r).ID))
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			internalError(s, w, err)
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "forbidden", "instance owner or admin permission is required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) requireInstanceOwner(next http.Handler) http.Handler {
	return s.requireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.repo == nil {
			internalError(s, w, errors.New("repository is unavailable"))
			return
		}
		allowed, err := s.repo.IsInstanceOwner(r.Context(), mustUUID(accountFrom(r).ID))
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			internalError(s, w, err)
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "forbidden", "instance owner permission is required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	checkedAt := time.Now().UTC()
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	healthContext, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	components := make([]adminComponentStatus, 0, 6)
	coreHealthy := true

	apiStatus := "healthy"
	components = append(components, adminComponentStatus{Name: "api", Status: apiStatus})

	databaseStatus := "healthy"
	if s.repo == nil || s.repo.Ping(healthContext) != nil {
		databaseStatus = "unavailable"
		coreHealthy = false
	}
	components = append(components, adminComponentStatus{Name: "postgres", Status: databaseStatus})

	redisStatus := "unavailable"
	if s.redis != nil {
		if err := s.redis.Ping(healthContext).Err(); err == nil {
			redisStatus = "healthy"
		} else {
			coreHealthy = false
		}
	} else {
		coreHealthy = false
	}
	components = append(components, adminComponentStatus{Name: "redis", Status: redisStatus})

	// The tenant MySQL cluster is optional and separate from the control-plane
	// PostgreSQL. It never degrades the core instance status (the platform runs
	// without it), but its reachability and fair-use state are surfaced here so
	// the operator sees the hosted-database door at a glance. The component is
	// listed whenever the feature is enabled, even if the cluster did not come
	// up, so a misprovisioned cluster is visible rather than silently omitted.
	if s.config.ManagedDatabasesEnabled {
		tenantStatus := "unavailable"
		tenantSummary := ""
		if s.managedDatabases != nil {
			summary := s.managedDatabases.AdminSummary(healthContext)
			if summary.Reachable {
				tenantStatus = "healthy"
				tenantSummary = fmt.Sprintf(
					"%d database%s · %d over quota",
					summary.DatabaseCount,
					pluralSuffix(summary.DatabaseCount),
					summary.WriteLockedCount,
				)
			}
		}
		components = append(components, adminComponentStatus{
			Name:    "tenant_mysql",
			Status:  tenantStatus,
			Summary: tenantSummary,
		})
	}

	telemetryStatus := "unavailable"
	if telemetryStore, ok := s.telemetry.(telemetry.Pinger); ok {
		if telemetryStore.Ping(healthContext) == nil {
			telemetryStatus = "healthy"
		}
	}

	instanceStatus := "degraded"
	if coreHealthy {
		instanceStatus = "healthy"
	}
	var operations *domain.AdminOperationSummary
	var recentDeployments []domain.AdminRecentDeployment
	if s.repo != nil {
		if summary, err := s.repo.AdminOperationSummary(healthContext); err == nil {
			operations = &summary
		}
		// Recent activity is decorative but honest; a slow or failing query
		// must not block the rest of the overview, so it degrades to omitted.
		if deployments, err := s.repo.ListRecentAppDeployments(healthContext, 5); err == nil {
			recentDeployments = deployments
		}
	}
	var httpOverview *adminHTTPOverview
	if explorer, ok := s.telemetry.(telemetry.OverviewExplorer); ok {
		if result, err := explorer.QueryHTTPOverview(healthContext, telemetry.HTTPOverviewQuery{Range: queryRange}); err == nil {
			if result.SampleCount > 0 {
				httpOverview = &adminHTTPOverview{
					RequestRate:  result.RequestRate,
					ErrorRate:    result.ErrorRate,
					P50LatencyMS: result.P50LatencyMS,
					P95LatencyMS: result.P95LatencyMS,
					P99LatencyMS: result.P99LatencyMS,
					SampleCount:  result.SampleCount,
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, adminOverviewResponse{
		InstanceStatus:    instanceStatus,
		CheckedAt:         checkedAt,
		Components:        components,
		Telemetry:         adminTelemetryStatus{Status: telemetryStatus},
		HTTP:              httpOverview,
		Operations:        operations,
		RecentDeployments: recentDeployments,
	})
}
