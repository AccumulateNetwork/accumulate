// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package results provides a SQLite-based database for storing and querying
// test run results, metrics, and performance data for the Accumulate DAG-BFT
// testing framework.
//
// The package supports:
//   - Test run metadata (commit hash, configuration, timestamps)
//   - Time-series performance metrics (TPS, latency, resource usage)
//   - Test configuration parameters (deduplicated by hash)
//   - Error records
//   - Node health data
//
// Basic usage:
//
//	db, err := results.Open("./test-results.db")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	config := &results.Config{
//	    TransactionsPerSecond: 100,
//	    Duration:              5 * time.Minute,
//	    NumClients:            10,
//	}
//	configHash, _ := db.SaveConfig(config)
//	runID, _ := db.StartTestRun(commitHash, branch, configHash, "Test run")
//
//	// Record metrics
//	db.SaveMetric(&results.MetricRecord{
//	    RunID:      runID,
//	    Timestamp:  time.Now(),
//	    MetricName: "tps",
//	    Value:      95.5,
//	    Unit:       "transactions/second",
//	})
//
//	// Complete run
//	db.UpdateTestRun(runID, "completed", totalTxs, totalErrors)
//
// For detailed usage examples, see README.md and USAGE.md.
package results
