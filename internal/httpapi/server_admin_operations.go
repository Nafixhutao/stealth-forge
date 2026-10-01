package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/telemetry"
	"github.com/google/uuid"
)

func (s *Server) adminOperations(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		internalError(s, w, errors.New("repository is unavailable"))
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "validation_error", "limit must be an integer between 1 and 100")
			return
		}
		limit = parsed
	}
	items, err := s.repo.ListAdminOperations(r.Context(), queryRange.From, queryRange.To, limit)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "validation_error", "operations query is invalid")
			return
		}
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminOperationsResponse{Items: items})
}

func (s *Server) adminAuditEvents(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		internalError(s, w, errors.New("repository is unavailable"))
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "validation_error", "limit must be an integer between 1 and 100")
			return
		}
		limit = parsed
	}
	var before *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "before must be a UUID cursor")
			return
		}
		before = &parsed
	}
	items, next, err := s.repo.ListInstanceAuditEvents(r.Context(), limit, before)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "validation_error", "audit query is invalid")
			return
		}
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminAuditResponse{Items: items, NextCursor: next})
}

func (s *Server) adminTimeRange(w http.ResponseWriter, r *http.Request) (telemetry.TimeRange, bool) {
	to := time.Now().UTC()
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "to must be an RFC3339 timestamp")
			return telemetry.TimeRange{}, false
		}
		to = parsed.UTC()
	}
	from := to.Add(-time.Hour)
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "from must be an RFC3339 timestamp")
			return telemetry.TimeRange{}, false
		}
		from = parsed.UTC()
	}
	queryRange := telemetry.TimeRange{From: from, To: to}
	if !to.After(from) || to.Sub(from) > s.config.TelemetryMaxQueryRange {
		writeError(w, http.StatusBadRequest, "validation_error", "time range is invalid or exceeds the configured limit")
		return telemetry.TimeRange{}, false
	}
	return queryRange, true
}

func (s *Server) adminLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "limit must be an integer")
			return 0, false
		}
		limit = parsed
	}
	if limit < 1 || limit > s.config.TelemetryMaxQueryRows {
		writeError(w, http.StatusBadRequest, "validation_error", "limit exceeds the configured telemetry query limit")
		return 0, false
	}
	return limit, true
}

func parseFloatQuery(r *http.Request, key string, minimum, maximum float64) (float64, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < minimum || parsed > maximum {
		return 0, errors.New("invalid number")
	}
	return parsed, nil
}

func (s *Server) writeTelemetryResultError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, telemetry.ErrInvalidQuery) {
		writeError(w, http.StatusBadRequest, "validation_error", "telemetry query is invalid")
		return true
	}
	// Backend failures are intentionally generic. ClickHouse errors can contain
	// query fragments or deployment-specific details and must not reach the
	// browser.
	writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
	return true
}
