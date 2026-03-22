# Load Test Monitoring Dashboard

A compact, real-time terminal-based dashboard for monitoring Accumulate load test execution.

## Features

- Live TPS counter (target vs actual)
- Transaction latency metrics (P50, P95, P99)
- Resource utilization (CPU, memory, goroutines)
- Block production rate and height tracking
- Node health status monitoring
- Disk space tracking with visual warnings
- Compact single-screen layout
- Real-time updates

## Installation

```bash
go build -o monitor ./test/cmd/monitor
```

## Usage

```bash
./monitor [options]
```

### Options

- `-s <url>` - Accumulate server URL (default: `http://127.0.1.1:26660`)
- `-p <seconds>` - Update period in seconds (default: `2`)
- `-t <tps>` - Target TPS for comparison (optional)

### Examples

Basic monitoring:
```bash
./monitor -s http://localhost:26660
```

Monitor with target TPS:
```bash
./monitor -s http://localhost:26660 -t 1000 -p 1
```

Monitor remote node:
```bash
./monitor -s https://testnet.accumulatenetwork.io -p 5
```

## Keyboard Controls

- `q` or `Q` - Quit the application
- `r` or `R` - Reset stats (future feature)

## Dashboard Layout

```
┌─────────────────────────────────────────────────────────────┐
│               Accumulate Load Test Monitor                  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  TRANSACTION PERFORMANCE                                    │
│    Target TPS:          1000                                │
│    Actual TPS:           987.45 (OK)                        │
│                                                              │
│  TRANSACTION LATENCY                                        │
│    P50 (median):         125ms                              │
│    P95:                  250ms                              │
│    P99:                  500ms                              │
│                                                              │
│  BLOCKCHAIN STATUS                                          │
│    Block Height:         12345                              │
│    Block Rate (TPS):     987.45                             │
│    Node Health:          HEALTHY                            │
│                                                              │
│  RESOURCE UTILIZATION                                       │
│    CPU Usage:            45.2%                              │
│    Memory Used:          512 MB                             │
│    Memory Total:         2048 MB                            │
│    Memory Usage:         [##########--------------------] 25.0% │
│    Goroutines:           234                                │
│                                                              │
│  DISK SPACE                                                 │
│    Used:                 45 GB                              │
│    Total:                100 GB                             │
│    Disk Usage:           [#############-----------------] 45.0% │
│                                                              │
│  SYSTEM INFO                                                │
│    Last Update:          14:23:45                           │
│    Uptime:               2h34m12s                           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
Press 'q' to quit | Press 'r' to reset stats | Update period: 2s
```

## Technical Details

### Metrics Collection

The dashboard polls the Accumulate node API at regular intervals to collect:

- **TPS Metrics**: Retrieved via the `/v2` metrics endpoint
- **Node Status**: Retrieved via the `/v2` status endpoint
- **System Resources**: Collected from Go runtime statistics
- **Latency**: Measured from API request round-trip times

### Color Coding

- **Green**: Normal/healthy status
- **Yellow**: Warning/degraded performance
- **Red**: Critical issues/errors

### Resource Thresholds

- Memory: Yellow > 70%, Red > 90%
- Disk: Yellow > 80%, Red > 90%
- TPS: Yellow < 70% of target, Red < 95% of target

## Integration with Load Generator

The monitor is designed to run alongside the load generator tool:

Terminal 1 - Load Generator:
```bash
./load -s http://127.0.1.1:26660/v2 -t 1000 -d 300
```

Terminal 2 - Monitor:
```bash
./monitor -s http://127.0.1.1:26660 -t 1000 -p 2
```

## Troubleshooting

### Cannot connect to server

Ensure the Accumulate node is running and accessible at the specified URL.

### Metrics not updating

Check that the node's API is enabled and responding. Increase the update period if the node is under heavy load.

### High latency readings

API latency includes network round-trip time. For local testing, latency should be low (< 50ms).

## Part of Epic #3838

This tool is part of the dagbft-integration testing framework (Epic #3838).

Related tools:
- `test/cmd/load` - Load test generator
- `test/cmd/devnet` - Development network management
