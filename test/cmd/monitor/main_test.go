// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"
	"time"
)

func TestMetricsUpdateSystem(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	m.UpdateSystem()

	// Check that memory metrics are updated
	if m.memoryUsedMB == 0 {
		t.Error("Expected non-zero memory used after UpdateSystem()")
	}

	if m.memoryTotalMB == 0 {
		t.Error("Expected non-zero memory total after UpdateSystem()")
	}

	// CPU should be calculated based on goroutines
	if m.cpuPercent < 0 || m.cpuPercent > 100 {
		t.Errorf("Expected CPU percent in range [0, 100], got %f", m.cpuPercent)
	}
}

func TestMetricsConcurrentAccess(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	done := make(chan struct{})

	// Concurrent updates
	go func() {
		for i := 0; i < 50; i++ {
			m.Update(100.0, 95.0, uint64(i), "HEALTHY")
			time.Sleep(5 * time.Millisecond)
		}
		close(done)
	}()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		_ = m.GetSnapshot()
		time.Sleep(2 * time.Millisecond)
	}

	<-done
}

func TestMetricsAddLatency_MaxBuffer(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	// Add more than 100 latencies
	for i := 0; i < 150; i++ {
		m.AddLatency(time.Duration(i) * time.Millisecond)
	}

	m.mu.RLock()
	count := len(m.requestLatencies)
	m.mu.RUnlock()

	if count > 100 {
		t.Errorf("Expected max 100 latencies in buffer, got %d", count)
	}

	// Should have kept the latest values
	if count == 100 {
		m.mu.RLock()
		firstLatency := m.requestLatencies[0]
		m.mu.RUnlock()

		// First latency should be from iteration 50 (150 - 100 = 50)
		expected := 50 * time.Millisecond
		if firstLatency != expected {
			t.Errorf("Expected first latency to be %v, got %v", expected, firstLatency)
		}
	}
}

func TestMetricsLatencyPercentiles_Ordering(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	// Add known latencies
	for i := 1; i <= 100; i++ {
		m.AddLatency(time.Duration(i) * time.Millisecond)
	}

	m.UpdateSystem()
	snap := m.GetSnapshot()

	// Verify ordering: P50 <= P95 <= P99
	if snap.latencyP50 > snap.latencyP95 {
		t.Errorf("P50 (%v) should be <= P95 (%v)", snap.latencyP50, snap.latencyP95)
	}

	if snap.latencyP95 > snap.latencyP99 {
		t.Errorf("P95 (%v) should be <= P99 (%v)", snap.latencyP95, snap.latencyP99)
	}
}

func TestMetricsLatencyPercentiles_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected bool // Whether percentiles should be calculated
	}{
		{"No latencies", 0, false},
		{"One latency", 1, true},
		{"Two latencies", 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Metrics{
				errors: make(map[string]int64),
			}

			for i := 0; i < tt.count; i++ {
				m.AddLatency(time.Duration(10) * time.Millisecond)
			}

			m.UpdateSystem()
			snap := m.GetSnapshot()

			if tt.expected {
				if snap.latencyP50 == 0 {
					t.Error("Expected non-zero P50 latency")
				}
			} else {
				if snap.latencyP50 != 0 || snap.latencyP95 != 0 || snap.latencyP99 != 0 {
					t.Error("Expected zero percentiles with no latencies")
				}
			}
		})
	}
}

func TestMakeBar_Bounds(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		width   int
	}{
		{"Negative percent", -50, 30},
		{"Zero percent", 0, 30},
		{"Normal percent", 50, 30},
		{"Over 100 percent", 150, 30},
		{"At 70 threshold", 70, 30},
		{"At 90 threshold", 90, 30},
		{"At 100 percent", 100, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := makeBar(tt.percent, tt.width)

			// Should always have brackets
			if len(bar) == 0 {
				t.Fatal("Bar should not be empty")
			}

			// Should start with [ and contain ]
			if bar[0] != '[' {
				t.Error("Bar should start with '['")
			}
		})
	}
}

func TestMetricsUpdate_MultipleFields(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	// Update all fields
	m.Update(123.45, 98.76, 54321, "DEGRADED")

	snap := m.GetSnapshot()

	if snap.actualTPS != 123.45 {
		t.Errorf("Expected actualTPS 123.45, got %f", snap.actualTPS)
	}

	if snap.blockTPS != 98.76 {
		t.Errorf("Expected blockTPS 98.76, got %f", snap.blockTPS)
	}

	if snap.blockHeight != 54321 {
		t.Errorf("Expected blockHeight 54321, got %d", snap.blockHeight)
	}

	if snap.nodeStatus != "DEGRADED" {
		t.Errorf("Expected nodeStatus DEGRADED, got %s", snap.nodeStatus)
	}

	// lastUpdate should be recent
	if time.Since(snap.lastUpdate) > 1*time.Second {
		t.Error("lastUpdate should be recent")
	}
}

func TestMetricsGetSnapshot_ThreadSafe(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			m.Update(float64(i), float64(i), uint64(i), "HEALTHY")
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()

	// Multiple reader goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					snap := m.GetSnapshot()
					// Basic validation (blockHeight is uint64, cannot be negative)
					if snap.actualTPS < 0 {
						t.Error("Invalid snapshot values")
					}
				}
			}
		}()
	}

	<-done
	time.Sleep(50 * time.Millisecond) // Let readers finish
}

func TestMetricsAddLatency_SingleValue(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	latency := 42 * time.Millisecond
	m.AddLatency(latency)

	m.UpdateSystem()
	snap := m.GetSnapshot()

	// With a single value, all percentiles should be the same
	if snap.latencyP50 != latency {
		t.Errorf("Expected P50 to be %v, got %v", latency, snap.latencyP50)
	}

	if snap.latencyP95 != latency {
		t.Errorf("Expected P95 to be %v, got %v", latency, snap.latencyP95)
	}

	if snap.latencyP99 != latency {
		t.Errorf("Expected P99 to be %v, got %v", latency, snap.latencyP99)
	}
}

func TestMetricsSnapshot_TargetTPSPreserved(t *testing.T) {
	targetTPS := 250.0
	m := &Metrics{
		targetTPS: targetTPS,
		errors:    make(map[string]int64),
	}

	snap := m.GetSnapshot()

	if snap.targetTPS != targetTPS {
		t.Errorf("Expected targetTPS %f, got %f", targetTPS, snap.targetTPS)
	}
}

func TestMetricsInitialState(t *testing.T) {
	m := &Metrics{
		targetTPS: 100.0,
		errors:    make(map[string]int64),
	}

	snap := m.GetSnapshot()

	// Initial state should have defaults
	if snap.actualTPS != 0 {
		t.Errorf("Expected initial actualTPS 0, got %f", snap.actualTPS)
	}

	if snap.blockHeight != 0 {
		t.Errorf("Expected initial blockHeight 0, got %d", snap.blockHeight)
	}

	if snap.nodeStatus != "" {
		t.Errorf("Expected empty initial nodeStatus, got %s", snap.nodeStatus)
	}
}
