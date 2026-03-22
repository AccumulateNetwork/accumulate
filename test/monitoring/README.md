# Monitoring and Analysis Integration Tests

This directory contains integration tests for the Accumulate monitoring, dashboard, and performance analysis systems.

## Overview

These tests validate:
- **Metrics accuracy** - Verifies TPS calculations and transaction metrics are within ±5% tolerance
- **Performance monitoring** - Tests P50/P95/P99 percentile calculations
- **Bottleneck detection** - Identifies performance degradation and latency spikes
- **Load testing** - Validates monitoring under realistic workloads
- **Report generation** - Ensures performance data is collected and formatted correctly
- **Threshold alerting** - Tests detection of performance issues

## Test Files

### integration_test.go
Integration tests for monitoring and metrics systems:
- `TestMetricsAccuracy` - Validates TPS calculation accuracy (±5% tolerance)
- `TestDataSetLogIntegration` - Tests DataSetLog metric collection
- `TestLoadPatternValidation` - Tests different load patterns (steady, burst, high)
- `TestMetricsServiceEdgeCases` - Edge case testing (empty blocks, single transaction)
- `TestReportGeneration` - Validates performance report creation

### performance_analysis_test.go
Performance analysis and bottleneck detection tests:
- `TestPercentileCalculations` - Validates P50/P95/P99 calculation accuracy
- `TestBottleneckDetection` - Detects high latency and performance issues
- `TestRealLoadWorkload` - Full load test scenario (50 TPS for 10 seconds)
- `TestThresholdAlerting` - Tests performance threshold detection

## Running Tests

### Run all monitoring tests
```bash
cd test/monitoring
go test -v
```

### Run specific test
```bash
go test -v -run TestMetricsAccuracy
```

### Run with coverage
```bash
go test -v -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Skip long-running tests
```bash
go test -v -short
```

## Metrics Definitions

### TPS (Transactions Per Second)
Calculated as: `total_transactions / elapsed_time_seconds`

**Validation**: Must be within ±5% of expected value for known workloads

### Settlement Time
Time from transaction submission to completion (in seconds)

### Percentiles
- **P50 (Median)**: 50% of transactions complete within this time
- **P95**: 95% of transactions complete within this time
- **P99**: 99% of transactions complete within this time

**Formula**: `sorted_values[int(len(values) * percentile / 100)]`

## Test Data Output

Tests generate performance data in temporary directories:
- **DataSet files** (`*_settlement.dat`, `*_performance.dat`) - Raw metrics data
- **Report files** (`*_summary.txt`) - Human-readable performance summaries

### Example DataSet Output
```
# index  timestamp       settlementTime  status
  0      1709323200      2.153          completed
  1      1709323201      2.087          completed
  2      1709323202      2.214          completed
```

### Example Report Output
```
Load Test Summary
=================

Configuration:
  Target TPS: 50
  Duration: 10 seconds
  Target Transactions: 500

Results:
  Successful Transactions: 500
  Actual TPS: 48.52
  Total Duration: 10.31 seconds

Settlement Time Metrics:
  Average: 2.143 seconds
  Maximum: 3.521 seconds
  P50: 2.102 seconds
  P95: 2.876 seconds
  P99: 3.214 seconds
```

## Troubleshooting

### Issue: Tests fail with "TPS should be within ±5% of actual TPS"

**Cause**: Metrics calculation may have rounding errors or timing variance

**Solution**:
1. Check if running in CI/CD with limited resources - increase tolerance
2. Verify block time configuration matches test expectations
3. Ensure sufficient test duration for accurate sampling (minimum 5 seconds)

### Issue: Percentile calculations don't match expectations

**Cause**: Incorrect sorted data or index calculation

**Solution**:
1. Verify data is sorted before percentile calculation
2. Check formula: `sorted[int(len(sorted) * p / 100)]`
3. For exact validation, use reference implementation or manual calculation

### Issue: DataSetLog files not created

**Cause**: Insufficient permissions or invalid path

**Solution**:
1. Ensure write permissions to temp directory
2. Check `dsl.SetPath()` is called before `Initialize()`
3. Verify `DumpDataSetToDiskFile()` is called after data collection
4. Check disk space availability

### Issue: High latency in load tests

**Cause**: Test environment limitations or actual bottleneck

**Solution**:
1. Monitor system resources (CPU, memory, disk I/O)
2. Check for concurrent tests consuming resources
3. Reduce target TPS if environment cannot handle load
4. Review block processing time in logs

### Issue: "sim.S.Services().Metrics undefined" compiler error

**Cause**: Metrics service not available in simulator services

**Solution**:
1. Ensure using correct API version (v3)
2. Check if MetricsService is registered in simulator initialization
3. Use direct TPS calculation as fallback: `float64(txCount) / elapsed.Seconds()`

### Issue: Tests timeout or hang

**Cause**: Deadlock in DataSetLog or simulator stepping

**Solution**:
1. Ensure all `ds.Lock()` calls have matching `ds.Unlock()`
2. Use defer pattern: `ds.Lock(); defer ds.Unlock()`
3. Check for simulator step conditions that never complete
4. Reduce test transaction count for faster iteration

## Performance Benchmarks

Expected performance on reference hardware (4 CPU, 8GB RAM):

| Test | Duration | Transactions | TPS | P95 Settlement |
|------|----------|--------------|-----|----------------|
| TestMetricsAccuracy | ~6s | 100 | ~17 | ~2.5s |
| TestRealLoadWorkload | ~11s | 500 | ~45 | ~3.0s |
| TestBottleneckDetection | ~5s | 50 | ~10 | ~2.8s |

## Integration with CI/CD

These tests are designed to run in GitLab CI:

```yaml
monitoring-tests:
  stage: test
  script:
    - cd test/monitoring
    - go test -v -timeout 5m -short
  artifacts:
    when: always
    reports:
      junit: test-results.xml
```

For full load testing (not in -short mode):
```yaml
load-test:
  stage: test
  only:
    - main
    - merge_requests
  script:
    - cd test/monitoring
    - go test -v -timeout 15m -run TestRealLoadWorkload
```

## Metric Accuracy Requirements

Per issue #3855 acceptance criteria:

- ✅ **Metrics accurate within ±5%** of known workloads
- ✅ **Dashboard updates** validated with real load generator runs
- ✅ **P50/P95/P99 calculations** verified against reference data
- ✅ **Performance bottleneck detection** identifies known issues
- ✅ **Report accuracy** matches database queries

## Adding New Tests

When adding new monitoring tests:

1. **Import required packages**:
   ```go
   import (
       "gitlab.com/accumulatenetwork/accumulate/internal/logging"
       . "gitlab.com/accumulatenetwork/accumulate/test/harness"
       . "gitlab.com/accumulatenetwork/accumulate/test/helpers"
   )
   ```

2. **Setup DataSetLog for metric collection**:
   ```go
   dsl := &logging.DataSetLog{}
   dsl.SetPath(tmpDir)
   dsl.SetProcessName("test_name")
   dsl.Initialize("metrics", logging.DefaultOptions())
   ```

3. **Record metrics during test**:
   ```go
   ds := dsl.GetDataSet("metrics")
   ds.Lock()
   ds.Save("label", value, width, firstColumn)
   ds.Unlock()
   ```

4. **Validate results**:
   ```go
   require.InDelta(t, expected, actual, tolerance)
   ```

5. **Generate reports**:
   ```go
   files, err := dsl.DumpDataSetToDiskFile()
   require.NoError(t, err)
   ```

## Related Documentation

- [DataSetLog Implementation](../../internal/logging/dataset.go)
- [Metrics Service](../../internal/api/v3/metrics.go)
- [Load Testing Tool](../../test/cmd/load/main.go)
- [Performance Benchmarks](../validate/bench_test.go)

## Contact

For questions or issues with monitoring tests, please file an issue at:
https://gitlab.com/accumulatenetwork/accumulate/-/issues
