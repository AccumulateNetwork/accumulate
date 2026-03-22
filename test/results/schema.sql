-- Test Results Database Schema
-- This schema stores test run metadata, metrics, and health data for comparison and analysis

-- Test runs table - stores metadata about each test execution
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
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (config_hash) REFERENCES test_configs(config_hash)
);

-- Test configurations table - stores test configuration parameters
CREATE TABLE IF NOT EXISTS test_configs (
    config_hash TEXT PRIMARY KEY,
    parameters_json TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Time-series metrics table - stores performance metrics over time
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

-- Test errors table - stores errors encountered during test runs
CREATE TABLE IF NOT EXISTS test_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    operation_type TEXT,
    error_message TEXT NOT NULL,
    error_details_json TEXT,
    FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

-- Node health table - stores node health and resource usage
CREATE TABLE IF NOT EXISTS node_health (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    node_id TEXT NOT NULL,
    partition_id TEXT,
    status TEXT NOT NULL CHECK(status IN ('healthy', 'degraded', 'unhealthy', 'stopped')),
    resources_json TEXT,
    FOREIGN KEY (run_id) REFERENCES test_runs(id) ON DELETE CASCADE
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_test_runs_commit ON test_runs(commit_hash);
CREATE INDEX IF NOT EXISTS idx_test_runs_branch ON test_runs(branch);
CREATE INDEX IF NOT EXISTS idx_test_runs_config ON test_runs(config_hash);
CREATE INDEX IF NOT EXISTS idx_test_runs_start_time ON test_runs(start_time);
CREATE INDEX IF NOT EXISTS idx_test_runs_status ON test_runs(status);

CREATE INDEX IF NOT EXISTS idx_metrics_run_id ON metrics_timeseries(run_id);
CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics_timeseries(timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_name ON metrics_timeseries(metric_name);
CREATE INDEX IF NOT EXISTS idx_metrics_run_metric ON metrics_timeseries(run_id, metric_name);

CREATE INDEX IF NOT EXISTS idx_errors_run_id ON test_errors(run_id);
CREATE INDEX IF NOT EXISTS idx_errors_timestamp ON test_errors(timestamp);
CREATE INDEX IF NOT EXISTS idx_errors_operation ON test_errors(operation_type);

CREATE INDEX IF NOT EXISTS idx_node_health_run_id ON node_health(run_id);
CREATE INDEX IF NOT EXISTS idx_node_health_timestamp ON node_health(timestamp);
CREATE INDEX IF NOT EXISTS idx_node_health_node_id ON node_health(node_id);
CREATE INDEX IF NOT EXISTS idx_node_health_status ON node_health(status);
