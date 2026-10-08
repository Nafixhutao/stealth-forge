package hostmetrics

import (
	"strings"
	"testing"
)

func TestParseCPU(t *testing.T) {
	// user nice system idle iowait irq softirq steal guest guest_nice
	stat := "cpu  100 0 50 800 50 0 0 0 0 0\ncpu0 50 0 25 400 25 0 0 0 0 0\n"
	busy, idle := parseCPU(strings.NewReader(stat))
	// idle = idle(800) + iowait(50) = 850; busy = user+nice+system+irq+softirq+steal = 150
	if busy != 150 || idle != 850 {
		t.Fatalf("parseCPU = busy %d idle %d, want 150/850", busy, idle)
	}
}

func TestParseMem(t *testing.T) {
	meminfo := "MemTotal:       16000000 kB\nMemFree:         2000000 kB\nMemAvailable:    9000000 kB\nBuffers:          100000 kB\n"
	used, total := parseMem(strings.NewReader(meminfo))
	if total != 16000000*1024 {
		t.Fatalf("total = %d, want %d", total, uint64(16000000)*1024)
	}
	if used != (16000000-9000000)*1024 {
		t.Fatalf("used = %d, want %d", used, uint64(7000000)*1024)
	}
}

func TestParseMemWithoutAvailable(t *testing.T) {
	// Older kernels may omit MemAvailable; MemFree is the fallback.
	used, total := parseMem(strings.NewReader("MemTotal: 1000 kB\nMemFree: 400 kB\n"))
	if total != 1000*1024 || used != 600*1024 {
		t.Fatalf("used/total = %d/%d, want %d/%d", used, total, uint64(600)*1024, uint64(1000)*1024)
	}
}

func TestParseNet(t *testing.T) {
	// Interfaces: lo is excluded; eth0 and eth1 are summed.
	dev := strings.Join([]string{
		"Inter-|   Receive                                                |  Transmit",
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed",
		"    lo: 5000       50    0    0    0     0          0         0     5000      50    0    0    0     0       0          0",
		"  eth0: 1000       10    0    0    0     0          0         0      200      2    0    0    0     0       0          0",
		"  eth1: 3000       30    0    0    0     0          0         0      400      4    0    0    0     0       0          0",
	}, "\n")
	rx, tx := parseNet(strings.NewReader(dev))
	if rx != 4000 || tx != 600 {
		t.Fatalf("parseNet = rx %d tx %d, want 4000/600", rx, tx)
	}
}

func TestParseNetExcludesVirtual(t *testing.T) {
	// Physical ens34 is summed; veth/br/docker/lo are skipped.
	dev := strings.Join([]string{
		"    lo: 100000     100    0    0    0     0          0         0   100000     100    0    0    0     0       0          0",
		"  ens34: 1000       10    0    0    0     0          0         0      200      2    0    0    0     0       0          0",
		"veth123: 999999     99    0    0    0     0          0         0   999999     99    0    0    0     0       0          0",
		"docker0: 55555     55    0    0    0     0          0         0    55555     55    0    0    0     0       0          0",
		"br-abcd: 77777     77    0    0    0     0          0         0    77777     77    0    0    0     0       0          0",
	}, "\n")
	rx, tx := parseNet(strings.NewReader(dev))
	if rx != 1000 || tx != 200 {
		t.Fatalf("parseNet = rx %d tx %d, want 1000/200", rx, tx)
	}
}

func TestSnapshotEmpty(t *testing.T) {
	c := New(0, 0, "/")
	snapshot := c.Snapshot()
	if len(snapshot.History) != 0 {
		t.Fatalf("history = %d, want 0", len(snapshot.History))
	}
}
