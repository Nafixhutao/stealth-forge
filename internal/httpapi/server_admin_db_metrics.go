package httpapi

import (
	"net/http"
)

// adminDBMetrics serves the in-process PostgreSQL snapshot (connections,
// active queries, cache hit ratio, transaction rate, replication, size,
// uptime, plus a short in-memory history) that the admin Overview renders on
// its database card. Like the host metrics endpoint it never touches the
// telemetry backend, and an optional `window_minutes` query narrows the
// returned history so the client never derives a timestamp during render.
func (s *Server) adminDBMetrics(w http.ResponseWriter, r *http.Request) {
	if s.dbMetrics == nil {
		writeError(
			w,
			http.StatusServiceUnavailable,
			"db_metrics_unavailable",
			"database metrics are unavailable",
		)
		return
	}
	since, ok := parseWindowMinutes(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.dbMetrics.SnapshotSince(since))
}
