package observability

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolCollector exports pgxpool saturation so pool exhaustion — a common
// API-wide latency and failure cause — is observable and alertable.
type poolCollector struct {
	pool            *pgxpool.Pool
	connections     *prometheus.Desc
	acquires        *prometheus.Desc
	acquireDuration *prometheus.Desc
	emptyAcquires   *prometheus.Desc
	canceledAcquire *prometheus.Desc
}

// NewPoolCollector returns a Prometheus collector for the shared database
// pool. A nil pool produces a collector that emits nothing.
func NewPoolCollector(pool *pgxpool.Pool) prometheus.Collector {
	return &poolCollector{
		pool: pool,
		connections: prometheus.NewDesc(
			"stealth_db_pool_connections",
			"Current PostgreSQL pool connections by state.",
			[]string{"state"},
			nil,
		),
		acquires: prometheus.NewDesc(
			"stealth_db_pool_acquires_total",
			"Cumulative successful pool acquisitions.",
			nil,
			nil,
		),
		acquireDuration: prometheus.NewDesc(
			"stealth_db_pool_acquire_duration_seconds_total",
			"Cumulative time spent waiting to acquire a pool connection.",
			nil,
			nil,
		),
		emptyAcquires: prometheus.NewDesc(
			"stealth_db_pool_empty_acquires_total",
			"Cumulative acquisitions that had to wait for a free connection.",
			nil,
			nil,
		),
		canceledAcquire: prometheus.NewDesc(
			"stealth_db_pool_canceled_acquires_total",
			"Cumulative canceled pool acquisitions.",
			nil,
			nil,
		),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connections
	ch <- c.acquires
	ch <- c.acquireDuration
	ch <- c.emptyAcquires
	ch <- c.canceledAcquire
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.pool == nil {
		return
	}
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stat.TotalConns()), "total")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stat.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stat.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stat.ConstructingConns()), "constructing")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stat.MaxConns()), "max")
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(stat.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireDuration, prometheus.CounterValue, stat.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.emptyAcquires, prometheus.CounterValue, float64(stat.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.canceledAcquire, prometheus.CounterValue, float64(stat.CanceledAcquireCount()))
}
