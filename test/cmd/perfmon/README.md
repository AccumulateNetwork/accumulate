# Performance Monitoring and Tuning Framework

This tool provides comprehensive performance monitoring and iterative tuning for Accumulate load testing, specifically designed for Epic #3838 dagbft-integration testing.

## Features

- **Continuous Metrics Collection**: TPS, latency (P50/P95/P99), error rates, resource usage
- **Iterative Load Testing**: Automatically increase load from baseline to maximum
- **Bottleneck Analysis**: AI-driven identification of performance constraints
- **Resource Monitoring**: CPU, memory, disk I/O, and network metrics
- **Automated Reporting**: JSON, CSV, and text reports with visualizations
- **Parameter Tuning**: Recommendations for configuration adjustments
- **Performance Envelope**: Determine maximum sustainable TPS

## Requirements

- Running Accumulate devnet or network
- `accumulated` processes running
- Load test tool (`test/cmd/load`)
- Optional: gnuplot for visualization

## Quick Start

### 1. Start a devnet

```bash
# From project root
cd test/cmd/devnet
go run . start
```

### 2. Run baseline performance test

```bash
cd test/cmd/perfmon
go run . -initial-tps 100 -max-tps 500 -tps-increment 100 -iteration-minutes 3
```

### 3. View results

```bash
cd perfmon_results
cat final_report.txt
```

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `http://127.0.1.1:26660/v2` | Accumulate server URL |
| `-output` | `./perfmon_results` | Output directory for results |
| `-interval` | `5s` | Metrics sampling interval |
| `-duration` | `2m` | Duration per test iteration (deprecated, use -iteration-minutes) |
| `-initial-tps` | `100` | Initial TPS for baseline test |
| `-max-tps` | `1000` | Maximum TPS to test |
| `-tps-increment` | `100` | TPS increment per iteration |
| `-iteration-minutes` | `5` | Minutes to run each TPS level |

## Output Files

### Per-Run Files

- `run_<TPS>_tps_<timestamp>_metrics.json` - Complete metrics data
- `run_<TPS>_tps_<timestamp>_report.txt` - Human-readable summary
- `run_<TPS>_tps_<timestamp>_metrics.csv` - Time-series data
- `load_<TPS>_tps.log` - Load test execution logs

### Aggregate Files

- `final_report.txt` - Comprehensive report across all runs
- `summary.csv` - Summary metrics for all runs
- `plot.gnu` - gnuplot script for visualization

### Visualizations (if gnuplot available)

- `tps_comparison.png` - Target vs achieved TPS
- `error_rate.png` - Error rates by load
- `latency.png` - Latency percentiles
- `cpu_usage.png` - CPU usage patterns
- `memory_usage.png` - Memory usage patterns

## Metrics Collected

### Transaction Metrics
- **TPS**: Transactions per second (actual achieved)
- **Target TPS**: Configured load level
- **Successful Txs**: Count of successful transactions
- **Failed Txs**: Count of failed transactions
- **Error Rate**: Percentage of failed transactions

### Latency Metrics
- **P50**: 50th percentile latency (median)
- **P95**: 95th percentile latency
- **P99**: 99th percentile latency
- **Average**: Mean latency

### Consensus Metrics
- **Block Height**: Current blockchain height
- **Block Production Rate**: Blocks per second
- **Anchor Transaction Delays**: (tracked in block data)

### Resource Metrics
- **CPU Usage**: Process CPU percentage
- **Memory Usage**: Process memory in MB
- **Disk I/O**: Read/write throughput in MB/s
- **Network I/O**: RX/TX throughput in MB/s

## Bottleneck Analysis

The tool automatically identifies bottlenecks based on:

1. **Target TPS Achievement**: Warns if < 90% of target achieved
2. **Error Rate**: Alerts if > 5% transaction failures
3. **CPU Saturation**: Flags if > 90% CPU usage
4. **High Latency**: Notes if P99 > 5000ms
5. **Memory Growth**: Detects > 50% memory increase during test
6. **Block Production**: Flags if < 0.5 blocks/second

## Recommendations

The AI-driven recommendation engine suggests:

- **Horizontal Scaling**: When CPU saturated
- **Configuration Tuning**: For latency or throughput issues
- **Resource Optimization**: When memory or I/O constrained
- **Network Improvements**: For consensus delays
- **Load Increase**: When performance is healthy

## Example Workflow

### Baseline Test (100 TPS)

```bash
go run . -initial-tps 100 -max-tps 100 -iteration-minutes 5 -output ./baseline
```

This establishes baseline performance metrics.

### Iterative Tuning

```bash
# Test 100-500 TPS in 100 TPS increments, 5 minutes each
go run . -initial-tps 100 -max-tps 500 -tps-increment 100 -iteration-minutes 5
```

The tool will:
1. Run at 100 TPS for 5 minutes
2. Collect and analyze metrics
3. Generate bottleneck analysis
4. If healthy, increase to 200 TPS
5. Repeat until max TPS or performance degradation

### Finding Maximum Sustainable TPS

```bash
# Test up to 2000 TPS
go run . -initial-tps 100 -max-tps 2000 -tps-increment 200 -iteration-minutes 10
```

The tool automatically stops if:
- Error rate exceeds 20%
- Achieved TPS < 50% of target
- CPU constantly > 95%

## Integration with dagbft-integration

### Testing dagbft vs Previous Consensus

1. Baseline test with previous consensus:
```bash
git checkout <previous-consensus-branch>
# Start devnet
go run test/cmd/perfmon -output ./results_old
```

2. Test with dagbft-integration:
```bash
git checkout dagbft-integration
# Start devnet
go run test/cmd/perfmon -output ./results_dagbft
```

3. Compare results:
```bash
diff -u results_old/final_report.txt results_dagbft/final_report.txt
```

### CI/CD Integration

Add to `.gitlab-ci.yml`:

```yaml
performance-test:
  stage: test
  script:
    - go build -o accumulated ./cmd/accumulated
    - go run test/cmd/devnet start &
    - sleep 30
    - go run test/cmd/perfmon -initial-tps 100 -max-tps 500 -iteration-minutes 3
  artifacts:
    paths:
      - perfmon_results/
    reports:
      junit: perfmon_results/*_metrics.json
```

## Architecture

### Components

1. **Monitor**: Main coordinator
   - Manages test iterations
   - Collects metrics
   - Analyzes results

2. **Metrics Collection**
   - API v2/v3 integration
   - System resource monitoring
   - Block and transaction tracking

3. **Analysis Engine**
   - Bottleneck identification
   - Trend analysis
   - Recommendation generation

4. **Reporting**
   - JSON for machine processing
   - CSV for spreadsheet analysis
   - Text for human reading
   - Visualization scripts

### Data Flow

```
Load Test → API Calls → Accumulate Network
                            ↓
Monitor ← API Queries ← Metrics Service
   ↓
System Metrics (CPU, Memory, I/O)
   ↓
Analysis Engine → Bottlenecks + Recommendations
   ↓
Reports (JSON, CSV, Text, Plots)
```

## Troubleshooting

### "Failed to create API client"
- Ensure devnet is running
- Check URL is correct
- Verify network connectivity

### "Load test tool not found"
```bash
# Build the load tool
cd test/cmd/load
go build -o load
```

### "No metrics collected"
- Increase sampling interval (`-interval 10s`)
- Check accumulated processes are running
- Verify API endpoints are accessible

### High variance in results
- Increase iteration duration (`-iteration-minutes 10`)
- Ensure system is not under other load
- Run multiple iterations and average

## Performance Baselines

### Expected Performance (Reference)

| Configuration | Target TPS | Expected Achievement | Notes |
|--------------|-----------|---------------------|-------|
| 1 BVN, 1 Val | 100 | > 95% | Baseline |
| 1 BVN, 2 Val | 200 | > 90% | Light consensus overhead |
| 2 BVN, 2 Val | 500 | > 85% | Cross-partition traffic |
| 2 BVN, 4 Val | 1000 | > 80% | Full load test |

Actual results will vary based on hardware, network, and configuration.

## Contributing

When adding new metrics:

1. Update `PerformanceMetrics` struct
2. Add collection in `gatherMetrics()`
3. Include in summary calculations
4. Add to CSV export
5. Update bottleneck analysis if relevant

## References

- Epic #3838: dagbft-integration testing framework
- Issue #3844: Performance monitoring and iterative tuning
- [Load Testing Documentation](../load/README.md)
- [Devnet Setup Guide](../devnet/README.md)
