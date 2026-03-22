# Test Report Tool

A CLI tool for analyzing and comparing load test results for the Accumulate network.

## Overview

The `testreport` tool provides comprehensive analysis of load test results, including:
- Single run summaries
- Commit-to-commit comparisons
- Configuration comparisons
- Trend analysis over time
- Automated regression detection

## Installation

```bash
cd test/cmd/testreport
go build
```

## Database

Test results are stored in a SQLite database (default: `/tmp/test-results/results.db`). The database stores:
- Test run metadata (commit, branch, timestamps)
- Performance metrics (TPS, latency, error rates)
- Individual transaction samples
- Resource utilization data

## Commands

### Summary

Generate a detailed report for a single test run:

```bash
testreport summary -id 5
testreport summary -id 5 -format html -output /tmp/test-reports/run5.html
```

### Compare

Compare two test runs to identify regressions or improvements:

```bash
# Compare by run IDs
testreport compare -base 5 -compare 6

# Compare by commit hashes
testreport compare -commit abc123 -commit def456

# Generate HTML comparison report
testreport compare -base 5 -compare 6 -format html -output comparison.html
```

The comparison automatically detects:
- **Regressions**: TPS decreases >10%, latency increases >15%, error rate increases >5%
- **Improvements**: TPS increases >5%, latency decreases >5%

### Trend

Analyze how metrics change over time:

```bash
# Analyze average TPS over last 30 days
testreport trend -metric avg_tps -days 30

# Analyze P95 latency
testreport trend -metric p95_latency -days 14

# Filter by specific commit
testreport trend -metric avg_tps -commit abc123
```

Available metrics:
- `avg_tps` - Average transactions per second
- `peak_tps` - Peak transactions per second
- `avg_latency` - Average latency (ms)
- `p95_latency` - 95th percentile latency (ms)
- `p99_latency` - 99th percentile latency (ms)
- `error_rate` - Error rate percentage

### Import

Import test results into the database:

```bash
testreport import \
  -data /tmp/load_tester \
  -commit abc123def456 \
  -branch feature/dagbft \
  -tps 100 \
  -concurrency 25 \
  -duration 300 \
  -notes "Testing with new consensus algorithm"
```

## Output Formats

### Markdown (default)

Plain text markdown suitable for terminal output or documentation:

```bash
testreport summary -id 5
```

### JSON

Machine-readable format for automation:

```bash
testreport summary -id 5 -format json
```

### HTML

Interactive HTML reports with styling:

```bash
testreport summary -id 5 -format html -output report.html
```

## Example Workflow

1. **Run a load test:**
   ```bash
   cd test/cmd/load
   ./load -s http://localhost:26660/v2 -t 100 -d 300
   ```

2. **Import results:**
   ```bash
   COMMIT=$(git rev-parse HEAD)
   BRANCH=$(git branch --show-current)
   testreport import \
     -data load_tester \
     -commit $COMMIT \
     -branch $BRANCH \
     -tps 100 \
     -duration 300
   ```

3. **Generate reports:**
   ```bash
   # Summary
   testreport summary -id 1 -format html -output /tmp/test-reports/latest.html

   # Compare with previous run
   testreport compare -base 1 -compare 2 -format markdown

   # Analyze trends
   testreport trend -metric avg_tps -days 7
   ```

## Report Types

### Single Run Summary

Provides comprehensive metrics for a single test run:
- Configuration details (TPS target, concurrency, duration)
- Transaction results (total, passed, failed)
- Throughput metrics (avg TPS, peak TPS)
- Latency distribution (min, avg, max, P50, P95, P99)
- Stability metrics (error rate, node crashes/restarts)

### Commit Comparison

Compares two test runs from different commits:
- Side-by-side metric comparison
- Percentage change calculations
- Automated regression detection
- Highlighted improvements

Use cases:
- Before/after performance testing
- Feature impact analysis
- Optimization validation

### Configuration Comparison

Compares runs with different configurations on the same commit:
- Impact of TPS targets
- Concurrency effects
- Network configuration differences

### Trend Analysis

Analyzes metric changes over multiple runs:
- Time-series visualization
- Linear regression trend detection
- Identifies improving/degrading/stable trends
- Historical context for performance changes

## Regression Detection

The tool automatically flags regressions based on thresholds:

| Metric | Threshold |
|--------|-----------|
| Average TPS | >10% decrease |
| Peak TPS | >10% decrease |
| Average Latency | >15% increase |
| P95 Latency | >15% increase |
| P99 Latency | >15% increase |
| Error Rate | >5% increase |
| Node Crashes | Any increase |

## Integration with CI/CD

Example GitLab CI configuration:

```yaml
load_test:
  script:
    - cd test/cmd/load
    - ./load -s http://devnet:26660/v2 -t 100 -d 300
    - cd ../testreport
    - |
      ./testreport import \
        -data ../../load_tester \
        -commit $CI_COMMIT_SHA \
        -branch $CI_COMMIT_REF_NAME \
        -tps 100 \
        -duration 300
    - |
      ./testreport compare \
        -commit $CI_COMMIT_BEFORE_SHA \
        -commit $CI_COMMIT_SHA \
        -format html \
        -output /tmp/test-reports/comparison.html
  artifacts:
    paths:
      - /tmp/test-reports/
    reports:
      junit: test-results.xml
```

## Database Schema

The SQLite database includes the following tables:

- `test_runs` - Main test run records
- `transaction_samples` - Individual transaction data points
- `operation_metrics` - Per-operation type metrics
- `resource_metrics` - Resource utilization over time

See `test/testresults/schema.go` for complete schema definition.

## Development

The reporting tool is organized into packages:

- `test/testresults/schema.go` - Data models
- `test/testresults/database.go` - Database operations
- `test/testresults/analysis.go` - Comparison and trend analysis
- `test/testresults/reports.go` - Report formatting
- `test/cmd/testreport/main.go` - CLI interface

## Future Enhancements

Planned features:
- Automated data parsing from load tester output
- Resource utilization tracking integration
- Per-operation metrics (faucet, send_tokens, etc.)
- Custom regression thresholds
- Report templates
- Email/Slack notifications for regressions
- Dashboard web UI
