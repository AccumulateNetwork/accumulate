# Performance Monitoring and Tuning Guide

## Overview

The `perfmon` tool provides comprehensive performance monitoring and load testing capabilities for Accumulate networks. It collects detailed metrics including throughput (TPS), latency, error rates, resource usage, and block production, then generates actionable tuning recommendations.

This guide covers the complete workflow from baseline testing through iterative tuning to achieve optimal network performance.

## Quick Start

### Basic Performance Test

```bash
# Run a 100 TPS test for 5 minutes
debug perfmon localhost:16695 100 5m

# With custom output directory
debug perfmon localhost:16695 100 5m --output ./my-results

# With specific address
debug perfmon localhost:16695 100 5m acc://1234567890abcdef/ACME
```

### Analyze Results

```bash
# Analyze the performance report
debug perfmon-tuner ./perfmon-results/report_20260322_143000.json

# Save recommendations to file
debug perfmon-tuner ./perfmon-results/report_20260322_143000.json --output recommendations.json

# Compare with baseline
debug perfmon-tuner ./perfmon-results/report_current.json --baseline ./perfmon-results/report_baseline.json
```

## Command Reference

### perfmon

Performance monitoring and load testing with detailed metrics collection.

**Syntax:**
```bash
debug perfmon [server] [tps] [duration] [address]
```

**Arguments:**
- `server`: Server endpoint (e.g., `localhost:16695`, `https://testnet.accumulatenetwork.io`)
- `tps`: Target transactions per second (integer)
- `duration`: Test duration (e.g., `5m`, `1h`, `30s`)
- `address`: (Optional) Lite account address to use for testing

**Flags:**
- `--output DIR`: Output directory for metrics and reports (default: `./perfmon-results`)
- `--interval DURATION`: Reporting interval for metrics (default: `10s`)
- `--metrics`: Enable detailed metrics collection (default: `true`)
- `--cpuprofile`: Enable CPU profiling (default: `false`)
- `--memprofile`: Enable memory profiling (default: `false`)

**Examples:**
```bash
# Basic test
debug perfmon localhost:16695 100 5m

# High-throughput test with frequent reporting
debug perfmon localhost:16695 1000 10m --interval 5s

# Extended test with profiling
debug perfmon localhost:16695 500 1h --cpuprofile --memprofile
```

### perfmon-tuner

AI-driven analysis of performance reports with tuning recommendations.

**Syntax:**
```bash
debug perfmon-tuner [report-file]
```

**Arguments:**
- `report-file`: Path to JSON performance report

**Flags:**
- `--output FILE`: Save recommendations as JSON
- `--threshold FLOAT`: TPS achievement threshold (0.0-1.0, default: 0.8)
- `--baseline FILE`: Baseline report for comparison
- `--verbose, -v`: Verbose output with additional details

**Examples:**
```bash
# Basic analysis
debug perfmon-tuner ./perfmon-results/report_20260322_143000.json

# Save recommendations
debug perfmon-tuner report.json --output tuning.json

# Compare with baseline
debug perfmon-tuner current.json --baseline baseline.json -v

# Strict threshold
debug perfmon-tuner report.json --threshold 0.95
```

## Metrics Collected

### Throughput Metrics

- **Target TPS**: Requested transaction rate
- **Achieved TPS**: Actual sustained transaction rate
- **Submitted Total**: Total transactions submitted
- **Success Total**: Successfully submitted transactions
- **Error Total**: Failed transactions
- **Success Rate**: Percentage of successful transactions

### Latency Metrics

All latency measurements in milliseconds:

- **P50 (Median)**: 50th percentile latency
- **P95**: 95th percentile latency
- **P99**: 99th percentile latency
- **Mean**: Average latency
- **Min**: Minimum observed latency
- **Max**: Maximum observed latency

### Error Metrics

- **Errors by Type**: Breakdown of errors by category
  - `build`: Transaction building errors
  - `normalize`: Message normalization errors
  - `submit`: Submission errors
  - `tx_error`: Transaction execution errors

### Resource Metrics

- **Average CPU**: Mean CPU usage during test
- **Max CPU**: Peak CPU usage
- **Average Memory**: Mean memory consumption
- **Max Memory**: Peak memory usage

### Block Metrics

- **Block Count**: Total blocks produced
- **Average Block Interval**: Mean time between blocks (ms)
- **Block Rate**: Blocks produced per minute

## Output Files

### JSON Report

Location: `{output-dir}/report_{timestamp}.json`

Complete performance metrics in JSON format. Includes all collected data points for programmatic analysis.

**Structure:**
```json
{
  "start_time": "2026-03-22T14:30:00Z",
  "end_time": "2026-03-22T14:35:00Z",
  "duration_seconds": 300.0,
  "target_tps": 100,
  "achieved_tps": 95.3,
  "submitted_total": 28590,
  "success_total": 28450,
  "error_total": 140,
  "success_rate_percent": 99.51,
  "latency_p50_ms": 125.3,
  "latency_p95_ms": 380.2,
  "latency_p99_ms": 520.8,
  "latency_mean_ms": 145.6,
  "latency_min_ms": 45.2,
  "latency_max_ms": 1250.4,
  "errors_by_type": {
    "tx_error": 140
  },
  "avg_cpu_percent": 45.2,
  "max_cpu_percent": 78.3,
  "avg_memory_bytes": 1073741824,
  "max_memory_bytes": 2147483648,
  "block_count": 150,
  "avg_block_interval_ms": 2000.0,
  "blocks_per_minute": 30.0
}
```

### CSV Metrics

Location: `{output-dir}/metrics_{timestamp}.csv`

Time-series data for graphing and trend analysis.

**Format:**
```csv
timestamp,submitted,success,errors
1711116000,28590,28450,140
```

### Tuning Recommendations

Location: Specified by `--output` flag in `perfmon-tuner`

AI-generated recommendations based on performance analysis.

**Structure:**
```json
{
  "report_file": "report.json",
  "analysis_time": "12345",
  "summary": {
    "tps_achievement_percent": 95.3,
    "success_rate_percent": 99.51,
    "error_rate_percent": 0.49,
    "avg_latency_ms": 145.6,
    "overall_health": "HEALTHY",
    "critical_issues": 0,
    "warning_issues": 0
  },
  "bottlenecks": [],
  "recommendations": [
    {
      "category": "optimization",
      "parameter": "general",
      "suggested_value": "fine_tune",
      "reason": "System is performing well, consider incremental optimizations",
      "expected_impact": "Marginal improvements in efficiency",
      "priority": "LOW"
    }
  ]
}
```

## Iterative Tuning Workflow

### Phase 1: Baseline Measurement

Establish baseline performance before making any changes.

```bash
# 1. Start with conservative settings
# Run baseline test
debug perfmon localhost:16695 100 10m --output ./baseline

# 2. Analyze baseline
debug perfmon-tuner ./baseline/report_*.json --output ./baseline/recommendations.json

# 3. Save baseline for comparison
cp ./baseline/report_*.json ./baseline/baseline_reference.json
```

### Phase 2: Identify Bottlenecks

The tuner automatically identifies performance bottlenecks:

**Throughput Bottleneck:**
- TPS achievement < 80% of target
- Recommendations: Increase block size, reduce timeouts, expand mempool

**Latency Bottleneck:**
- P99 latency > 2000ms (warning) or > 5000ms (critical)
- Recommendations: Tune database, optimize network, adjust consensus

**Error Rate Bottleneck:**
- Success rate < 95% (warning) or < 90% (critical)
- Recommendations: Review validation, increase timeouts, tune network

**Block Production Bottleneck:**
- Block rate < 6 blocks/minute
- Recommendations: Reduce TimeoutCommit, adjust consensus parameters

### Phase 3: Apply Tuning

Implement recommendations from highest to lowest priority.

#### Consensus Parameters

Edit your network configuration:

```yaml
# For high throughput
consensus:
  timeout-propose: 1s          # Reduce from default 3s
  timeout-commit: 500ms        # Reduce from default 1s
  timeout-prevote: 2s          # Increase for reliability
  create-empty-blocks: true    # Maintain consistent timing
```

#### Block Configuration

```yaml
# Increase block capacity
blocks:
  max-bytes: 22020096         # 21MB (increase from default)
  max-gas: -1                 # Remove gas limit
```

#### Mempool Settings

```yaml
# Buffer more transactions
mempool:
  size: 10000                 # Increase capacity
  max-txs-bytes: 104857600    # 100MB total
  cache-size: 20000           # Larger cache
```

#### Database Tuning

```yaml
# Badger database optimization
database:
  num-memtables: 3            # More in-memory tables
  num-level-zero-tables: 3    # Allow more L0 tables
  compaction-strategy: leveled # Consistent performance
```

#### Network Configuration

```yaml
# Higher bandwidth limits
network:
  send-rate: 52428800         # 50MB/s
  recv-rate: 52428800         # 50MB/s
  max-packet-msg-payload-size: 10240  # 10KB packets
```

### Phase 4: Test and Compare

After applying changes, run another test and compare.

```bash
# 1. Run new test with same parameters
debug perfmon localhost:16695 100 10m --output ./iteration-1

# 2. Compare with baseline
debug perfmon-tuner ./iteration-1/report_*.json \
  --baseline ./baseline/baseline_reference.json \
  --output ./iteration-1/analysis.json

# 3. Review comparison
cat ./iteration-1/analysis.json | jq '.baseline_comparison'
```

**Interpreting Results:**

- **IMPROVEMENT**: Metrics improved, continue this direction
- **REGRESSION**: Metrics worsened, revert changes
- **NEUTRAL**: No significant change, try different parameters

### Phase 5: Iterate

Repeat the cycle until target performance is achieved.

```bash
# Continue iterating
debug perfmon localhost:16695 100 10m --output ./iteration-2
debug perfmon-tuner ./iteration-2/report_*.json \
  --baseline ./iteration-1/report_*.json

debug perfmon localhost:16695 100 10m --output ./iteration-3
debug perfmon-tuner ./iteration-3/report_*.json \
  --baseline ./iteration-2/report_*.json
```

## AI-Driven Parameter Adjustments

The tuner provides intelligent recommendations based on observed behavior.

### High Priority Recommendations

Applied when critical bottlenecks are detected:

**For Low Throughput:**
```yaml
consensus:
  timeout-commit: 500ms
blocks:
  max-bytes: 22020096
```

**For High Latency:**
```yaml
database:
  num-memtables: 3
  num-level-zero-tables: 3
network:
  send-rate: 52428800
  recv-rate: 52428800
```

**For High Error Rate:**
```yaml
consensus:
  timeout-prevote: 2s
network:
  max-packet-msg-payload-size: 10240
```

### Medium Priority Recommendations

Fine-tuning after addressing critical issues:

```yaml
mempool:
  size: 10000
  max-txs-bytes: 104857600
blocks:
  max-gas: -1
```

### Low Priority Recommendations

Optimization for already-healthy systems:

```yaml
consensus:
  create-empty-blocks: true
database:
  compaction-strategy: leveled
```

## Comparison with Previous Consensus

The new consensus implementation offers significant improvements over the legacy system:

### Architecture Differences

**Legacy System:**
- Sequential block production
- Single-threaded validation
- Limited batching
- Fixed timeouts

**New Implementation:**
- Parallel transaction processing
- Optimized state management
- Dynamic batching
- Configurable consensus parameters

### Performance Improvements

Expected improvements with proper tuning:

- **Throughput**: 2-5x increase in sustained TPS
- **Latency**: 30-50% reduction in P99 latency
- **Reliability**: 99%+ success rate under load
- **Resource Efficiency**: Better CPU and memory utilization

### Migration Strategy

When migrating from legacy consensus:

1. **Establish Legacy Baseline**
   ```bash
   debug perfmon legacy-node:16695 100 10m --output ./legacy-baseline
   ```

2. **Deploy New Consensus with Conservative Settings**
   ```bash
   debug perfmon new-node:16695 100 10m --output ./new-conservative
   ```

3. **Compare Initial Performance**
   ```bash
   debug perfmon-tuner ./new-conservative/report_*.json \
     --baseline ./legacy-baseline/report_*.json
   ```

4. **Iteratively Tune New System**
   Follow the iterative tuning workflow above

5. **Validate at Target Load**
   ```bash
   debug perfmon new-node:16695 1000 1h --output ./final-validation
   ```

## Best Practices

### Testing Methodology

1. **Consistent Test Parameters**
   - Use same TPS, duration, and server for comparisons
   - Test during similar network conditions
   - Run multiple iterations to average out variance

2. **Gradual Tuning**
   - Change one category of parameters at a time
   - Make incremental adjustments
   - Validate each change before proceeding

3. **Load Progression**
   - Start with low TPS (100-500)
   - Gradually increase to target load
   - Identify breaking point before backing off

4. **Duration Selection**
   - Short tests (5-10 min): Quick iteration
   - Medium tests (30-60 min): Stability validation
   - Long tests (2-4 hours): Production readiness

### Configuration Guidelines

1. **Start Conservative**
   ```yaml
   consensus:
     timeout-propose: 3s
     timeout-commit: 1s
   blocks:
     max-bytes: 10485760  # 10MB
   ```

2. **Tune for Throughput First**
   - Reduce timeouts
   - Increase block size
   - Expand mempool

3. **Optimize Latency Second**
   - Database tuning
   - Network optimization
   - Resource allocation

4. **Validate Reliability Last**
   - Extended soak tests
   - Error rate analysis
   - Stress testing

### Resource Planning

Monitor resource usage and plan capacity:

**CPU:**
- Target: 50-70% average utilization
- Peaks: Allow for 80-90% during bursts
- Critical: Scale if sustained > 80%

**Memory:**
- Target: 60-70% of available RAM
- Peaks: Allow headroom for 80-85%
- Critical: Scale if sustained > 80%

**Disk:**
- SSD required for production
- Monitor IOPS and throughput
- Plan for 2-3x growth

**Network:**
- Measure bandwidth utilization
- Plan for 2x peak traffic
- Consider burst capacity

## Troubleshooting

### Low TPS Achievement

**Symptoms:**
- Achieved TPS << Target TPS
- High transaction queue depth

**Solutions:**
1. Increase block size (`blocks.max-bytes`)
2. Reduce commit timeout (`consensus.timeout-commit`)
3. Expand mempool (`mempool.size`)
4. Check network bandwidth

### High Latency

**Symptoms:**
- P99 > 2000ms
- Increasing over time

**Solutions:**
1. Tune database (`num-memtables`, `num-level-zero-tables`)
2. Increase network rates (`send-rate`, `recv-rate`)
3. Check disk I/O performance
4. Review application logic

### High Error Rate

**Symptoms:**
- Success rate < 95%
- Specific error types dominating

**Solutions:**
1. Review error breakdown in report
2. Increase relevant timeouts
3. Check transaction validation logic
4. Verify network connectivity

### Slow Block Production

**Symptoms:**
- Block rate < 6 blocks/min
- Large block intervals

**Solutions:**
1. Reduce `timeout-commit`
2. Enable `create-empty-blocks`
3. Optimize validator connectivity
4. Check consensus participation

### Memory Leaks

**Symptoms:**
- Increasing memory over time
- Out of memory errors

**Solutions:**
1. Enable `--memprofile` for analysis
2. Review database cache settings
3. Check for transaction accumulation
4. Monitor goroutine count

### Network Saturation

**Symptoms:**
- Dropped messages
- Timeout errors
- Uneven load distribution

**Solutions:**
1. Increase `send-rate` and `recv-rate`
2. Optimize `max-packet-msg-payload-size`
3. Review network topology
4. Check bandwidth availability

## Advanced Topics

### Custom Metrics Collection

Extend the perfmon tool for custom metrics:

```go
// Add custom metrics to PerformanceMetrics struct
type PerformanceMetrics struct {
    // ... existing fields ...
    CustomMetric1  float64
    CustomMetric2  []int64
}
```

### Automated Tuning Scripts

Use shell scripts to automate iterative tuning:

```bash
# See scripts/perfmon-workflow.sh for full example
./perfmon-workflow.sh localhost:16695 100 5m 5
```

### Multi-Node Testing

Test across multiple nodes for realistic scenarios:

```bash
# Run tests against different nodes
debug perfmon node1:16695 100 5m --output ./node1-results
debug perfmon node2:16695 100 5m --output ./node2-results
debug perfmon node3:16695 100 5m --output ./node3-results

# Compare results
scripts/perfmon-compare.sh ./node1-results/report_*.json \
                          ./node2-results/report_*.json \
                          ./node3-results/report_*.json
```

### Continuous Monitoring

Integrate perfmon into CI/CD:

```yaml
# Example GitLab CI job
performance-test:
  script:
    - debug perfmon $TEST_NODE 100 5m --output ./results
    - debug perfmon-tuner ./results/report_*.json --threshold 0.95
  artifacts:
    paths:
      - results/
    when: always
```

## Appendix

### Parameter Reference

Complete list of tunable parameters:

**Consensus:**
- `timeout-propose`: Block proposal timeout
- `timeout-prevote`: Prevote timeout
- `timeout-precommit`: Precommit timeout
- `timeout-commit`: Commit timeout
- `create-empty-blocks`: Whether to create empty blocks
- `create-empty-blocks-interval`: Empty block interval

**Blocks:**
- `max-bytes`: Maximum block size
- `max-gas`: Maximum gas per block
- `time-iota-ms`: Block timestamp increment

**Mempool:**
- `size`: Maximum transaction count
- `max-txs-bytes`: Maximum total transaction bytes
- `cache-size`: Transaction cache size

**Database:**
- `num-memtables`: Number of in-memory tables
- `num-level-zero-tables`: L0 tables before compaction
- `compaction-strategy`: Compaction algorithm

**Network:**
- `send-rate`: Maximum send rate (bytes/sec)
- `recv-rate`: Maximum receive rate (bytes/sec)
- `max-packet-msg-payload-size`: Maximum packet size

### Glossary

- **TPS**: Transactions Per Second
- **P50/P95/P99**: Latency percentiles
- **Mempool**: Transaction pool awaiting inclusion
- **L0**: Level 0 in LSM-tree database
- **Compaction**: Database reorganization process
- **Consensus**: Agreement protocol between validators

### References

- [Accumulate Documentation](https://docs.accumulatenetwork.io/)
- [Performance Tuning Best Practices](https://docs.accumulatenetwork.io/performance)
- [Network Configuration Guide](https://docs.accumulatenetwork.io/config)
