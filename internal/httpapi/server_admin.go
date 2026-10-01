package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/telemetry"
)

type adminComponentStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
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
	InstanceStatus string                        `json:"instance_status"`
	CheckedAt      time.Time                     `json:"checked_at"`
	Components     []adminComponentStatus        `json:"components"`
	Telemetry      adminTelemetryStatus          `json:"telemetry"`
	HTTP           *adminHTTPOverview            `json:"http,omitempty"`
	Operations     *domain.AdminOperationSummary `json:"operations,omitempty"`
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

	clickhouseStatus := "unavailable"
	if s.telemetry != nil {
		if err := s.telemetry.Ping(healthContext); err == nil {
			clickhouseStatus = "healthy"
		}
	}
	components = append(components, adminComponentStatus{Name: "clickhouse", Status: clickhouseStatus})
	collectorStatus := "unknown"
	if s.config.TelemetryCollectorHealthURL != "" {
		collectorStatus = "unavailable"
		if s.collectorHealthy(r.WithContext(healthContext)) {
			collectorStatus = "healthy"
		}
	}
	components = append(components, adminComponentStatus{Name: "otel-collector", Status: collectorStatus})
	telemetryStatus := "unavailable"
	if clickhouseStatus == "healthy" && (collectorStatus == "healthy" || collectorStatus == "unknown") {
		telemetryStatus = "healthy"
	}

	instanceStatus := "degraded"
	if coreHealthy {
		instanceStatus = "healthy"
	}
	var operations *domain.AdminOperationSummary
	if s.repo != nil {
		if summary, err := s.repo.AdminOperationSummary(healthContext); err == nil {
			operations = &summary
		}
	}
	var httpOverview *adminHTTPOverview
	if explorer, ok := s.telemetry.(telemetry.OverviewExplorer); ok {
		if result, err := explorer.QueryHTTPOverview(healthContext, telemetry.HTTPOverviewQuery{Range: queryRange}); err == nil {
			if result.SampleCount > 0 {
				httpOverview = &adminHTTPOverview{RequestRate: result.RequestRate, ErrorRate: result.ErrorRate, P50LatencyMS: result.P50LatencyMS, P95LatencyMS: result.P95LatencyMS, P99LatencyMS: result.P99LatencyMS, SampleCount: result.SampleCount}
			}
		}
	}
	writeJSON(w, http.StatusOK, adminOverviewResponse{
		InstanceStatus: instanceStatus,
		CheckedAt:      checkedAt,
		Components:     components,
		Telemetry:      adminTelemetryStatus{Status: telemetryStatus},
		HTTP:           httpOverview,
		Operations:     operations,
	})
}

func (s *Server) collectorHealthy(r *http.Request) bool {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.config.TelemetryCollectorHealthURL, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}
