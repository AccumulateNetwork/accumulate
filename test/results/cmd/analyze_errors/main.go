package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"gitlab.com/accumulatenetwork/accumulate/test/results"
)

// Example: Analyze error patterns in a test run
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: analyze_errors <run_id>")
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

	fmt.Printf("Test Run Analysis\n")
	fmt.Printf("=================\n")
	fmt.Printf("Run ID:       %d\n", run.ID)
	fmt.Printf("Commit:       %s\n", run.CommitHash)
	fmt.Printf("Branch:       %s\n", run.Branch)
	fmt.Printf("Status:       %s\n", run.Status)
	fmt.Printf("Transactions: %d\n", run.TotalTransactions)
	fmt.Printf("Errors:       %d\n\n", run.TotalErrors)

	// Get error summary
	errSummary, err := db.GetErrorSummary(runID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Error Summary by Operation Type:\n")
	fmt.Printf("%-30s %10s %10s\n", "Operation Type", "Count", "% of Total")
	fmt.Println("--------------------------------------------------------")

	total := int64(0)
	for _, count := range errSummary {
		total += count
	}

	for opType, count := range errSummary {
		percentage := float64(count) / float64(total) * 100
		fmt.Printf("%-30s %10d %9.1f%%\n", opType, count, percentage)
	}

	fmt.Printf("\nTotal Errors: %d\n", total)

	// Calculate error rate
	if run.TotalTransactions > 0 {
		errorRate := float64(run.TotalErrors) / float64(run.TotalTransactions) * 100
		fmt.Printf("Error Rate:   %.2f%%\n", errorRate)
	}
}
