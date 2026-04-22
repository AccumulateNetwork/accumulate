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
	"os/exec"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/test/results"
)

// LoadTestRecorder integrates the load generator with the results database
type LoadTestRecorder struct {
	db     *results.Database
	runID  int64
	config *results.Config
}

// NewLoadTestRecorder creates a new recorder for the load test
func NewLoadTestRecorder(tps, duration, numClients int) (*LoadTestRecorder, error) {
	dbPath := os.Getenv("ACCUMULATE_TEST_RESULTS_DB")
	if dbPath == "" {
		dbPath = "./test-results.db"
	}

	db, err := results.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open results database: %w", err)
	}

	config := &results.Config{
		TransactionsPerSecond: tps,
		Duration:              time.Duration(duration) * time.Second,
		NumClients:            numClients,
		TransactionTypes:      []string{"Faucet"},
		Custom: map[string]interface{}{
			"transactions_per_client": maxGoroutines,
		},
	}

	return &LoadTestRecorder{
		db:     db,
		config: config,
	}, nil
}

// StartRun starts a new test run in the database
func (r *LoadTestRecorder) StartRun(notes string) error {
	configHash, err := r.db.SaveConfig(r.config)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	commitHash := getCurrentCommitHash()
	branch := getCurrentBranch()

	runID, err := r.db.StartTestRun(commitHash, branch, configHash, notes)
	if err != nil {
		return fmt.Errorf("failed to start test run: %w", err)
	}

	r.runID = runID
	log.Printf("Started test run %d (commit: %s, branch: %s)", runID, commitHash, branch)
	return nil
}

// RecordMetric records a metric to the database
func (r *LoadTestRecorder) RecordMetric(metricName string, value float64, unit string, labels map[string]string) error {
	metric := &results.MetricRecord{
		RunID:      r.runID,
		Timestamp:  time.Now(),
		MetricName: metricName,
		Value:      value,
		Unit:       unit,
		Labels:     labels,
	}

	return r.db.SaveMetric(metric)
}

// RecordMetricBatch records multiple metrics efficiently
func (r *LoadTestRecorder) RecordMetricBatch(metrics []*results.MetricRecord) error {
	// Set the run ID for all metrics
	for _, m := range metrics {
		m.RunID = r.runID
	}
	return r.db.SaveMetricBatch(metrics)
}

// RecordError records an error to the database
func (r *LoadTestRecorder) RecordError(operationType, errorMessage string, details map[string]interface{}) error {
	errRecord := &results.ErrorRecord{
		RunID:         r.runID,
		Timestamp:     time.Now(),
		OperationType: operationType,
		ErrorMessage:  errorMessage,
		ErrorDetails:  details,
	}

	return r.db.SaveError(errRecord)
}

// CompleteRun marks the test run as completed
func (r *LoadTestRecorder) CompleteRun(status string, totalTxs, totalErrors int64) error {
	err := r.db.UpdateTestRun(r.runID, status, totalTxs, totalErrors)
	if err != nil {
		return fmt.Errorf("failed to complete test run: %w", err)
	}

	log.Printf("Completed test run %d: status=%s, txs=%d, errors=%d", r.runID, status, totalTxs, totalErrors)
	return nil
}

// Close closes the database connection
func (r *LoadTestRecorder) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// GetRunID returns the current run ID
func (r *LoadTestRecorder) GetRunID() int64 {
	return r.runID
}

// Helper functions to get git information
func getCurrentCommitHash() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Warning: failed to get commit hash: %v", err)
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getCurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Warning: failed to get branch: %v", err)
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
