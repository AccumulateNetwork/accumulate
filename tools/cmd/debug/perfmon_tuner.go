// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var cmdPerfMonTuner = &cobra.Command{
	Use:   "perfmon-tuner [report-file]",
	Short: "Analyze performance reports and provide AI-driven tuning recommendations",
	Args:  cobra.ExactArgs(1),
	Run:   perfMonTuner,
}

var (
	perfMonTunerFlags = struct {
		OutputFile      string
		Threshold       float64
		CompareBaseline string
		Verbose         bool
	}{}
)

func init() {
	cmd.AddCommand(cmdPerfMonTuner)
	cmdPerfMonTuner.Flags().StringVar(&perfMonTunerFlags.OutputFile, "output", "", "Output file for recommendations (JSON)")
	cmdPerfMonTuner.Flags().Float64Var(&perfMonTunerFlags.Threshold, "threshold", 0.8, "TPS achievement threshold (0.0-1.0)")
	cmdPerfMonTuner.Flags().StringVar(&perfMonTunerFlags.CompareBaseline, "baseline", "", "Baseline report for comparison")
	cmdPerfMonTuner.Flags().BoolVarP(&perfMonTunerFlags.Verbose, "verbose", "v", false, "Verbose output")
}

// TuningRecommendation represents a single tuning suggestion
type TuningRecommendation struct {
	Category    string                 `json:"category"`
	Priority    string                 `json:"priority"`
	Description string                 `json:"description"`
	Expected    string                 `json:"expected"`
	Actions     []string               `json:"actions,omitempty"`

	// Legacy fields for compatibility with existing code
	Parameter   string                 `json:"parameter,omitempty"`
	CurrentVal  interface{}            `json:"current_value,omitempty"`
	Suggested   interface{}            `json:"suggested_value,omitempty"`
	Reason      string                 `json:"reason,omitempty"`
	Impact      string                 `json:"expected_impact,omitempty"`
	Additional  map[string]interface{} `json:"additional_info,omitempty"`
}

// BottleneckAnalysis identifies performance bottlenecks
type BottleneckAnalysis struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Description string  `json:"description"`
	Metric      string  `json:"metric"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold,omitempty"`
}

// TuningReport contains the complete tuning analysis
type TuningReport struct {
	Recommendations  []TuningRecommendation `json:"recommendations"`
	Regressions      []Regression           `json:"regressions,omitempty"`

	// Legacy fields for compatibility with existing code
	ReportFile         string              `json:"report_file,omitempty"`
	AnalysisTime       string              `json:"analysis_time,omitempty"`
	Summary            TuningSummary       `json:"summary,omitempty"`
	Bottlenecks        []BottleneckAnalysis `json:"bottlenecks,omitempty"`
	BaselineComparison *BaselineComparison `json:"baseline_comparison,omitempty"`
}

// TuningSummary provides high-level analysis
type TuningSummary struct {
	TPSAchievement     float64 `json:"tps_achievement_percent"`
	SuccessRate        float64 `json:"success_rate_percent"`
	ErrorRate          float64 `json:"error_rate_percent"`
	AvgLatency         float64 `json:"avg_latency_ms"`
	OverallHealth      string  `json:"overall_health"`
	CriticalIssues     int     `json:"critical_issues"`
	WarningIssues      int     `json:"warning_issues"`
}

// BaselineComparison compares current performance with baseline
type BaselineComparison struct {
	BaselineFile       string  `json:"baseline_file"`
	TPSDelta           float64 `json:"tps_delta_percent"`
	LatencyDelta       float64 `json:"latency_delta_percent"`
	SuccessRateDelta   float64 `json:"success_rate_delta_percent"`
	Verdict            string  `json:"verdict"`
}

func perfMonTuner(_ *cobra.Command, args []string) {
	reportFile := args[0]

	// Load performance report
	report, err := loadPerformanceReport(reportFile)
	if err != nil {
		fatalf("Failed to load report: %v", err)
	}

	// Analyze report
	tuningReport := analyzeReport(report, reportFile)

	// Compare with baseline if provided
	if perfMonTunerFlags.CompareBaseline != "" {
		baseline, err := loadPerformanceReport(perfMonTunerFlags.CompareBaseline)
		if err != nil {
			fatalf("Failed to load baseline: %v", err)
		}
		tuningReport.BaselineComparison = compareWithBaseline(report, baseline, perfMonTunerFlags.CompareBaseline)
	}

	// Print analysis
	printTuningReport(tuningReport)

	// Save to file if requested
	if perfMonTunerFlags.OutputFile != "" {
		err := saveTuningReport(tuningReport, perfMonTunerFlags.OutputFile)
		if err != nil {
			fatalf("Failed to save tuning report: %v", err)
		}
		fmt.Printf("\nTuning recommendations saved to: %s\n", perfMonTunerFlags.OutputFile)
	}
}

func loadPerformanceReport(filename string) (*PerformanceReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var report PerformanceReport
	err = json.Unmarshal(data, &report)
	if err != nil {
		return nil, err
	}

	return &report, nil
}

func analyzeReport(report *PerformanceReport, filename string) *TuningReport {
	tuning := &TuningReport{
		ReportFile:   filename,
		AnalysisTime: getCurrentTimestamp(),
	}

	// Calculate summary metrics
	tuning.Summary = calculateSummary(report)

	// Identify bottlenecks
	tuning.Bottlenecks = identifyBottlenecks(report)

	// Generate recommendations
	tuning.Recommendations = generateRecommendations(report, tuning.Bottlenecks)

	return tuning
}

func calculateSummary(report *PerformanceReport) TuningSummary {
	summary := TuningSummary{
		SuccessRate: report.SuccessRate,
		AvgLatency:  report.LatencyMean,
	}

	// Calculate TPS achievement
	if report.TargetTPS > 0 {
		summary.TPSAchievement = (report.AchievedTPS / float64(report.TargetTPS)) * 100
	}

	// Calculate error rate
	if report.SubmittedTotal > 0 {
		summary.ErrorRate = (float64(report.ErrorTotal) / float64(report.SubmittedTotal)) * 100
	}

	// Determine overall health
	criticalCount := 0
	warningCount := 0

	if summary.TPSAchievement < 50 {
		criticalCount++
	} else if summary.TPSAchievement < perfMonTunerFlags.Threshold*100 {
		warningCount++
	}

	if summary.SuccessRate < 90 {
		criticalCount++
	} else if summary.SuccessRate < 95 {
		warningCount++
	}

	if report.LatencyP99 > 5000 {
		criticalCount++
	} else if report.LatencyP99 > 2000 {
		warningCount++
	}

	summary.CriticalIssues = criticalCount
	summary.WarningIssues = warningCount

	if criticalCount > 0 {
		summary.OverallHealth = "CRITICAL"
	} else if warningCount > 0 {
		summary.OverallHealth = "WARNING"
	} else {
		summary.OverallHealth = "HEALTHY"
	}

	return summary
}

func identifyBottlenecks(report *PerformanceReport) []BottleneckAnalysis {
	var bottlenecks []BottleneckAnalysis

	// TPS bottleneck
	tpsAchievement := 0.0
	if report.TargetTPS > 0 {
		tpsAchievement = (report.AchievedTPS / float64(report.TargetTPS)) * 100
	}

	if tpsAchievement < perfMonTunerFlags.Threshold*100 {
		severity := "WARNING"
		if tpsAchievement < 50 {
			severity = "CRITICAL"
		}
		bottlenecks = append(bottlenecks, BottleneckAnalysis{
			Type:        "throughput",
			Severity:    severity,
			Description: fmt.Sprintf("Achieved TPS (%.2f) is below target (%d)", report.AchievedTPS, report.TargetTPS),
			Metric:      "achieved_tps",
			Value:       report.AchievedTPS,
			Threshold:   float64(report.TargetTPS) * perfMonTunerFlags.Threshold,
		})
	}

	// Latency bottleneck
	if report.LatencyP99 > 2000 {
		severity := "WARNING"
		if report.LatencyP99 > 5000 {
			severity = "CRITICAL"
		}
		bottlenecks = append(bottlenecks, BottleneckAnalysis{
			Type:        "latency",
			Severity:    severity,
			Description: fmt.Sprintf("P99 latency (%.2f ms) is high", report.LatencyP99),
			Metric:      "latency_p99_ms",
			Value:       report.LatencyP99,
			Threshold:   2000,
		})
	}

	// Error rate bottleneck
	if report.SuccessRate < 95 {
		severity := "WARNING"
		if report.SuccessRate < 90 {
			severity = "CRITICAL"
		}
		bottlenecks = append(bottlenecks, BottleneckAnalysis{
			Type:        "errors",
			Severity:    severity,
			Description: fmt.Sprintf("Success rate (%.2f%%) is low", report.SuccessRate),
			Metric:      "success_rate",
			Value:       report.SuccessRate,
			Threshold:   95,
		})
	}

	// Block rate bottleneck
	if report.BlockRate > 0 && report.BlockRate < 6 {
		bottlenecks = append(bottlenecks, BottleneckAnalysis{
			Type:        "block_production",
			Severity:    "INFO",
			Description: fmt.Sprintf("Block rate (%.2f blocks/min) is low", report.BlockRate),
			Metric:      "block_rate",
			Value:       report.BlockRate,
			Threshold:   6,
		})
	}

	// Sort by severity
	sort.Slice(bottlenecks, func(i, j int) bool {
		severityOrder := map[string]int{"CRITICAL": 0, "WARNING": 1, "INFO": 2}
		return severityOrder[bottlenecks[i].Severity] < severityOrder[bottlenecks[j].Severity]
	})

	return bottlenecks
}

func generateRecommendations(report *PerformanceReport, bottlenecks []BottleneckAnalysis) []TuningRecommendation {
	var recommendations []TuningRecommendation

	// Analyze each bottleneck
	for _, bottleneck := range bottlenecks {
		switch bottleneck.Type {
		case "throughput":
			recommendations = append(recommendations, generateThroughputRecommendations(report, bottleneck)...)
		case "latency":
			recommendations = append(recommendations, generateLatencyRecommendations(report, bottleneck)...)
		case "errors":
			recommendations = append(recommendations, generateErrorRecommendations(report, bottleneck)...)
		case "block_production":
			recommendations = append(recommendations, generateBlockRecommendations(report, bottleneck)...)
		}
	}

	// Add general optimization recommendations
	if len(bottlenecks) == 0 {
		recommendations = append(recommendations, generateOptimizationRecommendations(report)...)
	}

	// Sort by priority
	sort.Slice(recommendations, func(i, j int) bool {
		priorityOrder := map[string]int{"HIGH": 0, "MEDIUM": 1, "LOW": 2}
		return priorityOrder[recommendations[i].Priority] < priorityOrder[recommendations[j].Priority]
	})

	return recommendations
}

func generateThroughputRecommendations(report *PerformanceReport, bottleneck BottleneckAnalysis) []TuningRecommendation {
	var recs []TuningRecommendation

	// Consensus timeout adjustments
	if report.AvgBlockInterval > 0 && report.AvgBlockInterval > 2000 {
		recs = append(recs, TuningRecommendation{
			Category:   "consensus",
			Parameter:  "TimeoutPropose",
			Suggested:  "1s",
			Reason:     "Block intervals are high, reducing propose timeout may increase throughput",
			Impact:     "Faster block production, potentially 20-30% TPS improvement",
			Priority:   "HIGH",
		})
		recs = append(recs, TuningRecommendation{
			Category:   "consensus",
			Parameter:  "TimeoutCommit",
			Suggested:  "500ms",
			Reason:     "Reduce time between blocks to increase throughput",
			Impact:     "More blocks per minute, improved TPS",
			Priority:   "HIGH",
		})
	}

	// Mempool settings
	recs = append(recs, TuningRecommendation{
		Category:   "mempool",
		Parameter:  "Size",
		Suggested:  10000,
		Reason:     "Increase mempool capacity to handle higher transaction volumes",
		Impact:     "Better transaction buffering, reduced rejection rate",
		Priority:   "MEDIUM",
	})

	recs = append(recs, TuningRecommendation{
		Category:   "mempool",
		Parameter:  "MaxTxsBytes",
		Suggested:  104857600, // 100MB
		Reason:     "Allow more transaction data in mempool",
		Impact:     "Support larger batches, improved throughput",
		Priority:   "MEDIUM",
	})

	// Block size limits
	recs = append(recs, TuningRecommendation{
		Category:   "blocks",
		Parameter:  "MaxBytes",
		Suggested:  22020096, // 21MB
		Reason:     "Increase max block size to include more transactions per block",
		Impact:     "More transactions per block, higher TPS",
		Priority:   "HIGH",
	})

	recs = append(recs, TuningRecommendation{
		Category:   "blocks",
		Parameter:  "MaxGas",
		Suggested:  -1,
		Reason:     "Remove gas limit to maximize transactions per block",
		Impact:     "No artificial gas constraints on throughput",
		Priority:   "MEDIUM",
	})

	return recs
}

func generateLatencyRecommendations(report *PerformanceReport, bottleneck BottleneckAnalysis) []TuningRecommendation {
	var recs []TuningRecommendation

	// Database tuning
	recs = append(recs, TuningRecommendation{
		Category:   "database",
		Parameter:  "NumMemtables",
		Suggested:  3,
		Reason:     "High latency may indicate database write bottleneck",
		Impact:     "Faster write throughput, reduced P99 latency",
		Priority:   "HIGH",
	})

	recs = append(recs, TuningRecommendation{
		Category:   "database",
		Parameter:  "NumLevelZeroTables",
		Suggested:  3,
		Reason:     "Allow more L0 tables before compaction to reduce write stalls",
		Impact:     "Smoother write performance, reduced latency spikes",
		Priority:   "MEDIUM",
	})

	// Network tuning
	recs = append(recs, TuningRecommendation{
		Category:   "network",
		Parameter:  "SendRate",
		Suggested:  52428800, // 50MB/s
		Reason:     "Increase network bandwidth to reduce message delays",
		Impact:     "Faster message propagation, lower latency",
		Priority:   "MEDIUM",
	})

	recs = append(recs, TuningRecommendation{
		Category:   "network",
		Parameter:  "RecvRate",
		Suggested:  52428800, // 50MB/s
		Reason:     "Match send rate for balanced network performance",
		Impact:     "Better network utilization, reduced latency",
		Priority:   "MEDIUM",
	})

	return recs
}

func generateErrorRecommendations(report *PerformanceReport, bottleneck BottleneckAnalysis) []TuningRecommendation {
	var recs []TuningRecommendation

	// Analyze error types
	if len(report.ErrorsByType) > 0 {
		if txErrors, ok := report.ErrorsByType["tx_error"]; ok && txErrors > 0 {
			recs = append(recs, TuningRecommendation{
				Category:   "application",
				Parameter:  "transaction_validation",
				Suggested:  "review_validation_rules",
				Reason:     fmt.Sprintf("High transaction error count (%d)", txErrors),
				Impact:     "Reduced error rate, improved success rate",
				Priority:   "HIGH",
				Additional: map[string]interface{}{
					"error_count": txErrors,
				},
			})
		}

		if submitErrors, ok := report.ErrorsByType["submit"]; ok && submitErrors > 0 {
			recs = append(recs, TuningRecommendation{
				Category:   "network",
				Parameter:  "MaxPacketMsgPayloadSize",
				Suggested:  10240, // 10KB
				Reason:     fmt.Sprintf("Submission errors detected (%d), may need larger packet size", submitErrors),
				Impact:     "Reduced submission failures",
				Priority:   "MEDIUM",
				Additional: map[string]interface{}{
					"error_count": submitErrors,
				},
			})
		}
	}

	// Timeout recommendations
	recs = append(recs, TuningRecommendation{
		Category:   "consensus",
		Parameter:  "TimeoutPrevote",
		Suggested:  "2s",
		Reason:     "Increase timeout to reduce timeout-related errors",
		Impact:     "More reliable consensus, fewer timeout errors",
		Priority:   "LOW",
	})

	return recs
}

func generateBlockRecommendations(report *PerformanceReport, bottleneck BottleneckAnalysis) []TuningRecommendation {
	var recs []TuningRecommendation

	recs = append(recs, TuningRecommendation{
		Category:   "consensus",
		Parameter:  "TimeoutCommit",
		Suggested:  "500ms",
		Reason:     "Reduce block interval to increase block production rate",
		Impact:     "More blocks per minute, better transaction throughput",
		Priority:   "HIGH",
	})

	recs = append(recs, TuningRecommendation{
		Category:   "consensus",
		Parameter:  "CreateEmptyBlocks",
		Suggested:  true,
		Reason:     "Enable empty block creation for consistent block times",
		Impact:     "More predictable block production",
		Priority:   "LOW",
	})

	return recs
}

func generateOptimizationRecommendations(report *PerformanceReport) []TuningRecommendation {
	var recs []TuningRecommendation

	// System is healthy, suggest fine-tuning
	recs = append(recs, TuningRecommendation{
		Category:   "optimization",
		Parameter:  "general",
		Suggested:  "fine_tune",
		Reason:     "System is performing well, consider incremental optimizations",
		Impact:     "Marginal improvements in efficiency",
		Priority:   "LOW",
	})

	// Database compaction
	recs = append(recs, TuningRecommendation{
		Category:   "database",
		Parameter:  "CompactionStrategy",
		Suggested:  "leveled",
		Reason:     "Leveled compaction provides consistent performance",
		Impact:     "More predictable latency over time",
		Priority:   "LOW",
	})

	return recs
}

func compareWithBaseline(current, baseline *PerformanceReport, baselineFile string) *BaselineComparison {
	comp := &BaselineComparison{
		BaselineFile: baselineFile,
	}

	// Calculate deltas
	if baseline.AchievedTPS > 0 {
		comp.TPSDelta = ((current.AchievedTPS - baseline.AchievedTPS) / baseline.AchievedTPS) * 100
	}

	if baseline.LatencyMean > 0 {
		comp.LatencyDelta = ((current.LatencyMean - baseline.LatencyMean) / baseline.LatencyMean) * 100
	}

	if baseline.SuccessRate > 0 {
		comp.SuccessRateDelta = ((current.SuccessRate - baseline.SuccessRate) / baseline.SuccessRate) * 100
	}

	// Determine verdict
	improvements := 0
	regressions := 0

	if comp.TPSDelta > 5 {
		improvements++
	} else if comp.TPSDelta < -5 {
		regressions++
	}

	if comp.LatencyDelta < -5 {
		improvements++
	} else if comp.LatencyDelta > 5 {
		regressions++
	}

	if comp.SuccessRateDelta > 1 {
		improvements++
	} else if comp.SuccessRateDelta < -1 {
		regressions++
	}

	if regressions > improvements {
		comp.Verdict = "REGRESSION"
	} else if improvements > regressions {
		comp.Verdict = "IMPROVEMENT"
	} else {
		comp.Verdict = "NEUTRAL"
	}

	return comp
}

func printTuningReport(report *TuningReport) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("PERFORMANCE TUNING ANALYSIS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Report: %s\n", report.ReportFile)
	fmt.Printf("Analysis Time: %s\n\n", report.AnalysisTime)

	// Summary
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Overall Health: %s\n", report.Summary.OverallHealth)
	fmt.Printf("TPS Achievement: %.2f%%\n", report.Summary.TPSAchievement)
	fmt.Printf("Success Rate: %.2f%%\n", report.Summary.SuccessRate)
	fmt.Printf("Error Rate: %.2f%%\n", report.Summary.ErrorRate)
	fmt.Printf("Average Latency: %.2f ms\n", report.Summary.AvgLatency)
	fmt.Printf("Critical Issues: %d\n", report.Summary.CriticalIssues)
	fmt.Printf("Warning Issues: %d\n\n", report.Summary.WarningIssues)

	// Baseline comparison
	if report.BaselineComparison != nil {
		fmt.Println("BASELINE COMPARISON")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("Baseline: %s\n", report.BaselineComparison.BaselineFile)
		fmt.Printf("Verdict: %s\n", report.BaselineComparison.Verdict)
		fmt.Printf("TPS Change: %+.2f%%\n", report.BaselineComparison.TPSDelta)
		fmt.Printf("Latency Change: %+.2f%%\n", report.BaselineComparison.LatencyDelta)
		fmt.Printf("Success Rate Change: %+.2f%%\n\n", report.BaselineComparison.SuccessRateDelta)
	}

	// Bottlenecks
	if len(report.Bottlenecks) > 0 {
		fmt.Println("IDENTIFIED BOTTLENECKS")
		fmt.Println(strings.Repeat("-", 80))
		for i, bottleneck := range report.Bottlenecks {
			fmt.Printf("%d. [%s] %s\n", i+1, bottleneck.Severity, bottleneck.Type)
			fmt.Printf("   %s\n", bottleneck.Description)
			fmt.Printf("   Metric: %s = %.2f", bottleneck.Metric, bottleneck.Value)
			if bottleneck.Threshold > 0 {
				fmt.Printf(" (threshold: %.2f)", bottleneck.Threshold)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		fmt.Println("TUNING RECOMMENDATIONS")
		fmt.Println(strings.Repeat("=", 80))

		currentCategory := ""
		for i, rec := range report.Recommendations {
			if rec.Category != currentCategory {
				currentCategory = rec.Category
				fmt.Printf("\n%s\n", strings.ToUpper(currentCategory))
				fmt.Println(strings.Repeat("-", 80))
			}

			fmt.Printf("%d. [%s] %s\n", i+1, rec.Priority, rec.Parameter)
			if rec.CurrentVal != nil {
				fmt.Printf("   Current: %v\n", rec.CurrentVal)
			}
			fmt.Printf("   Suggested: %v\n", rec.Suggested)
			fmt.Printf("   Reason: %s\n", rec.Reason)
			fmt.Printf("   Impact: %s\n", rec.Impact)

			if perfMonTunerFlags.Verbose && rec.Additional != nil && len(rec.Additional) > 0 {
				fmt.Printf("   Additional Info: %v\n", rec.Additional)
			}
			fmt.Println()
		}
	}

	fmt.Println(strings.Repeat("=", 80) + "\n")
}

func saveTuningReport(report *TuningReport, filename string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if dir != "" && dir != "." {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return err
		}
	}

	return os.WriteFile(filename, data, 0644)
}

func getCurrentTimestamp() string {
	return time.Now().Format(time.RFC3339)
}
