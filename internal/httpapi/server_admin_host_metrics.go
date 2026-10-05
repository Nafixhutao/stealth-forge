package httpapi

import "net/http"

// adminHostMetrics serves the in-process host resource snapshot (CPU, memory,
// disk, network, plus a short in-memory history) that the admin Overview renders
// on its resource cards and chart. It is the lightweight replacement for the
// ClickHouse-backed infrastructure metrics for host resources, so the Overview
// keeps working when the telemetry backend is not deployed.
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
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.hostMetrics.Snapshot())
}
