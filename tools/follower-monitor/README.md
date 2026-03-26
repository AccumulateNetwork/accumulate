# follower-monitor

Web-based dashboard for monitoring and controlling Accumulate follower nodes.

## Overview

This tool provides a web interface for:
- Real-time sync status monitoring
- Progress tracking against mainnet
- Live log viewing with filtering
- Start/stop control of the follower process
- Binary update detection

## Building

```bash
cd tools/follower-monitor
go build -o follower-monitor
```

## Usage

```bash
./follower-monitor --work-dir /path/to/follower-data
```

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--work-dir` | Follower work directory (contains dnn/, bvnn/, logs/) | Required |
| `--artifacts-dir` | Artifacts directory with newer binaries for update detection | None |
| `--bind` | Address to bind to | 127.0.0.1 |
| `--port` | HTTP server port | 9999 |
| `--interval` | Update interval in seconds | 10 |

## Security

By default, the monitor binds to localhost only (127.0.0.1) because it includes control functions (start/stop) that should not be exposed publicly.

To expose externally (not recommended for production):
```bash
./follower-monitor --work-dir /path/to/data --bind 0.0.0.0
```

## Features

### Status Tab

Displays real-time information:
- DN and BVN block heights
- Sync progress percentage
- Catching up status
- Peer count
- Latest block time
- Comparison against mainnet heights

### Logs Tab

- Live log streaming
- Filter by log level (DEBUG, INFO, WARN, ERROR)
- Text search filtering
- Partition filtering (DN/BVN)
- Auto-scroll option
- Line limit control

### Controls

- **Start**: Start the follower process
- **Stop**: Gracefully stop the follower
- **Update**: Apply new binary from artifacts directory (if available)

## Expected Directory Structure

```
work-dir/
  accumulated           # Binary
  follower.pid          # PID file
  dnn/                  # DN partition
  bvnn/                 # BVN partition
  logs/
    follower.log        # Combined log file
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web dashboard |
| `/status` | GET | JSON status response |
| `/logs` | GET | Recent log lines |
| `/start` | POST | Start follower |
| `/stop` | POST | Stop follower |
| `/update` | POST | Update binary |

### Status Response Format

```json
{
  "running": true,
  "pid": 12345,
  "dn": {
    "height": 12345678,
    "catching_up": true,
    "peers": 5,
    "latest_block_time": "2025-12-07T12:00:00Z",
    "mainnet_height": 12500000,
    "sync_progress": 98.76
  },
  "bvn": {
    "height": 12345600,
    "catching_up": true,
    "peers": 3,
    "latest_block_time": "2025-12-07T12:00:00Z",
    "mainnet_height": 12500000,
    "sync_progress": 98.75
  },
  "update_available": false
}
```

## Examples

### Basic usage:

```bash
./follower-monitor --work-dir /home/user/accumulate-follower
```

### With update detection:

```bash
./follower-monitor --work-dir /home/user/accumulate-follower \
  --artifacts-dir /home/user/artifacts
```

### Custom port and external access:

```bash
./follower-monitor --work-dir /home/user/accumulate-follower \
  --port 8888 --bind 0.0.0.0
```

## Mainnet Reference

The monitor compares local block heights against mainnet endpoints:
- DN: `https://mainnet.accumulatenetwork.io/v2` (port 16592)
- BVN: `https://mainnet.accumulatenetwork.io/v2` (port 16692)

## Related Tools

- **deploy-follower**: Automated follower deployment tool

## Troubleshooting

1. **Can't connect to follower RPC**: Ensure follower is running and ports 16592/16692 are accessible
2. **Update not detected**: Verify artifacts-dir path and that new binary exists
3. **Permission denied on start/stop**: Ensure monitor has permission to execute binaries and write PID files
4. **High memory usage**: Reduce log line limit or increase update interval
