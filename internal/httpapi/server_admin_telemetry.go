package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/telemetry"
	"github.com/go-chi/chi/v5"
)

func (s *Server) adminTelemetryLogs(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := s.telemetry.QueryLogs(r.Context(), telemetry.LogsQuery{
		Range:   queryRange,
		Service: r.URL.Query().Get("service"),
		Level:   r.URL.Query().Get("level"),
		Search:  r.URL.Query().Get("query"),
		Limit:   limit,
	})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

// adminTelemetryLogTail keeps the transport streaming and the query bounded.
// It deliberately does not expose ClickHouse's native stream or credentials:
// every poll is still an authenticated, parameterized domain query, and the
// browser receives only redacted log records. The short-lived connection is
// also safe to cancel when the operator navigates away.
func (s *Server) adminTelemetryLogTail(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	if limit > 250 {
		limit = 250
	}
	service := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("query")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		writeError(w, http.StatusInternalServerError, "stream_unavailable", "live log streaming is unavailable")
		return
	}

	var cursor *telemetry.LogCursor
	if raw := strings.TrimSpace(r.Header.Get("Last-Event-ID")); raw != "" {
		if decoded, err := telemetry.DecodeLogCursor(raw); err == nil {
			cursor = &decoded
		}
	}
	maxRange := s.config.TelemetryMaxQueryRange
	if maxRange <= 0 {
		maxRange = 30 * 24 * time.Hour
	}
	lookback := 5 * time.Minute
	if maxRange < lookback {
		lookback = maxRange
	}
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()

	writeEvent := func(event string, value any) bool {
		payload, err := json.Marshal(value)
		if err != nil {
			return false
		}
		if _, err := io.WriteString(w, "event: "+event+"\ndata: "+string(payload)+"\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	writeLogEvent := func(item telemetry.LogRecord) bool {
		payload, err := json.Marshal(item)
		if err != nil {
			return false
		}
		id := telemetry.EncodeLogCursor(telemetry.LogCursor{
			Timestamp: item.Timestamp,
			EventID:   item.EventID,
		})
		if _, err := io.WriteString(w, "id: "+id+"\nevent: log\ndata: "+string(payload)+"\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	writeHeartbeat := func() bool {
		if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	for {
		to := time.Now().UTC()
		from := to.Add(-lookback)
		query := telemetry.LogsQuery{
			Range:    telemetry.TimeRange{From: from, To: to},
			Service:  service,
			Level:    level,
			Search:   search,
			Limit:    limit,
			LiveTail: true,
		}
		if cursor == nil {
			if queryRange.From.After(from) {
				query.Range.From = queryRange.From
			}
		} else {
			query.After = cursor
			query.Range.From = cursor.Timestamp
			if query.Range.From.Before(to.Add(-maxRange)) {
				query.Range.From = to.Add(-maxRange)
			}
			if !to.After(query.Range.From) {
				query.Range.From = to.Add(-time.Nanosecond)
			}
		}
		result, err := s.telemetry.QueryLogs(r.Context(), query)
		if err != nil {
			_ = writeEvent("stream_error", map[string]string{"message": "telemetry backend is unavailable"})
			return
		}
		if cursor == nil {
			// The normal query is newest-first. Emit the initial bounded window
			// oldest-first so the cursor advances monotonically to its tail.
			for index := len(result.Items) - 1; index >= 0; index-- {
				item := result.Items[index]
				itemCursor := telemetry.LogCursor{Timestamp: item.Timestamp, EventID: item.EventID}
				cursor = &itemCursor
				if !writeLogEvent(item) {
					return
				}
			}
		} else {
			// Cursor queries are already chronological. The comparison is kept
			// here as a defense-in-depth guard for test doubles and future store
			// implementations.
			for _, item := range result.Items {
				itemCursor := telemetry.LogCursor{Timestamp: item.Timestamp, EventID: item.EventID}
				if !itemCursor.After(*cursor) {
					continue
				}
				cursor = &itemCursor
				if !writeLogEvent(item) {
					return
				}
			}
		}
		if !writeHeartbeat() {
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-poll.C:
		}
	}
}

func (s *Server) adminTelemetryLogVolume(w http.ResponseWriter, r *http.Request) {
	explorer, ok := s.telemetry.(telemetry.Explorer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := explorer.QueryLogVolume(r.Context(), telemetry.LogVolumeQuery{
		Range:   queryRange,
		Service: r.URL.Query().Get("service"),
		Level:   r.URL.Query().Get("level"),
		Search:  r.URL.Query().Get("query"),
		Limit:   limit,
	})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) adminTelemetryTraces(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	minMS, err := parseFloatQuery(r, "min_duration_ms", 0, 24*60*60*1000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "min_duration_ms must be a non-negative number within one day")
		return
	}
	result, err := s.telemetry.QueryTraces(r.Context(), telemetry.TracesQuery{
		Range:   queryRange,
		Service: r.URL.Query().Get("service"),
		TraceID: r.URL.Query().Get("trace_id"),
		MinMs:   minMS,
		Limit:   limit,
	})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) adminTelemetryErrors(w http.ResponseWriter, r *http.Request) {
	explorer, ok := s.telemetry.(telemetry.Explorer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := explorer.QueryErrorGroups(r.Context(), telemetry.ErrorGroupsQuery{
		Range:   queryRange,
		Service: r.URL.Query().Get("service"),
		Search:  r.URL.Query().Get("query"),
		Limit:   limit,
	})
	if err == nil && s.repo != nil && len(result.Items) > 0 {
		fingerprints := make([]string, 0, len(result.Items))
		for _, item := range result.Items {
			fingerprints = append(fingerprints, item.Fingerprint)
		}
		statuses, statusErr := s.repo.ListAdminErrorGroupStatuses(r.Context(), fingerprints)
		if statusErr != nil {
			internalError(s, w, statusErr)
			return
		}
		for index := range result.Items {
			if status, exists := statuses[result.Items[index].Fingerprint]; exists {
				result.Items[index].Status = status
			}
		}
	}
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) updateAdminTelemetryErrorStatus(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		internalError(s, w, errors.New("repository is unavailable"))
		return
	}
	fingerprint := chi.URLParam(r, "fingerprint")
	var request adminErrorGroupStatusRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	state, err := s.repo.UpdateAdminErrorGroupStatus(r.Context(), mustUUID(accountFrom(r).ID), fingerprint, request.Status)
	if errors.Is(err, repository.ErrInvalidAdminErrorGroup) {
		writeError(w, http.StatusBadRequest, "validation_error", "error group status is invalid")
		return
	}
	if errors.Is(err, repository.ErrForbidden) {
		writeError(w, http.StatusForbidden, "forbidden", "instance owner or admin permission is required")
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminErrorGroupStatusResponse{
		Fingerprint: state.Fingerprint,
		Status:      state.Status,
		UpdatedAt:   state.UpdatedAt,
	})
}

func (s *Server) adminTelemetryServices(w http.ResponseWriter, r *http.Request) {
	explorer, ok := s.telemetry.(telemetry.Explorer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := explorer.QueryServiceMap(r.Context(), telemetry.ServiceMapQuery{Range: queryRange, Limit: limit})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) adminInfrastructureMetrics(w http.ResponseWriter, r *http.Request) {
	explorer, ok := s.telemetry.(telemetry.InfrastructureExplorer)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := explorer.QueryInfrastructure(r.Context(), telemetry.InfrastructureQuery{
		Range: queryRange,
		Scope: r.URL.Query().Get("scope"),
		Limit: limit,
	})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) adminTelemetryMetrics(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := s.telemetry.QueryMetrics(r.Context(), telemetry.MetricsQuery{
		Range:   queryRange,
		Service: r.URL.Query().Get("service"),
		Name:    r.URL.Query().Get("name"),
		Limit:   limit,
	})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) adminTelemetrySources(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		writeError(w, http.StatusServiceUnavailable, "telemetry_unavailable", "telemetry backend is unavailable")
		return
	}
	queryRange, ok := s.adminTimeRange(w, r)
	if !ok {
		return
	}
	limit, ok := s.adminLimit(w, r)
	if !ok {
		return
	}
	result, err := s.telemetry.ListSources(r.Context(), telemetry.SourcesQuery{Range: queryRange, Limit: limit})
	if !s.writeTelemetryResultError(w, err) {
		writeJSON(w, http.StatusOK, result)
	}
}
