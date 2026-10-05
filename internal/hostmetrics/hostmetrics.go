// Package hostmetrics samples local host resource usage directly from the
// kernel. It is a lightweight, dependency-free source for the admin Overview's
// CPU/memory/disk/network cards, replacing the ClickHouse-backed telemetry path
// for host resources. A small in-memory ring buffer backs short history charts.
package hostmetrics

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	procStat   = "/proc/stat"
	procMem    = "/proc/meminfo"
	procNetDev = "/proc/net/dev"
)

// Sample is one host resource observation. Byte counters are absolute; the
// network fields are per-second rates derived from the previous sample.
type Sample struct {
	Timestamp            time.Time `json:"timestamp"`
	CPUPercent           float64   `json:"cpu_percent"`
	MemoryUsedBytes      uint64    `json:"memory_used_bytes"`
	MemoryTotalBytes     uint64    `json:"memory_total_bytes"`
	DiskUsedBytes        uint64    `json:"disk_used_bytes"`
	DiskTotalBytes       uint64    `json:"disk_total_bytes"`
	NetworkRxBytesPerSec float64   `json:"network_rx_bytes_per_sec"`
	NetworkTxBytesPerSec float64   `json:"network_tx_bytes_per_sec"`
}

// Snapshot is the current sample plus bounded recent history for charts.
type Snapshot struct {
	Current Sample   `json:"current"`
	History []Sample `json:"history"`
}

// Collector samples host metrics on an interval and retains a bounded history.
type Collector struct {
	mu       sync.Mutex
	history  []Sample
	max      int
	interval time.Duration
	diskPath string

	prevBusy, prevIdle uint64
	prevRx, prevTx     uint64
	prevAt             time.Time
	havePrev           bool
}

// New builds a collector. interval controls the sampling cadence, maxHistory the
// retained sample count, and diskPath the filesystem whose usage is reported.
func New(interval time.Duration, maxHistory int, diskPath string) *Collector {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if maxHistory <= 0 {
		maxHistory = 60
	}
	if strings.TrimSpace(diskPath) == "" {
		diskPath = "/"
	}
	return &Collector{max: maxHistory, interval: interval, diskPath: diskPath}
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
	now := time.Now().UTC()
	busy, idle := readCPU()
	rx, tx := readNet()

	sample := Sample{Timestamp: now}
	sample.MemoryUsedBytes, sample.MemoryTotalBytes = readMem()
	sample.DiskUsedBytes, sample.DiskTotalBytes = readDisk(c.diskPath)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.havePrev {
		elapsed := now.Sub(c.prevAt).Seconds()
		if elapsed > 0 {
			totalDelta := (busy + idle) - (c.prevBusy + c.prevIdle)
			busyDelta := busy - c.prevBusy
			if totalDelta > 0 {
				sample.CPUPercent = round2(float64(busyDelta) / float64(totalDelta) * 100)
			}
			sample.NetworkRxBytesPerSec = round2(float64(rx-c.prevRx) / elapsed)
			sample.NetworkTxBytesPerSec = round2(float64(tx-c.prevTx) / elapsed)
		}
	}
	c.prevBusy, c.prevIdle, c.prevRx, c.prevTx, c.prevAt = busy, idle, rx, tx, now
	c.havePrev = true

	c.history = append(c.history, sample)
	if len(c.history) > c.max {
		c.history = c.history[len(c.history)-c.max:]
	}
}

// Snapshot returns the latest sample and a copy of the retained history.
func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	history := make([]Sample, len(c.history))
	copy(history, c.history)
	snapshot := Snapshot{History: history}
	if len(history) > 0 {
		snapshot.Current = history[len(history)-1]
	}
	return snapshot
}

func readCPU() (busy, idle uint64) {
	file, err := os.Open(procStat)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	return parseCPU(file)
}

// parseCPU sums the aggregate "cpu" line into busy and idle jiffies.
func parseCPU(r io.Reader) (busy, idle uint64) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var values []uint64
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, 0
			}
			values = append(values, value)
		}
		// user nice system idle iowait irq softirq steal ...
		idle = values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		for index, value := range values {
			if index == 3 || index == 4 {
				continue
			}
			busy += value
		}
		return busy, idle
	}
	return 0, 0
}

func readMem() (used, total uint64) {
	file, err := os.Open(procMem)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	return parseMem(file)
}

// parseMem returns used = MemTotal - MemAvailable (kB → bytes). When the kernel
// omits MemAvailable, MemFree is used as the fallback.
func parseMem(r io.Reader) (used, total uint64) {
	var totalKB, availableKB, freeKB uint64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = value
		case "MemAvailable:":
			availableKB = value
		case "MemFree:":
			freeKB = value
		}
	}
	if totalKB == 0 {
		return 0, 0
	}
	available := availableKB
	if available == 0 {
		available = freeKB
	}
	usedKB := uint64(0)
	if totalKB > available {
		usedKB = totalKB - available
	}
	return usedKB * 1024, totalKB * 1024
}

func readNet() (rx, tx uint64) {
	file, err := os.Open(procNetDev)
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	return parseNet(file)
}

// parseNet sums received/transmitted bytes across non-loopback interfaces.
func parseNet(r io.Reader) (rx, tx uint64) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "lo" || name == "" {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 9 {
			continue
		}
		rxBytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		txBytes, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			continue
		}
		rx += rxBytes
		tx += txBytes
	}
	return rx, tx
}

func readDisk(path string) (used, total uint64) {
	var stat statfs
	if err := statfsCall(path, &stat); err != nil {
		return 0, 0
	}
	blockSize := uint64(stat.Bsize)
	total = stat.Blocks * blockSize
	free := stat.Bavail * blockSize
	if total > free {
		used = total - free
	}
	return used, total
}

func round2(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}
