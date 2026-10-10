package dbmetrics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// stubRow returns canned scan results in order.
type stubRow struct {
	values []any
}

func (r stubRow) Scan(dest ...any) error {
	for index, value := range r.values {
		if index >= len(dest) {
			break
		}
		switch target := dest[index].(type) {
		case *int64:
			*target = value.(int64)
		case *uint64:
			*target = value.(uint64)
		case *float64:
			*target = value.(float64)
		case *string:
			*target = value.(string)
		}
	}
	return nil
}

var sampleRowValues = []any{int64(12), int64(3), int64(200), uint64(980), uint64(20), uint64(1000), int64(0), uint64(1 << 30), 3600.0}

type stubQuerier struct{}

func (stubQuerier) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "server_version") {
		return stubRow{values: []any{"PostgreSQL 17.2"}}
	}
	return stubRow{values: sampleRowValues}
}

func TestCollectorComputesCacheRatioAndTransactionRate(t *testing.T) {
	querier := stubQuerier{}
	collector := New(querier, time.Second, 10)

	collector.sampleOnce()
	snapshot := collector.Snapshot()
	if !snapshot.Reachable {
		t.Fatal("first sample should mark the collector reachable")
	}
	if snapshot.ServerVersion == "" {
		t.Fatal("server version should be captured on the first sample")
	}
	current := snapshot.Current
	if current.Connections != 12 || current.MaxConnections != 200 {
		t.Fatalf("unexpected connection sample: %+v", current)
	}
	if !current.CacheHitRatioKnown {
		t.Fatal("cache ratio should be known when blocks were read")
	}
	// 980 hit / (980 + 20 read) = 98%.
	if current.CacheHitRatioPercent != 98 {
		t.Fatalf("cache hit ratio = %v, want 98", current.CacheHitRatioPercent)
	}
	// The first sample has no predecessor, so the rate must be zero.
	if current.TransactionsPerSec != 0 {
		t.Fatalf("first sample transaction rate = %v, want 0", current.TransactionsPerSec)
	}

	// One second later the counter advanced by 250 xacts → 250/s.
	sampleRowValues[5] = uint64(1250)
	time.Sleep(10 * time.Millisecond)
	collector.sampleOnce()
	snapshot = collector.Snapshot()
	if snapshot.Current.TransactionsPerSec <= 0 {
		t.Fatalf("transaction rate = %v, want positive", snapshot.Current.TransactionsPerSec)
	}
	if len(snapshot.History) != 2 {
		t.Fatalf("history length = %d, want 2", len(snapshot.History))
	}
}

func TestCollectorUnreachableKeepsHistoryAndFlipsReachable(t *testing.T) {
	collector := New(stubQuerier{}, time.Second, 10)
	collector.sampleOnce()

	// Simulate a database error by making Scan fail.
	collector.pool = failingQuerier{}
	collector.sampleOnce()
	snapshot := collector.Snapshot()
	if snapshot.Reachable {
		t.Fatal("snapshot should report unreachable after a failed sample")
	}
	if len(snapshot.History) != 1 {
		t.Fatalf("failed samples must not append history, length = %d", len(snapshot.History))
	}
}

type failingQuerier struct{}

func (failingQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return failingRow{}
}

type failingRow struct{}

func (failingRow) Scan(_ ...any) error { return context.DeadlineExceeded }

func TestWindowedFiltersBySince(t *testing.T) {
	collector := New(stubQuerier{}, time.Second, 10)
	collector.sampleOnce()

	// Backdate the single sample, then sample again so the ring holds one old
	// and one fresh sample.
	collector.mu.Lock()
	collector.history[0].Timestamp = time.Now().UTC().Add(-2 * time.Hour)
	collector.mu.Unlock()
	collector.sampleOnce()

	since := time.Now().UTC().Add(-time.Hour)
	snapshot := collector.SnapshotSince(since)
	if len(snapshot.History) != 1 {
		t.Fatalf("windowed history length = %d, want 1 (only the fresh sample)", len(snapshot.History))
	}
	if snapshot.History[0].Timestamp.Before(since) {
		t.Fatal("windowed history returned a sample older than the cutoff")
	}
}
