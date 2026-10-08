package httpapi

import (
	"net/http"
	"strconv"
	"time"
)

// adminHostMetrics serves the in-process host resource snapshot (CPU, memory,
// disk, network, plus a short in-memory history) that the admin Overview renders
// on its resource cards and chart. It is the lightweight replacement for the
// ClickHouse-backed infrastructure metrics for host resources, so the Overview
// keeps working when the telemetry backend is not deployed. An optional
// `window_minutes` query narrows the returned history; the server computes the
// cutoff so the client never has to derive a timestamp during render.
func (s *Server) adminHostMetrics(w http.ResponseWriter, r *http.Request) {
	if s.hostMetrics == nil {
		writeError(
			w,
			http.StatusServiceUnavailable,
			"host_metrics_unavailable",
			"host metrics are unavailable",
		)
		return
	}
	since := time.Time{}
	if raw := r.URL.Query().Get("window_minutes"); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes <= 0 || minutes > 24*60 {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "window_minutes must be an integer between 1 and 1440")
			return
		}
		since = time.Now().UTC().Add(-time.Duration(minutes) * time.Minute)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.hostMetrics.SnapshotSince(since))
}
