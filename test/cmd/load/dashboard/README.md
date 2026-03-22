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

## Known Limitations

- Percentile calculations are approximate (simple sorting)
- System metrics require Linux /proc filesystem for full functionality
- No persistent metric storage (in-memory only)
- Terminal width/height are fixed (120x40)
- Assumes UTF-8 terminal with ANSI color support
