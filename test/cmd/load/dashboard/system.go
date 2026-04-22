// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SystemMetrics holds system resource metrics
type SystemMetrics struct {
	mu sync.RWMutex

	CPUPercent    float64
	MemoryUsedMB  float64
	MemoryTotalMB float64
	DiskReadMB    float64
	DiskWriteMB   float64
	NetworkRxMB   float64
	NetworkTxMB   float64

	// Previous values for delta calculations
	lastCPU         cpuStat
	lastDisk        diskStat
	lastNet         netStat
	lastCollectTime time.Time
}

type cpuStat struct {
	user   uint64
	nice   uint64
	system uint64
	idle   uint64
	iowait uint64
	irq    uint64
	soft   uint64
}

type diskStat struct {
	readBytes  uint64
	writeBytes uint64
}

type netStat struct {
	rxBytes uint64
	txBytes uint64
}

// NewSystemMetrics creates a new system metrics collector
func NewSystemMetrics() *SystemMetrics {
	sm := &SystemMetrics{
		lastCollectTime: time.Now(),
	}
	// Initialize baseline values
	_ = sm.collect()
	return sm
}

// Update collects current system metrics
func (sm *SystemMetrics) Update() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.collect()
}

// GetSnapshot returns a copy of current metrics
func (sm *SystemMetrics) GetSnapshot() SystemMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Return a copy with only the consumer-facing fields
	return SystemMetrics{
		CPUPercent:    sm.CPUPercent,
		MemoryUsedMB:  sm.MemoryUsedMB,
		MemoryTotalMB: sm.MemoryTotalMB,
		DiskReadMB:    sm.DiskReadMB,
		DiskWriteMB:   sm.DiskWriteMB,
		NetworkRxMB:   sm.NetworkRxMB,
		NetworkTxMB:   sm.NetworkTxMB,
	}
}

func (sm *SystemMetrics) collect() error {
	now := time.Now()
	elapsed := now.Sub(sm.lastCollectTime).Seconds()
	if elapsed == 0 {
		elapsed = 1 // Avoid division by zero
	}

	// Collect CPU stats
	if err := sm.collectCPU(elapsed); err != nil {
		return fmt.Errorf("collect CPU: %w", err)
	}

	// Collect memory stats
	if err := sm.collectMemory(); err != nil {
		return fmt.Errorf("collect memory: %w", err)
	}

	// Collect disk I/O stats
	if err := sm.collectDisk(elapsed); err != nil {
		return fmt.Errorf("collect disk: %w", err)
	}

	// Collect network stats
	if err := sm.collectNetwork(elapsed); err != nil {
		return fmt.Errorf("collect network: %w", err)
	}

	sm.lastCollectTime = now
	return nil
}

func (sm *SystemMetrics) collectCPU(elapsed float64) error {
	if runtime.GOOS != "linux" {
		// Fallback for non-Linux systems
		sm.CPUPercent = 0
		return nil
	}

	f, err := os.Open("/proc/stat")
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			return fmt.Errorf("invalid cpu stat line")
		}

		curr := cpuStat{
			user:   parseUint64(fields[1]),
			nice:   parseUint64(fields[2]),
			system: parseUint64(fields[3]),
			idle:   parseUint64(fields[4]),
			iowait: parseUint64(fields[5]),
			irq:    parseUint64(fields[6]),
			soft:   parseUint64(fields[7]),
		}

		// Calculate deltas
		if sm.lastCPU.user > 0 { // Skip first measurement
			prevIdle := sm.lastCPU.idle + sm.lastCPU.iowait
			currIdle := curr.idle + curr.iowait

			prevNonIdle := sm.lastCPU.user + sm.lastCPU.nice + sm.lastCPU.system +
				sm.lastCPU.irq + sm.lastCPU.soft
			currNonIdle := curr.user + curr.nice + curr.system + curr.irq + curr.soft

			prevTotal := prevIdle + prevNonIdle
			currTotal := currIdle + currNonIdle

			totald := float64(currTotal - prevTotal)
			idled := float64(currIdle - prevIdle)

			if totald > 0 {
				sm.CPUPercent = (totald - idled) / totald * 100
			}
		}

		sm.lastCPU = curr
		break
	}

	return scanner.Err()
}

func (sm *SystemMetrics) collectMemory() error {
	if runtime.GOOS != "linux" {
		// Fallback for non-Linux systems
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		sm.MemoryUsedMB = float64(m.Alloc) / 1024 / 1024
		sm.MemoryTotalMB = float64(m.Sys) / 1024 / 1024
		return nil
	}

	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return err
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			memTotal = parseUint64(fields[1])
		case "MemAvailable:":
			memAvailable = parseUint64(fields[1])
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	sm.MemoryTotalMB = float64(memTotal) / 1024
	sm.MemoryUsedMB = float64(memTotal-memAvailable) / 1024
	return nil
}

func (sm *SystemMetrics) collectDisk(elapsed float64) error {
	if runtime.GOOS != "linux" {
		// Fallback for non-Linux systems
		sm.DiskReadMB = 0
		sm.DiskWriteMB = 0
		return nil
	}

	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return err
	}
	defer f.Close()

	var totalRead, totalWrite uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}

		// Skip loop and ram devices
		deviceName := fields[2]
		if strings.HasPrefix(deviceName, "loop") ||
			strings.HasPrefix(deviceName, "ram") {
			continue
		}

		// Sectors read and written (field 5 and 9)
		// Each sector is 512 bytes
		sectorsRead := parseUint64(fields[5])
		sectorsWritten := parseUint64(fields[9])

		totalRead += sectorsRead * 512
		totalWrite += sectorsWritten * 512
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	curr := diskStat{
		readBytes:  totalRead,
		writeBytes: totalWrite,
	}

	// Calculate rates (MB/s)
	if sm.lastDisk.readBytes > 0 && elapsed > 0 {
		readDelta := float64(curr.readBytes-sm.lastDisk.readBytes) / 1024 / 1024
		writeDelta := float64(curr.writeBytes-sm.lastDisk.writeBytes) / 1024 / 1024

		sm.DiskReadMB = readDelta / elapsed
		sm.DiskWriteMB = writeDelta / elapsed
	}

	sm.lastDisk = curr
	return nil
}

func (sm *SystemMetrics) collectNetwork(elapsed float64) error {
	if runtime.GOOS != "linux" {
		// Fallback for non-Linux systems
		sm.NetworkRxMB = 0
		sm.NetworkTxMB = 0
		return nil
	}

	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return err
	}
	defer f.Close()

	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(f)
	// Skip header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		// Skip loopback
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		totalRx += parseUint64(fields[0])
		totalTx += parseUint64(fields[8])
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	curr := netStat{
		rxBytes: totalRx,
		txBytes: totalTx,
	}

	// Calculate rates (MB/s)
	if sm.lastNet.rxBytes > 0 && elapsed > 0 {
		rxDelta := float64(curr.rxBytes-sm.lastNet.rxBytes) / 1024 / 1024
		txDelta := float64(curr.txBytes-sm.lastNet.txBytes) / 1024 / 1024

		sm.NetworkRxMB = rxDelta / elapsed
		sm.NetworkTxMB = txDelta / elapsed
	}

	sm.lastNet = curr
	return nil
}

func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}
