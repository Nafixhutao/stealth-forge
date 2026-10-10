// Package redismetrics samples the Redis instance through the API's own
// client. It backs the admin Overview's Redis panel the same way dbmetrics
// backs the database panel: an in-process sampler with a small in-memory
// ring, reading only numbers Redis itself reports.
package redismetrics

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// InfoClient is the slice of the go-redis client the sampler needs. The
// return type matches go-redis's Info signature exactly (Go has no covariant
// returns, so an interface return would not be satisfied by *redis.Client);
// tests stub with a *redis.StringCmd carrying canned values.
type InfoClient interface {
	Info(ctx context.Context, section ...string) *redis.StringCmd
}

// Sample is one Redis observation. OpsPerSec comes from Redis's own
// instantaneous counter, so the first sample is already meaningful.
type Sample struct {
	Timestamp time.Time `json:"timestamp"`
	// ConnectedClients excludes this sampler's own connection counting
	// nuances; Redis reports the raw number.
	Connections  int64   `json:"connections"`
	MaxClients   int64   `json:"max_clients"`
	OpsPerSec    float64 `json:"ops_per_sec"`
	HitRateKnown bool    `json:"hit_rate_known"`
	// HitRatePercent is 0-100 and only meaningful when Known is set; a fresh
	// instance with no reads or misses has no rate.
	HitRatePercent  float64 `json:"hit_rate_percent"`
	MemoryUsedBytes uint64  `json:"memory_used_bytes"`
	// MemoryMaxBytes is zero when Redis has no maxmemory cap configured, so
	// the console renders usage without a limit bar rather than inventing one.
	MemoryMaxBytes uint64  `json:"memory_max_bytes"`
	Keys           int64   `json:"keys"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
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

// Collector samples Redis metrics on an interval and retains bounded history.
type Collector struct {
	mu       sync.Mutex
	client   InfoClient
	history  []Sample
	max      int
	interval time.Duration

	serverVersion string
	lastOK        bool
}

// New builds a collector. interval controls the sampling cadence and
// maxHistory the retained sample count.
func New(client InfoClient, interval time.Duration, maxHistory int) *Collector {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if maxHistory <= 0 {
		maxHistory = 720
	}
	return &Collector{client: client, max: maxHistory, interval: interval}
}

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
	sample, version, ok := c.querySample()
	c.mu.Lock()
	defer c.mu.Unlock()
	if ok {
		c.lastOK = true
		if c.serverVersion == "" {
			c.serverVersion = version
		}
		c.append(sample)
	} else {
		c.lastOK = false
	}
}

// parseInfo turns a Redis INFO payload into a sample plus the server
// version. It returns ok=false when the payload lacks a version line, which
// means it was not a valid INFO response.
func parseInfo(payload string) (sample Sample, version string, ok bool) {
	sample.Timestamp = time.Now().UTC()
	var hits, misses float64
	section := ""
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "# ") {
			section = strings.TrimPrefix(line, "# ")
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch {
		case section == "Server" && key == "redis_version":
			version = value
		case section == "Server" && key == "uptime_in_seconds":
			sample.UptimeSeconds, _ = strconv.ParseFloat(value, 64)
		case section == "Clients" && key == "connected_clients":
			sample.Connections, _ = strconv.ParseInt(value, 10, 64)
		case section == "Clients" && key == "maxclients":
			sample.MaxClients, _ = strconv.ParseInt(value, 10, 64)
		case section == "Stats" && key == "instantaneous_ops_per_sec":
			sample.OpsPerSec, _ = strconv.ParseFloat(value, 64)
		case section == "Stats" && key == "keyspace_hits":
			hits, _ = strconv.ParseFloat(value, 64)
		case section == "Stats" && key == "keyspace_misses":
			misses, _ = strconv.ParseFloat(value, 64)
		case section == "Memory" && key == "used_memory":
			sample.MemoryUsedBytes, _ = strconv.ParseUint(value, 10, 64)
		case section == "Memory" && key == "maxmemory":
			sample.MemoryMaxBytes, _ = strconv.ParseUint(value, 10, 64)
		case section == "Keyspace" && strings.HasPrefix(key, "db"):
			// db0:keys=12,expires=0,avg_ttl=0 — take the keys count.
			for _, field := range strings.Split(value, ",") {
				if name, number, isPair := strings.Cut(field, "="); isPair && name == "keys" {
					count, _ := strconv.ParseInt(number, 10, 64)
					sample.Keys += count
				}
			}
		}
	}
	if reads := hits + misses; reads > 0 {
		sample.HitRateKnown = true
		sample.HitRatePercent = round1(hits / reads * 100)
	}
	return sample, version, version != ""
}

func (c *Collector) querySample() (Sample, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reply := c.client.Info(ctx)
	if reply.Err() != nil {
		return Sample{}, "", false
	}
	return parseInfo(reply.Val())
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
// the other collectors so the Overview time-range selector behaves the same.
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
