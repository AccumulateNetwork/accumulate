// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testresults

import (
	"testing"
	"time"
)

func TestCompareTestRuns(t *testing.T) {
	baseRun := &TestRun{
		ID:          1,
		CommitHash:  "abc123",
		AverageTPS:  100.0,
		PeakTPS:     150.0,
		AvgLatency:  50.0,
		P95Latency:  100.0,
		P99Latency:  150.0,
		ErrorRate:   1.0,
		NodeCrashes: 0,
	}

	// Test regression case
	t.Run("DetectTPSRegression", func(t *testing.T) {
		compareRun := &TestRun{
			ID:          2,
			CommitHash:  "def456",
			AverageTPS:  85.0, // 15% decrease - should trigger regression
			PeakTPS:     150.0,
			AvgLatency:  50.0,
			P95Latency:  100.0,
			P99Latency:  150.0,
			ErrorRate:   1.0,
			NodeCrashes: 0,
		}

		result := CompareTestRuns(baseRun, compareRun)
		if len(result.Regressions) == 0 {
			t.Error("Expected regression to be detected for TPS decrease")
		}
	})

	// Test improvement case
	t.Run("DetectLatencyImprovement", func(t *testing.T) {
		compareRun := &TestRun{
			ID:          2,
			CommitHash:  "def456",
			AverageTPS:  100.0,
			PeakTPS:     150.0,
			AvgLatency:  40.0, // 20% decrease - should be improvement
			P95Latency:  80.0,
			P99Latency:  120.0,
			ErrorRate:   1.0,
			NodeCrashes: 0,
		}

		result := CompareTestRuns(baseRun, compareRun)
		if len(result.Improvements) == 0 {
			t.Error("Expected improvement to be detected for latency decrease")
		}
	})

	// Test node crash regression
	t.Run("DetectNodeCrashRegression", func(t *testing.T) {
		compareRun := &TestRun{
			ID:          2,
			CommitHash:  "def456",
			AverageTPS:  100.0,
			PeakTPS:     150.0,
			AvgLatency:  50.0,
			P95Latency:  100.0,
			P99Latency:  150.0,
			ErrorRate:   1.0,
			NodeCrashes: 2, // Crashes detected
		}

		result := CompareTestRuns(baseRun, compareRun)
		if len(result.Regressions) == 0 {
			t.Error("Expected regression to be detected for node crashes")
		}
	})
}

func TestAnalyzeTrend(t *testing.T) {
	baseTime := time.Now().Add(-7 * 24 * time.Hour)

	runs := []*TestRun{
		{
			ID:         1,
			StartTime:  baseTime,
			AverageTPS: 100.0,
		},
		{
			ID:         2,
			StartTime:  baseTime.Add(24 * time.Hour),
			AverageTPS: 105.0,
		},
		{
			ID:         3,
			StartTime:  baseTime.Add(48 * time.Hour),
			AverageTPS: 110.0,
		},
		{
			ID:         4,
			StartTime:  baseTime.Add(72 * time.Hour),
			AverageTPS: 115.0,
		},
	}

	t.Run("ImprovingTPSTrend", func(t *testing.T) {
		trend := AnalyzeTrend(runs, "avg_tps")
		if trend == nil {
			t.Fatal("Expected trend analysis result")
		}

		if trend.Trend != "improving" {
			t.Errorf("Expected improving trend, got %s", trend.Trend)
		}

		if trend.Slope <= 0 {
			t.Errorf("Expected positive slope for improving TPS, got %f", trend.Slope)
		}
	})

	// Test degrading trend
	degradingRuns := []*TestRun{
		{
			ID:         1,
			StartTime:  baseTime,
			AvgLatency: 50.0,
		},
		{
			ID:         2,
			StartTime:  baseTime.Add(24 * time.Hour),
			AvgLatency: 55.0,
		},
		{
			ID:         3,
			StartTime:  baseTime.Add(48 * time.Hour),
			AvgLatency: 60.0,
		},
	}

	t.Run("DegradingLatencyTrend", func(t *testing.T) {
		trend := AnalyzeTrend(degradingRuns, "avg_latency")
		if trend == nil {
			t.Fatal("Expected trend analysis result")
		}

		if trend.Trend != "degrading" {
			t.Errorf("Expected degrading trend for increasing latency, got %s", trend.Trend)
		}
	})
}

func TestCalculatePercentiles(t *testing.T) {
	samples := []*TransactionSample{
		{SettlementTime: 0.010}, // 10ms
		{SettlementTime: 0.020}, // 20ms
		{SettlementTime: 0.030}, // 30ms
		{SettlementTime: 0.040}, // 40ms
		{SettlementTime: 0.050}, // 50ms
		{SettlementTime: 0.060}, // 60ms
		{SettlementTime: 0.070}, // 70ms
		{SettlementTime: 0.080}, // 80ms
		{SettlementTime: 0.090}, // 90ms
		{SettlementTime: 0.100}, // 100ms
	}

	p50, p95, p99 := CalculatePercentiles(samples)

	// P50 should be around 55ms (median of 10 values)
	if p50 < 50 || p50 > 60 {
		t.Errorf("P50 out of expected range: %f", p50)
	}

	// P95 should be close to 95ms
	if p95 < 90 || p95 > 100 {
		t.Errorf("P95 out of expected range: %f", p95)
	}

	// P99 should be close to 99ms
	if p99 < 95 || p99 > 100 {
		t.Errorf("P99 out of expected range: %f", p99)
	}
}

func TestCompareMetric(t *testing.T) {
	t.Run("HigherIsBetter", func(t *testing.T) {
		comp := compareMetric("TPS", 100.0, 90.0, false)
		if comp.PercentChange >= 0 {
			t.Error("Expected negative percent change")
		}
		if comp.Delta != -10.0 {
			t.Errorf("Expected delta of -10.0, got %f", comp.Delta)
		}
	})

	t.Run("LowerIsBetter", func(t *testing.T) {
		comp := compareMetric("Latency", 100.0, 120.0, true)
		if comp.PercentChange <= 0 {
			t.Error("Expected positive percent change")
		}
		if comp.Delta != 20.0 {
			t.Errorf("Expected delta of 20.0, got %f", comp.Delta)
		}
		if !comp.IsRegression {
			t.Error("Expected regression to be flagged for latency increase")
		}
	})
}
