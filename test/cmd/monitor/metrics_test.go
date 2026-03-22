// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"
	"time"
)

func TestMetricsUpdate(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	// Test basic update
	m.Update(100.5, 99.8, 12345, "HEALTHY")

	snap := m.GetSnapshot()
	if snap.actualTPS != 100.5 {
		t.Errorf("Expected actualTPS 100.5, got %f", snap.actualTPS)
	}
	if snap.blockTPS != 99.8 {
		t.Errorf("Expected blockTPS 99.8, got %f", snap.blockTPS)
	}
	if snap.blockHeight != 12345 {
		t.Errorf("Expected blockHeight 12345, got %d", snap.blockHeight)
	}
	if snap.nodeStatus != "HEALTHY" {
		t.Errorf("Expected nodeStatus HEALTHY, got %s", snap.nodeStatus)
	}
}

func TestMetricsLatency(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	// Add some latency samples
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	for _, lat := range latencies {
		m.AddLatency(lat)
	}

	m.UpdateSystem()
	snap := m.GetSnapshot()

	// Check that percentiles are calculated
	if snap.latencyP50 == 0 {
		t.Error("Expected non-zero P50 latency")
	}
	if snap.latencyP95 == 0 {
		t.Error("Expected non-zero P95 latency")
	}
	if snap.latencyP99 == 0 {
		t.Error("Expected non-zero P99 latency")
	}

	// P99 should be >= P95 >= P50
	if snap.latencyP99 < snap.latencyP95 {
		t.Errorf("P99 (%v) should be >= P95 (%v)", snap.latencyP99, snap.latencyP95)
	}
	if snap.latencyP95 < snap.latencyP50 {
		t.Errorf("P95 (%v) should be >= P50 (%v)", snap.latencyP95, snap.latencyP50)
	}
}

func TestMetricsLatencyBufferLimit(t *testing.T) {
	m := &Metrics{
		errors: make(map[string]int64),
	}

	// Add more than 100 samples
	for i := range 150 {
		m.AddLatency(time.Duration(i) * time.Millisecond)
	}

	m.mu.RLock()
	bufLen := len(m.requestLatencies)
	m.mu.RUnlock()

	if bufLen > 100 {
		t.Errorf("Latency buffer should be limited to 100 samples, got %d", bufLen)
	}
}

func TestMakeBar(t *testing.T) {
	tests := []struct {
		name    string
		percent float64
		width   int
	}{
		{"Zero percent", 0, 10},
		{"Fifty percent", 50, 20},
		{"Full", 100, 10},
		{"Over 100", 150, 10},
		{"Negative", -10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := makeBar(tt.percent, tt.width)
			if bar == "" {
				t.Error("Bar should not be empty")
			}
			// Bar should start with [ and end with ]
			if bar[0] != '[' {
				t.Error("Bar should start with [")
			}
		})
	}
}

func TestMetricsSnapshot(t *testing.T) {
	m := &Metrics{
		errors:             make(map[string]int64),
		actualTPS:          123.45,
		targetTPS:          100.0,
		blockProductionTPS: 120.0,
		blockHeight:        5000,
		nodeStatus:         "HEALTHY",
		memoryUsedMB:       512,
		memoryTotalMB:      2048,
		cpuPercent:         45.2,
	}

	snap := m.GetSnapshot()

	if snap.actualTPS != m.actualTPS {
		t.Error("Snapshot should match actual TPS")
	}
	if snap.targetTPS != m.targetTPS {
		t.Error("Snapshot should match target TPS")
	}
	if snap.blockHeight != m.blockHeight {
		t.Error("Snapshot should match block height")
	}
	if snap.nodeStatus != m.nodeStatus {
		t.Error("Snapshot should match node status")
	}
}
