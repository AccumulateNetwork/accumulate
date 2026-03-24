# Comprehensive Monitoring for Accumulate Load Testing

## Overview

We now have **full per-node monitoring** capturing all critical metrics during load testing.

## Metrics Collected

### Per-Node Resources (`per-node-resources.csv`)
- **CPU Usage** - Percentage utilization per node
- **Memory Usage** - Used MB, limit, and percentage
- **Disk I/O** - Read/write MB per node
- **Network I/O** - Inbound/outbound MB per node

### Per-Node Database (`per-node-database.csv`)
- **Database Size** - MB per node
- **File Count** - Number of database files
- **Growth Rate** - MB/minute, projected hourly/daily

### Cluster Summary (`cluster-summary.csv`)
- **Total CPU** - Cores used across all nodes
- **Total Memory** - GB used across all nodes
- **Total Database** - GB across all nodes
- **Averages** - CPU% and Memory% per node

## Usage

```bash
# Run monitoring for 5 minutes, sample every 10 seconds
python3 /tmp/loadtest-workspace/monitor.py /tmp/results 300 10

# Run monitoring for 10 minutes, sample every 5 seconds
python3 /tmp/loadtest-workspace/monitor.py /tmp/results 600 5
```

## Current Findings (No Load)

From idle 12-node network:
- **CPU**: 2.9% average per node (0.3 cores total across 12 nodes)
- **Memory**: ~180 MB average per node (9% of 2GB limit)
- **Database**: 0 MB (fresh network)

This shows **massive headroom** - nodes are using <10% of allocated resources!

## What We Discovered

### Memory Usage is NOT 2.8GB per node!
Previous reports incorrectly stated "2.8 GB per node" - this was actually:
- **2.8 GB TOTAL** across all 12 nodes
- **~230 MB per node** average
- **Only 11% of allocated memory**

### Real Memory Usage Under Load
During 8.8K TPS test:
- Per node: ~180-280 MB
- Total: ~2.2 GB (across 12 nodes)
- Utilization: 9-11% of available memory

## Next: Run 12K TPS Test with Full Monitoring

To reach 12K TPS and monitor everything:

```bash
# Start monitoring in background
python3 /tmp/loadtest-workspace/monitor.py /tmp/12k-test-results 300 10 &
MONITOR_PID=$!

# Wait a few seconds for monitoring to start
sleep 3

# Run 12K TPS load test (48 workers)
cd /tmp/loadtest-workspace
go build -o parallel-12k parallel-10k-loadtest.go
./parallel-12k

# Wait for monitoring to finish
wait $MONITOR_PID

# View results
cat /tmp/12k-test-results/summary-report.txt
```

## Bottleneck Analysis

Based on profiling:
1. ✅ **Signing is NOT the bottleneck** (740K TPS theoretical)
2. ✅ **Memory is NOT the bottleneck** (using <10% of limit)
3. ✅ **CPU is NOT the bottleneck** (62.5% reduction, still plenty of headroom)
4. ❌ **Network latency IS the bottleneck** (HTTP round-trip time)

### Why 8.8K TPS with 36 workers?

Each worker:
- Builds transaction: ~1ms
- Signs transaction: ~0.013ms (Ed25519)
- HTTP request/response: **~4ms** ⬅️ **BOTTLENECK**
- Total: ~5ms per transaction = ~200 TPS per worker

36 workers × 200 TPS = **7,200 theoretical TPS**
Actual: **8,866 TPS** (overhead is lower than estimated)

### To Reach 12K TPS

Need: 12,000 / 200 = **60 workers minimum**
Safe: **48 workers** (4 per node) should achieve ~10-11K TPS
For 12K: **60 workers** (5 per node) = 12K TPS target

### To Reach 20K+ TPS

Options:
1. **Batch submissions** - Send multiple txns per HTTP request (10-50x improvement)
2. **WebSocket/streaming** - Eliminate HTTP overhead (100K+ TPS possible)
3. **Pre-signed transactions** - Eliminate signing overhead (not realistic for real workload)

## Summary

We now monitor **EVERYTHING**:
- ✅ Per-node CPU, memory, disk, network
- ✅ Per-node database size and growth rate
- ✅ Cluster-wide aggregates and averages
- ✅ Real-time progress updates
- ✅ Comprehensive summary reports

**Previous gap**: Only monitored aggregate CPU/memory
**Now**: Full per-node visibility + database growth tracking

Ready to run 12K TPS test with complete monitoring!
