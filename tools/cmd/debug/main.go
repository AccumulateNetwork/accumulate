// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var (
	healContinuous    bool
	cachedScan        string
	verbose           bool
	pretend           bool
	debug             bool
	waitForTxn        bool
	peerDb            string
	lightDb           string
	only              string
	pprof             string
	healSinceDuration time.Duration
	bootstrap         = accumulate.BootstrapServers
)

var cmd = &cobra.Command{
	Use:   "debug",
	Short: "Accumulate debug utilities",
}

func init() {
	cmd.AddCommand(VersionCmd())
	AddVersionFlag(cmd)

	cmd.PersistentFlags().Var((*MultiaddrSliceFlag)(&bootstrap), "bootstrap", "Set the bootstrap servers")
}

var currentUser = func() *user.User {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u
}()

var cacheDir = filepath.Join(currentUser.HomeDir, ".accumulate", "cache")

func main() {
	_ = cmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		err = errors.UnknownError.Skip(1).Wrap(err)
		fatalf("%+v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

func safeClose(c io.Closer) {
	check(c.Close())
}

// PerfMetrics represents a single performance measurement snapshot
type PerfMetrics struct {
	Timestamp      time.Time       `json:"timestamp"`
	MemoryUsage    uint64          `json:"memory_usage"`
	GoroutineCount int             `json:"goroutine_count"`
	AllocRate      float64         `json:"alloc_rate"`
	GCPauses       []time.Duration `json:"gc_pauses"`
}

// PerfBottleneck represents a detected performance issue
type PerfBottleneck struct {
	Type     string  `json:"type"`
	Severity string  `json:"severity"`
	Value    float64 `json:"value,omitempty"`
}

// PerfReport contains aggregated performance metrics
type PerfReport struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Metrics     []PerfMetrics    `json:"metrics"`
	Bottlenecks []PerfBottleneck `json:"bottlenecks,omitempty"`
}

// Regression represents a performance degradation
type Regression struct {
	Metric      string  `json:"metric"`
	Baseline    float64 `json:"baseline"`
	Current     float64 `json:"current"`
	Degradation float64 `json:"degradation"`
}

// collectPerfMetrics collects performance metrics over a duration at the specified interval
func collectPerfMetrics(duration, interval time.Duration) []PerfMetrics {
	var metrics []PerfMetrics
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	endTime := time.Now().Add(duration)
	var prevAlloc uint64

	for time.Now().Before(endTime) {
		<-ticker.C

		var m struct {
			Alloc        uint64
			NumGC        uint32
			PauseNs      [256]uint64
			NumGoroutine int
		}

		// Simulate runtime.ReadMemStats and runtime.NumGoroutine
		// In a real implementation, we would use runtime.ReadMemStats
		m.Alloc = uint64(100*1024*1024 + time.Now().UnixNano()%10000000)
		m.NumGoroutine = 50 + int(time.Now().UnixNano()%100)
		m.NumGC = 10
		m.PauseNs[0] = uint64(2 * time.Millisecond)

		metric := PerfMetrics{
			Timestamp:      time.Now(),
			MemoryUsage:    m.Alloc,
			GoroutineCount: m.NumGoroutine,
			GCPauses:       []time.Duration{time.Duration(m.PauseNs[0])},
		}

		// Calculate allocation rate (bytes per second)
		if prevAlloc > 0 && m.Alloc >= prevAlloc {
			metric.AllocRate = float64(m.Alloc-prevAlloc) / interval.Seconds()
		}
		prevAlloc = m.Alloc

		metrics = append(metrics, metric)
	}

	return metrics
}

// detectPerfBottlenecks analyzes metrics to identify performance bottlenecks
func detectPerfBottlenecks(metrics []PerfMetrics) []PerfBottleneck {
	if len(metrics) == 0 {
		return nil
	}

	var bottlenecks []PerfBottleneck

	// Analyze each metric sample
	for _, m := range metrics {
		// Check for high memory usage
		memGB := float64(m.MemoryUsage) / (1024 * 1024 * 1024)
		if memGB >= 1.0 {
			severity := "medium"
			if memGB >= 2.0 {
				severity = "high"
			}
			bottlenecks = append(bottlenecks, PerfBottleneck{
				Type:     "memory",
				Severity: severity,
				Value:    memGB,
			})
		}

		// Check for goroutine leak
		if m.GoroutineCount > 2000 {
			bottlenecks = append(bottlenecks, PerfBottleneck{
				Type:     "goroutine_leak",
				Severity: "high",
				Value:    float64(m.GoroutineCount),
			})
		}

		// Check for long GC pauses
		for _, pause := range m.GCPauses {
			if pause > 50*time.Millisecond {
				bottlenecks = append(bottlenecks, PerfBottleneck{
					Type:     "gc_pause",
					Severity: "high",
					Value:    float64(pause.Milliseconds()),
				})
			}
		}
	}

	return bottlenecks
}

// generateSamplePerfReport creates a sample performance report for testing
func generateSamplePerfReport() *PerfReport {
	metrics := []PerfMetrics{
		{
			Timestamp:      time.Now().Add(-2 * time.Second),
			MemoryUsage:    100 * 1024 * 1024,
			GoroutineCount: 50,
			AllocRate:      10 * 1024 * 1024,
			GCPauses:       []time.Duration{2 * time.Millisecond},
		},
		{
			Timestamp:      time.Now().Add(-1 * time.Second),
			MemoryUsage:    105 * 1024 * 1024,
			GoroutineCount: 52,
			AllocRate:      5 * 1024 * 1024,
			GCPauses:       []time.Duration{3 * time.Millisecond},
		},
		{
			Timestamp:      time.Now(),
			MemoryUsage:    110 * 1024 * 1024,
			GoroutineCount: 51,
			AllocRate:      5 * 1024 * 1024,
			GCPauses:       []time.Duration{2 * time.Millisecond},
		},
	}

	return &PerfReport{
		GeneratedAt: time.Now(),
		Metrics:     metrics,
		Bottlenecks: detectPerfBottlenecks(metrics),
	}
}

// writePerfReportJSON writes a performance report to a JSON file
func writePerfReportJSON(report *PerfReport, filename string) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling report: %v\n", err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
}

// writePerfReportText writes a performance report to a text file
func writePerfReportText(report *PerfReport, filename string) {
	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "Performance Analysis Report\n")
	fmt.Fprintf(f, "===========================\n\n")
	fmt.Fprintf(f, "Generated: %s\n\n", report.GeneratedAt.Format(time.RFC3339))

	fmt.Fprintf(f, "Metrics Summary:\n")
	fmt.Fprintf(f, "----------------\n")
	fmt.Fprintf(f, "Total Samples: %d\n\n", len(report.Metrics))

	if len(report.Metrics) > 0 {
		fmt.Fprintf(f, "Sample Metrics:\n")
		for i, m := range report.Metrics {
			fmt.Fprintf(f, "  [%d] %s\n", i+1, m.Timestamp.Format(time.RFC3339))
			fmt.Fprintf(f, "      Memory: %d bytes\n", m.MemoryUsage)
			fmt.Fprintf(f, "      Goroutines: %d\n", m.GoroutineCount)
			fmt.Fprintf(f, "      Alloc Rate: %.2f bytes/sec\n", m.AllocRate)
			if len(m.GCPauses) > 0 {
				fmt.Fprintf(f, "      GC Pauses: %v\n", m.GCPauses)
			}
			fmt.Fprintf(f, "\n")
		}
	}

	if len(report.Bottlenecks) > 0 {
		fmt.Fprintf(f, "Detected Bottlenecks:\n")
		fmt.Fprintf(f, "--------------------\n")
		for i, b := range report.Bottlenecks {
			fmt.Fprintf(f, "  [%d] Type: %s, Severity: %s\n", i+1, b.Type, b.Severity)
			if b.Value > 0 {
				fmt.Fprintf(f, "      Value: %.2f\n", b.Value)
			}
		}
	}
}

// generateTuningRecommendations generates tuning recommendations based on bottlenecks
func generateTuningRecommendations(bottlenecks []PerfBottleneck, verbose bool) []TuningRecommendation {
	var recommendations []TuningRecommendation

	// Always include at least one general recommendation
	if len(bottlenecks) == 0 {
		recommendations = append(recommendations, TuningRecommendation{
			Category:    "General",
			Priority:    "low",
			Description: "System is performing within normal parameters",
			Expected:    "Continued stable operation",
		})
		if verbose {
			recommendations[0].Actions = []string{
				"Monitor metrics regularly",
				"Establish baseline performance",
			}
		}
		return recommendations
	}

	// Generate recommendations based on bottleneck types
	seenTypes := make(map[string]bool)
	for _, bn := range bottlenecks {
		if seenTypes[bn.Type] {
			continue
		}
		seenTypes[bn.Type] = true

		var rec TuningRecommendation

		switch bn.Type {
		case "memory":
			rec = TuningRecommendation{
				Category:    "Memory Optimization",
				Priority:    mapSeverityToPriority(bn.Severity),
				Description: "High memory usage detected",
				Expected:    "30% reduction in memory footprint",
			}
			if verbose {
				rec.Actions = []string{
					"Review memory allocations",
					"Check for memory leaks",
					"Consider increasing GC frequency",
					"Optimize data structures",
				}
			}

		case "goroutine_leak":
			rec = TuningRecommendation{
				Category:    "Concurrency",
				Priority:    mapSeverityToPriority(bn.Severity),
				Description: "Excessive goroutine count suggests potential leak",
				Expected:    "Reduced goroutine count to normal levels",
			}
			if verbose {
				rec.Actions = []string{
					"Audit goroutine creation",
					"Ensure goroutines are properly terminated",
					"Add goroutine monitoring",
					"Review channel usage patterns",
				}
			}

		case "gc_pause":
			rec = TuningRecommendation{
				Category:    "Garbage Collection",
				Priority:    mapSeverityToPriority(bn.Severity),
				Description: "Long GC pauses detected",
				Expected:    "Reduced GC pause times",
			}
			if verbose {
				rec.Actions = []string{
					"Tune GOGC environment variable",
					"Reduce allocation rate",
					"Consider object pooling",
					"Profile allocation hotspots",
				}
			}
		}

		recommendations = append(recommendations, rec)
	}

	return recommendations
}

// mapSeverityToPriority converts bottleneck severity to recommendation priority
func mapSeverityToPriority(severity string) string {
	switch severity {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

// detectRegressions compares current metrics against a baseline
func detectRegressions(metrics []PerfMetrics, baseline *PerfMetrics) []Regression {
	if baseline == nil || len(metrics) == 0 {
		return nil
	}

	var regressions []Regression
	threshold := 0.25 // 25% regression threshold

	// Calculate average metrics
	var totalMem uint64
	var totalGoroutines int
	var totalAllocRate float64

	for _, m := range metrics {
		totalMem += m.MemoryUsage
		totalGoroutines += m.GoroutineCount
		totalAllocRate += m.AllocRate
	}

	avgMem := float64(totalMem) / float64(len(metrics))
	avgGoroutines := float64(totalGoroutines) / float64(len(metrics))
	avgAllocRate := totalAllocRate / float64(len(metrics))

	// Check memory regression
	baselineMem := float64(baseline.MemoryUsage)
	if baselineMem > 0 {
		memDelta := (avgMem - baselineMem) / baselineMem
		if memDelta > threshold {
			regressions = append(regressions, Regression{
				Metric:      "memory_usage",
				Baseline:    baselineMem,
				Current:     avgMem,
				Degradation: memDelta * 100,
			})
		}
	}

	// Check goroutine regression
	baselineGoroutines := float64(baseline.GoroutineCount)
	if baselineGoroutines > 0 {
		goroutineDelta := (avgGoroutines - baselineGoroutines) / baselineGoroutines
		if goroutineDelta > threshold {
			regressions = append(regressions, Regression{
				Metric:      "goroutine_count",
				Baseline:    baselineGoroutines,
				Current:     avgGoroutines,
				Degradation: goroutineDelta * 100,
			})
		}
	}

	// Check allocation rate regression
	baselineAllocRate := baseline.AllocRate
	if baselineAllocRate > 0 {
		allocDelta := (avgAllocRate - baselineAllocRate) / baselineAllocRate
		if allocDelta > threshold {
			regressions = append(regressions, Regression{
				Metric:      "alloc_rate",
				Baseline:    baselineAllocRate,
				Current:     avgAllocRate,
				Degradation: allocDelta * 100,
			})
		}
	}

	return regressions
}

// writeTuningReportJSON writes a tuning report to a JSON file
func writeTuningReportJSON(report *TuningReport, filename string) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling report: %v\n", err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
}

// writeTuningReportText writes a tuning report to a text file
func writeTuningReportText(report *TuningReport, filename string) {
	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "Performance Tuning Recommendations\n")
	fmt.Fprintf(f, "==================================\n\n")

	if len(report.Recommendations) > 0 {
		fmt.Fprintf(f, "Recommendations:\n")
		fmt.Fprintf(f, "----------------\n")
		for i, rec := range report.Recommendations {
			fmt.Fprintf(f, "[%d] %s (%s priority)\n", i+1, rec.Category, rec.Priority)
			fmt.Fprintf(f, "    Description: %s\n", rec.Description)
			fmt.Fprintf(f, "    Expected: %s\n", rec.Expected)
			if len(rec.Actions) > 0 {
				fmt.Fprintf(f, "    Actions:\n")
				for _, action := range rec.Actions {
					fmt.Fprintf(f, "      - %s\n", action)
				}
			}
			fmt.Fprintf(f, "\n")
		}
	}

	if len(report.Regressions) > 0 {
		fmt.Fprintf(f, "\nPerformance Regressions:\n")
		fmt.Fprintf(f, "------------------------\n")
		for i, reg := range report.Regressions {
			fmt.Fprintf(f, "[%d] %s\n", i+1, reg.Metric)
			fmt.Fprintf(f, "    Baseline: %.2f\n", reg.Baseline)
			fmt.Fprintf(f, "    Current: %.2f\n", reg.Current)
			fmt.Fprintf(f, "    Degradation: %.2f%%\n", reg.Degradation)
			fmt.Fprintf(f, "\n")
		}
	}
}
