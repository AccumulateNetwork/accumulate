// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfig tests configuration loading from JSON file.
func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"serverUrl": "http://localhost:8080/v3",
		"targetTps": 50,
		"duration": 30
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config := &Config{ConfigFile: configPath}
	err := loadConfig(config)

	if err != nil {
		t.Errorf("loadConfig() unexpected error: %v", err)
	}
	if config.ServerURL != "http://localhost:8080/v3" {
		t.Errorf("ServerURL = %s, want http://localhost:8080/v3", config.ServerURL)
	}
	if config.TargetTPS != 50 {
		t.Errorf("TargetTPS = %d, want 50", config.TargetTPS)
	}
}

// TestNewMetrics tests metrics initialization.
func TestNewMetrics(t *testing.T) {
	m := newMetrics()

	if m == nil {
		t.Fatal("newMetrics() returned nil")
	}

	expectedOps := []string{
		"token-transfer-lite", "token-transfer-adi", "write-data",
		"create-account", "key-rotation", "create-keypage", "create-keybook",
	}

	for _, op := range expectedOps {
		if _, ok := m.OperationCounts[op]; !ok {
			t.Errorf("OperationCounts missing entry for %s", op)
		}
		if _, ok := m.OperationLatency[op]; !ok {
			t.Errorf("OperationLatency missing entry for %s", op)
		}
	}
}

// TestLatencyTracker tests latency tracking functionality.
func TestLatencyTracker(t *testing.T) {
	lt := &LatencyTracker{
		samples: make([]time.Duration, 0, 1000),
	}

	// Record some latencies
	durations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		150 * time.Millisecond,
	}

	for _, d := range durations {
		lt.Record(d)
	}

	count, avg, min, max := lt.Stats()

	if count != uint64(len(durations)) {
		t.Errorf("count = %d, want %d", count, len(durations))
	}
	if avg != 150 {
		t.Errorf("avg = %d, want 150", avg)
	}
	if min != 100 {
		t.Errorf("min = %d, want 100", min)
	}
	if max != 200 {
		t.Errorf("max = %d, want 200", max)
	}
}

// TestOperationSelection tests the weighted operation selection.
func TestOperationSelection(t *testing.T) {
	config := &Config{
		OperationMix: map[string]int{
			"op1": 50,
			"op2": 30,
			"op3": 20,
		},
	}

	lg := &LoadGenerator{
		config:  config,
		metrics: newMetrics(),
	}

	totalWeight := 100
	iterations := 1000
	counts := make(map[string]int)

	// Select operations many times
	for i := 0; i < iterations; i++ {
		op := lg.selectOperation(totalWeight)
		counts[op]++
	}

	// Check that all operations were selected at least once
	for op := range config.OperationMix {
		if counts[op] == 0 {
			t.Errorf("Operation %s was never selected", op)
		}
	}
}

// TestRampUpCalculation tests TPS ramp-up calculation.
func TestRampUpCalculation(t *testing.T) {
	tests := []struct {
		targetTPS     int
		rampUpSeconds int
		currentSecond int
		expectedTPS   int
	}{
		{100, 10, 1, 10},
		{100, 10, 5, 50},
		{100, 10, 10, 100},
		{100, 10, 15, 100},
	}

	for _, tt := range tests {
		currentTPS := tt.targetTPS
		if tt.currentSecond <= tt.rampUpSeconds && tt.rampUpSeconds > 0 {
			currentTPS = (tt.targetTPS * tt.currentSecond) / tt.rampUpSeconds
			if currentTPS < 1 {
				currentTPS = 1
			}
		}

		if currentTPS != tt.expectedTPS {
			t.Errorf("TPS at second %d = %d, want %d",
				tt.currentSecond, currentTPS, tt.expectedTPS)
		}
	}
}
