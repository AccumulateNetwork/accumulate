// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testresults

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDatabaseOperations(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	t.Run("InsertAndGetTestRun", func(t *testing.T) {
		run := &TestRun{
			CommitHash:    "abc123def456",
			Branch:        "feature/test",
			StartTime:     time.Now().Add(-5 * time.Minute),
			EndTime:       time.Now(),
			Duration:      300,
			TargetTPS:     100,
			Concurrency:   25,
			NetworkConfig: `{"nodes": 3}`,
			TestConfig:    `{"type": "load"}`,
			TotalTx:       30000,
			TxPassed:      29500,
			TxFailed:      500,
			AverageTPS:    98.5,
			PeakTPS:       120.0,
			MinLatency:    10.0,
			MaxLatency:    500.0,
			AvgLatency:    50.0,
			P50Latency:    45.0,
			P95Latency:    100.0,
			P99Latency:    200.0,
			ErrorRate:     1.67,
			NodeCrashes:   0,
			NodeRestarts:  1,
			Notes:         "Test run with new feature",
			ResultsPath:   "/tmp/test-results/run1",
		}

		id, err := db.InsertTestRun(run)
		if err != nil {
			t.Fatalf("Failed to insert test run: %v", err)
		}

		if id == 0 {
			t.Error("Expected non-zero ID")
		}

		// Retrieve the run
		retrieved, err := db.GetTestRun(id)
		if err != nil {
			t.Fatalf("Failed to get test run: %v", err)
		}

		if retrieved.CommitHash != run.CommitHash {
			t.Errorf("CommitHash mismatch: expected %s, got %s", run.CommitHash, retrieved.CommitHash)
		}

		if retrieved.AverageTPS != run.AverageTPS {
			t.Errorf("AverageTPS mismatch: expected %f, got %f", run.AverageTPS, retrieved.AverageTPS)
		}
	})

	t.Run("GetTestRunsByCommit", func(t *testing.T) {
		commitHash := "test-commit-123"

		// Insert multiple runs with same commit
		for i := 0; i < 3; i++ {
			run := &TestRun{
				CommitHash:  commitHash,
				Branch:      "main",
				StartTime:   time.Now().Add(-time.Duration(i) * time.Hour),
				EndTime:     time.Now(),
				Duration:    300,
				TargetTPS:   100,
				Concurrency: 25,
				TotalTx:     30000,
				TxPassed:    29000,
				TxFailed:    1000,
				AverageTPS:  95.0 + float64(i),
				PeakTPS:     110.0,
				AvgLatency:  50.0,
				P50Latency:  45.0,
				P95Latency:  100.0,
				P99Latency:  200.0,
				ErrorRate:   3.33,
			}

			_, err := db.InsertTestRun(run)
			if err != nil {
				t.Fatalf("Failed to insert test run: %v", err)
			}
		}

		// Retrieve runs by commit
		runs, err := db.GetTestRunsByCommit(commitHash)
		if err != nil {
			t.Fatalf("Failed to get test runs by commit: %v", err)
		}

		if len(runs) != 3 {
			t.Errorf("Expected 3 runs, got %d", len(runs))
		}
	})

	t.Run("GetTestRunsByDateRange", func(t *testing.T) {
		now := time.Now()
		start := now.Add(-24 * time.Hour)
		end := now

		runs, err := db.GetTestRunsByDateRange(start, end)
		if err != nil {
			t.Fatalf("Failed to get test runs by date range: %v", err)
		}

		// Should have at least the runs we inserted
		if len(runs) == 0 {
			t.Error("Expected at least one run in date range")
		}
	})

	t.Run("TransactionSamples", func(t *testing.T) {
		// Insert a test run first
		run := &TestRun{
			CommitHash:  "sample-test",
			Branch:      "main",
			StartTime:   time.Now().Add(-5 * time.Minute),
			EndTime:     time.Now(),
			Duration:    300,
			TargetTPS:   100,
			Concurrency: 25,
			TotalTx:     100,
			TxPassed:    100,
			TxFailed:    0,
			AverageTPS:  100.0,
			PeakTPS:     120.0,
			AvgLatency:  50.0,
			P50Latency:  45.0,
			P95Latency:  100.0,
			P99Latency:  200.0,
			ErrorRate:   0.0,
		}

		runID, err := db.InsertTestRun(run)
		if err != nil {
			t.Fatalf("Failed to insert test run: %v", err)
		}

		// Insert transaction samples
		for i := 0; i < 10; i++ {
			sample := &TransactionSample{
				RunID:          runID,
				TxIndex:        i,
				ClientID:       i % 3,
				SimTime:        float64(i),
				SettlementTime: 0.050 + float64(i)*0.001,
				Timestamp:      time.Now(),
				Success:        true,
				ErrorType:      "",
			}

			if err := db.InsertTransactionSample(sample); err != nil {
				t.Fatalf("Failed to insert transaction sample: %v", err)
			}
		}

		// Retrieve samples
		samples, err := db.GetTransactionSamples(runID)
		if err != nil {
			t.Fatalf("Failed to get transaction samples: %v", err)
		}

		if len(samples) != 10 {
			t.Errorf("Expected 10 samples, got %d", len(samples))
		}
	})
}

func TestDatabaseCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "test.db")

	// Should create parent directory
	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Check that file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}
