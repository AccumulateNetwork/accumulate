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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/test/testresults"
)

func TestUsage(t *testing.T) {
	// Test that usage contains all expected sections
	if !strings.Contains(usage, "Commands:") {
		t.Error("Usage should contain Commands section")
	}
	if !strings.Contains(usage, "summary") {
		t.Error("Usage should mention summary command")
	}
	if !strings.Contains(usage, "compare") {
		t.Error("Usage should mention compare command")
	}
	if !strings.Contains(usage, "trend") {
		t.Error("Usage should mention trend command")
	}
	if !strings.Contains(usage, "import") {
		t.Error("Usage should mention import command")
	}
}

func TestStringSliceFlag(t *testing.T) {
	var flags StringSliceFlag

	// Test adding values
	if err := flags.Set("value1"); err != nil {
		t.Errorf("Failed to set value: %v", err)
	}
	if err := flags.Set("value2"); err != nil {
		t.Errorf("Failed to set value: %v", err)
	}

	if len(flags) != 2 {
		t.Errorf("Expected 2 values, got %d", len(flags))
	}

	str := flags.String()
	if !strings.Contains(str, "value1") || !strings.Contains(str, "value2") {
		t.Errorf("String() should contain both values, got: %s", str)
	}
}

func TestRunSummaryCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := testresults.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert a test run
	run := &testresults.TestRun{
		CommitHash:    "abc123",
		Branch:        "main",
		StartTime:     time.Now().Add(-5 * time.Minute),
		EndTime:       time.Now(),
		Duration:      300,
		TargetTPS:     100,
		Concurrency:   25,
		TotalTx:       30000,
		TxPassed:      29500,
		TxFailed:      500,
		AverageTPS:    98.5,
		PeakTPS:       120.0,
		AvgLatency:    50.0,
		P50Latency:    45.0,
		P95Latency:    100.0,
		P99Latency:    200.0,
		ErrorRate:     1.67,
		NetworkConfig: "{}",
		TestConfig:    "{}",
	}

	id, err := db.InsertTestRun(run)
	if err != nil {
		t.Fatalf("Failed to insert test run: %v", err)
	}

	// Set flags
	*runID = id

	t.Run("SummaryMarkdown", func(t *testing.T) {
		*format = "markdown"
		output, err := runSummary(db)
		if err != nil {
			t.Fatalf("runSummary failed: %v", err)
		}

		if !strings.Contains(output, "Test Run Report") {
			t.Error("Output should contain report title")
		}
		if !strings.Contains(output, "abc123") {
			t.Error("Output should contain commit hash")
		}
		if !strings.Contains(output, "98.5") {
			t.Error("Output should contain average TPS")
		}
	})

	t.Run("SummaryJSON", func(t *testing.T) {
		*format = "json"
		output, err := runSummary(db)
		if err != nil {
			t.Fatalf("runSummary failed: %v", err)
		}

		if !strings.Contains(output, "commit_hash") {
			t.Error("JSON output should contain commit_hash field")
		}
		if !strings.Contains(output, "abc123") {
			t.Error("JSON output should contain commit hash value")
		}
	})

	t.Run("SummaryHTML", func(t *testing.T) {
		*format = "html"
		output, err := runSummary(db)
		if err != nil {
			t.Fatalf("runSummary failed: %v", err)
		}

		if !strings.Contains(output, "<html>") {
			t.Error("HTML output should contain html tag")
		}
		if !strings.Contains(output, "abc123") {
			t.Error("HTML output should contain commit hash")
		}
	})

	t.Run("SummaryMissingID", func(t *testing.T) {
		*runID = 0
		_, err := runSummary(db)
		if err == nil {
			t.Error("Expected error when run ID is missing")
		}
		if !strings.Contains(err.Error(), "run ID required") {
			t.Errorf("Expected 'run ID required' error, got: %v", err)
		}
		*runID = id // Reset for other tests
	})

	t.Run("SummaryInvalidID", func(t *testing.T) {
		*runID = 99999
		_, err := runSummary(db)
		if err == nil {
			t.Error("Expected error for invalid run ID")
		}
		*runID = id // Reset for other tests
	})
}

func TestRunCompareCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := testresults.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert two test runs
	baseRun := &testresults.TestRun{
		CommitHash:    "base123",
		Branch:        "main",
		StartTime:     time.Now().Add(-10 * time.Minute),
		EndTime:       time.Now().Add(-5 * time.Minute),
		Duration:      300,
		TargetTPS:     100,
		Concurrency:   25,
		TotalTx:       30000,
		TxPassed:      29000,
		TxFailed:      1000,
		AverageTPS:    100.0,
		PeakTPS:       120.0,
		AvgLatency:    50.0,
		P50Latency:    45.0,
		P95Latency:    100.0,
		P99Latency:    200.0,
		ErrorRate:     3.33,
		NetworkConfig: "{}",
		TestConfig:    "{}",
	}

	baseRunID, err := db.InsertTestRun(baseRun)
	if err != nil {
		t.Fatalf("Failed to insert base run: %v", err)
	}

	compareRun := &testresults.TestRun{
		CommitHash:    "compare456",
		Branch:        "main",
		StartTime:     time.Now().Add(-5 * time.Minute),
		EndTime:       time.Now(),
		Duration:      300,
		TargetTPS:     100,
		Concurrency:   25,
		TotalTx:       30000,
		TxPassed:      29500,
		TxFailed:      500,
		AverageTPS:    98.5,
		PeakTPS:       120.0,
		AvgLatency:    50.0,
		P50Latency:    45.0,
		P95Latency:    100.0,
		P99Latency:    200.0,
		ErrorRate:     1.67,
		NetworkConfig: "{}",
		TestConfig:    "{}",
	}

	compareRunID, err := db.InsertTestRun(compareRun)
	if err != nil {
		t.Fatalf("Failed to insert compare run: %v", err)
	}

	t.Run("CompareByID", func(t *testing.T) {
		*baseID = baseRunID
		*compareID = compareRunID
		*format = "markdown"
		commits = nil

		output, err := runCompare(db)
		if err != nil {
			t.Fatalf("runCompare failed: %v", err)
		}

		if !strings.Contains(output, "Comparison Report") {
			t.Error("Output should contain comparison title")
		}
	})

	t.Run("CompareByCommit", func(t *testing.T) {
		*baseID = 0
		*compareID = 0
		commits = StringSliceFlag{"base123", "compare456"}
		*format = "markdown"

		output, err := runCompare(db)
		if err != nil {
			t.Fatalf("runCompare failed: %v", err)
		}

		if !strings.Contains(output, "base123") || !strings.Contains(output, "compare456") {
			t.Error("Output should contain both commit hashes")
		}
	})

	t.Run("CompareMissingBase", func(t *testing.T) {
		*baseID = 0
		*compareID = compareRunID
		commits = nil

		_, err := runCompare(db)
		if err == nil {
			t.Error("Expected error when base run is missing")
		}
		if !strings.Contains(err.Error(), "base run required") {
			t.Errorf("Expected 'base run required' error, got: %v", err)
		}
	})

	t.Run("CompareMissingCompare", func(t *testing.T) {
		*baseID = baseRunID
		*compareID = 0
		commits = nil

		_, err := runCompare(db)
		if err == nil {
			t.Error("Expected error when compare run is missing")
		}
		if !strings.Contains(err.Error(), "compare run required") {
			t.Errorf("Expected 'compare run required' error, got: %v", err)
		}
	})

	t.Run("CompareJSON", func(t *testing.T) {
		*baseID = baseRunID
		*compareID = compareRunID
		*format = "json"
		commits = nil

		output, err := runCompare(db)
		if err != nil {
			t.Fatalf("runCompare failed: %v", err)
		}

		if !strings.Contains(output, "base_run") {
			t.Error("JSON output should contain base_run field")
		}
	})
}

func TestRunTrendCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := testresults.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert multiple test runs over time
	baseTime := time.Now().Add(-7 * 24 * time.Hour)
	for i := 0; i < 5; i++ {
		run := &testresults.TestRun{
			CommitHash:    fmt.Sprintf("trendcommit%08d", i),
			Branch:        "main",
			StartTime:     baseTime.Add(time.Duration(i) * 24 * time.Hour),
			EndTime:       baseTime.Add(time.Duration(i)*24*time.Hour + 5*time.Minute),
			Duration:      300,
			TargetTPS:     100,
			Concurrency:   25,
			TotalTx:       30000,
			TxPassed:      29000,
			TxFailed:      1000,
			AverageTPS:    100.0 + float64(i)*2,
			PeakTPS:       120.0,
			AvgLatency:    50.0 - float64(i)*1,
			P50Latency:    45.0,
			P95Latency:    100.0,
			P99Latency:    200.0,
			ErrorRate:     3.33,
			NetworkConfig: "{}",
			TestConfig:    "{}",
		}

		_, err := db.InsertTestRun(run)
		if err != nil {
			t.Fatalf("Failed to insert test run: %v", err)
		}
	}

	t.Run("TrendAnalysisAverageTPS", func(t *testing.T) {
		*metric = "avg_tps"
		*days = 30
		*format = "markdown"
		commits = nil

		output, err := runTrend(db)
		if err != nil {
			t.Fatalf("runTrend failed: %v", err)
		}

		if !strings.Contains(output, "Trend Analysis") {
			t.Error("Output should contain trend analysis title")
		}
	})

	t.Run("TrendAnalysisJSON", func(t *testing.T) {
		*metric = "avg_tps"
		*days = 30
		*format = "json"
		commits = nil

		output, err := runTrend(db)
		if err != nil {
			t.Fatalf("runTrend failed: %v", err)
		}

		if !strings.Contains(output, "metric") {
			t.Error("JSON output should contain metric field")
		}
	})

	t.Run("TrendInsufficientData", func(t *testing.T) {
		// Use a date range with no data
		*metric = "avg_tps"
		*days = 0 // No data in the last 0 days
		*format = "markdown"
		commits = nil

		_, err := runTrend(db)
		if err == nil {
			t.Error("Expected error for insufficient data")
		}
	})
}

func TestRunImportCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := testresults.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	t.Run("ImportBasic", func(t *testing.T) {
		dataDir := t.TempDir()
		*dataPath = dataDir
		*branch = "test-branch"
		*targetTPS = 100
		*concurrency = 25
		*duration = 300
		commits = StringSliceFlag{"import123"}

		output, err := runImport(db)
		if err != nil {
			t.Fatalf("runImport failed: %v", err)
		}

		if !strings.Contains(output, "Imported test run") {
			t.Error("Output should confirm import")
		}

		// Verify the run was inserted
		runs, err := db.GetTestRunsByCommit("import123")
		if err != nil {
			t.Fatalf("Failed to get imported run: %v", err)
		}
		if len(runs) != 1 {
			t.Errorf("Expected 1 imported run, got %d", len(runs))
		}
	})

	t.Run("ImportMissingData", func(t *testing.T) {
		*dataPath = ""
		commits = StringSliceFlag{"test"}
		*branch = "main"

		_, err := runImport(db)
		if err == nil {
			t.Error("Expected error when data path is missing")
		}
	})

	t.Run("ImportMissingCommit", func(t *testing.T) {
		*dataPath = "/tmp/test"
		commits = nil
		*branch = "main"

		_, err := runImport(db)
		if err == nil {
			t.Error("Expected error when commit is missing")
		}
	})

	t.Run("ImportMissingBranch", func(t *testing.T) {
		*dataPath = "/tmp/test"
		commits = StringSliceFlag{"test"}
		*branch = ""

		_, err := runImport(db)
		if err == nil {
			t.Error("Expected error when branch is missing")
		}
	})
}

func TestWriteOutput(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("WriteToStdout", func(t *testing.T) {
		*outputFile = ""
		// Just verify it doesn't error
		err := writeOutput("test output")
		if err != nil {
			t.Errorf("writeOutput to stdout failed: %v", err)
		}
	})

	t.Run("WriteToFile", func(t *testing.T) {
		outputPath := filepath.Join(tmpDir, "output.txt")
		*outputFile = outputPath

		err := writeOutput("test output")
		if err != nil {
			t.Fatalf("writeOutput to file failed: %v", err)
		}

		// Verify file was created
		data, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		if string(data) != "test output" {
			t.Errorf("File content mismatch: expected 'test output', got '%s'", string(data))
		}
	})

	t.Run("WriteToNestedDirectory", func(t *testing.T) {
		outputPath := filepath.Join(tmpDir, "subdir", "nested", "output.txt")
		*outputFile = outputPath

		err := writeOutput("nested output")
		if err != nil {
			t.Fatalf("writeOutput to nested directory failed: %v", err)
		}

		// Verify file was created
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			t.Error("Output file was not created in nested directory")
		}
	})
}

func TestParseLoadTestData(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("NoDataFiles", func(t *testing.T) {
		run := &testresults.TestRun{}
		err := parseLoadTestData(tmpDir, run)
		if err == nil {
			t.Error("Expected error when no data files exist")
		}
	})

	t.Run("DataFilesExist", func(t *testing.T) {
		// Create a mock data file
		dataFile := filepath.Join(tmpDir, "load_settlement_0.dat")
		if err := os.WriteFile(dataFile, []byte("mock data"), 0644); err != nil {
			t.Fatalf("Failed to create mock data file: %v", err)
		}

		run := &testresults.TestRun{}
		err := parseLoadTestData(tmpDir, run)
		// Currently returns error as parsing not implemented, but should find the file
		if !strings.Contains(err.Error(), "not yet implemented") {
			t.Errorf("Expected 'not yet implemented' error, got: %v", err)
		}
	})
}

// TestMainIntegration tests the main function integration
func TestMainIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Build the test binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "testreport")

	build := exec.Command("go", "build", "-o", binaryPath, ".")
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build testreport: %v\n%s", err, output)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	t.Run("NoArguments", func(t *testing.T) {
		cmd := exec.Command(binaryPath)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Error("Expected error when no arguments provided")
		}
		if !strings.Contains(string(output), "Usage:") {
			t.Error("Should display usage when no arguments provided")
		}
	})

	t.Run("ImportAndSummary", func(t *testing.T) {
		dataDir := t.TempDir()

		// Import a test run
		importCmd := exec.Command(binaryPath,
			"import",
			"-db", dbPath,
			"-data", dataDir,
			"-commit", "integration123",
			"-branch", "main",
			"-tps", "100",
		)
		output, err := importCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Import failed: %v\n%s", err, output)
		}

		if !strings.Contains(string(output), "Imported test run") {
			t.Errorf("Import should confirm success, got: %s", output)
		}

		// Get the summary
		summaryCmd := exec.Command(binaryPath,
			"summary",
			"-db", dbPath,
			"-id", "1",
			"-format", "markdown",
		)
		output, err = summaryCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Summary failed: %v\n%s", err, output)
		}

		if !strings.Contains(string(output), "integration123") {
			t.Errorf("Summary should contain commit hash, got: %s", output)
		}
	})
}

// Benchmark tests
func BenchmarkRunSummary(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, err := testresults.NewDatabase(dbPath)
	if err != nil {
		b.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	run := &testresults.TestRun{
		CommitHash:    "bench123",
		Branch:        "main",
		StartTime:     time.Now().Add(-5 * time.Minute),
		EndTime:       time.Now(),
		Duration:      300,
		TargetTPS:     100,
		Concurrency:   25,
		TotalTx:       30000,
		TxPassed:      29500,
		TxFailed:      500,
		AverageTPS:    98.5,
		PeakTPS:       120.0,
		AvgLatency:    50.0,
		P50Latency:    45.0,
		P95Latency:    100.0,
		P99Latency:    200.0,
		ErrorRate:     1.67,
		NetworkConfig: "{}",
		TestConfig:    "{}",
	}

	id, err := db.InsertTestRun(run)
	if err != nil {
		b.Fatalf("Failed to insert test run: %v", err)
	}

	*runID = id
	*format = "markdown"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := runSummary(db)
		if err != nil {
			b.Fatalf("runSummary failed: %v", err)
		}
	}
}

func init() {
	// Reset flags for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	dbPath = flag.String("db", "/tmp/test-results/results.db", "Path to results database")
	format = flag.String("format", "markdown", "Output format: markdown, json, html")
	outputFile = flag.String("output", "", "Output file path (default: stdout)")
	runID = flag.Int64("id", 0, "Test run ID")
	baseID = flag.Int64("base", 0, "Base run ID")
	compareID = flag.Int64("compare", 0, "Compare run ID")
	metric = flag.String("metric", "avg_tps", "Metric to analyze")
	days = flag.Int("days", 30, "Number of days to analyze")
	dataPath = flag.String("data", "", "Path to test data files")
	branch = flag.String("branch", "", "Branch name")
	targetTPS = flag.Int("tps", 100, "Target TPS")
	concurrency = flag.Int("concurrency", 25, "Concurrency level")
	duration = flag.Int("duration", 5, "Test duration in seconds")
	notes = flag.String("notes", "", "Test notes")
}
