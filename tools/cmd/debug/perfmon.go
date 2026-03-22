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
	"runtime"
	rpprof "runtime/pprof"
	"time"

	"github.com/spf13/cobra"
)

var cmdPerfmon = &cobra.Command{
	Use:   "perfmon",
	Short: "Performance monitoring and analysis",
	Run:   runPerfmon,
}

var perfmonFlags struct {
	duration     time.Duration
	interval     time.Duration
	output       string
	format       string
	baseline     string
	cpuProfile   string
	memProfile   string
	generateSample bool
}

func init() {
	cmd.AddCommand(cmdPerfmon)

	cmdPerfmon.Flags().DurationVar(&perfmonFlags.duration, "duration", 10*time.Second, "Duration to collect metrics")
	cmdPerfmon.Flags().DurationVar(&perfmonFlags.interval, "interval", 1*time.Second, "Interval between metric collections")
	cmdPerfmon.Flags().StringVarP(&perfmonFlags.output, "output", "o", "", "Output file for report (default: stdout)")
	cmdPerfmon.Flags().StringVarP(&perfmonFlags.format, "format", "f", "text", "Output format: text, json")
	cmdPerfmon.Flags().StringVarP(&perfmonFlags.baseline, "baseline", "b", "", "Path to baseline metrics for comparison")
	cmdPerfmon.Flags().StringVar(&perfmonFlags.cpuProfile, "cpuprofile", "", "Write CPU profile to file")
	cmdPerfmon.Flags().StringVar(&perfmonFlags.memProfile, "memprofile", "", "Write memory profile to file")
	cmdPerfmon.Flags().BoolVar(&perfmonFlags.generateSample, "sample", false, "Generate sample metrics for testing")
}

// PerfMetrics represents collected performance data
type PerfMetrics struct {
	Timestamp      time.Time       `json:"timestamp"`
	CPUUsage       float64         `json:"cpu_usage"`
	MemoryUsage    uint64          `json:"memory_usage"`
	GoroutineCount int             `json:"goroutine_count"`
	AllocRate      float64         `json:"alloc_rate"`
	GCPauses       []time.Duration `json:"gc_pauses"`
}

// PerfBottleneck represents a detected performance issue
type PerfBottleneck struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
}

// PerfReport contains the analysis results
type PerfReport struct {
	Metrics     []PerfMetrics    `json:"metrics"`
	Bottlenecks []PerfBottleneck `json:"bottlenecks"`
	GeneratedAt time.Time        `json:"generated_at"`
}

func runPerfmon(cmd *cobra.Command, args []string) {
	if perfmonFlags.cpuProfile != "" {
		f, err := os.Create(perfmonFlags.cpuProfile)
		checkf(err, "create CPU profile")
		defer f.Close()
		
		err = rpprof.StartCPUProfile(f)
		checkf(err, "start CPU profile")
		defer rpprof.StopCPUProfile()
	}

	var report *PerfReport

	if perfmonFlags.generateSample {
		report = generateSamplePerfReport()
	} else {
		// Collect live metrics
		metrics := collectPerfMetrics(perfmonFlags.duration, perfmonFlags.interval)
		report = &PerfReport{
			Metrics:     metrics,
			GeneratedAt: time.Now(),
		}

		// Detect bottlenecks
		report.Bottlenecks = detectPerfBottlenecks(metrics)
	}

	if perfmonFlags.memProfile != "" {
		f, err := os.Create(perfmonFlags.memProfile)
		checkf(err, "create memory profile")
		defer f.Close()
		
		runtime.GC()
		err = rpprof.WriteHeapProfile(f)
		checkf(err, "write memory profile")
	}

	// Output report
	if perfmonFlags.format == "json" {
		writePerfReportJSON(report, perfmonFlags.output)
	} else {
		writePerfReportText(report, perfmonFlags.output)
	}
}

func collectPerfMetrics(duration, interval time.Duration) []PerfMetrics {
	var metrics []PerfMetrics
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

		var gcPauses []time.Duration
		for i := uint32(0); i < m.NumGC && i < 256; i++ {
			gcPauses = append(gcPauses, time.Duration(m.PauseNs[(m.NumGC-1-i+256)%256]))
		}

		metric := PerfMetrics{
			Timestamp:      time.Now(),
			CPUUsage:       float64(runtime.NumCPU()),
			MemoryUsage:    m.Alloc,
			GoroutineCount: runtime.NumGoroutine(),
			AllocRate:      allocRate,
			GCPauses:       gcPauses,
		}

		metrics = append(metrics, metric)
	}

	return metrics
}

func detectPerfBottlenecks(metrics []PerfMetrics) []PerfBottleneck {
	if len(metrics) == 0 {
		return nil
	}

	var bottlenecks []PerfBottleneck

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

	// High memory usage (>1GB)
	const memThreshold = 1024 * 1024 * 1024
	if avgMem > memThreshold {
		severity := "medium"
		if avgMem > 2*memThreshold {
			severity = "high"
		}

		bottlenecks = append(bottlenecks, PerfBottleneck{
			Type:        "memory",
			Severity:    severity,
			Description: "High memory usage detected",
			Value:       avgMem,
			Threshold:   memThreshold,
		})
	}

	// Goroutine leak (>1000)
	const goroutineThreshold = 1000
	if avgGoroutines > goroutineThreshold {
		bottlenecks = append(bottlenecks, PerfBottleneck{
			Type:        "goroutine_leak",
			Severity:    "medium",
			Description: "Excessive goroutine count",
			Value:       avgGoroutines,
			Threshold:   goroutineThreshold,
		})
	}

	// Long GC pauses (>10ms)
	if maxGCPause > 10*time.Millisecond {
		severity := "low"
		if maxGCPause > 50*time.Millisecond {
			severity = "medium"
		}

		bottlenecks = append(bottlenecks, PerfBottleneck{
			Type:        "gc_pause",
			Severity:    severity,
			Description: "Long GC pause times",
			Value:       float64(maxGCPause.Milliseconds()),
			Threshold:   10,
		})
	}

	return bottlenecks
}

func generateSamplePerfReport() *PerfReport {
	metrics := []PerfMetrics{
		{
			Timestamp:      time.Now().Add(-10 * time.Second),
			CPUUsage:       45.5,
			MemoryUsage:    512 * 1024 * 1024,
			GoroutineCount: 150,
			AllocRate:      50 * 1024 * 1024,
			GCPauses:       []time.Duration{5 * time.Millisecond, 3 * time.Millisecond},
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

	return &PerfReport{
		Metrics:     metrics,
		Bottlenecks: detectPerfBottlenecks(metrics),
		GeneratedAt: time.Now(),
	}
}

func writePerfReportJSON(report *PerfReport, outputPath string) {
	data, err := json.MarshalIndent(report, "", "  ")
	checkf(err, "marshal report")

	if outputPath != "" {
		err = os.WriteFile(outputPath, data, 0644)
		checkf(err, "write output file")
	} else {
		fmt.Println(string(data))
	}
}

func writePerfReportText(report *PerfReport, outputPath string) {
	output := fmt.Sprintf("Performance Analysis Report\n")
	output += fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
	output += fmt.Sprintf("================================================================================\n\n")

	if len(report.Metrics) > 0 {
		var totalMem, totalGoroutines float64
		for _, m := range report.Metrics {
			totalMem += float64(m.MemoryUsage)
			totalGoroutines += float64(m.GoroutineCount)
		}

		output += fmt.Sprintf("Metrics Collected: %d samples\n", len(report.Metrics))
		output += fmt.Sprintf("  Average Memory Usage: %.2f MB\n", totalMem/float64(len(report.Metrics))/(1024*1024))
		output += fmt.Sprintf("  Average Goroutines: %.0f\n\n", totalGoroutines/float64(len(report.Metrics)))
	}

	if len(report.Bottlenecks) > 0 {
		output += fmt.Sprintf("Bottlenecks Detected: %d\n", len(report.Bottlenecks))
		output += fmt.Sprintf("--------------------------------------------------------------------------------\n")
		for i, bn := range report.Bottlenecks {
			output += fmt.Sprintf("%d. [%s] %s: %s\n", i+1, bn.Severity, bn.Type, bn.Description)
			output += fmt.Sprintf("   Value: %.2f, Threshold: %.2f\n", bn.Value, bn.Threshold)
		}
	} else {
		output += "No bottlenecks detected\n"
	}

	if outputPath != "" {
		err := os.WriteFile(outputPath, []byte(output), 0644)
		checkf(err, "write output file")
	} else {
		fmt.Print(output)
	}
}
