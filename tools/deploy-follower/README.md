# deploy-follower

Deploy an Accumulate follower node locally from snapshots.

## Overview

This tool automates the deployment of an Accumulate follower node by:
1. Creating the required directory structure
2. Initializing partitions from snapshots using `accumulated restore-genesis`
3. Generating configuration files
4. Creating start/stop scripts

## Building

```bash
cd tools/deploy-follower
go build -o deploy-follower
```

## Usage

### Basic deployment:

```bash
./deploy-follower \
  --work-dir /path/to/follower-data \
  --dn-snapshot /path/to/directory.snap \
  --bvn-snapshot /path/to/cyclops.snap \
  --accumulated /path/to/accumulated \
  --network mainnet \
  --bvn Cyclops
```

### Using a config file:

```bash
./deploy-follower --artifacts-dir /path/to/artifacts --work-dir /path/to/follower-data
```

### Start immediately after deploy:

```bash
./deploy-follower --work-dir /path/to/data ... --start
```

## Command Line Options

| Flag | Description | Required |
|------|-------------|----------|
| `--work-dir` | Directory to store follower data | Yes |
| `--dn-snapshot` | Path to Directory Network snapshot | Yes |
| `--bvn-snapshot` | Path to BVN snapshot | Yes |
| `--accumulated` | Path to accumulated binary | Yes |
| `--monitor` | Path to follower-monitor binary | No |
| `--network` | Network name (default: mainnet) | No |
| `--bvn` | BVN name (default: Cyclops) | No |
| `--start` | Start the follower after deployment | No |
| `--status` | Show status of existing deployment | No |
| `--stop` | Stop a running follower | No |
| `--config` | Path to config.yaml file | No |
| `--artifacts-dir` | Path to artifacts directory (looks for config.yaml) | No |

## Config File Format

The tool supports YAML configuration files:

```yaml
network: mainnet
bvn: Cyclops

binaries:
  accumulated: ./accumulated
  follower_monitor: ./follower-monitor

snapshots:
  directory: ./snapshots/directory.snap
  cyclops: ./snapshots/cyclops.snap
  date: "2025-12-01"

deployment:
  work_dir: /path/to/follower-data
```

## Directory Structure Created

```
work-dir/
  accumulated           # Binary copy
  follower-monitor      # Binary copy (if provided)
  accumulate.toml       # Main configuration
  start.sh              # Start script
  stop.sh               # Stop script
  start-monitor.sh      # Monitor start script (if monitor provided)
  follower.pid          # PID file when running
  dnn/                  # DN partition directory
    config/
      genesis.json
      tendermint.toml
      priv_validator_key.json
      node_key.json
    data/
      state.db/
      blockstore.db/
      evidence.db/
      tx_index.db/
      accumulate.db/
  bvnn/                 # BVN partition directory
    config/
    data/
  logs/
    init-dn.log
    init-bvn.log
    follower.log
```

## Generated Configuration

The tool generates `accumulate.toml` with:
- Dual-mode follower configuration
- Bootstrap peers for peer discovery
- Logging configured for plain format at info level

Default ports:
- DN P2P: 16591
- DN RPC: 16592
- BVN P2P: 16691
- BVN RPC: 16692

## Examples

### Check deployment status:

```bash
./deploy-follower --work-dir /path/to/data --status
```

### Stop a running follower:

```bash
./deploy-follower --work-dir /path/to/data --stop
```

### Manual start after deployment:

```bash
cd /path/to/data
./start.sh
```

## RPC Endpoints

After starting, query sync status:

```bash
# DN status
curl -s http://localhost:16592/status | jq '.result.sync_info'

# BVN status
curl -s http://localhost:16692/status | jq '.result.sync_info'
```

## Related Tools

- **follower-monitor**: Web-based dashboard for monitoring follower status
- **accumulated restore-genesis**: The underlying command used for initialization

## Troubleshooting

1. **Initialization fails**: Check logs in `logs/init-dn.log` and `logs/init-bvn.log`
2. **Follower won't start**: Check `logs/follower.log` for errors
3. **Can't connect to peers**: Verify network connectivity to bootstrap peers
4. **Sync stuck**: Check if persistent_peers in tendermint.toml are reachable
