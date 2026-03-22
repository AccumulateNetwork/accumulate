// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/test/testresults"
)

const usage = `testreport - Load test reporting tool

Usage:
  testreport <command> [options]

Commands:
  summary      Generate a summary report for a single test run
  compare      Compare two test runs
  trend        Analyze trend over multiple runs
  import       Import test results into database

Options:
  -db string
        Path to results database (default: /tmp/test-results/results.db)
  -format string
        Output format: markdown, json, html (default: markdown)
  -output string
        Output file path (default: stdout)

Summary Command:
  testreport summary -id <run_id>

Compare Command:
  testreport compare -base <run_id> -compare <run_id>
  testreport compare -commit <hash1> -commit <hash2>

Trend Command:
  testreport trend -metric <metric_name> [-days <n>] [-commit <hash>]

  Metrics: avg_tps, peak_tps, avg_latency, p95_latency, p99_latency, error_rate

Import Command:
  testreport import -data <path> -commit <hash> -branch <branch> [options]

Examples:
  # Generate summary for run ID 5
  testreport summary -id 5

  # Compare two runs
  testreport compare -base 5 -compare 6 -format html -output comparison.html

  # Analyze TPS trend over last 30 days
  testreport trend -metric avg_tps -days 30

  # Import test results
  testreport import -data /tmp/load_tester -commit abc123 -branch main
`

var (
	dbPath     = flag.String("db", "/tmp/test-results/results.db", "Path to results database")
	format     = flag.String("format", "markdown", "Output format: markdown, json, html")
	outputFile = flag.String("output", "", "Output file path (default: stdout)")

	// Summary flags
	runID = flag.Int64("id", 0, "Test run ID")

	// Compare flags
	baseID    = flag.Int64("base", 0, "Base run ID")
	compareID = flag.Int64("compare", 0, "Compare run ID")
	commits   StringSliceFlag

	// Trend flags
	metric = flag.String("metric", "avg_tps", "Metric to analyze")
	days   = flag.Int("days", 30, "Number of days to analyze")

	// Import flags
	dataPath    = flag.String("data", "", "Path to test data files")
	branch      = flag.String("branch", "", "Branch name")
	targetTPS   = flag.Int("tps", 100, "Target TPS")
	concurrency = flag.Int("concurrency", 25, "Concurrency level")
	duration    = flag.Int("duration", 5, "Test duration in seconds")
	notes       = flag.String("notes", "", "Test notes")
)

type StringSliceFlag []string

func (s *StringSliceFlag) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *StringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	flag.Var(&commits, "commit", "Commit hash (can be specified multiple times)")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	command := os.Args[1]
	flag.CommandLine.Parse(os.Args[2:])

	db, err := testresults.NewDatabase(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	var output string

	switch command {
	case "summary":
		output, err = runSummary(db)
	case "compare":
		output, err = runCompare(db)
	case "trend":
		output, err = runTrend(db)
	case "import":
		output, err = runImport(db)
	default:
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := writeOutput(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}
}

func runSummary(db *testresults.Database) (string, error) {
	if *runID == 0 {
		return "", fmt.Errorf("run ID required (-id)")
	}

	run, err := db.GetTestRun(*runID)
	if err != nil {
		return "", fmt.Errorf("getting test run: %w", err)
	}

	return testresults.FormatSingleRunReport(run, testresults.ReportFormat(*format))
}

func runCompare(db *testresults.Database) (string, error) {
	var baseRun, compareRun *testresults.TestRun
	var err error

	// Get base run
	if *baseID != 0 {
		baseRun, err = db.GetTestRun(*baseID)
		if err != nil {
			return "", fmt.Errorf("getting base run: %w", err)
		}
	} else if len(commits) > 0 {
		runs, err := db.GetTestRunsByCommit(commits[0])
		if err != nil || len(runs) == 0 {
			return "", fmt.Errorf("getting runs for base commit: %w", err)
		}
		baseRun = runs[0]
	} else {
		return "", fmt.Errorf("base run required (-base <id> or -commit <hash>)")
	}

	// Get compare run
	if *compareID != 0 {
		compareRun, err = db.GetTestRun(*compareID)
		if err != nil {
			return "", fmt.Errorf("getting compare run: %w", err)
		}
	} else if len(commits) > 1 {
		runs, err := db.GetTestRunsByCommit(commits[1])
		if err != nil || len(runs) == 0 {
			return "", fmt.Errorf("getting runs for compare commit: %w", err)
		}
		compareRun = runs[0]
	} else {
		return "", fmt.Errorf("compare run required (-compare <id> or second -commit <hash>)")
	}

	comparison := testresults.CompareTestRuns(baseRun, compareRun)
	return testresults.FormatComparisonReport(comparison, testresults.ReportFormat(*format))
}

func runTrend(db *testresults.Database) (string, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -*days)

	runs, err := db.GetTestRunsByDateRange(startTime, endTime)
	if err != nil {
		return "", fmt.Errorf("getting test runs: %w", err)
	}

	if len(runs) < 2 {
		return "", fmt.Errorf("need at least 2 runs for trend analysis (found %d)", len(runs))
	}

	// Filter by commit if specified
	if len(commits) > 0 {
		var filtered []*testresults.TestRun
		for _, run := range runs {
			if run.CommitHash == commits[0] {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}

	trend := testresults.AnalyzeTrend(runs, *metric)
	if trend == nil {
		return "", fmt.Errorf("insufficient data for trend analysis")
	}

	return testresults.FormatTrendReport(trend, testresults.ReportFormat(*format))
}

func runImport(db *testresults.Database) (string, error) {
	if *dataPath == "" || len(commits) == 0 || *branch == "" {
		return "", fmt.Errorf("import requires -data, -commit, and -branch")
	}

	// Parse data files (simplified - would need actual implementation)
	run := &testresults.TestRun{
		CommitHash:    commits[0],
		Branch:        *branch,
		StartTime:     time.Now().Add(-time.Duration(*duration) * time.Second),
		EndTime:       time.Now(),
		Duration:      *duration,
		TargetTPS:     *targetTPS,
		Concurrency:   *concurrency,
		ResultsPath:   *dataPath,
		NetworkConfig: "{}",
		TestConfig:    "{}",
	}

	// Try to parse actual data from files
	if err := parseLoadTestData(*dataPath, run); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not parse load test data: %v\n", err)
		fmt.Fprintf(os.Stderr, "Creating stub entry. Update with actual data.\n")
	}

	id, err := db.InsertTestRun(run)
	if err != nil {
		return "", fmt.Errorf("inserting test run: %w", err)
	}

	return fmt.Sprintf("Imported test run with ID: %d\n", id), nil
}

func parseLoadTestData(dataPath string, run *testresults.TestRun) error {
	// Look for settlement data files
	pattern := filepath.Join(dataPath, "load_settlement_*.dat")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no data files found matching %s", pattern)
	}

	// Parse the first file to extract basic metrics
	// This is a simplified parser - would need full implementation
	run.TotalTx = 0
	run.TxPassed = 0
	run.TxFailed = 0
	run.AverageTPS = 0
	run.PeakTPS = 0
	run.MinLatency = 0
	run.MaxLatency = 0
	run.AvgLatency = 0
	run.P50Latency = 0
	run.P95Latency = 0
	run.P99Latency = 0
	run.ErrorRate = 0
	run.NodeCrashes = 0
	run.NodeRestarts = 0

	// TODO: Implement actual parsing of .dat files
	// For now, return error to indicate stub data
	return fmt.Errorf("data parsing not yet implemented")
}

func writeOutput(output string) error {
	if *outputFile == "" {
		fmt.Print(output)
		return nil
	}

	// Ensure output directory exists
	dir := filepath.Dir(*outputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	return os.WriteFile(*outputFile, []byte(output), 0644)
}
