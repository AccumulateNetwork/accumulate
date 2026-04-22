// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testresults

import (
	"time"
)

// TestRun represents a single load test execution
type TestRun struct {
	ID              int64     `json:"id"`
	CommitHash      string    `json:"commit_hash"`
	Branch          string    `json:"branch"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Duration        int       `json:"duration"` // Duration in seconds
	TargetTPS       int       `json:"target_tps"`
	Concurrency     int       `json:"concurrency"`
	NetworkConfig   string    `json:"network_config"`   // JSON config
	TestConfig      string    `json:"test_config"`      // JSON config
	TotalTx         int       `json:"total_tx"`
	TxPassed        int       `json:"tx_passed"`
	TxFailed        int       `json:"tx_failed"`
	AverageTPS      float64   `json:"average_tps"`
	PeakTPS         float64   `json:"peak_tps"`
	MinLatency      float64   `json:"min_latency"`      // milliseconds
	MaxLatency      float64   `json:"max_latency"`      // milliseconds
	AvgLatency      float64   `json:"avg_latency"`      // milliseconds
	P50Latency      float64   `json:"p50_latency"`      // milliseconds
	P95Latency      float64   `json:"p95_latency"`      // milliseconds
	P99Latency      float64   `json:"p99_latency"`      // milliseconds
	ErrorRate       float64   `json:"error_rate"`       // percentage
	NodeCrashes     int       `json:"node_crashes"`
	NodeRestarts    int       `json:"node_restarts"`
	Notes           string    `json:"notes"`
	ResultsPath     string    `json:"results_path"`     // Path to raw data files
}

// OperationMetrics represents metrics for a specific operation type
type OperationMetrics struct {
	RunID         int64   `json:"run_id"`
	OperationType string  `json:"operation_type"` // e.g., "faucet", "send_tokens", "create_identity"
	Count         int     `json:"count"`
	SuccessCount  int     `json:"success_count"`
	ErrorCount    int     `json:"error_count"`
	AvgLatency    float64 `json:"avg_latency"`
	P95Latency    float64 `json:"p95_latency"`
	P99Latency    float64 `json:"p99_latency"`
}

// ResourceMetrics represents resource utilization during a test run
type ResourceMetrics struct {
	RunID          int64     `json:"run_id"`
	Timestamp      time.Time `json:"timestamp"`
	NodeID         string    `json:"node_id"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryMB       float64   `json:"memory_mb"`
	DiskReadMBps   float64   `json:"disk_read_mbps"`
	DiskWriteMBps  float64   `json:"disk_write_mbps"`
	NetworkInMbps  float64   `json:"network_in_mbps"`
	NetworkOutMbps float64   `json:"network_out_mbps"`
}

// TransactionSample represents individual transaction data points
type TransactionSample struct {
	RunID          int64     `json:"run_id"`
	TxIndex        int       `json:"tx_index"`
	ClientID       int       `json:"client_id"`
	SimTime        float64   `json:"sim_time"`
	SettlementTime float64   `json:"settlement_time"` // seconds
	Timestamp      time.Time `json:"timestamp"`
	Success        bool      `json:"success"`
	ErrorType      string    `json:"error_type"`
}

// ComparisonResult represents the result of comparing two test runs
type ComparisonResult struct {
	BaseRun       *TestRun           `json:"base_run"`
	CompareRun    *TestRun           `json:"compare_run"`
	Metrics       map[string]Comparison `json:"metrics"`
	Regressions   []string           `json:"regressions"`
	Improvements  []string           `json:"improvements"`
	Summary       string             `json:"summary"`
}

// Comparison represents a single metric comparison
type Comparison struct {
	MetricName  string  `json:"metric_name"`
	BaseValue   float64 `json:"base_value"`
	CompareValue float64 `json:"compare_value"`
	Delta       float64 `json:"delta"`
	PercentChange float64 `json:"percent_change"`
	IsRegression bool   `json:"is_regression"`
}

// TrendAnalysis represents trend data over multiple runs
type TrendAnalysis struct {
	Runs        []*TestRun         `json:"runs"`
	MetricName  string             `json:"metric_name"`
	Values      []float64          `json:"values"`
	Timestamps  []time.Time        `json:"timestamps"`
	Trend       string             `json:"trend"` // "improving", "degrading", "stable"
	Slope       float64            `json:"slope"`
}
