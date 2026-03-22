# Test Results Database - Usage Guide

## Quick Start

### 1. Database Setup

The database is automatically created when you first use it:

```go
import "gitlab.com/accumulatenetwork/accumulate/test/results"

db, err := results.Open("./test-results.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

Set the database location via environment variable:
```bash
export ACCUMULATE_TEST_RESULTS_DB="/path/to/test-results.db"
```

### 2. Recording a Test Run

```go
// Define configuration
config := &results.Config{
    TransactionsPerSecond: 100,
    Duration:              5 * time.Minute,
    NumClients:            10,
    NetworkTopology:       "3-partition",
    NumValidators:         9,
}

// Save config and start run
configHash, _ := db.SaveConfig(config)
runID, _ := db.StartTestRun(commitHash, branch, configHash, "Load test")

// Record metrics during test
db.SaveMetric(&results.MetricRecord{
    RunID:      runID,
    Timestamp:  time.Now(),
    MetricName: "tps",
    Value:      95.5,
    Unit:       "transactions/second",
})

// Complete the run
db.UpdateTestRun(runID, "completed", totalTxs, totalErrors)
```

### 3. Querying Results

```go
// Get test run details
run, _ := db.GetTestRun(runID)

// List recent runs
runs, _ := db.ListTestRuns("", "main", nil, nil, 10)

// Get metric statistics
stats, _ := db.GetMetricStats(runID, "tps")
fmt.Printf("TPS: %.2f ± %.2f (min: %.2f, max: %.2f)\n",
    stats.Avg, stats.StdDev, stats.Min, stats.Max)

// Compare two runs
comparisons, _ := db.CompareMetrics(runID1, runID2)
for _, cmp := range comparisons {
    fmt.Printf("%s: %.1f%% change\n", cmp.Metric, cmp.PercentDiff)
}
```

## Integration Examples

### Load Generator Integration

```go
// In test/cmd/load/main.go
func main() {
    // Create recorder
    recorder, err := NewLoadTestRecorder(*tps, *duration, numClients)
    if err != nil {
        log.Fatal(err)
    }
    defer recorder.Close()

    // Start run
    recorder.StartRun("Load test run")

    // During test execution
    for _, settlementTime := range settlementTimes {
        recorder.RecordMetric("settlement_time", settlementTime, "seconds", nil)
    }

    // Record errors
    if err != nil {
        recorder.RecordError("Faucet", err.Error(), map[string]interface{}{
            "account": accountURL,
        })
    }

    // Complete run
    recorder.CompleteRun("completed", totalTxs, totalErrors)
}
```

### Node Health Monitoring

```go
// Create monitor
monitor := results.NewMonitor(db, runID, "validator-0", "BVN0", 10*time.Second)

// Create collector
collector := results.NewSimpleCollector("validator-0", "BVN0", func() string {
    if node.IsRunning() {
        return "healthy"
    }
    return "stopped"
})

// Start monitoring
monitor.Start(collector)

// ... run tests ...

// Stop monitoring
monitor.Stop()
```

### Batch Recording for Performance

```go
// Batch metrics
metrics := make([]*results.MetricRecord, 0, 1000)
for i := 0; i < 1000; i++ {
    metrics = append(metrics, &results.MetricRecord{
        RunID:      runID,
        Timestamp:  time.Now(),
        MetricName: "tps",
        Value:      float64(i),
    })
}
db.SaveMetricBatch(metrics)

// Batch health records
batchRecorder := results.NewBatchHealthRecorder(db, runID)
for _, node := range nodes {
    batchRecorder.Add(node.ID, node.Partition, "healthy",
        results.GetRuntimeResources())
}
batchRecorder.Flush()
```

## Common Query Patterns

### Find Performance Regressions

```go
// Get runs from last week
endDate := time.Now()
startDate := endDate.AddDate(0, 0, -7)
runs, _ := db.GetRunsByDateRange(startDate, endDate)

// Compare each consecutive pair
for i := 0; i < len(runs)-1; i++ {
    comparisons, _ := db.CompareMetrics(runs[i].ID, runs[i+1].ID)
    for _, cmp := range comparisons {
        if cmp.PercentDiff < -10 { // 10% regression
            fmt.Printf("REGRESSION in %s: %.1f%%\n", cmp.Metric, cmp.PercentDiff)
        }
    }
}
```

### Analyze Error Patterns

```go
run, _ := db.GetTestRun(runID)
errSummary, _ := db.GetErrorSummary(runID)

for opType, count := range errSummary {
    rate := float64(count) / float64(run.TotalTransactions) * 100
    fmt.Printf("%s: %d errors (%.2f%%)\n", opType, count, rate)
}
```

### Track Node Health

```go
healthSummary, _ := db.GetNodeHealthSummary(runID)

for nodeID, statuses := range healthSummary {
    total := int64(0)
    for _, count := range statuses {
        total += count
    }

    healthy := statuses["healthy"]
    healthyPercent := float64(healthy) / float64(total) * 100

    if healthyPercent < 95 {
        fmt.Printf("WARNING: Node %s only %.1f%% healthy\n", nodeID, healthyPercent)
    }
}
```

### Time Series Analysis

```go
timestamps, values, _ := db.GetMetricTimeSeries(runID, "tps")

// Calculate moving average
windowSize := 10
for i := windowSize; i < len(values); i++ {
    sum := 0.0
    for j := i - windowSize; j < i; j++ {
        sum += values[j]
    }
    avg := sum / float64(windowSize)
    fmt.Printf("%s: %.2f\n", timestamps[i].Format(time.RFC3339), avg)
}
```

## Data Retention

Configure automatic cleanup:

```bash
# Delete runs older than 30 days
deleted, _ := db.DeleteOldRuns(30)
fmt.Printf("Deleted %d old test runs\n", deleted)
```

Add to cron:
```bash
# Daily cleanup at 2am
0 2 * * * /path/to/cleanup-script
```

## Command-Line Tools

### Compare Commits
```bash
go run gitlab.com/accumulatenetwork/accumulate/test/results/cmd/compare_commits abc123 def456
```

### Analyze Errors
```bash
go run gitlab.com/accumulatenetwork/accumulate/test/results/cmd/analyze_errors 42
```

### Generate Report
```bash
go run gitlab.com/accumulatenetwork/accumulate/test/results/cmd/generate_report 42
```

## Environment Variables

- `ACCUMULATE_TEST_RESULTS_DB`: Database file path (default: `./test-results.db`)
- `ACCUMULATE_TEST_RETENTION_DAYS`: Retention period in days (default: 30)

## Best Practices

1. **Use batch operations** for saving multiple metrics/records
2. **Set appropriate retention** to manage database size
3. **Add labels to metrics** for better filtering and analysis
4. **Record configurations** to ensure reproducible tests
5. **Monitor database size** and vacuum periodically
6. **Backup regularly** for important test data

## Troubleshooting

### Database is locked
- Ensure only one process writes at a time
- Use batch operations to reduce lock contention

### Large database size
- Run `DeleteOldRuns()` regularly
- Vacuum the database: `VACUUM`
- Consider archiving old data

### Slow queries
- Ensure indexes are created (automatic on first open)
- Use appropriate filters in queries
- Consider aggregating old data
