// Copyright 2026 The Accumulate Authors
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
	p95Idx := n*95/100 - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	p99Idx := n*99/100 - 1
	if p99Idx < 0 {
		p99Idx = 0
	}
	p95 = sorted[p95Idx]
	p99 = sorted[p99Idx]

	return avg, p95, p99
}
