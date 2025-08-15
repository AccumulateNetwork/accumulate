# Analyze Tool

## Overview

The `analyze` tool provides comprehensive analysis capabilities for the Accumulate Network. It performs network analysis, transaction analysis, performance profiling, and system diagnostics to help developers and operators understand network behavior and performance characteristics.

## Installation

```bash
# Build the analyze tool
go build -o bin/analyze ./tools/cmd/analyze

# Or build all tools
make tools
```

## Usage

```bash
./bin/analyze [command] [flags]
```

## Commands

### Network Analysis

```bash
# Analyze network topology
./bin/analyze network --topology

# Check network health
./bin/analyze network --health

# Analyze consensus performance
./bin/analyze network --consensus

# Check validator performance
./bin/analyze network --validators
```

### Transaction Analysis

```bash
# Analyze transaction patterns
./bin/analyze transactions --pattern-analysis

# Check transaction throughput
./bin/analyze transactions --throughput

# Analyze transaction fees
./bin/analyze transactions --fee-analysis

# Check transaction success rates
./bin/analyze transactions --success-rates
```

### Performance Analysis

```bash
# System performance overview
./bin/analyze performance --overview

# Memory usage analysis
./bin/analyze performance --memory

# CPU usage patterns
./bin/analyze performance --cpu

# I/O performance analysis
./bin/analyze performance --io
```

### Database Analysis

```bash
# Database size analysis
./bin/analyze database --size-analysis

# Query performance analysis
./bin/analyze database --query-performance

# Index usage analysis
./bin/analyze database --index-usage

# Storage efficiency analysis
./bin/analyze database --storage-efficiency
```

## Configuration

### Environment Variables

```bash
# Set analysis target
export ANALYZE_TARGET=http://localhost:8080

# Set output format
export ANALYZE_OUTPUT_FORMAT=json

# Set analysis depth
export ANALYZE_DEPTH=detailed

# Set time range for analysis
export ANALYZE_TIME_RANGE=24h
```

### Configuration File

Create `analyze.yaml`:

```yaml
target:
  url: "http://localhost:8080"
  timeout: "30s"
  
output:
  format: "json"  # json, yaml, table, csv
  file: "./analysis-report.json"
  verbose: true
  
analysis:
  depth: "detailed"  # basic, detailed, comprehensive
  time_range: "24h"
  include_historical: true
  
network:
  check_connectivity: true
  analyze_consensus: true
  validator_metrics: true
  
performance:
  cpu_profiling: true
  memory_profiling: true
  io_analysis: true
  
database:
  size_analysis: true
  query_performance: true
  index_optimization: true
```

## Analysis Types

### Network Health Analysis

```bash
# Complete network health check
./bin/analyze network --health --comprehensive

# Output includes:
# - Node connectivity status
# - Consensus participation rates
# - Block production metrics
# - Network latency measurements
# - Validator performance scores
```

### Transaction Flow Analysis

```bash
# Analyze transaction processing pipeline
./bin/analyze transactions --flow-analysis --time-range 1h

# Output includes:
# - Transaction submission rates
# - Processing latency distribution
# - Success/failure ratios
# - Fee analysis
# - Bottleneck identification
```

### Performance Profiling

```bash
# System performance profiling
./bin/analyze performance --profile --duration 5m

# Output includes:
# - CPU usage patterns
# - Memory allocation profiles
# - Goroutine analysis
# - I/O wait times
# - Resource utilization trends
```

## Output Formats

### JSON Output

```bash
./bin/analyze network --health --output json
```

```json
{
  "timestamp": "2025-01-17T08:00:00Z",
  "analysis_type": "network_health",
  "summary": {
    "overall_health": "good",
    "score": 85,
    "issues_found": 2
  },
  "details": {
    "connectivity": {
      "nodes_reachable": 15,
      "nodes_total": 16,
      "avg_latency_ms": 45
    },
    "consensus": {
      "participation_rate": 0.95,
      "block_time_avg": "1.2s",
      "missed_blocks": 3
    }
  }
}
```

### Table Output

```bash
./bin/analyze performance --overview --output table
```

```
Performance Analysis Summary
============================
Metric                  | Current | Average | Trend
------------------------|---------|---------|-------
CPU Usage              | 45%     | 42%     | ↑
Memory Usage           | 2.1GB   | 1.9GB   | ↑
Disk I/O               | 150MB/s | 120MB/s | ↑
Network I/O            | 50MB/s  | 45MB/s  | →
Transaction Rate       | 150/s   | 140/s   | ↑
```

### CSV Export

```bash
./bin/analyze transactions --throughput --output csv --file throughput.csv
```

## Integration with Monitoring

### Prometheus Integration

```bash
# Export metrics to Prometheus format
./bin/analyze performance --prometheus --output prometheus.txt

# Continuous monitoring mode
./bin/analyze monitor --prometheus --interval 30s --port 9091
```

### Grafana Dashboard

```bash
# Generate Grafana dashboard JSON
./bin/analyze dashboard --grafana --output accumulate-dashboard.json

# Include custom panels
./bin/analyze dashboard --grafana --panels network,performance,transactions
```

### Alerting Integration

```bash
# Check thresholds and generate alerts
./bin/analyze alerts --config alerts.yaml --output alerts.json

# Webhook integration
./bin/analyze alerts --webhook https://hooks.slack.com/services/...
```

## CI/CD Integration

### Performance Regression Detection

```yaml
performance_analysis:
  stage: analysis
  script:
    - ./bin/analyze performance --baseline baseline.json --current current.json
    - ./bin/analyze regression --threshold 10% --output regression-report.json
  artifacts:
    reports:
      performance: regression-report.json
  only:
    - merge_requests
```

### Automated Health Checks

```yaml
health_check:
  stage: monitor
  script:
    - ./bin/analyze network --health --output json > health-report.json
    - ./bin/analyze validate --health-report health-report.json --min-score 80
  artifacts:
    when: on_failure
    paths:
      - health-report.json
```

## Advanced Analysis

### Custom Analysis Scripts

```bash
# Run custom analysis with script
./bin/analyze custom --script ./custom-analysis.lua

# Batch analysis
./bin/analyze batch --config batch-analysis.yaml
```

### Historical Analysis

```bash
# Analyze trends over time
./bin/analyze trends --time-range 7d --metrics cpu,memory,transactions

# Compare time periods
./bin/analyze compare --period1 "2025-01-10:2025-01-11" --period2 "2025-01-16:2025-01-17"
```

### Predictive Analysis

```bash
# Predict resource usage
./bin/analyze predict --metric memory --horizon 24h

# Capacity planning
./bin/analyze capacity --growth-rate 20% --time-horizon 30d
```

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Connection timeout | Check target URL and network connectivity |
| Insufficient data | Increase time range or check data availability |
| High memory usage | Use streaming mode: `--stream` |
| Slow analysis | Reduce analysis depth: `--depth basic` |

### Debug Mode

```bash
# Enable debug logging
./bin/analyze --debug network --health

# Verbose output
./bin/analyze --verbose performance --overview

# Trace mode for detailed debugging
./bin/analyze --trace transactions --flow-analysis
```

## Examples

### Complete System Analysis

```bash
#!/bin/bash
# comprehensive-analysis.sh

echo "Starting comprehensive system analysis..."

# Network analysis
./bin/analyze network --health --output json > network-health.json

# Performance analysis
./bin/analyze performance --overview --output json > performance.json

# Transaction analysis
./bin/analyze transactions --pattern-analysis --output json > transactions.json

# Database analysis
./bin/analyze database --size-analysis --output json > database.json

# Generate combined report
./bin/analyze report --combine network-health.json,performance.json,transactions.json,database.json --output comprehensive-report.html

echo "Analysis complete. Report: comprehensive-report.html"
```

### Performance Monitoring Script

```bash
#!/bin/bash
# monitor-performance.sh

while true; do
  timestamp=$(date +%Y%m%d_%H%M%S)
  
  # Collect performance metrics
  ./bin/analyze performance --overview --output json > "perf_${timestamp}.json"
  
  # Check for alerts
  ./bin/analyze alerts --config alerts.yaml --input "perf_${timestamp}.json"
  
  sleep 300  # 5 minutes
done
```

## See Also

- [Debug Tool](debug.md) - Debugging utilities and diagnostics
- [Performance Tests](../../test/docs/performance-tests.md) - Performance testing strategies
- [Monitoring Guide](../../docs/monitoring.md) - System monitoring and observability
