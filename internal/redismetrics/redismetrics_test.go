package redismetrics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const sampleInfo = `# Server
redis_version:8.2.0
uptime_in_seconds:123456
# Clients
connected_clients:24
maxclients:10000
# Memory
used_memory:1048576
maxmemory:0
# Stats
instantaneous_ops_per_sec:812
keyspace_hits:980
keyspace_misses:20
# Keyspace
db0:keys=42,expires=5,avg_ttl=0
`

type stubReply struct{ payload string }

func (r *stubReply) Val() string { return r.payload }
func (r *stubReply) Err() error  { return nil }

type stubClient struct{ payload string }

func (c stubClient) Info(_ context.Context, _ ...string) *redis.StringCmd {
	cmd := &redis.StringCmd{}
	cmd.SetVal(c.payload)
	return cmd
}

func TestParseInfoExtractsAllSections(t *testing.T) {
	sample, version, ok := parseInfo(sampleInfo)
	if !ok {
		t.Fatal("payload with a version line should parse")
	}
	if version != "8.2.0" {
		t.Fatalf("version = %q, want 8.2.0", version)
	}
	if sample.Connections != 24 || sample.MaxClients != 10000 {
		t.Fatalf("unexpected client counts: %+v", sample)
	}
	if sample.OpsPerSec != 812 {
		t.Fatalf("ops/sec = %v, want 812", sample.OpsPerSec)
	}
	// 980 hits / (980 + 20 misses) = 98%.
	if !sample.HitRateKnown || sample.HitRatePercent != 98 {
		t.Fatalf("hit rate = known=%v %v, want known 98", sample.HitRateKnown, sample.HitRatePercent)
	}
	if sample.MemoryUsedBytes != 1<<20 {
		t.Fatalf("used memory = %d, want 1MiB", sample.MemoryUsedBytes)
	}
	if sample.MemoryMaxBytes != 0 {
		t.Fatal("maxmemory 0 must stay 0 (no cap configured)")
	}
	if sample.Keys != 42 {
		t.Fatalf("keys = %d, want 42", sample.Keys)
	}
	if sample.UptimeSeconds != 123456 {
		t.Fatalf("uptime = %v, want 123456", sample.UptimeSeconds)
	}
}

func TestParseInfoWithoutReadsHasNoHitRate(t *testing.T) {
	payload := strings.Replace(sampleInfo, "keyspace_hits:980\n", "", 1)
	payload = strings.Replace(payload, "keyspace_misses:20\n", "", 1)
	sample, _, ok := parseInfo(payload)
	if !ok {
		t.Fatal("payload should parse")
	}
	if sample.HitRateKnown {
		t.Fatal("hit rate must be unknown when there are no reads")
	}
}

func TestCollectorUnreachableKeepsHistory(t *testing.T) {
	collector := New(stubClient{payload: sampleInfo}, time.Second, 10)
	collector.sampleOnce()
	if s := collector.Snapshot(); !s.Reachable || s.ServerVersion != "8.2.0" {
		t.Fatalf("first sample should be reachable with a version: %+v", s)
	}

	collector.client = failingClient{}
	collector.sampleOnce()
	snapshot := collector.Snapshot()
	if snapshot.Reachable {
		t.Fatal("snapshot should report unreachable after a failed sample")
	}
	if len(snapshot.History) != 1 {
		t.Fatalf("failed samples must not append history, length = %d", len(snapshot.History))
	}
}

type failingClient struct{}

func (failingClient) Info(_ context.Context, _ ...string) *redis.StringCmd {
	cmd := &redis.StringCmd{}
	cmd.SetErr(context.DeadlineExceeded)
	return cmd
}

func TestSnapshotSinceFiltersOldSamples(t *testing.T) {
	collector := New(stubClient{payload: sampleInfo}, time.Second, 10)
	collector.sampleOnce()
	collector.mu.Lock()
	collector.history[0].Timestamp = time.Now().UTC().Add(-2 * time.Hour)
	collector.mu.Unlock()
	collector.sampleOnce()

	snapshot := collector.SnapshotSince(time.Now().UTC().Add(-time.Hour))
	if len(snapshot.History) != 1 {
		t.Fatalf("windowed history length = %d, want 1", len(snapshot.History))
	}
}
