// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var cmdPerfTune = &cobra.Command{
	Use:   "perftune [metrics-file]",
	Short: "Generate tuning recommendations from performance metrics",
	Args:  cobra.ExactArgs(1),
	Run:   runPerfTune,
}

var perfTuneFlags struct {
	baseline string
	output   string
	format   string
	verbose  bool
}

func init() {
	cmd.AddCommand(cmdPerfTune)

	cmdPerfTune.Flags().StringVarP(&perfTuneFlags.baseline, "baseline", "b", "", "Path to baseline metrics for regression detection")
	cmdPerfTune.Flags().StringVarP(&perfTuneFlags.output, "output", "o", "", "Output file for recommendations (default: stdout)")
	cmdPerfTune.Flags().StringVarP(&perfTuneFlags.format, "format", "f", "text", "Output format: text, json")
	cmdPerfTune.Flags().BoolVarP(&perfTuneFlags.verbose, "verbose", "v", false, "Verbose output with detailed explanations")
}

// TuningRecommendation provides optimization suggestions
type TuningRecommendation struct {
	Category    string `json:"category"`
	Priority    string `json:"priority"`
	Description string `json:"description"`
	Expected    string `json:"expected_improvement"`
	Actions     []string `json:"actions,omitempty"`
}

// Regression represents a performance regression
type Regression struct {
	Metric      string  `json:"metric"`
	Baseline    float64 `json:"baseline"`
	Current     float64 `json:"current"`
	Degradation float64 `json:"degradation"`
}

// TuningReport contains tuning analysis
type TuningReport struct {
	Recommendations []TuningRecommendation `json:"recommendations"`
	Regressions     []Regression           `json:"regressions,omitempty"`
}

func runPerfTune(cmd *cobra.Command, args []string) {
	metricsFile := args[0]

	// Load metrics
	data, err := os.ReadFile(metricsFile)
	checkf(err, "read metrics file")

	var metrics []PerfMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		fatalf("failed to parse metrics: %v", err)
	}

	// Detect bottlenecks
	bottlenecks := detectPerfBottlenecks(metrics)

	// Generate recommendations
	recommendations := generateTuningRecommendations(bottlenecks, perfTuneFlags.verbose)

	report := &TuningReport{
		Recommendations: recommendations,
	}

	// Check for regressions if baseline provided
	if perfTuneFlags.baseline != "" {
		baselineData, err := os.ReadFile(perfTuneFlags.baseline)
		checkf(err, "read baseline file")

		var baseline PerfMetrics
		if err := json.Unmarshal(baselineData, &baseline); err != nil {
			fatalf("failed to parse baseline: %v", err)
		}

		report.Regressions = detectRegressions(metrics, &baseline)
	}

	// Output report
	if perfTuneFlags.format == "json" {
		writeTuningReportJSON(report, perfTuneFlags.output)
	} else {
		writeTuningReportText(report, perfTuneFlags.output)
	}
}

func generateTuningRecommendations(bottlenecks []PerfBottleneck, verbose bool) []TuningRecommendation {
	var recommendations []TuningRecommendation

	for _, bn := range bottlenecks {
		var rec TuningRecommendation

		switch bn.Type {
		case "memory":
			rec = TuningRecommendation{
				Category:    "Memory Optimization",
				Priority:    bn.Severity,
				Description: "High memory usage detected. Consider reducing allocations and implementing memory limits.",
				Expected:    "30-50% reduction in memory usage",
			}
			if verbose {
				rec.Actions = []string{
					"Implement object pooling using sync.Pool",
					"Profile heap allocations with pprof",
					"Use memory ballast to reduce GC frequency",
					"Preallocate slices and maps with known capacity",
					"Reduce pointer usage to decrease GC scan work",
				}
			}

		case "goroutine_leak":
			rec = TuningRecommendation{
				Category:    "Concurrency",
				Priority:    bn.Severity,
				Description: "Excessive goroutine count suggests potential leaks. Ensure proper cleanup.",
				Expected:    "Stable goroutine count",
			}
			if verbose {
				rec.Actions = []string{
					"Add context cancellation to all goroutines",
					"Use errgroup for structured concurrency",
					"Implement goroutine leak detection in tests",
					"Add timeouts to long-running operations",
					"Use bounded worker pools instead of unbounded goroutines",
				}
			}

		case "gc_pause":
			rec = TuningRecommendation{
				Category:    "GC Tuning",
				Priority:    bn.Severity,
				Description: "Long GC pause times detected. Reduce allocation rate or tune GC parameters.",
				Expected:    "50-70% reduction in GC pause times",
			}
			if verbose {
				rec.Actions = []string{
					"Reduce allocation rate (see allocation recommendations)",
					"Increase GOGC environment variable (default 100)",
					"Use memory ballast for more stable GC cycles",
					"Profile allocations to find hotspots",
					"Consider using off-heap storage for large data",
				}
			}

		case "allocation_rate":
			rec = TuningRecommendation{
				Category:    "Allocation Optimization",
				Priority:    bn.Severity,
				Description: "High allocation rate increases GC pressure. Reuse objects where possible.",
				Expected:    "40-60% reduction in allocation rate",
			}
			if verbose {
				rec.Actions = []string{
					"Use sync.Pool for frequently allocated objects",
					"Reuse buffers with buffer pools",
					"Preallocate slices with make([]T, 0, capacity)",
					"Avoid string concatenation in loops (use strings.Builder)",
					"Profile allocations with 'go tool pprof -alloc_space'",
				}
			}
		}

		recommendations = append(recommendations, rec)
	}

	// Sort by priority
	sort.Slice(recommendations, func(i, j int) bool {
		priorityOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return priorityOrder[recommendations[i].Priority] < priorityOrder[recommendations[j].Priority]
	})

	// Add general recommendation if no bottlenecks
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

func detectRegressions(metrics []PerfMetrics, baseline *PerfMetrics) []Regression {
	if len(metrics) == 0 || baseline == nil {
		return nil
	}

	var regressions []Regression

	// Calculate averages
	var totalMem, totalGoroutines, totalAllocRate float64
	for _, m := range metrics {
		totalMem += float64(m.MemoryUsage)
		totalGoroutines += float64(m.GoroutineCount)
		totalAllocRate += m.AllocRate
	}

	avgMem := totalMem / float64(len(metrics))
	avgGoroutines := totalGoroutines / float64(len(metrics))
	avgAllocRate := totalAllocRate / float64(len(metrics))

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

func writeTuningReportJSON(report *TuningReport, outputPath string) {
	data, err := json.MarshalIndent(report, "", "  ")
	checkf(err, "marshal report")

	if outputPath != "" {
		err = os.WriteFile(outputPath, data, 0644)
		checkf(err, "write output file")
	} else {
		fmt.Println(string(data))
	}
}

func writeTuningReportText(report *TuningReport, outputPath string) {
	var output strings.Builder

	output.WriteString("Performance Tuning Recommendations\n")
	output.WriteString(strings.Repeat("=", 80) + "\n\n")

	if len(report.Regressions) > 0 {
		output.WriteString(fmt.Sprintf("Performance Regressions Detected: %d\n", len(report.Regressions)))
		output.WriteString(strings.Repeat("-", 80) + "\n")
		for i, reg := range report.Regressions {
			output.WriteString(fmt.Sprintf("%d. %s: %.2f%% degradation\n", i+1, reg.Metric, reg.Degradation))
			output.WriteString(fmt.Sprintf("   Baseline: %.2f, Current: %.2f\n", reg.Baseline, reg.Current))
		}
		output.WriteString("\n")
	}

	if len(report.Recommendations) > 0 {
		output.WriteString(fmt.Sprintf("Recommendations: %d\n", len(report.Recommendations)))
		output.WriteString(strings.Repeat("-", 80) + "\n")
		for i, rec := range report.Recommendations {
			output.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, strings.ToUpper(rec.Priority), rec.Category))
			output.WriteString(fmt.Sprintf("   %s\n", rec.Description))
			output.WriteString(fmt.Sprintf("   Expected: %s\n", rec.Expected))
			
			if len(rec.Actions) > 0 {
				output.WriteString("   Actions:\n")
				for _, action := range rec.Actions {
					output.WriteString(fmt.Sprintf("     - %s\n", action))
				}
			}
			output.WriteString("\n")
		}
	} else {
		output.WriteString("No recommendations at this time.\n")
	}

	outputStr := output.String()
	if outputPath != "" {
		err := os.WriteFile(outputPath, []byte(outputStr), 0644)
		checkf(err, "write output file")
	} else {
		fmt.Print(outputStr)
	}
}
