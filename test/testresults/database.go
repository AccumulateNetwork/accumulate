// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testresults

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS test_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	commit_hash TEXT NOT NULL,
	branch TEXT NOT NULL,
	start_time DATETIME NOT NULL,
	end_time DATETIME NOT NULL,
	duration INTEGER NOT NULL,
	target_tps INTEGER NOT NULL,
	concurrency INTEGER NOT NULL,
	network_config TEXT,
	test_config TEXT,
	total_tx INTEGER NOT NULL,
	tx_passed INTEGER NOT NULL,
	tx_failed INTEGER NOT NULL,
	average_tps REAL NOT NULL,
	peak_tps REAL NOT NULL,
	min_latency REAL NOT NULL,
	max_latency REAL NOT NULL,
	avg_latency REAL NOT NULL,
	p50_latency REAL NOT NULL,
	p95_latency REAL NOT NULL,
	p99_latency REAL NOT NULL,
	error_rate REAL NOT NULL,
	node_crashes INTEGER DEFAULT 0,
	node_restarts INTEGER DEFAULT 0,
	notes TEXT,
	results_path TEXT
);

CREATE INDEX IF NOT EXISTS idx_commit_hash ON test_runs(commit_hash);
CREATE INDEX IF NOT EXISTS idx_branch ON test_runs(branch);
CREATE INDEX IF NOT EXISTS idx_start_time ON test_runs(start_time);

CREATE TABLE IF NOT EXISTS operation_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL,
	operation_type TEXT NOT NULL,
	count INTEGER NOT NULL,
	success_count INTEGER NOT NULL,
	error_count INTEGER NOT NULL,
	avg_latency REAL NOT NULL,
	p95_latency REAL NOT NULL,
	p99_latency REAL NOT NULL,
	FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_operation_run_id ON operation_metrics(run_id);

CREATE TABLE IF NOT EXISTS transaction_samples (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL,
	tx_index INTEGER NOT NULL,
	client_id INTEGER NOT NULL,
	sim_time REAL NOT NULL,
	settlement_time REAL NOT NULL,
	timestamp DATETIME NOT NULL,
	success BOOLEAN NOT NULL,
	error_type TEXT,
	FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tx_run_id ON transaction_samples(run_id);

CREATE TABLE IF NOT EXISTS resource_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL,
	timestamp DATETIME NOT NULL,
	node_id TEXT NOT NULL,
	cpu_percent REAL NOT NULL,
	memory_mb REAL NOT NULL,
	disk_read_mbps REAL NOT NULL,
	disk_write_mbps REAL NOT NULL,
	network_in_mbps REAL NOT NULL,
	network_out_mbps REAL NOT NULL,
	FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_resource_run_id ON resource_metrics(run_id);
`

type Database struct {
	db *sql.DB
}

// NewDatabase creates or opens a database at the specified path
func NewDatabase(dbPath string) (*Database, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Create schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// InsertTestRun inserts a new test run and returns its ID
func (d *Database) InsertTestRun(run *TestRun) (int64, error) {
	result, err := d.db.Exec(`
		INSERT INTO test_runs (
			commit_hash, branch, start_time, end_time, duration, target_tps, concurrency,
			network_config, test_config, total_tx, tx_passed, tx_failed,
			average_tps, peak_tps, min_latency, max_latency, avg_latency,
			p50_latency, p95_latency, p99_latency, error_rate,
			node_crashes, node_restarts, notes, results_path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.CommitHash, run.Branch, run.StartTime, run.EndTime, run.Duration,
		run.TargetTPS, run.Concurrency, run.NetworkConfig, run.TestConfig,
		run.TotalTx, run.TxPassed, run.TxFailed, run.AverageTPS, run.PeakTPS,
		run.MinLatency, run.MaxLatency, run.AvgLatency, run.P50Latency,
		run.P95Latency, run.P99Latency, run.ErrorRate, run.NodeCrashes,
		run.NodeRestarts, run.Notes, run.ResultsPath,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting test run: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting inserted ID: %w", err)
	}

	return id, nil
}

// GetTestRun retrieves a test run by ID
func (d *Database) GetTestRun(id int64) (*TestRun, error) {
	run := &TestRun{}
	err := d.db.QueryRow(`
		SELECT id, commit_hash, branch, start_time, end_time, duration, target_tps, concurrency,
			network_config, test_config, total_tx, tx_passed, tx_failed,
			average_tps, peak_tps, min_latency, max_latency, avg_latency,
			p50_latency, p95_latency, p99_latency, error_rate,
			node_crashes, node_restarts, notes, results_path
		FROM test_runs WHERE id = ?
	`, id).Scan(
		&run.ID, &run.CommitHash, &run.Branch, &run.StartTime, &run.EndTime,
		&run.Duration, &run.TargetTPS, &run.Concurrency, &run.NetworkConfig,
		&run.TestConfig, &run.TotalTx, &run.TxPassed, &run.TxFailed,
		&run.AverageTPS, &run.PeakTPS, &run.MinLatency, &run.MaxLatency,
		&run.AvgLatency, &run.P50Latency, &run.P95Latency, &run.P99Latency,
		&run.ErrorRate, &run.NodeCrashes, &run.NodeRestarts, &run.Notes,
		&run.ResultsPath,
	)
	if err != nil {
		return nil, fmt.Errorf("getting test run: %w", err)
	}

	return run, nil
}

// GetTestRunsByCommit retrieves all test runs for a specific commit
func (d *Database) GetTestRunsByCommit(commitHash string) ([]*TestRun, error) {
	rows, err := d.db.Query(`
		SELECT id, commit_hash, branch, start_time, end_time, duration, target_tps, concurrency,
			network_config, test_config, total_tx, tx_passed, tx_failed,
			average_tps, peak_tps, min_latency, max_latency, avg_latency,
			p50_latency, p95_latency, p99_latency, error_rate,
			node_crashes, node_restarts, notes, results_path
		FROM test_runs WHERE commit_hash = ?
		ORDER BY start_time DESC
	`, commitHash)
	if err != nil {
		return nil, fmt.Errorf("querying test runs: %w", err)
	}
	defer rows.Close()

	var runs []*TestRun
	for rows.Next() {
		run := &TestRun{}
		if err := rows.Scan(
			&run.ID, &run.CommitHash, &run.Branch, &run.StartTime, &run.EndTime,
			&run.Duration, &run.TargetTPS, &run.Concurrency, &run.NetworkConfig,
			&run.TestConfig, &run.TotalTx, &run.TxPassed, &run.TxFailed,
			&run.AverageTPS, &run.PeakTPS, &run.MinLatency, &run.MaxLatency,
			&run.AvgLatency, &run.P50Latency, &run.P95Latency, &run.P99Latency,
			&run.ErrorRate, &run.NodeCrashes, &run.NodeRestarts, &run.Notes,
			&run.ResultsPath,
		); err != nil {
			return nil, fmt.Errorf("scanning test run: %w", err)
		}
		runs = append(runs, run)
	}

	return runs, rows.Err()
}

// GetTestRunsByDateRange retrieves test runs within a date range
func (d *Database) GetTestRunsByDateRange(start, end time.Time) ([]*TestRun, error) {
	rows, err := d.db.Query(`
		SELECT id, commit_hash, branch, start_time, end_time, duration, target_tps, concurrency,
			network_config, test_config, total_tx, tx_passed, tx_failed,
			average_tps, peak_tps, min_latency, max_latency, avg_latency,
			p50_latency, p95_latency, p99_latency, error_rate,
			node_crashes, node_restarts, notes, results_path
		FROM test_runs
		WHERE start_time >= ? AND start_time <= ?
		ORDER BY start_time ASC
	`, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying test runs: %w", err)
	}
	defer rows.Close()

	var runs []*TestRun
	for rows.Next() {
		run := &TestRun{}
		if err := rows.Scan(
			&run.ID, &run.CommitHash, &run.Branch, &run.StartTime, &run.EndTime,
			&run.Duration, &run.TargetTPS, &run.Concurrency, &run.NetworkConfig,
			&run.TestConfig, &run.TotalTx, &run.TxPassed, &run.TxFailed,
			&run.AverageTPS, &run.PeakTPS, &run.MinLatency, &run.MaxLatency,
			&run.AvgLatency, &run.P50Latency, &run.P95Latency, &run.P99Latency,
			&run.ErrorRate, &run.NodeCrashes, &run.NodeRestarts, &run.Notes,
			&run.ResultsPath,
		); err != nil {
			return nil, fmt.Errorf("scanning test run: %w", err)
		}
		runs = append(runs, run)
	}

	return runs, rows.Err()
}

// GetTestRunsByConfig retrieves test runs matching specific configuration parameters
func (d *Database) GetTestRunsByConfig(targetTPS, concurrency int) ([]*TestRun, error) {
	rows, err := d.db.Query(`
		SELECT id, commit_hash, branch, start_time, end_time, duration, target_tps, concurrency,
			network_config, test_config, total_tx, tx_passed, tx_failed,
			average_tps, peak_tps, min_latency, max_latency, avg_latency,
			p50_latency, p95_latency, p99_latency, error_rate,
			node_crashes, node_restarts, notes, results_path
		FROM test_runs
		WHERE target_tps = ? AND concurrency = ?
		ORDER BY start_time DESC
	`, targetTPS, concurrency)
	if err != nil {
		return nil, fmt.Errorf("querying test runs: %w", err)
	}
	defer rows.Close()

	var runs []*TestRun
	for rows.Next() {
		run := &TestRun{}
		if err := rows.Scan(
			&run.ID, &run.CommitHash, &run.Branch, &run.StartTime, &run.EndTime,
			&run.Duration, &run.TargetTPS, &run.Concurrency, &run.NetworkConfig,
			&run.TestConfig, &run.TotalTx, &run.TxPassed, &run.TxFailed,
			&run.AverageTPS, &run.PeakTPS, &run.MinLatency, &run.MaxLatency,
			&run.AvgLatency, &run.P50Latency, &run.P95Latency, &run.P99Latency,
			&run.ErrorRate, &run.NodeCrashes, &run.NodeRestarts, &run.Notes,
			&run.ResultsPath,
		); err != nil {
			return nil, fmt.Errorf("scanning test run: %w", err)
		}
		runs = append(runs, run)
	}

	return runs, rows.Err()
}

// InsertTransactionSample inserts a transaction sample
func (d *Database) InsertTransactionSample(sample *TransactionSample) error {
	_, err := d.db.Exec(`
		INSERT INTO transaction_samples (
			run_id, tx_index, client_id, sim_time, settlement_time,
			timestamp, success, error_type
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		sample.RunID, sample.TxIndex, sample.ClientID, sample.SimTime,
		sample.SettlementTime, sample.Timestamp, sample.Success, sample.ErrorType,
	)
	return err
}

// GetTransactionSamples retrieves transaction samples for a test run
func (d *Database) GetTransactionSamples(runID int64) ([]*TransactionSample, error) {
	rows, err := d.db.Query(`
		SELECT run_id, tx_index, client_id, sim_time, settlement_time,
			timestamp, success, error_type
		FROM transaction_samples
		WHERE run_id = ?
		ORDER BY tx_index
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("querying transaction samples: %w", err)
	}
	defer rows.Close()

	var samples []*TransactionSample
	for rows.Next() {
		sample := &TransactionSample{}
		if err := rows.Scan(
			&sample.RunID, &sample.TxIndex, &sample.ClientID, &sample.SimTime,
			&sample.SettlementTime, &sample.Timestamp, &sample.Success, &sample.ErrorType,
		); err != nil {
			return nil, fmt.Errorf("scanning transaction sample: %w", err)
		}
		samples = append(samples, sample)
	}

	return samples, rows.Err()
}

// ToJSON converts a TestRun to JSON
func (r *TestRun) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
