package httpapi

import (
	"net/http"
	"runtime"
	"strconv"
	"time"
)

// adminAPIMetrics serves the API process's own runtime vitals: goroutines,
// heap, GC pause, and process uptime. It reads the runtime directly, so it
// works without any external backend and adds no sampling load — numbers are
// gathered at request time.
func (s *Server) adminAPIMetrics(w http.ResponseWriter, r *http.Request) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	var lastGCPauseMS float64
	if stats.NumGC > 0 {
		lastGCPauseMS = float64(stats.PauseNs[(stats.NumGC-1)%uint32(len(stats.PauseNs))]) / 1e6
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminAPIMetricsResponse{
		Goroutines:        runtime.NumGoroutine(),
		HeapAllocBytes:    stats.HeapAlloc,
		HeapObjects:       stats.HeapObjects,
		StackBytes:        stats.StackSys,
		SysBytes:          stats.Sys,
		GCRuns:            stats.NumGC,
		LastGCPauseMS:     round1MS(lastGCPauseMS),
		GoVersion:         runtime.Version(),
		ProcessUptimeSecs: time.Since(s.startedAt).Seconds(),
	})
}

// adminAPIMetricsResponse is the /v1/admin/api-metrics payload.
type adminAPIMetricsResponse struct {
	Goroutines        int     `json:"goroutines"`
	HeapAllocBytes    uint64  `json:"heap_alloc_bytes"`
	HeapObjects       uint64  `json:"heap_objects"`
	StackBytes        uint64  `json:"stack_bytes"`
	SysBytes          uint64  `json:"sys_bytes"`
	GCRuns            uint32  `json:"gc_runs"`
	LastGCPauseMS     float64 `json:"last_gc_pause_ms"`
	GoVersion         string  `json:"go_version"`
	ProcessUptimeSecs float64 `json:"process_uptime_seconds"`
}

func round1MS(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}

// parseWindowMinutes is shared validation for the metrics window query.
func parseWindowMinutes(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	raw := r.URL.Query().Get("window_minutes")
	if raw == "" {
		return time.Time{}, true
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes <= 0 || minutes > 24*60 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "window_minutes must be an integer between 1 and 1440")
		return time.Time{}, false
	}
	return time.Now().UTC().Add(-time.Duration(minutes) * time.Minute), true
}
