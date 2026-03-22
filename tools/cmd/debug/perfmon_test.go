// Copyright 2025 The Accumulate Authors
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

func TestCollectPerfMetrics(t *testing.T) {
	duration := 2 * time.Second
	interval := 500 * time.Millisecond

	metrics := collectPerfMetrics(duration, interval)

	require.GreaterOrEqual(t, len(metrics), 2, "should collect at least 2 samples")
	
	for _, m := range metrics {
		require.NotZero(t, m.Timestamp)
		require.NotZero(t, m.MemoryUsage)
		require.Positive(t, m.GoroutineCount)
	}
}

func TestDetectPerfBottlenecks(t *testing.T) {
	tests := []struct {
		name     string
		metrics  []PerfMetrics
		expected int
		hasType  map[string]bool
	}{
		{
			name: "clean metrics",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 0,
		},
		{
			name: "high memory",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    2 * 1024 * 1024 * 1024,
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 1,
			hasType:  map[string]bool{"memory": true},
		},
		{
			name: "goroutine leak",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 3000,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 1,
			hasType:  map[string]bool{"goroutine_leak": true},
		},
		{
			name: "long GC pause",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{70 * time.Millisecond},
				},
			},
			expected: 1,
			hasType:  map[string]bool{"gc_pause": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bottlenecks := detectPerfBottlenecks(tt.metrics)
			require.Len(t, bottlenecks, tt.expected)

			if tt.hasType != nil {
				for _, bn := range bottlenecks {
					require.True(t, tt.hasType[bn.Type])
				}
			}
		})
	}
}

func TestGenerateSamplePerfReport(t *testing.T) {
	report := generateSamplePerfReport()
	
	require.NotNil(t, report)
	require.Greater(t, len(report.Metrics), 0)
	require.NotZero(t, report.GeneratedAt)
	
	for _, m := range report.Metrics {
		require.NotZero(t, m.MemoryUsage)
		require.Positive(t, m.GoroutineCount)
	}
}

func TestWritePerfReportJSON(t *testing.T) {
	report := generateSamplePerfReport()
	
	tmpFile, err := os.CreateTemp("", "perf-report-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	writePerfReportJSON(report, tmpFile.Name())

	// Verify file contents
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var loaded PerfReport
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	require.Len(t, loaded.Metrics, len(report.Metrics))
}

func TestWritePerfReportText(t *testing.T) {
	report := generateSamplePerfReport()
	
	tmpFile, err := os.CreateTemp("", "perf-report-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	writePerfReportText(report, tmpFile.Name())

	// Verify file contents
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.Contains(t, string(data), "Performance Analysis Report")
}

func TestGenerateTuningRecommendations_Debug(t *testing.T) {
	tests := []struct {
		name        string
		bottlenecks []PerfBottleneck
		verbose     bool
		minRecs     int
	}{
		{
			name:        "no bottlenecks",
			bottlenecks: []PerfBottleneck{},
			verbose:     false,
			minRecs:     1,
		},
		{
			name: "memory bottleneck without verbose",
			bottlenecks: []PerfBottleneck{
				{Type: "memory", Severity: "high"},
			},
			verbose: false,
			minRecs: 1,
		},
		{
			name: "memory bottleneck with verbose",
			bottlenecks: []PerfBottleneck{
				{Type: "memory", Severity: "high"},
			},
			verbose: true,
			minRecs: 1,
		},
		{
			name: "multiple bottlenecks",
			bottlenecks: []PerfBottleneck{
				{Type: "memory", Severity: "critical"},
				{Type: "goroutine_leak", Severity: "high"},
				{Type: "gc_pause", Severity: "medium"},
			},
			verbose: true,
			minRecs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := generateTuningRecommendations(tt.bottlenecks, tt.verbose)
			require.GreaterOrEqual(t, len(recs), tt.minRecs)

			for _, rec := range recs {
				require.NotEmpty(t, rec.Category)
				require.NotEmpty(t, rec.Priority)
				require.NotEmpty(t, rec.Description)
				
				if tt.verbose && len(tt.bottlenecks) > 0 {
					require.NotEmpty(t, rec.Actions, "verbose mode should include actions")
				}
			}
		})
	}
}

func TestDetectRegressions_Debug(t *testing.T) {
	baseline := &PerfMetrics{
		MemoryUsage:    500 * 1024 * 1024,
		GoroutineCount: 100,
		AllocRate:      50 * 1024 * 1024,
	}

	tests := []struct {
		name     string
		metrics  []PerfMetrics
		expected int
	}{
		{
			name: "no regression",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    500 * 1024 * 1024,
					GoroutineCount: 100,
					AllocRate:      50 * 1024 * 1024,
				},
			},
			expected: 0,
		},
		{
			name: "memory regression",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    650 * 1024 * 1024, // 30% increase
					GoroutineCount: 100,
					AllocRate:      50 * 1024 * 1024,
				},
			},
			expected: 1,
		},
		{
			name: "all regressions",
			metrics: []PerfMetrics{
				{
					MemoryUsage:    700 * 1024 * 1024, // 40% increase
					GoroutineCount: 150,               // 50% increase
					AllocRate:      80 * 1024 * 1024,  // 60% increase
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regressions := detectRegressions(tt.metrics, baseline)
			require.Len(t, regressions, tt.expected)
		})
	}
}

func TestWriteTuningReportJSON(t *testing.T) {
	report := &TuningReport{
		Recommendations: []TuningRecommendation{
			{
				Category:    "Memory Optimization",
				Priority:    "high",
				Description: "Test recommendation",
				Expected:    "30% improvement",
				Actions:     []string{"Action 1", "Action 2"},
			},
		},
	}

	tmpFile, err := os.CreateTemp("", "tuning-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	writeTuningReportJSON(report, tmpFile.Name())

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var loaded TuningReport
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	require.Len(t, loaded.Recommendations, 1)
}

func TestWriteTuningReportText(t *testing.T) {
	report := &TuningReport{
		Recommendations: []TuningRecommendation{
			{
				Category:    "Memory Optimization",
				Priority:    "high",
				Description: "Test recommendation",
				Expected:    "30% improvement",
				Actions:     []string{"Action 1", "Action 2"},
			},
		},
		Regressions: []Regression{
			{
				Metric:      "memory_usage",
				Baseline:    1000,
				Current:     1500,
				Degradation: 50,
			},
		},
	}

	tmpFile, err := os.CreateTemp("", "tuning-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	writeTuningReportText(report, tmpFile.Name())

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, data)
	
	content := string(data)
	require.Contains(t, content, "Performance Tuning Recommendations")
	require.Contains(t, content, "Memory Optimization")
	require.Contains(t, content, "memory_usage")
}

func TestPerfMetricsAllocationRateCalculation(t *testing.T) {
	// Test that allocation rate is calculated correctly
	duration := 1 * time.Second
	interval := 500 * time.Millisecond

	metrics := collectPerfMetrics(duration, interval)
	
	// First metric should have zero alloc rate (no previous sample)
	// Subsequent metrics should have calculated alloc rate
	if len(metrics) > 1 {
		for i := 1; i < len(metrics); i++ {
			// Alloc rate can be zero or positive, but should be defined
			require.GreaterOrEqual(t, metrics[i].AllocRate, float64(0))
		}
	}
}

func TestBottleneckSeverityLevels(t *testing.T) {
	tests := []struct {
		name     string
		memGB    float64
		severity string
	}{
		{"1.5 GB - medium", 1.5, "medium"},
		{"2.5 GB - high", 2.5, "high"},
		{"5 GB - high", 5.0, "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := []PerfMetrics{
				{
					MemoryUsage:    uint64(tt.memGB * 1024 * 1024 * 1024),
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			}

			bottlenecks := detectPerfBottlenecks(metrics)
			
			var memBottleneck *PerfBottleneck
			for i := range bottlenecks {
				if bottlenecks[i].Type == "memory" {
					memBottleneck = &bottlenecks[i]
					break
				}
			}

			require.NotNil(t, memBottleneck)
			require.Equal(t, tt.severity, memBottleneck.Severity)
		})
	}
}

func TestEmptyMetricsHandling(t *testing.T) {
	bottlenecks := detectPerfBottlenecks([]PerfMetrics{})
	require.Empty(t, bottlenecks)

	regressions := detectRegressions([]PerfMetrics{}, &PerfMetrics{})
	require.Empty(t, regressions)

	regressions = detectRegressions([]PerfMetrics{{MemoryUsage: 1000}}, nil)
	require.Empty(t, regressions)
}
