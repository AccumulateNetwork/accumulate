// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"
)

// PerformanceMetrics represents collected performance data
type PerformanceMetrics struct {
	Timestamp      time.Time              `json:"timestamp"`
	CPUUsage       float64                `json:"cpu_usage"`
	MemoryUsage    uint64                 `json:"memory_usage"`
	GoroutineCount int                    `json:"goroutine_count"`
	AllocRate      float64                `json:"alloc_rate"`
	GCPauses       []time.Duration        `json:"gc_pauses"`
	CustomMetrics  map[string]interface{} `json:"custom_metrics,omitempty"`
}

// Bottleneck represents a detected performance issue
type Bottleneck struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"` // "critical", "high", "medium", "low"
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
}

// PerformanceReport contains the analysis results
type PerformanceReport struct {
	Metrics      []PerformanceMetrics `json:"metrics"`
	Bottlenecks  []Bottleneck         `json:"bottlenecks"`
	Baseline     *PerformanceMetrics  `json:"baseline,omitempty"`
	Regressions  []Regression         `json:"regressions,omitempty"`
	Tuning       []TuningRecommendation `json:"tuning,omitempty"`
	GeneratedAt  time.Time            `json:"generated_at"`
}

// Regression represents a performance regression
type Regression struct {
	Metric      string  `json:"metric"`
	Baseline    float64 `json:"baseline"`
	Current     float64 `json:"current"`
	Degradation float64 `json:"degradation"` // percentage
}

// TuningRecommendation provides optimization suggestions
type TuningRecommendation struct {
	Category    string `json:"category"`
	Priority    string `json:"priority"`
	Description string `json:"description"`
	Expected    string `json:"expected_improvement"`
}

var (
	metricsFile     = flag.String("metrics", "", "Path to metrics JSON file")
	baselineFile    = flag.String("baseline", "", "Path to baseline metrics JSON file")
	outputFile      = flag.String("output", "", "Output file for report (default: stdout)")
	outputFormat    = flag.String("format", "text", "Output format: text, json")
	cpuProfile      = flag.String("cpuprofile", "", "Write CPU profile to file")
	memProfile      = flag.String("memprofile", "", "Write memory profile to file")
	duration        = flag.Duration("duration", 10*time.Second, "Duration to collect metrics")
	interval        = flag.Duration("interval", 1*time.Second, "Interval between metric collections")
	generateSample  = flag.Bool("sample", false, "Generate sample metrics for testing")
	analyzeOnly     = flag.Bool("analyze", false, "Analyze existing metrics without collecting new ones")
)

func main() {
	flag.Parse()

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fatalf("could not create CPU profile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fatalf("could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	var report *PerformanceReport
	var err error

	if *generateSample {
		report = generateSampleReport()
	} else if *analyzeOnly {
		if *metricsFile == "" {
			fatalf("--metrics required when using --analyze")
		}
		report, err = analyzeMetrics(*metricsFile, *baselineFile)
		if err != nil {
			fatalf("analysis failed: %v", err)
		}
	} else {
		// Collect live metrics
		metrics := collectMetrics(*duration, *interval)
		report = &PerformanceReport{
			Metrics:     metrics,
			GeneratedAt: time.Now(),
		}

		// Detect bottlenecks
		report.Bottlenecks = detectBottlenecks(metrics)

		// Load baseline if provided
		if *baselineFile != "" {
			baseline, err := loadBaseline(*baselineFile)
			if err != nil {
				fatalf("failed to load baseline: %v", err)
			}
			report.Baseline = baseline
			report.Regressions = detectRegressions(metrics, baseline)
		}

		// Generate tuning recommendations
		report.Tuning = generateTuningRecommendations(report.Bottlenecks, metrics)
	}

	// Write memory profile if requested
	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			fatalf("could not create memory profile: %v", err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fatalf("could not write memory profile: %v", err)
		}
	}

	// Output report
	if err := writeReport(report, *outputFile, *outputFormat); err != nil {
		fatalf("failed to write report: %v", err)
	}
}

// collectMetrics gathers performance metrics over time
func collectMetrics(duration, interval time.Duration) []PerformanceMetrics {
	var metrics []PerformanceMetrics
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	startTime := time.Now()
	var lastAlloc uint64

	for time.Since(startTime) < duration {
		<-ticker.C

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		allocRate := float64(0)
		if lastAlloc > 0 {
			allocRate = float64(m.Alloc-lastAlloc) / interval.Seconds()
		}
		lastAlloc = m.Alloc

		// Collect GC pause times
		var gcPauses []time.Duration
		for i := uint32(0); i < m.NumGC && i < 256; i++ {
			gcPauses = append(gcPauses, time.Duration(m.PauseNs[(m.NumGC-1-i+256)%256]))
		}

		metric := PerformanceMetrics{
			Timestamp:      time.Now(),
			CPUUsage:       float64(runtime.NumCPU()), // Simplified for testing
			MemoryUsage:    m.Alloc,
			GoroutineCount: runtime.NumGoroutine(),
			AllocRate:      allocRate,
			GCPauses:       gcPauses,
		}

		metrics = append(metrics, metric)
	}

	return metrics
}

// detectBottlenecks analyzes metrics to find performance issues
func detectBottlenecks(metrics []PerformanceMetrics) []Bottleneck {
	if len(metrics) == 0 {
		return nil
	}

	var bottlenecks []Bottleneck

	// Calculate averages
	var totalMem, totalGoroutines, totalAllocRate float64
	var maxGCPause time.Duration

	for _, m := range metrics {
		totalMem += float64(m.MemoryUsage)
		totalGoroutines += float64(m.GoroutineCount)
		totalAllocRate += m.AllocRate

		for _, pause := range m.GCPauses {
			if pause > maxGCPause {
				maxGCPause = pause
			}
		}
	}

	avgMem := totalMem / float64(len(metrics))
	avgGoroutines := totalGoroutines / float64(len(metrics))
	avgAllocRate := totalAllocRate / float64(len(metrics))

	// Check for high memory usage (>1GB average)
	const memThreshold = 1024 * 1024 * 1024
	if avgMem > memThreshold {
		severity := "medium"
		if avgMem > 2*memThreshold {
			severity = "high"
		}
		if avgMem > 4*memThreshold {
			severity = "critical"
		}

		bottlenecks = append(bottlenecks, Bottleneck{
			Type:        "memory",
			Severity:    severity,
			Description: "High memory usage detected",
			Value:       avgMem,
			Threshold:   memThreshold,
		})
	}

	// Check for goroutine leaks (>1000 goroutines)
	const goroutineThreshold = 1000
	if avgGoroutines > goroutineThreshold {
		severity := "medium"
		if avgGoroutines > 5000 {
			severity = "high"
		}
		if avgGoroutines > 10000 {
			severity = "critical"
		}

		bottlenecks = append(bottlenecks, Bottleneck{
			Type:        "goroutine_leak",
			Severity:    severity,
			Description: "Excessive goroutine count",
			Value:       avgGoroutines,
			Threshold:   goroutineThreshold,
		})
	}

	// Check for long GC pauses (>10ms)
	if maxGCPause > 10*time.Millisecond {
		severity := "low"
		if maxGCPause > 50*time.Millisecond {
			severity = "medium"
		}
		if maxGCPause > 100*time.Millisecond {
			severity = "high"
		}

		bottlenecks = append(bottlenecks, Bottleneck{
			Type:        "gc_pause",
			Severity:    severity,
			Description: "Long GC pause times",
			Value:       float64(maxGCPause.Milliseconds()),
			Threshold:   10,
		})
	}

	// Check for high allocation rate (>100MB/s)
	const allocThreshold = 100 * 1024 * 1024
	if avgAllocRate > allocThreshold {
		severity := "low"
		if avgAllocRate > 500*1024*1024 {
			severity = "medium"
		}
		if avgAllocRate > 1024*1024*1024 {
			severity = "high"
		}

		bottlenecks = append(bottlenecks, Bottleneck{
			Type:        "allocation_rate",
			Severity:    severity,
			Description: "High memory allocation rate",
			Value:       avgAllocRate,
			Threshold:   allocThreshold,
		})
	}

	// Sort by severity
	sort.Slice(bottlenecks, func(i, j int) bool {
		severityOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return severityOrder[bottlenecks[i].Severity] < severityOrder[bottlenecks[j].Severity]
	})

	return bottlenecks
}

// detectRegressions compares current metrics against baseline
func detectRegressions(metrics []PerformanceMetrics, baseline *PerformanceMetrics) []Regression {
	if len(metrics) == 0 || baseline == nil {
		return nil
	}

	var regressions []Regression

	// Calculate averages from current metrics
	var totalMem, totalGoroutines, totalAllocRate float64
	for _, m := range metrics {
		totalMem += float64(m.MemoryUsage)
		totalGoroutines += float64(m.GoroutineCount)
		totalAllocRate += m.AllocRate
	}

	avgMem := totalMem / float64(len(metrics))
	avgGoroutines := totalGoroutines / float64(len(metrics))
	avgAllocRate := totalAllocRate / float64(len(metrics))

	// Check for regressions (>10% degradation)
	const regressionThreshold = 10.0

	if baseline.MemoryUsage > 0 {
		degradation := ((avgMem - float64(baseline.MemoryUsage)) / float64(baseline.MemoryUsage)) * 100
		if degradation > regressionThreshold {
			regressions = append(regressions, Regression{
				Metric:      "memory_usage",
				Baseline:    float64(baseline.MemoryUsage),
				Current:     avgMem,
				Degradation: degradation,
			})
		}
	}

	if baseline.GoroutineCount > 0 {
		degradation := ((avgGoroutines - float64(baseline.GoroutineCount)) / float64(baseline.GoroutineCount)) * 100
		if degradation > regressionThreshold {
			regressions = append(regressions, Regression{
				Metric:      "goroutine_count",
				Baseline:    float64(baseline.GoroutineCount),
				Current:     avgGoroutines,
				Degradation: degradation,
			})
		}
	}

	if baseline.AllocRate > 0 {
		degradation := ((avgAllocRate - baseline.AllocRate) / baseline.AllocRate) * 100
		if degradation > regressionThreshold {
			regressions = append(regressions, Regression{
				Metric:      "alloc_rate",
				Baseline:    baseline.AllocRate,
				Current:     avgAllocRate,
				Degradation: degradation,
			})
		}
	}

	return regressions
}

// generateTuningRecommendations creates optimization suggestions
func generateTuningRecommendations(bottlenecks []Bottleneck, metrics []PerformanceMetrics) []TuningRecommendation {
	var recommendations []TuningRecommendation

	for _, bn := range bottlenecks {
		switch bn.Type {
		case "memory":
			recommendations = append(recommendations, TuningRecommendation{
				Category:    "Memory Optimization",
				Priority:    bn.Severity,
				Description: "Consider using object pooling, reduce allocations, or implement memory limits",
				Expected:    "30-50% reduction in memory usage",
			})

		case "goroutine_leak":
			recommendations = append(recommendations, TuningRecommendation{
				Category:    "Concurrency",
				Priority:    bn.Severity,
				Description: "Investigate goroutine leaks, ensure proper context cancellation and cleanup",
				Expected:    "Stable goroutine count",
			})

		case "gc_pause":
			recommendations = append(recommendations, TuningRecommendation{
				Category:    "GC Tuning",
				Priority:    bn.Severity,
				Description: "Reduce allocation rate, increase GOGC value, or use ballast to reduce GC frequency",
				Expected:    "50-70% reduction in GC pause times",
			})

		case "allocation_rate":
			recommendations = append(recommendations, TuningRecommendation{
				Category:    "Allocation Optimization",
				Priority:    bn.Severity,
				Description: "Reuse buffers, use sync.Pool for frequently allocated objects, preallocate slices",
				Expected:    "40-60% reduction in allocation rate",
			})
		}
	}

	// Add general recommendations if no bottlenecks
	if len(bottlenecks) == 0 {
		recommendations = append(recommendations, TuningRecommendation{
			Category:    "General",
			Priority:    "low",
			Description: "Performance is within normal parameters. Consider establishing this as a baseline.",
			Expected:    "No immediate action required",
		})
	}

	return recommendations
}

// analyzeMetrics loads and analyzes metrics from files
func analyzeMetrics(metricsPath, baselinePath string) (*PerformanceReport, error) {
	// Load metrics
	metricsData, err := os.ReadFile(metricsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics file: %w", err)
	}

	var metrics []PerformanceMetrics
	if err := json.Unmarshal(metricsData, &metrics); err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	report := &PerformanceReport{
		Metrics:     metrics,
		GeneratedAt: time.Now(),
	}

	// Detect bottlenecks
	report.Bottlenecks = detectBottlenecks(metrics)

	// Load and compare baseline if provided
	if baselinePath != "" {
		baseline, err := loadBaseline(baselinePath)
		if err != nil {
			return nil, err
		}
		report.Baseline = baseline
		report.Regressions = detectRegressions(metrics, baseline)
	}

	// Generate recommendations
	report.Tuning = generateTuningRecommendations(report.Bottlenecks, metrics)

	return report, nil
}

// loadBaseline loads baseline metrics from a file
func loadBaseline(path string) (*PerformanceMetrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}

	var baseline PerformanceMetrics
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to parse baseline: %w", err)
	}

	return &baseline, nil
}

// writeReport outputs the performance report
func writeReport(report *PerformanceReport, outputPath, format string) error {
	var output io.Writer = os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		output = f
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)

	case "text":
		return writeTextReport(output, report)

	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

// writeTextReport formats the report as human-readable text
func writeTextReport(w io.Writer, report *PerformanceReport) error {
	const separator = "================================================================================\n"
	const divider = "--------------------------------------------------------------------------------\n"

	fmt.Fprintf(w, "Performance Analysis Report\n")
	fmt.Fprintf(w, "Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(w, separator+"\n")

	// Metrics summary
	if len(report.Metrics) > 0 {
		fmt.Fprintf(w, "Metrics Collected: %d samples\n", len(report.Metrics))

		// Calculate averages
		var totalMem, totalGoroutines, totalAllocRate float64
		for _, m := range report.Metrics {
			totalMem += float64(m.MemoryUsage)
			totalGoroutines += float64(m.GoroutineCount)
			totalAllocRate += m.AllocRate
		}

		fmt.Fprintf(w, "  Average Memory Usage: %.2f MB\n", totalMem/float64(len(report.Metrics))/(1024*1024))
		fmt.Fprintf(w, "  Average Goroutines: %.0f\n", totalGoroutines/float64(len(report.Metrics)))
		fmt.Fprintf(w, "  Average Alloc Rate: %.2f MB/s\n", totalAllocRate/float64(len(report.Metrics))/(1024*1024))
		fmt.Fprintf(w, "\n")
	}

	// Bottlenecks
	if len(report.Bottlenecks) > 0 {
		fmt.Fprintf(w, "Bottlenecks Detected: %d\n", len(report.Bottlenecks))
		fmt.Fprint(w, divider)
		for i, bn := range report.Bottlenecks {
			fmt.Fprintf(w, "%d. [%s] %s: %s\n", i+1, strings.ToUpper(bn.Severity), bn.Type, bn.Description)
			fmt.Fprintf(w, "   Value: %.2f, Threshold: %.2f\n", bn.Value, bn.Threshold)
		}
		fmt.Fprintf(w, "\n")
	} else {
		fmt.Fprintf(w, "No bottlenecks detected\n\n")
	}

	// Regressions
	if len(report.Regressions) > 0 {
		fmt.Fprintf(w, "Performance Regressions: %d\n", len(report.Regressions))
		fmt.Fprint(w, divider)
		for i, reg := range report.Regressions {
			fmt.Fprintf(w, "%d. %s: %.2f%% degradation\n", i+1, reg.Metric, reg.Degradation)
			fmt.Fprintf(w, "   Baseline: %.2f, Current: %.2f\n", reg.Baseline, reg.Current)
		}
		fmt.Fprintf(w, "\n")
	}

	// Tuning recommendations
	if len(report.Tuning) > 0 {
		fmt.Fprintf(w, "Tuning Recommendations:\n")
		fmt.Fprint(w, divider)
		for i, rec := range report.Tuning {
			fmt.Fprintf(w, "%d. [%s] %s\n", i+1, strings.ToUpper(rec.Priority), rec.Category)
			fmt.Fprintf(w, "   %s\n", rec.Description)
			fmt.Fprintf(w, "   Expected: %s\n", rec.Expected)
		}
		fmt.Fprintf(w, "\n")
	}

	return nil
}

// generateSampleReport creates sample data for testing
func generateSampleReport() *PerformanceReport {
	metrics := []PerformanceMetrics{
		{
			Timestamp:      time.Now().Add(-10 * time.Second),
			CPUUsage:       45.5,
			MemoryUsage:    512 * 1024 * 1024,
			GoroutineCount: 150,
			AllocRate:      50 * 1024 * 1024,
			GCPauses:       []time.Duration{5 * time.Millisecond, 3 * time.Millisecond},
		},
		{
			Timestamp:      time.Now().Add(-5 * time.Second),
			CPUUsage:       52.3,
			MemoryUsage:    768 * 1024 * 1024,
			GoroutineCount: 175,
			AllocRate:      75 * 1024 * 1024,
			GCPauses:       []time.Duration{8 * time.Millisecond, 6 * time.Millisecond},
		},
		{
			Timestamp:      time.Now(),
			CPUUsage:       48.1,
			MemoryUsage:    640 * 1024 * 1024,
			GoroutineCount: 160,
			AllocRate:      60 * 1024 * 1024,
			GCPauses:       []time.Duration{4 * time.Millisecond, 7 * time.Millisecond},
		},
	}

	report := &PerformanceReport{
		Metrics:     metrics,
		Bottlenecks: detectBottlenecks(metrics),
		GeneratedAt: time.Now(),
	}

	report.Tuning = generateTuningRecommendations(report.Bottlenecks, metrics)

	return report
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
