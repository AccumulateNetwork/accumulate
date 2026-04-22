// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/test/results"
)

// Example: Generate a performance report for a test run
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: generate_report <run_id>")
		os.Exit(1)
	}

	runID, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil {
		log.Fatalf("Invalid run ID: %v", err)
	}

	dbPath := os.Getenv("ACCUMULATE_TEST_RESULTS_DB")
	if dbPath == "" {
		dbPath = "./test-results.db"
	}

	db, err := results.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get run info
	run, err := db.GetTestRun(runID)
	if err != nil {
		log.Fatal(err)
	}

	// Get config
	config, err := db.GetConfig(run.ConfigHash)
	if err != nil {
		log.Fatal(err)
	}

	// Generate report
	fmt.Printf("========================================\n")
	fmt.Printf("Test Run Performance Report\n")
	fmt.Printf("========================================\n\n")

	fmt.Printf("Run Information:\n")
	fmt.Printf("  Run ID:       %d\n", run.ID)
	fmt.Printf("  Commit Hash:  %s\n", run.CommitHash)
	fmt.Printf("  Branch:       %s\n", run.Branch)
	fmt.Printf("  Status:       %s\n", run.Status)
	fmt.Printf("  Start Time:   %s\n", run.StartTime.Format(time.RFC3339))
	if run.EndTime != nil {
		duration := run.EndTime.Sub(run.StartTime)
		fmt.Printf("  End Time:     %s\n", run.EndTime.Format(time.RFC3339))
		fmt.Printf("  Duration:     %s\n", duration.Round(time.Second))
	}
	if run.Notes != "" {
		fmt.Printf("  Notes:        %s\n", run.Notes)
	}
	fmt.Println()

	fmt.Printf("Configuration:\n")
	if config.TransactionsPerSecond > 0 {
		fmt.Printf("  Target TPS:   %d\n", config.TransactionsPerSecond)
	}
	if config.Duration > 0 {
		fmt.Printf("  Duration:     %s\n", config.Duration)
	}
	if config.NumClients > 0 {
		fmt.Printf("  Clients:      %d\n", config.NumClients)
	}
	if config.NumValidators > 0 {
		fmt.Printf("  Validators:   %d\n", config.NumValidators)
	}
	if config.NumPartitions > 0 {
		fmt.Printf("  Partitions:   %d\n", config.NumPartitions)
	}
	fmt.Println()

	fmt.Printf("Results:\n")
	fmt.Printf("  Total Txs:    %d\n", run.TotalTransactions)
	fmt.Printf("  Total Errors: %d\n", run.TotalErrors)
	if run.TotalTransactions > 0 {
		errorRate := float64(run.TotalErrors) / float64(run.TotalTransactions) * 100
		fmt.Printf("  Error Rate:   %.2f%%\n", errorRate)
	}
	fmt.Println()

	// Get key metrics
	metricNames := []string{"tps", "latency", "latency_p50", "latency_p95", "latency_p99"}
	fmt.Printf("Performance Metrics:\n")
	fmt.Printf("  %-15s %10s %10s %10s %10s\n", "Metric", "Min", "Avg", "Max", "StdDev")
	fmt.Printf("  %s\n", "-------------------------------------------------------------")

	for _, metricName := range metricNames {
		stats, err := db.GetMetricStats(runID, metricName)
		if err != nil || stats.Count == 0 {
			continue
		}

		fmt.Printf("  %-15s %10.2f %10.2f %10.2f %10.2f\n",
			metricName,
			stats.Min,
			stats.Avg,
			stats.Max,
			stats.StdDev)
	}
	fmt.Println()

	// Get error summary
	errSummary, err := db.GetErrorSummary(runID)
	if err == nil && len(errSummary) > 0 {
		fmt.Printf("Error Summary:\n")
		for opType, count := range errSummary {
			fmt.Printf("  %-30s %d\n", opType, count)
		}
		fmt.Println()
	}

	// Get node health summary
	healthSummary, err := db.GetNodeHealthSummary(runID)
	if err == nil && len(healthSummary) > 0 {
		fmt.Printf("Node Health Summary:\n")
		for nodeID, statuses := range healthSummary {
			fmt.Printf("  %s:\n", nodeID)
			for status, count := range statuses {
				fmt.Printf("    %-10s %d samples\n", status, count)
			}
		}
		fmt.Println()
	}

	fmt.Printf("========================================\n")
}
