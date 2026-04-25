// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package results

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database manages the test results database
type Database struct {
	db       *sql.DB
	filepath string
}

// Config represents test configuration parameters
type Config struct {
	TransactionsPerSecond int                    `json:"transactions_per_second,omitempty"`
	Duration              time.Duration          `json:"duration,omitempty"`
	NumClients            int                    `json:"num_clients,omitempty"`
	TransactionTypes      []string               `json:"transaction_types,omitempty"`
	NetworkTopology       string                 `json:"network_topology,omitempty"`
	NumValidators         int                    `json:"num_validators,omitempty"`
	NumPartitions         int                    `json:"num_partitions,omitempty"`
	Custom                map[string]interface{} `json:"custom,omitempty"`
}

// TestRun represents a test run record
type TestRun struct {
	ID                int64
	CommitHash        string
	Branch            string
	ConfigHash        string
	StartTime         time.Time
	EndTime           *time.Time
	Status            string
	TotalTransactions int64
	TotalErrors       int64
	Notes             string
}

// MetricRecord represents a time-series metric
type MetricRecord struct {
	RunID      int64
	Timestamp  time.Time
	MetricName string
	Value      float64
	Unit       string
	Labels     map[string]string
}

// ErrorRecord represents an error that occurred during testing
type ErrorRecord struct {
	RunID         int64
	Timestamp     time.Time
	OperationType string
	ErrorMessage  string
	ErrorDetails  map[string]interface{}
}

// NodeHealthRecord represents node health status
type NodeHealthRecord struct {
	RunID       int64
	Timestamp   time.Time
	NodeID      string
	PartitionID string
	Status      string
	Resources   map[string]interface{}
}

// Open opens or creates the test results database
func Open(path string) (*Database, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	schemaSQL, err := os.ReadFile(filepath.Join(filepath.Dir(path), "schema.sql"))
	if err != nil {
		// If schema.sql doesn't exist in the same directory, use embedded schema
		schemaSQL = []byte(embeddedSchema)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &Database{
		db:       db,
		filepath: path,
	}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// ComputeConfigHash computes a hash of the configuration
func ComputeConfigHash(config *Config) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// SaveConfig saves a test configuration to the database
func (d *Database) SaveConfig(config *Config) (string, error) {
	hash, err := ComputeConfigHash(config)
	if err != nil {
		return "", fmt.Errorf("failed to compute config hash: %w", err)
	}

	parametersJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	_, err = d.db.Exec(`
		INSERT OR IGNORE INTO test_configs (config_hash, parameters_json)
		VALUES (?, ?)
	`, hash, string(parametersJSON))
	if err != nil {
		return "", fmt.Errorf("failed to save config: %w", err)
	}

	return hash, nil
}

// StartTestRun creates a new test run record
func (d *Database) StartTestRun(commitHash, branch, configHash string, notes string) (int64, error) {
	result, err := d.db.Exec(`
		INSERT INTO test_runs (commit_hash, branch, config_hash, start_time, status, notes)
		VALUES (?, ?, ?, ?, 'running', ?)
	`, commitHash, branch, configHash, time.Now(), notes)
	if err != nil {
		return 0, fmt.Errorf("failed to start test run: %w", err)
	}

	runID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get run ID: %w", err)
	}

	return runID, nil
}

// UpdateTestRun updates a test run's status and end time
func (d *Database) UpdateTestRun(runID int64, status string, totalTxs, totalErrors int64) error {
	now := time.Now()
	_, err := d.db.Exec(`
		UPDATE test_runs
		SET status = ?, end_time = ?, total_transactions = ?, total_errors = ?
		WHERE id = ?
	`, status, now, totalTxs, totalErrors, runID)
	if err != nil {
		return fmt.Errorf("failed to update test run: %w", err)
	}
	return nil
}

// SaveMetric saves a metric data point
func (d *Database) SaveMetric(metric *MetricRecord) error {
	var labelsJSON *string
	if len(metric.Labels) > 0 {
		data, err := json.Marshal(metric.Labels)
		if err != nil {
			return fmt.Errorf("failed to marshal labels: %w", err)
		}
		jsonStr := string(data)
		labelsJSON = &jsonStr
	}

	_, err := d.db.Exec(`
		INSERT INTO metrics_timeseries (run_id, timestamp, metric_name, value, unit, labels_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, metric.RunID, metric.Timestamp, metric.MetricName, metric.Value, metric.Unit, labelsJSON)
	if err != nil {
		return fmt.Errorf("failed to save metric: %w", err)
	}
	return nil
}

// SaveMetricBatch saves multiple metrics in a single transaction
func (d *Database) SaveMetricBatch(metrics []*MetricRecord) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO metrics_timeseries (run_id, timestamp, metric_name, value, unit, labels_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, metric := range metrics {
		var labelsJSON *string
		if len(metric.Labels) > 0 {
			data, err := json.Marshal(metric.Labels)
			if err != nil {
				return fmt.Errorf("failed to marshal labels: %w", err)
			}
			jsonStr := string(data)
			labelsJSON = &jsonStr
		}

		_, err = stmt.Exec(metric.RunID, metric.Timestamp, metric.MetricName, metric.Value, metric.Unit, labelsJSON)
		if err != nil {
			return fmt.Errorf("failed to save metric: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// SaveError saves an error record
func (d *Database) SaveError(errRecord *ErrorRecord) error {
	var detailsJSON *string
	if len(errRecord.ErrorDetails) > 0 {
		data, err := json.Marshal(errRecord.ErrorDetails)
		if err != nil {
			return fmt.Errorf("failed to marshal error details: %w", err)
		}
		jsonStr := string(data)
		detailsJSON = &jsonStr
	}

	_, err := d.db.Exec(`
		INSERT INTO test_errors (run_id, timestamp, operation_type, error_message, error_details_json)
		VALUES (?, ?, ?, ?, ?)
	`, errRecord.RunID, errRecord.Timestamp, errRecord.OperationType, errRecord.ErrorMessage, detailsJSON)
	if err != nil {
		return fmt.Errorf("failed to save error: %w", err)
	}
	return nil
}

// SaveNodeHealth saves a node health record
func (d *Database) SaveNodeHealth(health *NodeHealthRecord) error {
	var resourcesJSON *string
	if len(health.Resources) > 0 {
		data, err := json.Marshal(health.Resources)
		if err != nil {
			return fmt.Errorf("failed to marshal resources: %w", err)
		}
		jsonStr := string(data)
		resourcesJSON = &jsonStr
	}

	_, err := d.db.Exec(`
		INSERT INTO node_health (run_id, timestamp, node_id, partition_id, status, resources_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, health.RunID, health.Timestamp, health.NodeID, health.PartitionID, health.Status, resourcesJSON)
	if err != nil {
		return fmt.Errorf("failed to save node health: %w", err)
	}
	return nil
}

// GetTestRun retrieves a test run by ID
func (d *Database) GetTestRun(runID int64) (*TestRun, error) {
	var run TestRun
	var endTime sql.NullTime

	err := d.db.QueryRow(`
		SELECT id, commit_hash, branch, config_hash, start_time, end_time, status,
		       total_transactions, total_errors, notes
		FROM test_runs
		WHERE id = ?
	`, runID).Scan(&run.ID, &run.CommitHash, &run.Branch, &run.ConfigHash, &run.StartTime,
		&endTime, &run.Status, &run.TotalTransactions, &run.TotalErrors, &run.Notes)
	if err != nil {
		return nil, fmt.Errorf("failed to get test run: %w", err)
	}

	if endTime.Valid {
		run.EndTime = &endTime.Time
	}

	return &run, nil
}

// ListTestRuns lists test runs with optional filters
func (d *Database) ListTestRuns(commitHash, branch string, startDate, endDate *time.Time, limit int) ([]*TestRun, error) {
	query := `
		SELECT id, commit_hash, branch, config_hash, start_time, end_time, status,
		       total_transactions, total_errors, notes
		FROM test_runs
		WHERE 1=1
	`
	args := []interface{}{}

	if commitHash != "" {
		query += " AND commit_hash = ?"
		args = append(args, commitHash)
	}
	if branch != "" {
		query += " AND branch = ?"
		args = append(args, branch)
	}
	if startDate != nil {
		query += " AND start_time >= ?"
		args = append(args, *startDate)
	}
	if endDate != nil {
		query += " AND start_time <= ?"
		args = append(args, *endDate)
	}

	query += " ORDER BY start_time DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list test runs: %w", err)
	}
	defer rows.Close()

	var runs []*TestRun
	for rows.Next() {
		var run TestRun
		var endTime sql.NullTime

		err := rows.Scan(&run.ID, &run.CommitHash, &run.Branch, &run.ConfigHash, &run.StartTime,
			&endTime, &run.Status, &run.TotalTransactions, &run.TotalErrors, &run.Notes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test run: %w", err)
		}

		if endTime.Valid {
			run.EndTime = &endTime.Time
		}

		runs = append(runs, &run)
	}

	return runs, nil
}

// DeleteOldRuns deletes test runs older than the specified retention period
func (d *Database) DeleteOldRuns(retentionDays int) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	result, err := d.db.Exec(`
		DELETE FROM test_runs
		WHERE start_time < ?
	`, cutoffDate)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old runs: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return deleted, nil
}

// Embedded schema as fallback
const embeddedSchema = `-- Embedded schema - see schema.sql for full version
CREATE TABLE IF NOT EXISTS test_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    commit_hash TEXT NOT NULL,
    branch TEXT NOT NULL,
    config_hash TEXT NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    status TEXT NOT NULL CHECK(status IN ('running', 'completed', 'failed', 'cancelled')),
    total_transactions INTEGER DEFAULT 0,
    total_errors INTEGER DEFAULT 0,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS test_configs (
    config_hash TEXT PRIMARY KEY,
    parameters_json TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS metrics_timeseries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT,
    labels_json TEXT,
    FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS test_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    operation_type TEXT,
    error_message TEXT NOT NULL,
    error_details_json TEXT,
    FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS node_health (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    node_id TEXT NOT NULL,
    partition_id TEXT,
    status TEXT NOT NULL CHECK(status IN ('healthy', 'degraded', 'unhealthy', 'stopped')),
    resources_json TEXT,
    FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);`
