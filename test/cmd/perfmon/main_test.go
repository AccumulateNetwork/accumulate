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

func TestCollectMetrics(t *testing.T) {
	duration := 3 * time.Second
	interval := 500 * time.Millisecond

	metrics := collectMetrics(duration, interval)

	// Should collect at least a few samples
	require.GreaterOrEqual(t, len(metrics), 3)
	
	// Verify each metric has required fields
	for _, m := range metrics {
		require.NotZero(t, m.Timestamp)
		require.NotZero(t, m.MemoryUsage)
		require.Positive(t, m.GoroutineCount)
	}
}

func TestDetectBottlenecks(t *testing.T) {
	tests := []struct {
		name     string
		metrics  []PerformanceMetrics
		expected int
		types    map[string]bool
	}{
		{
			name: "no bottlenecks",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024, // 100 MB
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024, // 10 MB/s
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 0,
		},
		{
			name: "high memory usage",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    2 * 1024 * 1024 * 1024, // 2 GB
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 1,
			types:    map[string]bool{"memory": true},
		},
		{
			name: "goroutine leak",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 5000,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 1,
			types:    map[string]bool{"goroutine_leak": true},
		},
		{
			name: "long GC pauses",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 50,
					AllocRate:      10 * 1024 * 1024,
					GCPauses:       []time.Duration{60 * time.Millisecond},
				},
			},
			expected: 1,
			types:    map[string]bool{"gc_pause": true},
		},
		{
			name: "high allocation rate",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    100 * 1024 * 1024,
					GoroutineCount: 50,
					AllocRate:      600 * 1024 * 1024, // 600 MB/s
					GCPauses:       []time.Duration{2 * time.Millisecond},
				},
			},
			expected: 1,
			types:    map[string]bool{"allocation_rate": true},
		},
		{
			name: "multiple bottlenecks",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    3 * 1024 * 1024 * 1024, // 3 GB
					GoroutineCount: 8000,
					AllocRate:      700 * 1024 * 1024,
					GCPauses:       []time.Duration{80 * time.Millisecond},
				},
			},
			expected: 4,
			types: map[string]bool{
				"memory":          true,
				"goroutine_leak":  true,
				"gc_pause":        true,
				"allocation_rate": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bottlenecks := detectBottlenecks(tt.metrics)
			require.Len(t, bottlenecks, tt.expected)

			if tt.types != nil {
				for _, bn := range bottlenecks {
					require.True(t, tt.types[bn.Type], "unexpected bottleneck type: %s", bn.Type)
				}
			}
		})
	}
}

func TestDetectRegressions(t *testing.T) {
	baseline := &PerformanceMetrics{
		MemoryUsage:    500 * 1024 * 1024, // 500 MB
		GoroutineCount: 100,
		AllocRate:      50 * 1024 * 1024, // 50 MB/s
	}

	tests := []struct {
		name     string
		metrics  []PerformanceMetrics
		expected int
	}{
		{
			name: "no regression",
			metrics: []PerformanceMetrics{
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
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    600 * 1024 * 1024, // 20% increase
					GoroutineCount: 100,
					AllocRate:      50 * 1024 * 1024,
				},
			},
			expected: 1,
		},
		{
			name: "multiple regressions",
			metrics: []PerformanceMetrics{
				{
					MemoryUsage:    650 * 1024 * 1024, // 30% increase
					GoroutineCount: 130,               // 30% increase
					AllocRate:      70 * 1024 * 1024,  // 40% increase
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regressions := detectRegressions(tt.metrics, baseline)
			require.Len(t, regressions, tt.expected)

			for _, reg := range regressions {
				require.Greater(t, reg.Degradation, 10.0, "regression should be > 10%%")
			}
		})
	}
}

func TestGenerateTuningRecommendations(t *testing.T) {
	tests := []struct {
		name        string
		bottlenecks []Bottleneck
		minRecs     int
	}{
		{
			name:        "no bottlenecks",
			bottlenecks: []Bottleneck{},
			minRecs:     1, // Should still provide general recommendation
		},
		{
			name: "memory bottleneck",
			bottlenecks: []Bottleneck{
				{Type: "memory", Severity: "high"},
			},
			minRecs: 1,
		},
		{
			name: "multiple bottlenecks",
			bottlenecks: []Bottleneck{
				{Type: "memory", Severity: "high"},
				{Type: "goroutine_leak", Severity: "medium"},
				{Type: "gc_pause", Severity: "low"},
			},
			minRecs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := generateTuningRecommendations(tt.bottlenecks, nil)
			require.GreaterOrEqual(t, len(recs), tt.minRecs)

			for _, rec := range recs {
				require.NotEmpty(t, rec.Category)
				require.NotEmpty(t, rec.Priority)
				require.NotEmpty(t, rec.Description)
				require.NotEmpty(t, rec.Expected)
			}
		})
	}
}

func TestLoadBaseline(t *testing.T) {
	// Create a temporary baseline file
	tmpFile, err := os.CreateTemp("", "baseline-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	baseline := PerformanceMetrics{
		Timestamp:      time.Now(),
		MemoryUsage:    500 * 1024 * 1024,
		GoroutineCount: 100,
		AllocRate:      50 * 1024 * 1024,
		GCPauses:       []time.Duration{5 * time.Millisecond},
	}

	data, err := json.Marshal(baseline)
	require.NoError(t, err)

	_, err = tmpFile.Write(data)
	require.NoError(t, err)
	tmpFile.Close()

	// Load the baseline
	loaded, err := loadBaseline(tmpFile.Name())
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, baseline.MemoryUsage, loaded.MemoryUsage)
	require.Equal(t, baseline.GoroutineCount, loaded.GoroutineCount)
}

func TestAnalyzeMetrics(t *testing.T) {
	// Create temporary metrics file
	tmpFile, err := os.CreateTemp("", "metrics-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	metrics := []PerformanceMetrics{
		{
			Timestamp:      time.Now(),
			MemoryUsage:    2 * 1024 * 1024 * 1024, // High memory to trigger bottleneck
			GoroutineCount: 100,
			AllocRate:      50 * 1024 * 1024,
			GCPauses:       []time.Duration{5 * time.Millisecond},
		},
	}

	data, err := json.Marshal(metrics)
	require.NoError(t, err)

	_, err = tmpFile.Write(data)
	require.NoError(t, err)
	tmpFile.Close()

	// Analyze metrics
	report, err := analyzeMetrics(tmpFile.Name(), "")
	require.NoError(t, err)
	require.NotNil(t, report)
	require.Len(t, report.Metrics, 1)
	require.GreaterOrEqual(t, len(report.Bottlenecks), 1) // Should detect high memory
	require.GreaterOrEqual(t, len(report.Tuning), 1)
}

func TestGenerateSampleReport(t *testing.T) {
	report := generateSampleReport()
	require.NotNil(t, report)
	require.Greater(t, len(report.Metrics), 0)
	require.NotZero(t, report.GeneratedAt)
}

func TestWriteReportJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "report-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	report := generateSampleReport()
	err = writeReport(report, tmpFile.Name(), "json")
	require.NoError(t, err)

	// Verify file was written
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Verify it's valid JSON
	var loaded PerformanceReport
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
}

func TestWriteReportText(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "report-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	report := generateSampleReport()
	err = writeReport(report, tmpFile.Name(), "text")
	require.NoError(t, err)

	// Verify file was written
	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.Contains(t, string(data), "Performance Analysis Report")
}

func TestBottleneckSeverityOrdering(t *testing.T) {
	metrics := []PerformanceMetrics{
		{
			MemoryUsage:    5 * 1024 * 1024 * 1024, // Critical
			GoroutineCount: 3000,                    // Medium
			AllocRate:      200 * 1024 * 1024,       // Medium
			GCPauses:       []time.Duration{15 * time.Millisecond}, // Low
		},
	}

	bottlenecks := detectBottlenecks(metrics)
	require.Greater(t, len(bottlenecks), 0)

	// Verify that critical comes first
	if len(bottlenecks) > 1 {
		severityOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		for i := 1; i < len(bottlenecks); i++ {
			require.LessOrEqual(t,
				severityOrder[bottlenecks[i-1].Severity],
				severityOrder[bottlenecks[i].Severity],
				"bottlenecks should be sorted by severity")
		}
	}
}
