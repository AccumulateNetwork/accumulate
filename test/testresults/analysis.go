// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testresults

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	// Thresholds for regression detection (percentage change)
	RegressionThresholdTPSDecrease   = -10.0 // 10% decrease in TPS
	RegressionThresholdLatencyIncrease = 15.0 // 15% increase in latency
	RegressionThresholdErrorRateIncrease = 5.0 // 5% increase in error rate
)

// CompareTestRuns compares two test runs and identifies regressions
func CompareTestRuns(base, compare *TestRun) *ComparisonResult {
	result := &ComparisonResult{
		BaseRun:    base,
		CompareRun: compare,
		Metrics:    make(map[string]Comparison),
		Regressions: []string{},
		Improvements: []string{},
	}

	// Compare TPS
	tpsComp := compareMetric("Average TPS", base.AverageTPS, compare.AverageTPS, false)
	result.Metrics["avg_tps"] = tpsComp
	if tpsComp.PercentChange <= RegressionThresholdTPSDecrease {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("Average TPS decreased by %.2f%% (%.2f → %.2f)",
				-tpsComp.PercentChange, base.AverageTPS, compare.AverageTPS))
	} else if tpsComp.PercentChange > 5.0 {
		result.Improvements = append(result.Improvements,
			fmt.Sprintf("Average TPS increased by %.2f%% (%.2f → %.2f)",
				tpsComp.PercentChange, base.AverageTPS, compare.AverageTPS))
	}

	// Compare Peak TPS
	peakTPSComp := compareMetric("Peak TPS", base.PeakTPS, compare.PeakTPS, false)
	result.Metrics["peak_tps"] = peakTPSComp
	if peakTPSComp.PercentChange <= RegressionThresholdTPSDecrease {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("Peak TPS decreased by %.2f%% (%.2f → %.2f)",
				-peakTPSComp.PercentChange, base.PeakTPS, compare.PeakTPS))
	}

	// Compare Average Latency (lower is better)
	avgLatComp := compareMetric("Average Latency", base.AvgLatency, compare.AvgLatency, true)
	result.Metrics["avg_latency"] = avgLatComp
	if avgLatComp.PercentChange >= RegressionThresholdLatencyIncrease {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("Average latency increased by %.2f%% (%.2f → %.2f ms)",
				avgLatComp.PercentChange, base.AvgLatency, compare.AvgLatency))
	} else if avgLatComp.PercentChange <= -5.0 {
		result.Improvements = append(result.Improvements,
			fmt.Sprintf("Average latency decreased by %.2f%% (%.2f → %.2f ms)",
				-avgLatComp.PercentChange, base.AvgLatency, compare.AvgLatency))
	}

	// Compare P95 Latency
	p95LatComp := compareMetric("P95 Latency", base.P95Latency, compare.P95Latency, true)
	result.Metrics["p95_latency"] = p95LatComp
	if p95LatComp.PercentChange >= RegressionThresholdLatencyIncrease {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("P95 latency increased by %.2f%% (%.2f → %.2f ms)",
				p95LatComp.PercentChange, base.P95Latency, compare.P95Latency))
	}

	// Compare P99 Latency
	p99LatComp := compareMetric("P99 Latency", base.P99Latency, compare.P99Latency, true)
	result.Metrics["p99_latency"] = p99LatComp
	if p99LatComp.PercentChange >= RegressionThresholdLatencyIncrease {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("P99 latency increased by %.2f%% (%.2f → %.2f ms)",
				p99LatComp.PercentChange, base.P99Latency, compare.P99Latency))
	}

	// Compare Error Rate
	errorRateComp := compareMetric("Error Rate", base.ErrorRate, compare.ErrorRate, true)
	result.Metrics["error_rate"] = errorRateComp
	if errorRateComp.PercentChange >= RegressionThresholdErrorRateIncrease {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("Error rate increased by %.2f%% (%.2f%% → %.2f%%)",
				errorRateComp.PercentChange, base.ErrorRate, compare.ErrorRate))
	}

	// Compare Node Crashes
	if compare.NodeCrashes > base.NodeCrashes {
		result.Regressions = append(result.Regressions,
			fmt.Sprintf("Node crashes increased (%d → %d)",
				base.NodeCrashes, compare.NodeCrashes))
	}

	// Generate summary
	if len(result.Regressions) > 0 {
		result.Summary = fmt.Sprintf("⚠️  REGRESSION DETECTED: %d regressions found", len(result.Regressions))
	} else if len(result.Improvements) > 0 {
		result.Summary = fmt.Sprintf("✓ IMPROVEMENT: %d improvements found", len(result.Improvements))
	} else {
		result.Summary = "✓ No significant changes detected"
	}

	return result
}

// compareMetric compares two metric values
// lowerIsBetter indicates whether lower values are better (like latency)
func compareMetric(name string, baseValue, compareValue float64, lowerIsBetter bool) Comparison {
	delta := compareValue - baseValue
	percentChange := 0.0
	if baseValue != 0 {
		percentChange = (delta / baseValue) * 100.0
	}

	isRegression := false
	if lowerIsBetter {
		// For metrics where lower is better (latency, error rate)
		isRegression = percentChange >= RegressionThresholdLatencyIncrease
	} else {
		// For metrics where higher is better (TPS)
		isRegression = percentChange <= RegressionThresholdTPSDecrease
	}

	return Comparison{
		MetricName:    name,
		BaseValue:     baseValue,
		CompareValue:  compareValue,
		Delta:         delta,
		PercentChange: percentChange,
		IsRegression:  isRegression,
	}
}

// AnalyzeTrend analyzes the trend of a specific metric over multiple runs
func AnalyzeTrend(runs []*TestRun, metricName string) *TrendAnalysis {
	if len(runs) < 2 {
		return nil
	}

	// Sort runs by start time
	sortedRuns := make([]*TestRun, len(runs))
	copy(sortedRuns, runs)
	sort.Slice(sortedRuns, func(i, j int) bool {
		return sortedRuns[i].StartTime.Before(sortedRuns[j].StartTime)
	})

	analysis := &TrendAnalysis{
		Runs:       sortedRuns,
		MetricName: metricName,
		Values:     make([]float64, len(sortedRuns)),
		Timestamps: make([]time.Time, len(sortedRuns)),
	}

	// Extract metric values
	for i, run := range sortedRuns {
		analysis.Timestamps[i] = run.StartTime
		switch metricName {
		case "avg_tps":
			analysis.Values[i] = run.AverageTPS
		case "peak_tps":
			analysis.Values[i] = run.PeakTPS
		case "avg_latency":
			analysis.Values[i] = run.AvgLatency
		case "p95_latency":
			analysis.Values[i] = run.P95Latency
		case "p99_latency":
			analysis.Values[i] = run.P99Latency
		case "error_rate":
			analysis.Values[i] = run.ErrorRate
		}
	}

	// Calculate linear regression slope
	analysis.Slope = calculateSlope(analysis.Values)

	// Determine trend
	if math.Abs(analysis.Slope) < 0.01 {
		analysis.Trend = "stable"
	} else if metricName == "avg_tps" || metricName == "peak_tps" {
		// For TPS, positive slope is improving
		if analysis.Slope > 0 {
			analysis.Trend = "improving"
		} else {
			analysis.Trend = "degrading"
		}
	} else {
		// For latency and error rate, negative slope is improving
		if analysis.Slope < 0 {
			analysis.Trend = "improving"
		} else {
			analysis.Trend = "degrading"
		}
	}

	return analysis
}

// calculateSlope calculates the slope of a linear regression
func calculateSlope(values []float64) float64 {
	n := float64(len(values))
	if n < 2 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Slope = (n * sumXY - sumX * sumY) / (n * sumX2 - sumX * sumX)
	numerator := n*sumXY - sumX*sumY
	denominator := n*sumX2 - sumX*sumX

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// CalculatePercentiles calculates latency percentiles from transaction samples
func CalculatePercentiles(samples []*TransactionSample) (p50, p95, p99 float64) {
	if len(samples) == 0 {
		return 0, 0, 0
	}

	// Convert to milliseconds and sort
	latencies := make([]float64, len(samples))
	for i, s := range samples {
		latencies[i] = s.SettlementTime * 1000 // convert seconds to ms
	}
	sort.Float64s(latencies)

	p50 = percentile(latencies, 50)
	p95 = percentile(latencies, 95)
	p99 = percentile(latencies, 99)

	return
}

// percentile calculates the specified percentile from a sorted slice
func percentile(sortedValues []float64, p float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}

	index := (p / 100.0) * float64(len(sortedValues)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sortedValues[lower]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}
