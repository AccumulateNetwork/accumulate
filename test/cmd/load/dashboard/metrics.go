// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"sort"
	"sync"
	"time"
)

// LoadMetricsSnapshot represents a point-in-time view of load test metrics
type LoadMetricsSnapshot struct {
	TotalTxs      int64
	SuccessfulTxs int64
	FailedTxs     int64
	CurrentTPS    float64
	AvgTPS        float64
	PeakTPS       float64
	AvgLatency    time.Duration
	P95Latency    time.Duration
	P99Latency    time.Duration
	ErrorRate     float64
	TargetTxs     int64
	StartTime     time.Time
}

// LoadMetrics tracks load test transaction metrics
type LoadMetrics struct {
	mu sync.RWMutex

	totalTxs      int64
	successfulTxs int64
	failedTxs     int64
	targetTxs     int64

	startTime   time.Time
	lastTxTime  time.Time
	peakTPS     float64
	windowStart time.Time
	windowTxs   int64

	latencies []time.Duration // Rolling window of recent latencies
	maxWindow int             // Maximum latency window size
}

// NewLoadMetrics creates a new load metrics tracker
func NewLoadMetrics(targetTxs int64) *LoadMetrics {
	now := time.Now()
	return &LoadMetrics{
		targetTxs:   targetTxs,
		startTime:   now,
		lastTxTime:  now,
		windowStart: now,
		latencies:   make([]time.Duration, 0, 1000),
		maxWindow:   1000,
	}
}

// RecordTransaction records a transaction result and latency
func (m *LoadMetrics) RecordTransaction(success bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.totalTxs++
	if success {
		m.successfulTxs++
	} else {
		m.failedTxs++
	}

	// Record latency
	m.latencies = append(m.latencies, latency)
	if len(m.latencies) > m.maxWindow {
		m.latencies = m.latencies[len(m.latencies)-m.maxWindow:]
	}

	m.lastTxTime = now

	// Update TPS window (1 second window)
	if now.Sub(m.windowStart) >= time.Second {
		currentTPS := float64(m.windowTxs) / now.Sub(m.windowStart).Seconds()
		if currentTPS > m.peakTPS {
			m.peakTPS = currentTPS
		}
		m.windowStart = now
		m.windowTxs = 0
	}
	m.windowTxs++
}

// GetSnapshot returns a thread-safe snapshot of current metrics
func (m *LoadMetrics) GetSnapshot() LoadMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	elapsed := now.Sub(m.startTime).Seconds()
	if elapsed == 0 {
		elapsed = 1
	}

	// Calculate current TPS (over last second)
	windowElapsed := now.Sub(m.windowStart).Seconds()
	if windowElapsed == 0 {
		windowElapsed = 1
	}
	currentTPS := float64(m.windowTxs) / windowElapsed

	// Calculate average TPS
	avgTPS := float64(m.totalTxs) / elapsed

	// Calculate error rate
	errorRate := 0.0
	if m.totalTxs > 0 {
		errorRate = float64(m.failedTxs) / float64(m.totalTxs) * 100
	}

	// Calculate latency stats
	avgLatency, p95Latency, p99Latency := m.calculateLatencyStats()

	peakTPS := m.peakTPS
	if currentTPS > peakTPS {
		peakTPS = currentTPS
	}

	return LoadMetricsSnapshot{
		TotalTxs:      m.totalTxs,
		SuccessfulTxs: m.successfulTxs,
		FailedTxs:     m.failedTxs,
		CurrentTPS:    currentTPS,
		AvgTPS:        avgTPS,
		PeakTPS:       peakTPS,
		AvgLatency:    avgLatency,
		P95Latency:    p95Latency,
		P99Latency:    p99Latency,
		ErrorRate:     errorRate,
		TargetTxs:     m.targetTxs,
		StartTime:     m.startTime,
	}
}

// calculateLatencyStats computes average and percentile latencies
// Note: caller must hold read lock
func (m *LoadMetrics) calculateLatencyStats() (avg, p95, p99 time.Duration) {
	if len(m.latencies) == 0 {
		return 0, 0, 0
	}

	// Calculate average
	var sum time.Duration
	for _, lat := range m.latencies {
		sum += lat
	}
	avg = sum / time.Duration(len(m.latencies))

	// Calculate percentiles
	sorted := make([]time.Duration, len(m.latencies))
	copy(sorted, m.latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	n := len(sorted)
	p95 = sorted[n*95/100]
	p99 = sorted[n*99/100]

	return avg, p95, p99
}

// MetricsSnapshot is a point-in-time copy of metrics without mutex
type MetricsSnapshot struct {
	// Transaction metrics
	TargetTPS    int
	ActualTPS    float64
	TotalTx      int
	SuccessTx    int
	FailedTx     int
	ErrorsByType map[string]int

	// Latency metrics (in milliseconds)
	LatencyP50 float64
	LatencyP95 float64
	LatencyP99 float64
	Latencies  []float64 // Rolling window of recent latencies

	// System resource metrics
	CPUPercent    float64
	MemoryPercent float64
	DiskIORead    float64 // MB/s
	DiskIOWrite   float64 // MB/s
	NetworkRx     float64 // MB/s
	NetworkTx     float64 // MB/s

	// Node metrics
	BlockRate      float64 // blocks per second
	NodeHealth     string  // "healthy", "degraded", "unhealthy"
	DiskSpaceGB    float64
	DiskSpaceTotal float64

	// Runtime metrics
	StartTime time.Time
	Runtime   time.Duration
}

// Metrics holds real-time load testing metrics
type Metrics struct {
	mu   sync.RWMutex
	data MetricsSnapshot
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		data: MetricsSnapshot{
			ErrorsByType: make(map[string]int),
			Latencies:    make([]float64, 0, 1000), // Keep last 1000 latencies
			StartTime:    time.Now(),
			NodeHealth:   "unknown",
		},
	}
}

// Update updates the metrics (thread-safe)
func (m *Metrics) Update(fn func(*MetricsSnapshot)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.data)
}

// Snapshot returns a copy of current metrics (thread-safe)
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy without the mutex
	snapshot := MetricsSnapshot{
		TargetTPS:      m.data.TargetTPS,
		ActualTPS:      m.data.ActualTPS,
		TotalTx:        m.data.TotalTx,
		SuccessTx:      m.data.SuccessTx,
		FailedTx:       m.data.FailedTx,
		LatencyP50:     m.data.LatencyP50,
		LatencyP95:     m.data.LatencyP95,
		LatencyP99:     m.data.LatencyP99,
		CPUPercent:     m.data.CPUPercent,
		MemoryPercent:  m.data.MemoryPercent,
		DiskIORead:     m.data.DiskIORead,
		DiskIOWrite:    m.data.DiskIOWrite,
		NetworkRx:      m.data.NetworkRx,
		NetworkTx:      m.data.NetworkTx,
		BlockRate:      m.data.BlockRate,
		NodeHealth:     m.data.NodeHealth,
		DiskSpaceGB:    m.data.DiskSpaceGB,
		DiskSpaceTotal: m.data.DiskSpaceTotal,
		StartTime:      m.data.StartTime,
		Runtime:        m.data.Runtime,
	}

	snapshot.ErrorsByType = make(map[string]int, len(m.data.ErrorsByType))
	for k, v := range m.data.ErrorsByType {
		snapshot.ErrorsByType[k] = v
	}

	// Copy latencies for calculation but don't expose the slice
	if len(m.data.Latencies) > 0 {
		snapshot.Latencies = make([]float64, len(m.data.Latencies))
		copy(snapshot.Latencies, m.data.Latencies)
	}

	return snapshot
}

// RecordTransaction records a transaction result
func (m *Metrics) RecordTransaction(success bool, latencyMs float64, errorType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data.TotalTx++
	if success {
		m.data.SuccessTx++
	} else {
		m.data.FailedTx++
		if errorType != "" {
			m.data.ErrorsByType[errorType]++
		}
	}

	// Add to rolling latency window
	m.data.Latencies = append(m.data.Latencies, latencyMs)
	// Keep only last 1000 latencies
	if len(m.data.Latencies) > 1000 {
		m.data.Latencies = m.data.Latencies[len(m.data.Latencies)-1000:]
	}

	// Update latency percentiles
	m.updatePercentiles()
}

// updatePercentiles calculates percentile values from latency data
func (m *Metrics) updatePercentiles() {
	if len(m.data.Latencies) == 0 {
		return
	}

	// Sort a copy to calculate percentiles
	sorted := make([]float64, len(m.data.Latencies))
	copy(sorted, m.data.Latencies)

	// Simple insertion sort for small arrays
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	// Calculate percentiles
	n := len(sorted)
	m.data.LatencyP50 = sorted[n*50/100]
	m.data.LatencyP95 = sorted[n*95/100]
	m.data.LatencyP99 = sorted[n*99/100]
}

// UpdateTPS updates the actual TPS calculation
func (m *Metrics) UpdateTPS() {
	m.mu.Lock()
	defer m.mu.Unlock()

	elapsed := time.Since(m.data.StartTime).Seconds()
	if elapsed > 0 {
		m.data.ActualTPS = float64(m.data.TotalTx) / elapsed
	}
	m.data.Runtime = time.Since(m.data.StartTime)
}
