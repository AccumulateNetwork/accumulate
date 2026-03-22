package main

import (
	"fmt"
	"log"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/test/results"
)

// Example: Compare performance between two commits
func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: compare_commits <commit1> <commit2>")
		os.Exit(1)
	}

	commit1 := os.Args[1]
	commit2 := os.Args[2]

	dbPath := os.Getenv("ACCUMULATE_TEST_RESULTS_DB")
	if dbPath == "" {
		dbPath = "./test-results.db"
	}

	db, err := results.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get runs for each commit
	runs1, err := db.GetRunsByCommit(commit1)
	if err != nil || len(runs1) == 0 {
		log.Fatalf("No runs found for commit %s", commit1)
	}

	runs2, err := db.GetRunsByCommit(commit2)
	if err != nil || len(runs2) == 0 {
		log.Fatalf("No runs found for commit %s", commit2)
	}

	// Use the most recent run for each commit
	runID1 := runs1[0].ID
	runID2 := runs2[0].ID

	fmt.Printf("Comparing commits:\n")
	fmt.Printf("  %s (run %d)\n", commit1, runID1)
	fmt.Printf("  %s (run %d)\n\n", commit2, runID2)

	// Compare metrics
	comparisons, err := db.CompareMetrics(runID1, runID2)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Performance Comparison:\n")
	fmt.Printf("%-20s %12s %12s %12s %12s\n", "Metric", "Commit 1", "Commit 2", "Difference", "% Change")
	fmt.Println("---------------------------------------------------------------------------------")

	for _, cmp := range comparisons {
		fmt.Printf("%-20s %12.2f %12.2f %12.2f %11.1f%%\n",
			cmp.Metric,
			cmp.Run1Value,
			cmp.Run2Value,
			cmp.Difference,
			cmp.PercentDiff)
	}

	// Compare error rates
	errors1, err := db.GetErrorSummary(runID1)
	if err != nil {
		log.Fatal(err)
	}

	errors2, err := db.GetErrorSummary(runID2)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nError Summary:\n")
	fmt.Printf("%-20s %12s %12s\n", "Operation Type", "Commit 1", "Commit 2")
	fmt.Println("-------------------------------------------------------")

	allOps := make(map[string]bool)
	for op := range errors1 {
		allOps[op] = true
	}
	for op := range errors2 {
		allOps[op] = true
	}

	for op := range allOps {
		fmt.Printf("%-20s %12d %12d\n", op, errors1[op], errors2[op])
	}
}
