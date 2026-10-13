package httpapi

import (
	"net/http"
)

// adminRedisMetrics serves the in-process Redis snapshot (clients, ops/sec,
// hit rate, memory, keys, uptime, plus a short in-memory history) that the
// admin Overview renders on its Redis panel. Like the other metrics endpoints
// it never touches the telemetry backend, and an optional `window_minutes`
// query narrows the returned history so the client never derives a timestamp
// during render.
func (s *Server) adminRedisMetrics(w http.ResponseWriter, r *http.Request) {
	if s.redisMetrics == nil {
		writeError(
			w,
			http.StatusServiceUnavailable,
			"redis_metrics_unavailable",
			"Redis metrics are unavailable",
		)
		return
	}
	since, ok := parseWindowMinutes(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.redisMetrics.SnapshotSince(since))
}
