# Network Monitoring Tools

Monitoring scripts for the load testing environment to prevent failures and provide health insights.

## Overview

The monitoring tools watch critical system resources and automatically stop the network before catastrophic failures occur (disk full, OOM, etc.).

## Scripts

### 1. disk-monitor.sh

Focused disk space monitoring with automatic network shutdown.

**Features:**
- Monitors available disk space
- Configurable threshold (default: 10GB)
- Automatic network stop on low space
- Continuous logging

**Usage:**

```bash
# Start with defaults (10GB threshold, 60s interval)
./test/monitor/disk-monitor.sh

# Custom threshold and check interval
./test/monitor/disk-monitor.sh --threshold 20 --check-interval 30

# Warning only (don't auto-stop)
./test/monitor/disk-monitor.sh --action warn

# Pause containers instead of stopping
./test/monitor/disk-monitor.sh --action pause
```

**Options:**

| Option | Description | Default |
|--------|-------------|---------|
| `--threshold <GB>` | Minimum free space in GB | 10 |
| `--check-interval <sec>` | Check frequency in seconds | 60 |
| `--compose-file <path>` | Docker compose file to manage | ../docker/docker-compose.yml |
| `--action <action>` | Action: stop\|pause\|warn | stop |
| `--log-file <path>` | Log file location | /tmp/disk-monitor.log |

**Actions:**

- `stop` - Stop Docker network when threshold reached
- `pause` - Pause Docker containers when threshold reached
- `warn` - Log warnings only, don't take action

### 2. network-monitor.sh

Comprehensive monitoring of all health metrics.

**Features:**
- Disk space monitoring
- Memory usage tracking
- Container health checks
- API endpoint monitoring
- Docker resource stats
- Automatic network shutdown on critical issues

**Usage:**

```bash
# Start with defaults
./test/monitor/network-monitor.sh

# Custom thresholds
./test/monitor/network-monitor.sh --disk-threshold 20 --memory-threshold 85

# Monitor only, don't auto-stop
./test/monitor/network-monitor.sh --no-auto-stop

# Faster checking
./test/monitor/network-monitor.sh --check-interval 15
```

**Options:**

| Option | Description | Default |
|--------|-------------|---------|
| `--disk-threshold <GB>` | Minimum free disk space | 10 |
| `--memory-threshold <%>` | Maximum memory usage | 90 |
| `--check-interval <sec>` | Check frequency | 30 |
| `--compose-file <path>` | Docker compose file | ../docker/docker-compose.yml |
| `--log-file <path>` | Log file path | /tmp/network-monitor.log |
| `--no-auto-stop` | Disable automatic shutdown | false |

**Monitored Metrics:**

- **Disk Space** - Stops network if below threshold
- **Memory Usage** - Warns on high usage
- **Container Health** - Counts failures, stops after 3
- **API Endpoints** - Checks BVN API responsiveness
- **Docker Stats** - CPU, memory per container

## Integration with Docker Deployment

### Run monitor alongside network

```bash
# Terminal 1: Start network
./test/docker/manage.sh start

# Terminal 2: Start monitor
./test/monitor/network-monitor.sh
```

### Background monitoring

```bash
# Start network
./test/docker/manage.sh start

# Start monitor in background
nohup ./test/monitor/network-monitor.sh > /tmp/monitor.out 2>&1 &

# Check monitor status
tail -f /tmp/monitor.out
tail -f /tmp/network-monitor.log

# Stop monitor
pkill -f network-monitor.sh
```

### Automated monitoring with systemd

Create `/etc/systemd/system/accumulate-monitor.service`:

```ini
[Unit]
Description=Accumulate Network Monitor
After=docker.service

[Service]
Type=simple
User=accumulate
WorkingDirectory=/path/to/accumulate
ExecStart=/path/to/accumulate/test/monitor/network-monitor.sh
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable accumulate-monitor
sudo systemctl start accumulate-monitor
sudo journalctl -u accumulate-monitor -f
```

## Log Files

### Log Locations

- **Monitor log**: `/tmp/network-monitor.log` - All checks and metrics
- **Alert log**: `/tmp/network-monitor.alerts` - Critical issues only
- **Disk monitor log**: `/tmp/disk-monitor.log` - Disk space checks

### Log Format

```
[2026-03-22 12:00:00] INFO: Starting network monitor
[2026-03-22 12:00:00] METRIC: Disk: 45GB free (72% used)
[2026-03-22 12:00:00] METRIC: Memory: 12GB / 32GB (37% used)
[2026-03-22 12:00:00] METRIC: Containers: 12/12 running, 0 exited
[2026-03-22 12:00:00] METRIC: API Health: 3/3 endpoints responding
```

### Viewing Logs

```bash
# Follow live
tail -f /tmp/network-monitor.log

# View recent errors
grep ERROR /tmp/network-monitor.log | tail -20

# View warnings
grep WARNING /tmp/network-monitor.log

# View alerts only
cat /tmp/network-monitor.alerts
```

## Alert Actions

### Disk Space Critical

When disk space falls below threshold:

1. **Warning logged** - First detection
2. **Alert created** - Added to alerts file
3. **Network stopped** - If auto-stop enabled
4. **Exit code 1** - Monitor exits

### Memory Critical

When memory usage exceeds threshold:

1. **Warning logged** - Every check while high
2. **Alert created** - On first detection
3. **Recovery logged** - When usage drops

### Container Failures

When containers exit unexpectedly:

1. **Warning logged** - First failure
2. **Count incremented** - Tracks consecutive failures
3. **Network stopped** - After 3 consecutive failures (if auto-stop)

### API Failures

When API endpoints don't respond:

1. **Warning logged** - Per failed endpoint
2. **Count tracked** - Number of healthy vs total
3. **No auto-stop** - Informational only

## Monitoring Best Practices

### Before Load Tests

1. **Check available space**:
   ```bash
   df -h .
   ```

2. **Set appropriate thresholds** based on available resources

3. **Start monitor before network**:
   ```bash
   ./test/monitor/network-monitor.sh &
   ./test/docker/manage.sh start
   ```

### During Load Tests

1. **Watch monitor output** for warnings

2. **Check logs periodically**:
   ```bash
   tail -20 /tmp/network-monitor.log
   ```

3. **Monitor resource trends**:
   ```bash
   watch -n 5 docker stats
   ```

### After Load Tests

1. **Review logs** for issues:
   ```bash
   grep -E "(ERROR|WARNING)" /tmp/network-monitor.log
   ```

2. **Check alerts**:
   ```bash
   cat /tmp/network-monitor.alerts
   ```

3. **Clean up**:
   ```bash
   rm /tmp/network-monitor.log /tmp/network-monitor.alerts
   ```

## Troubleshooting

### Monitor won't start

```bash
# Check if already running
ps aux | grep monitor

# Kill existing instance
pkill -f network-monitor.sh

# Check permissions
chmod +x test/monitor/*.sh
```

### False disk warnings

```bash
# Check actual usage
df -h .

# Increase threshold
./test/monitor/disk-monitor.sh --threshold 20
```

### Monitor exits immediately

```bash
# Check compose file exists
ls test/docker/docker-compose.yml

# Specify correct path
./test/monitor/network-monitor.sh --compose-file /path/to/docker-compose.yml
```

### High memory usage

```bash
# Check what's using memory
docker stats

# Reduce container limits in docker-compose.yml
# Or increase system memory
```

## Examples

### Basic monitoring

```bash
# Simple disk monitoring
./test/monitor/disk-monitor.sh

# Comprehensive monitoring
./test/monitor/network-monitor.sh
```

### Custom thresholds

```bash
# Strict thresholds
./test/monitor/network-monitor.sh \
  --disk-threshold 50 \
  --memory-threshold 80 \
  --check-interval 10
```

### Development mode (no auto-stop)

```bash
# Monitor only, manual intervention
./test/monitor/network-monitor.sh --no-auto-stop
```

### Production mode

```bash
# Aggressive protection
./test/monitor/network-monitor.sh \
  --disk-threshold 100 \
  --memory-threshold 85 \
  --check-interval 15
```

## See Also

- [Docker Deployment](../docker/README.md) - Network deployment guide
- [Test Wallet](../wallet/README.md) - Test account management
- [Load Generator](../../cmd/load-generator/) - Transaction generation
