// Package dbmetrics samples the PostgreSQL instance through the API's own
// connection pool. It backs the admin Overview's database card the same way
// hostmetrics backs the host resource cards: an in-process sampler with a
// small in-memory ring, no telemetry backend, no extra exporter, and only
// numbers PostgreSQL itself reports.
package dbmetrics

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// Querier is the slice of the pool the sampler needs. *pgxpool.Pool satisfies
// it; tests substitute a stub row.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Sample is one database observation. TransactionsPerSec is a rate derived
// from the previous sample, like hostmetrics' network rates.
type Sample struct {
	Timestamp time.Time `json:"timestamp"`
	// Connections counts every backend including this sampler's own.
	Connections        int64 `json:"connections"`
	MaxConnections     int64 `json:"max_connections"`
	ActiveQueries      int64 `json:"active_queries"`
	CacheHitRatioKnown bool  `json:"cache_hit_ratio_known"`
	// CacheHitRatioPercent is 0-100 and only meaningful when Known is set;
	// a database that has not read or hit any block yet has no ratio.
	CacheHitRatioPercent float64 `json:"cache_hit_ratio_percent"`
	TransactionsPerSec   float64 `json:"transactions_per_sec"`
	// QueryLatencyMS is the sampler's own round-trip to PostgreSQL: network
	// plus execution as seen from the API. It is a real measurement, not an
	// aggregate of every statement the instance serves.
	QueryLatencyMS float64 `json:"query_latency_ms"`
	// ReplicationStandbys is the streaming replication row count. Zero means
	// a standalone instance, which the console reports honestly.
	ReplicationStandbys int64   `json:"replication_standbys"`
	DatabaseSizeBytes   uint64  `json:"database_size_bytes"`
	UptimeSeconds       float64 `json:"uptime_seconds"`
	// XactsTotal and At back the per-second transaction rate; they are not
	// part of the API payload.
	XactsTotal uint64    `json:"-"`
	At         time.Time `json:"-"`
}

// Snapshot is the current sample plus bounded recent history for charts.
type Snapshot struct {
	Current       Sample   `json:"current"`
	History       []Sample `json:"history"`
	ServerVersion string   `json:"server_version"`
	// Reachable reports whether the most recent sample succeeded. Failed
	// samples leave the history untouched and flip this to false so the
	// console can show an unavailable state instead of stale numbers.
	Reachable bool `json:"reachable"`
}

// Collector samples database metrics on an interval and retains bounded history.
type Collector struct {
	mu       sync.Mutex
	pool     Querier
	history  []Sample
	max      int
	interval time.Duration

	serverVersion string
	lastOK        bool
}

// New builds a collector. interval controls the sampling cadence and
// maxHistory the retained sample count.
func New(pool Querier, interval time.Duration, maxHistory int) *Collector {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if maxHistory <= 0 {
		maxHistory = 720
	}
	return &Collector{pool: pool, max: maxHistory, interval: interval}
}

// sampleSQL reads every metric in one round trip. Statistics are scoped to the
// current database so a shared cluster does not blend other databases' counters.
const sampleSQL = `SELECT
	(SELECT count(*) FROM pg_stat_activity),
	(SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND pid <> pg_backend_pid()),
	(SELECT setting::bigint FROM pg_settings WHERE name = 'max_connections'),
	(SELECT COALESCE(blks_hit, 0) FROM pg_stat_database WHERE datname = current_database()),
	(SELECT COALESCE(blks_read, 0) FROM pg_stat_database WHERE datname = current_database()),
	(SELECT COALESCE(xact_commit + xact_rollback, 0) FROM pg_stat_database WHERE datname = current_database()),
	(SELECT count(*) FROM pg_stat_replication),
	(SELECT pg_database_size(current_database())),
	(SELECT EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time())))`

// Start samples immediately and then on the configured interval until ctx ends.
func (c *Collector) Start(ctx context.Context) {
	c.sampleOnce()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sampleOnce()
		}
	}
}

func (c *Collector) sampleOnce() {
	sample, ok := c.querySample()
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		c.lastOK = true
		if c.serverVersion == "" {
			c.serverVersion = c.queryServerVersion()
		}
		c.append(sample)
	} else {
		c.lastOK = false
	}
}

func (c *Collector) querySample() (Sample, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		connections, active, maxConn int64
		blksHit, blksRead            uint64
		xacts                        uint64
		standbys                     int64
		sizeBytes                    uint64
		uptime                       float64
	)
	started := time.Now()
	err := c.pool.QueryRow(ctx, sampleSQL).Scan(
		&connections, &active, &maxConn,
		&blksHit, &blksRead, &xacts,
		&standbys, &sizeBytes, &uptime,
	)
	latencyMS := round1(float64(time.Since(started).Nanoseconds()) / 1e6)
	if err != nil {
		return Sample{}, false
	}

	now := time.Now().UTC()
	sample := Sample{
		Timestamp:           now,
		Connections:         connections,
		MaxConnections:      maxConn,
		ActiveQueries:       active,
		QueryLatencyMS:      latencyMS,
		ReplicationStandbys: standbys,
		DatabaseSizeBytes:   sizeBytes,
		UptimeSeconds:       uptime,
		XactsTotal:          xacts,
		At:                  now,
	}
	if hitRead := blksHit + blksRead; hitRead > 0 {
		sample.CacheHitRatioKnown = true
		sample.CacheHitRatioPercent = round1(float64(blksHit) / float64(hitRead) * 100)
	}

	// Derive the transaction rate from the previous retained sample so the
	// rate matches the cadence the charts show.
	if len(c.history) > 0 {
		prev := c.history[len(c.history)-1]
		elapsed := now.Sub(prev.At).Seconds()
		if elapsed > 0 && xacts >= prev.XactsTotal {
			sample.TransactionsPerSec = round1(float64(xacts-prev.XactsTotal) / elapsed)
		}
	}
	return sample, true
}

func (c *Collector) queryServerVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var version string
	if err := c.pool.QueryRow(ctx, "SELECT current_setting('server_version')").Scan(&version); err != nil {
		return ""
	}
	return version
}

// append stores a sample and trims the ring. Callers must hold c.mu.
func (c *Collector) append(sample Sample) {
	c.history = append(c.history, sample)
	if len(c.history) > c.max {
		c.history = c.history[len(c.history)-c.max:]
	}
}

// Snapshot returns the latest sample and a copy of the retained history.
func (c *Collector) Snapshot() Snapshot {
	return c.windowed(time.Time{})
}

// SnapshotSince returns the retained samples at or after `since`, mirroring
// hostmetrics so the Overview time-range selector behaves identically.
func (c *Collector) SnapshotSince(since time.Time) Snapshot {
	return c.windowed(since)
}

func (c *Collector) windowed(since time.Time) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	history := c.history
	if !since.IsZero() {
		index := 0
		for index < len(history) && history[index].Timestamp.Before(since) {
			index++
		}
		history = history[index:]
	}
	out := make([]Sample, len(history))
	copy(out, history)
	snapshot := Snapshot{History: out, ServerVersion: c.serverVersion, Reachable: c.lastOK}
	if len(out) > 0 {
		snapshot.Current = out[len(out)-1]
	}
	return snapshot
}

func round1(value float64) float64 {
	return float64(int64(value*10+0.5)) / 10
}
