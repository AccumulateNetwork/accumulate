// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBottleneckDetectionAccuracy(t *testing.T) {
	// Test that bottleneck detection uses proper thresholds
	t.Run("memory threshold accuracy", func(t *testing.T) {
		testCases := []struct {
			memoryMB uint64
			expected bool
		}{
			{500, false},   // Below 1GB threshold
			{1024, false},  // Exactly at threshold - should not trigger (>)
			{1025, true},   // Just above 1GB - should trigger
			{2048, true},   // 2GB - should trigger with high severity
		}

		for _, tc := range testCases {
			metrics := []PerformanceMetrics{
				{MemoryUsage: tc.memoryMB * 1024 * 1024, GoroutineCount: 10, AllocRate: 1000},
			}
			bottlenecks := detectBottlenecks(metrics)
			
			hasMemoryBottleneck := false
			for _, bn := range bottlenecks {
				if bn.Type == "memory" {
					hasMemoryBottleneck = true
					break
				}
			}
			
			require.Equal(t, tc.expected, hasMemoryBottleneck,
				"Memory %d MB: expected bottleneck=%v", tc.memoryMB, tc.expected)
		}
	})

	t.Run("goroutine threshold accuracy", func(t *testing.T) {
		testCases := []struct {
			count    int
			expected bool
		}{
			{500, false},
			{1000, false},
			{1001, true},
			{5000, true},
		}

		for _, tc := range testCases {
			metrics := []PerformanceMetrics{
				{MemoryUsage: 100 * 1024 * 1024, GoroutineCount: tc.count, AllocRate: 1000},
			}
			bottlenecks := detectBottlenecks(metrics)
			
			hasGoroutineBottleneck := false
			for _, bn := range bottlenecks {
				if bn.Type == "goroutine_leak" {
					hasGoroutineBottleneck = true
					break
				}
			}
			
			require.Equal(t, tc.expected, hasGoroutineBottleneck,
				"Goroutines %d: expected bottleneck=%v", tc.count, tc.expected)
		}
	})
}

func TestRegressionDetectionThreshold(t *testing.T) {
	baseline := &PerformanceMetrics{
		MemoryUsage:    1000 * 1024 * 1024, // 1000 MB
		GoroutineCount: 100,
		AllocRate:      100 * 1024 * 1024,
	}

	testCases := []struct {
		name            string
		currentMemoryMB uint64
		expectedReg     bool
	}{
		{"5% increase", 1050, false},   // Below 10% threshold
		{"10% increase", 1100, false},  // Exactly at threshold
		{"11% increase", 1110, true},   // Above threshold
		{"20% increase", 1200, true},   // Well above threshold
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metrics := []PerformanceMetrics{
				{
					MemoryUsage:    tc.currentMemoryMB * 1024 * 1024,
					GoroutineCount: 100,
					AllocRate:      100 * 1024 * 1024,
				},
			}

			regressions := detectRegressions(metrics, baseline)
			hasMemoryRegression := false
			for _, reg := range regressions {
				if reg.Metric == "memory_usage" {
					hasMemoryRegression = true
					break
				}
			}

			require.Equal(t, tc.expectedReg, hasMemoryRegression)
		})
	}
}

func TestRecommendationPriority(t *testing.T) {
	testCases := []struct {
		name     string
		severity string
		priority string
	}{
		{"critical bottleneck", "critical", "critical"},
		{"high bottleneck", "high", "high"},
		{"medium bottleneck", "medium", "medium"},
		{"low bottleneck", "low", "low"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bottlenecks := []Bottleneck{
				{Type: "memory", Severity: tc.severity},
			}

			recs := generateTuningRecommendations(bottlenecks, nil)
			require.Len(t, recs, 1)
			require.Equal(t, tc.priority, recs[0].Priority)
		})
	}
}

func TestMetricsAveraging(t *testing.T) {
	// Test that metrics are properly averaged in bottleneck detection
	metrics := []PerformanceMetrics{
		{MemoryUsage: 500 * 1024 * 1024, GoroutineCount: 100, AllocRate: 50 * 1024 * 1024},
		{MemoryUsage: 1500 * 1024 * 1024, GoroutineCount: 200, AllocRate: 150 * 1024 * 1024},
		{MemoryUsage: 1000 * 1024 * 1024, GoroutineCount: 150, AllocRate: 100 * 1024 * 1024},
	}

	// Average memory is 1000MB (below threshold), so no memory bottleneck
	// Average goroutines is 150 (below threshold), so no goroutine bottleneck
	bottlenecks := detectBottlenecks(metrics)
	
	for _, bn := range bottlenecks {
		require.NotEqual(t, "memory", bn.Type, "should not detect memory bottleneck with average below threshold")
		require.NotEqual(t, "goroutine_leak", bn.Type, "should not detect goroutine bottleneck with average below threshold")
	}
}

func TestGCPauseDetection(t *testing.T) {
	testCases := []struct {
		name     string
		pauseMs  int64
		expected bool
		severity string
	}{
		{"short pause", 5, false, ""},
		{"threshold pause", 10, false, ""},
		{"medium pause", 60, true, "medium"},
		{"long pause", 150, true, "high"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metrics := []PerformanceMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{time.Duration(tc.pauseMs) * time.Millisecond},
				},
			}

			bottlenecks := detectBottlenecks(metrics)
			
			var gcBottleneck *Bottleneck
			for i := range bottlenecks {
				if bottlenecks[i].Type == "gc_pause" {
					gcBottleneck = &bottlenecks[i]
					break
				}
			}

			if tc.expected {
				require.NotNil(t, gcBottleneck, "should detect GC pause bottleneck")
				require.Equal(t, tc.severity, gcBottleneck.Severity)
			} else {
				require.Nil(t, gcBottleneck, "should not detect GC pause bottleneck")
			}
		})
	}
}

func TestMultipleMetricsSamples(t *testing.T) {
	// Test with multiple samples to ensure proper aggregation
	metrics := []PerformanceMetrics{
		{
			Timestamp:      time.Now().Add(-10 * time.Second),
			MemoryUsage:    5 * 1024 * 1024 * 1024, // High
			GoroutineCount: 50,
			AllocRate:      50 * 1024 * 1024,
			GCPauses:       []time.Duration{5 * time.Millisecond},
		},
		{
			Timestamp:      time.Now().Add(-5 * time.Second),
			MemoryUsage:    5 * 1024 * 1024 * 1024, // High
			GoroutineCount: 60,
			AllocRate:      60 * 1024 * 1024,
			GCPauses:       []time.Duration{6 * time.Millisecond},
		},
		{
			Timestamp:      time.Now(),
			MemoryUsage:    5 * 1024 * 1024 * 1024, // High
			GoroutineCount: 55,
			AllocRate:      55 * 1024 * 1024,
			GCPauses:       []time.Duration{7 * time.Millisecond},
		},
	}

	bottlenecks := detectBottlenecks(metrics)
	
	// Should detect high memory (average is 5GB)
	hasMemoryBottleneck := false
	for _, bn := range bottlenecks {
		if bn.Type == "memory" {
			hasMemoryBottleneck = true
			require.Equal(t, "critical", bn.Severity, "5GB average should be critical")
		}
	}
	require.True(t, hasMemoryBottleneck, "should detect memory bottleneck")
}

func TestRegressionCalculation(t *testing.T) {
	baseline := &PerformanceMetrics{
		MemoryUsage:    1000 * 1024 * 1024,
		GoroutineCount: 100,
		AllocRate:      100 * 1024 * 1024,
	}

	metrics := []PerformanceMetrics{
		{
			MemoryUsage:    1500 * 1024 * 1024, // 50% increase
			GoroutineCount: 150,                 // 50% increase
			AllocRate:      150 * 1024 * 1024,   // 50% increase
		},
	}

	regressions := detectRegressions(metrics, baseline)
	
	// Should detect all three regressions
	require.Len(t, regressions, 3)

	for _, reg := range regressions {
		require.Greater(t, reg.Degradation, 10.0, "degradation should be above threshold")
		require.InDelta(t, 50.0, reg.Degradation, 1.0, "degradation should be approximately 50%")
	}
}

func TestBaselineComparison(t *testing.T) {
	// Create baseline
	baselineTmp, err := os.CreateTemp("", "baseline-*.json")
	require.NoError(t, err)
	defer os.Remove(baselineTmp.Name())

	baseline := PerformanceMetrics{
		Timestamp:      time.Now(),
		MemoryUsage:    500 * 1024 * 1024,
		GoroutineCount: 100,
		AllocRate:      50 * 1024 * 1024,
	}

	baselineData, err := json.Marshal(baseline)
	require.NoError(t, err)
	_, err = baselineTmp.Write(baselineData)
	require.NoError(t, err)
	baselineTmp.Close()

	// Create metrics with regression
	metricsTmp, err := os.CreateTemp("", "metrics-*.json")
	require.NoError(t, err)
	defer os.Remove(metricsTmp.Name())

	metrics := []PerformanceMetrics{
		{
			Timestamp:      time.Now(),
			MemoryUsage:    700 * 1024 * 1024, // 40% increase
			GoroutineCount: 100,
			AllocRate:      50 * 1024 * 1024,
		},
	}

	metricsData, err := json.Marshal(metrics)
	require.NoError(t, err)
	_, err = metricsTmp.Write(metricsData)
	require.NoError(t, err)
	metricsTmp.Close()

	// Analyze with baseline
	report, err := analyzeMetrics(metricsTmp.Name(), baselineTmp.Name())
	require.NoError(t, err)
	require.NotNil(t, report.Baseline)
	require.GreaterOrEqual(t, len(report.Regressions), 1)
}

func TestRecommendationContent(t *testing.T) {
	bottlenecks := []Bottleneck{
		{Type: "memory", Severity: "high"},
		{Type: "goroutine_leak", Severity: "medium"},
		{Type: "gc_pause", Severity: "low"},
		{Type: "allocation_rate", Severity: "medium"},
	}

	recs := generateTuningRecommendations(bottlenecks, nil)
	require.Len(t, recs, 4)

	// Check that each recommendation type has appropriate content
	categories := make(map[string]bool)
	for _, rec := range recs {
		categories[rec.Category] = true
		require.NotEmpty(t, rec.Description)
		require.NotEmpty(t, rec.Expected)
	}

	require.True(t, categories["Memory Optimization"])
	require.True(t, categories["Concurrency"])
	require.True(t, categories["GC Tuning"])
	require.True(t, categories["Allocation Optimization"])
}

func TestEmptyMetrics(t *testing.T) {
	// Test behavior with empty metrics
	bottlenecks := detectBottlenecks([]PerformanceMetrics{})
	require.Empty(t, bottlenecks)

	regressions := detectRegressions([]PerformanceMetrics{}, &PerformanceMetrics{})
	require.Empty(t, regressions)
}

func TestNilBaseline(t *testing.T) {
	metrics := []PerformanceMetrics{
		{MemoryUsage: 1000 * 1024 * 1024, GoroutineCount: 100, AllocRate: 50 * 1024 * 1024},
	}

	regressions := detectRegressions(metrics, nil)
	require.Empty(t, regressions)
}
