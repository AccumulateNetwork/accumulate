# Load Test Dashboard

A real-time terminal dashboard for monitoring Accumulate load tests.

## Features

- **Load Test Metrics**
  - Transaction counts (total, successful, failed)
  - Current TPS, average TPS, and peak TPS
  - Latency statistics (average, p95, p99)
  - Error rate tracking
  - Progress bar for target completion

- **System Metrics**
  - CPU utilization percentage
  - Memory usage (used/total)
  - Disk I/O rates (read/write MB/s)
  - Network throughput (rx/tx MB/s)

- **Terminal UI**
  - ANSI color-coded display
  - Progress bars for visual feedback
  - Auto-refreshing metrics
  - Color-coded status indicators

## Usage

### Basic Usage

```go
package main

import (
    "context"
    "time"

    "gitlab.com/accumulatenetwork/accumulate/test/cmd/load/dashboard"
)

func main() {
    // Create dashboard with target of 10000 transactions
    d := dashboard.New(10000)

    // Start dashboard with 1-second updates
    ctx := context.Background()
    go d.Start(ctx, time.Second)
    defer d.Stop()

    // Record transactions as your load test runs
    for i := 0; i < 10000; i++ {
        start := time.Now()

        // Execute your transaction
        success := executeTransaction()

        // Record the result
        latency := time.Since(start)
        d.LoadMetrics().RecordTransaction(success, latency)
    }
}
```

### Custom Output Writer

```go
// Write to a buffer instead of stdout
var buf bytes.Buffer
d := dashboard.NewWithWriter(10000, &buf)
```

### Integration Example

```go
// Start dashboard
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

d := dashboard.New(targetTxCount)
go d.Start(ctx, time.Second)
defer d.Stop()

// Run your load test
metrics := d.LoadMetrics()
for _, tx := range transactions {
    start := time.Now()
    err := sendTransaction(tx)
    latency := time.Since(start)

    metrics.RecordTransaction(err == nil, latency)
}

// Dashboard continues updating until context is cancelled
```

## Architecture

### Components

- **LoadMetrics**: Tracks transaction statistics
  - Thread-safe metric collection
  - Rolling window for TPS calculation
  - Latency percentile computation
  - Error rate tracking

- **SystemMetrics**: Collects OS-level metrics
  - CPU utilization from /proc/stat
  - Memory usage from /proc/meminfo
  - Disk I/O from /proc/diskstats
  - Network traffic from /proc/net/dev
  - Cross-platform fallbacks for non-Linux systems

- **Display**: Renders terminal UI
  - ANSI escape code formatting
  - Color-coded status indicators
  - Progress bar visualization
  - Responsive layout

- **Dashboard**: Coordinates updates and rendering
  - Metric collection orchestration
  - Configurable update intervals
  - Clean shutdown handling

### Metric Collection

Load metrics are collected in real-time as transactions complete:

```go
metrics.RecordTransaction(success bool, latency time.Duration)
```

System metrics are polled at the dashboard update interval (typically 1 second):

```go
systemMetrics.Update()
```

### Thread Safety

All metric structures use `sync.RWMutex` for concurrent access:
- Writers hold exclusive locks during updates
- Readers can access snapshots without blocking

## System Requirements

### Linux

Full functionality on Linux systems with access to:
- `/proc/stat` - CPU statistics
- `/proc/meminfo` - Memory information
- `/proc/diskstats` - Disk I/O statistics
- `/proc/net/dev` - Network interface statistics

### Non-Linux

Limited system metrics (CPU/memory only) on:
- macOS
- Windows
- Other Unix-like systems

Uses `runtime.MemStats` for basic memory metrics when `/proc` is unavailable.

## Performance Considerations

- Metric collection is designed for minimal overhead
- Update intervals should be >= 1 second to avoid excessive CPU usage
- System metric collection reads from /proc filesystem (very low overhead)
- Latency percentiles calculated using simple sorting (acceptable for load tests)

## Testing

Run the test suite:

```bash
go test ./test/cmd/load/dashboard/...
```

Tests cover:
- Metric recording and calculation
- Concurrent access safety
- Dashboard lifecycle
- Display rendering
- System metric collection

## Color Coding

### Status Indicators

- **Green**: Normal operation (< 10% error rate, < 70% resource usage)
- **Yellow**: Warning (10-50% error rate, 70-90% resource usage)
- **Red**: Critical (> 50% error rate, > 90% resource usage)

### Metric Colors

- Cyan: Informational metrics (TPS, throughput)
- Green: Success counters
- Red: Error counters
- Yellow: Latency metrics
- Gray: Secondary information

## Metrics Formulas

### Transaction Performance Metrics

**Current TPS (Transactions Per Second)**
```
Current TPS = transactions_in_window / window_elapsed_seconds
```
- Uses a rolling 1-second window
- Resets when window expires

**Average TPS**
```
Average TPS = total_transactions / elapsed_time_since_start
```
- Calculated over entire test duration
- More stable than current TPS

**Peak TPS**
```
Peak TPS = max(all_window_tps_values)
```
- Highest TPS observed in any 1-second window
- Monotonically increasing (never decreases)

**Error Rate**
```
Error Rate (%) = (failed_transactions / total_transactions) × 100
```
- Returns 0% when no transactions have been recorded

### Latency Metrics

**Average Latency**
```
Average Latency = sum(all_latencies) / count(latencies)
```
- Simple arithmetic mean
- Uses up to last 1000 latencies

**Percentile Calculations (P50, P95, P99)**
```
1. Sort latencies in ascending order
2. P50 = sorted[count × 50/100]
3. P95 = sorted[count × 95/100]
4. P99 = sorted[count × 99/100]
```
- Uses simple index-based percentile (not interpolated)
- Based on rolling window of last 1000 latencies

### System Metrics

**CPU Percentage** (Linux only)
```
1. Read /proc/stat CPU counters (user, nice, system, idle, iowait, irq, soft)
2. Calculate deltas from previous reading
3. CPU % = ((total_delta - idle_delta) / total_delta) × 100
```
- Returns 0 on non-Linux systems

**Memory Usage**
```
Memory Used MB = (MemTotal - MemAvailable) / 1024
```
- Reads from /proc/meminfo on Linux
- Uses runtime.MemStats on non-Linux systems

**Disk I/O Rates** (Linux only)
```
1. Read /proc/diskstats for sectors read/written
2. Convert sectors to bytes (sectors × 512)
3. Disk Read MB/s = (read_bytes_delta / 1024 / 1024) / elapsed_seconds
4. Disk Write MB/s = (write_bytes_delta / 1024 / 1024) / elapsed_seconds
```
- Aggregates across all physical disks (skips loop, ram, partitions)
- Returns 0 on non-Linux systems

**Network Throughput** (Linux only)
```
1. Read /proc/net/dev for rx/tx bytes
2. Network Rx MB/s = (rx_bytes_delta / 1024 / 1024) / elapsed_seconds
3. Network Tx MB/s = (tx_bytes_delta / 1024 / 1024) / elapsed_seconds
```
- Aggregates across all interfaces except loopback
- Returns 0 on non-Linux systems

### Rolling Windows

**Latency Window**
- Maintains last 1000 latencies
- Older latencies are dropped when limit exceeded
- Used for percentile calculations

**TPS Window**
- 1-second sliding window
- Resets after window expires
- Used for current TPS calculation

## Known Limitations

- Percentile calculations are approximate (simple sorting)
- System metrics require Linux /proc filesystem for full functionality
- No persistent metric storage (in-memory only)
- Terminal width/height are fixed (120x40)
- Assumes UTF-8 terminal with ANSI color support
