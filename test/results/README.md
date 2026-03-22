# Test Results Database

This package provides a SQLite-based database for storing and querying test run results, metrics, and performance data for the Accumulate DAG-BFT testing framework.

## Overview

The results database stores:
- Test run metadata (commit hash, configuration, timestamps, status)
- Time-series performance metrics (TPS, latency, resource usage)
- Test configuration parameters
- Error records
- Node health data

## Schema

See `schema.sql` for the complete database schema. Key tables:

- **test_runs**: Metadata about each test execution
- **test_configs**: Test configuration parameters (deduplicated by hash)
- **metrics_timeseries**: Time-series performance metrics
- **test_errors**: Errors encountered during test runs
- **node_health**: Node health and resource usage data

## Usage

### Opening a Database

```go
import "gitlab.com/accumulatenetwork/accumulate/test/results"

db, err := results.Open("/path/to/test-results.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

### Recording a Test Run

```go
// Define test configuration
config := &results.Config{
    TransactionsPerSecond: 100,
    Duration:              5 * time.Minute,
    NumClients:            10,
    TransactionTypes:      []string{"SendTokens", "CreateAccount"},
    NetworkTopology:       "3-partition",
    NumValidators:         9,
    NumPartitions:         3,
}

// Save configuration (returns hash)
configHash, err := db.SaveConfig(config)
if err != nil {
    log.Fatal(err)
}

// Start test run
runID, err := db.StartTestRun(commitHash, branch, configHash, "Performance test")
if err != nil {
    log.Fatal(err)
}

// Record metrics during test
metric := &results.MetricRecord{
    RunID:      runID,
    Timestamp:  time.Now(),
    MetricName: "tps",
    Value:      95.5,
    Unit:       "transactions/second",
}
db.SaveMetric(metric)

// Batch save for better performance
metrics := []*results.MetricRecord{...}
db.SaveMetricBatch(metrics)

// Record errors
errRecord := &results.ErrorRecord{
    RunID:         runID,
    Timestamp:     time.Now(),
    OperationType: "SendTokens",
    ErrorMessage:  "insufficient balance",
}
db.SaveError(errRecord)

// Record node health
health := &results.NodeHealthRecord{
    RunID:       runID,
    Timestamp:   time.Now(),
    NodeID:      "validator-0",
    PartitionID: "BVN0",
    Status:      "healthy",
    Resources: map[string]interface{}{
        "cpu_percent":    25.5,
        "memory_percent": 45.2,
    },
}
db.SaveNodeHealth(health)

// Complete test run
db.UpdateTestRun(runID, "completed", totalTxs, totalErrors)
```

### Querying Results

```go
// Get a specific test run
run, err := db.GetTestRun(runID)

// List recent test runs
runs, err := db.ListTestRuns("", "main", nil, nil, 10)

// Get runs for a specific commit
runs, err := db.GetRunsByCommit(commitHash)

// Get runs in a date range
runs, err := db.GetRunsByDateRange(startDate, endDate)

// Get metric statistics
stats, err := db.GetMetricStats(runID, "tps")
fmt.Printf("TPS: avg=%.2f, min=%.2f, max=%.2f\n",
    stats.Avg, stats.Min, stats.Max)

// Get metric time series
timestamps, values, err := db.GetMetricTimeSeries(runID, "latency")

// Compare two test runs
comparisons, err := db.CompareMetrics(runID1, runID2)
for _, cmp := range comparisons {
    fmt.Printf("%s: %.2f -> %.2f (%.1f%% change)\n",
        cmp.Metric, cmp.Run1Value, cmp.Run2Value, cmp.PercentDiff)
}

// Get error summary
errSummary, err := db.GetErrorSummary(runID)
for opType, count := range errSummary {
    fmt.Printf("%s: %d errors\n", opType, count)
}

// Get node health summary
healthSummary, err := db.GetNodeHealthSummary(runID)
```

### Data Retention

```go
// Delete test runs older than 30 days
deleted, err := db.DeleteOldRuns(30)
fmt.Printf("Deleted %d old test runs\n", deleted)
```

## Configuration

The database location can be configured via environment variable:

```bash
export ACCUMULATE_TEST_RESULTS_DB="/path/to/test-results.db"
```

Default location: `./test-results.db`

## Integration with Load Generator

The load generator at `test/cmd/load/` should be modified to record metrics:

```go
// In load generator main.go
db, _ := results.Open(os.Getenv("ACCUMULATE_TEST_RESULTS_DB"))
defer db.Close()

config := &results.Config{
    TransactionsPerSecond: *tps,
    Duration:              time.Duration(*duration) * time.Second,
    NumClients:            *numClients,
}
configHash, _ := db.SaveConfig(config)

commitHash := getCurrentCommitHash()
runID, _ := db.StartTestRun(commitHash, "main", configHash, "Load test")

// During test execution
for settlementTime := range settlementTimes {
    db.SaveMetric(&results.MetricRecord{
        RunID:      runID,
        Timestamp:  time.Now(),
        MetricName: "settlement_time",
        Value:      settlementTime,
        Unit:       "ms",
    })
}

db.UpdateTestRun(runID, "completed", totalTxs, totalErrors)
```

## Integration with Simulator

The simulator can record node health and metrics:

```go
// In simulator monitoring code
for _, node := range partition.Nodes {
    db.SaveNodeHealth(&results.NodeHealthRecord{
        RunID:       runID,
        Timestamp:   time.Now(),
        NodeID:      node.ID,
        PartitionID: partition.ID,
        Status:      node.GetStatus(),
        Resources: map[string]interface{}{
            "memory_mb": node.GetMemoryUsage(),
            "goroutines": runtime.NumGoroutine(),
        },
    })
}
```

## Query Examples

See `cmd/` directory for complete query examples:

- `cmd/compare_commits/`: Compare performance between commits
- `cmd/analyze_errors/`: Analyze error patterns
- `cmd/generate_report/`: Generate performance reports

## Best Practices

1. **Batch metrics**: Use `SaveMetricBatch()` for better performance when saving many metrics
2. **Clean up**: Regularly run `DeleteOldRuns()` to manage database size
3. **Indexes**: The schema includes indexes for common queries
4. **Configuration hashing**: Configurations are deduplicated by hash to save space
5. **Foreign keys**: Deleting a test run cascades to delete all related metrics, errors, and health data

## Schema Versioning

The database schema is versioned. Future versions will include migration scripts in the `migrations/` directory.

Current schema version: 1.0
