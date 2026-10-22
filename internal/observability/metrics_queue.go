package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// QueueMetrics is a reusable bounded-cardinality metric set for a durable
// worker queue. Labels use only fixed result and operation vocabularies.
type QueueMetrics struct {
	Polls     prometheus.Counter
	Claimed   prometheus.Counter
	Completed *prometheus.CounterVec
	Duration  prometheus.Histogram
	Requeued  prometheus.Counter
	Errors    *prometheus.CounterVec
}

func newQueueMetrics(subsystem, help string) *QueueMetrics {
	return &QueueMetrics{
		Polls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "stealth", Subsystem: subsystem, Name: "polls_total",
			Help: "Queue poll cycles performed by the " + help + " worker.",
		}),
		Claimed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "stealth", Subsystem: subsystem, Name: "jobs_claimed_total",
			Help: "Jobs claimed by the " + help + " worker.",
		}),
		Completed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "stealth", Subsystem: subsystem, Name: "jobs_completed_total",
			Help: "Jobs transitioned to a terminal result by the " + help + " worker.",
		}, []string{"result"}),
		Duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "stealth", Subsystem: subsystem, Name: "job_duration_seconds",
			Help:    "Time spent processing one " + help + " job.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}),
		Requeued: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "stealth", Subsystem: subsystem, Name: "stale_jobs_requeued_total",
			Help: "Stale leases returned to the " + help + " queue after expiry.",
		}),
		Errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "stealth", Subsystem: subsystem, Name: "errors_total",
			Help: "Worker errors grouped by a fixed internal operation name.",
		}, []string{"operation"}),
	}
}

func (m *QueueMetrics) register(registry *prometheus.Registry) {
	if m == nil {
		return
	}
	registry.MustRegister(m.Polls, m.Claimed, m.Completed, m.Duration, m.Requeued, m.Errors)
}

// Poll records one queue poll cycle.
func (m *QueueMetrics) Poll() {
	if m != nil {
		m.Polls.Inc()
	}
}

// Requeue records stale leases released back to the queue.
func (m *QueueMetrics) Requeue(count int64) {
	if m != nil && count > 0 {
		m.Requeued.Add(float64(count))
	}
}

// Claim records one claimed job.
func (m *QueueMetrics) Claim() {
	if m != nil {
		m.Claimed.Inc()
	}
}

// Complete records one terminal job result and its duration.
func (m *QueueMetrics) Complete(result string, duration time.Duration) {
	if m != nil {
		m.Completed.WithLabelValues(result).Inc()
		m.Duration.Observe(duration.Seconds())
	}
}

// Error records a worker error under a fixed operation name.
func (m *QueueMetrics) Error(operation string) {
	if m != nil {
		m.Errors.WithLabelValues(operation).Inc()
	}
}
