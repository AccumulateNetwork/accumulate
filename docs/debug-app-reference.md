# Accumulate Debug App Reference

The Accumulate debug app is a comprehensive command-line utility for debugging, analyzing, and maintaining Accumulate networks. It provides tools for network analysis, database operations, healing, snapshot management, and various diagnostic functions.

## Table of Contents

- [Installation and Usage](#installation-and-usage)
- [Global Flags](#global-flags)
- [Command Categories](#command-categories)
- [Network Commands](#network-commands)
- [Database Commands](#database-commands)
- [Healing Commands](#healing-commands)
- [Snapshot Commands](#snapshot-commands)
- [Account Commands](#account-commands)
- [Genesis Commands](#genesis-commands)
- [Utility Commands](#utility-commands)
- [Advanced Commands](#advanced-commands)
- [Configuration and Caching](#configuration-and-caching)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Installation and Usage

### Building the Debug App

```bash
cd /path/to/accumulate/tools/cmd/debug
go build -o debug .
```

### Basic Usage

```bash
./debug [command] [subcommand] [flags] [arguments]
```

### Getting Help

```bash
# General help
./debug --help

# Command-specific help
./debug network --help
./debug database --help
```

## Global Flags

The debug app supports several global flags that apply to most commands:

| Flag | Description | Default |
|------|-------------|---------|
| `--bootstrap` | Set the bootstrap servers | Accumulate default servers |
| `--json, -j` | Output result as JSON | false |
| `--verbose, -v` | Enable debug logging | false |
| `--debug` | Debug network requests | false |
| `--pretend, -n` | Do not submit envelopes, only scan | false |

## Command Categories

The debug app organizes commands into logical categories:

1. **Network Commands** - Network scanning, status, and analysis
2. **Database Commands** - Database operations and utilities
3. **Healing Commands** - Network healing and synchronization
4. **Snapshot Commands** - Snapshot analysis and utilities
5. **Account Commands** - Account analysis and routing
6. **Genesis Commands** - Genesis block operations
7. **Utility Commands** - Encoding, verification, and misc tools

## Network Commands

### `debug network`

Network-related debugging and analysis commands.

#### `debug network scan [network]`

Scans the network for nodes and collects network topology information.

**Usage:**
```bash
./debug network scan mainnet
./debug network scan testnet
./debug network scan --json mainnet > network-scan.json
```

**Features:**
- Discovers all active nodes in the network
- Collects peer information and connectivity
- Outputs detailed network topology
- Supports JSON output for automation

**Output:**
- Node peer IDs and addresses
- Partition information
- Validator status
- Network connectivity map

#### `debug network scan-node [address]`

Scans a specific node for detailed information.

**Usage:**
```bash
./debug network scan-node https://mainnet.accumulatenetwork.io/v3
./debug network scan-node 192.168.1.100:16695
```

**Features:**
- Detailed node information
- API endpoint status
- Partition participation
- Version information

#### `debug network status [network]`

Checks the overall status and health of the network.

**Usage:**
```bash
./debug network status mainnet
./debug network status --cached-scan network-scan.json mainnet
./debug network status --verbose testnet
```

**Flags:**
- `--cached-scan`: Use a cached network scan file
- `--verbose`: Enable detailed logging

**Features:**
- Network health assessment
- Validator status across partitions
- Block height synchronization
- API endpoint availability
- Zombie node detection

**Output Information:**
- Per-node status including:
  - Peer ID and operator URL
  - Partition participation and validator status
  - Current block heights
  - API connectivity (v3 support)
  - Version information
  - Host information

## Database Commands

### `debug database` (alias: `debug db`)

Database utilities for analysis, maintenance, and operations.

#### `debug db analyze [database-path]`

Analyzes database structure and content for debugging purposes.

**Usage:**
```bash
./debug db analyze /path/to/node/database
./debug db analyze --output analysis.json /path/to/database
```

**Features:**
- Database structure analysis
- Record count statistics
- Storage usage analysis
- Corruption detection

#### `debug db clone [source] [destination]`

Clones a database from one location to another.

**Usage:**
```bash
./debug db clone /source/db /destination/db
./debug db clone --compress /source/db /destination/db.gz
```

**Features:**
- Full database replication
- Optional compression
- Integrity verification
- Progress reporting

#### `debug db sync [database-path]`

Synchronizes database with network state.

**Usage:**
```bash
./debug db sync /path/to/database
./debug db sync --network mainnet /path/to/database
```

**Features:**
- State synchronization
- Missing record detection
- Automatic healing
- Progress tracking

#### `debug db patch [database-path] [patch-file]`

Applies patches to database for fixes or updates.

**Usage:**
```bash
./debug db patch /path/to/database patch.json
./debug db patch --dry-run /path/to/database patch.json
```

**Features:**
- Selective record updates
- Dry-run capability
- Backup creation
- Rollback support

#### `debug db serve [database-path]`

Serves database content via HTTP API for analysis.

**Usage:**
```bash
./debug db serve /path/to/database
./debug db serve --port 8080 /path/to/database
```

**Features:**
- HTTP API interface
- Key-value store access
- Query capabilities
- Web interface

#### `debug db serve-api [database-path]`

Serves a full Accumulate API from the database.

**Usage:**
```bash
./debug db serve-api /path/to/database
./debug db serve-api --port 16695 /path/to/database
```

**Features:**
- Full v3 API compatibility
- Query and submit endpoints
- Network simulation
- Testing environment

#### `debug db explore [database-path]`

Interactive database exploration tool.

**Usage:**
```bash
./debug db explore /path/to/database
```

**Features:**
- Interactive REPL interface
- Key-value browsing
- Query execution
- Record inspection

## Healing Commands

### `debug heal`

Network healing and synchronization utilities.

#### `debug heal anchor [network]`

Heals anchor chains and cross-partition messaging.

**Usage:**
```bash
./debug heal anchor mainnet
./debug heal anchor --pretend testnet
./debug heal anchor --continuous mainnet
```

**Flags:**
- `--pretend, -n`: Do not submit envelopes, only scan
- `--continuous`: Run healing in a loop every minute
- `--cached-scan`: Use cached network scan
- `--peer-db`: Track peers using persistent database
- `--wait`: Wait for message finalization

**Features:**
- Cross-partition message healing
- Anchor chain synchronization
- Missing transaction detection
- Automatic retry mechanisms

#### `debug heal synth [network]`

Heals synthetic transactions and system operations.

**Usage:**
```bash
./debug heal synth mainnet
./debug heal synth --light-db cache.db testnet
./debug heal synth --continuous mainnet
```

**Flags:**
- `--light-db`: Light client database for persisting chain data
- `--continuous`: Run healing in a loop
- `--max-response-age`: Maximum age of response before considered stale

**Features:**
- Synthetic transaction healing
- System operation recovery
- Light client integration
- Persistent state tracking

### Common Healing Flags

All healing commands support these common flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--cached-scan` | Use cached network scan | Auto-generated |
| `--peer-db` | Persistent peer database | `~/.accumulate/cache/{network}-peers.json` |
| `--debug` | Debug network requests | false |
| `--max-response-age` | Max response age | 1 minute |
| `--pprof` | Address for pprof server | disabled |

## Snapshot Commands

### `debug snapshot` (alias: `debug snap`)

Snapshot analysis and utility commands.

#### `debug snap rich-list [snapshot]` (alias: `debug snap rich`)

Extracts the most valuable accounts from a snapshot.

**Usage:**
```bash
./debug snap rich-list snapshot.snap
./debug snap rich --top 100 snapshot.snap
./debug snap rich --min 1000000 snapshot.snap > rich-accounts.csv
```

**Flags:**
- `--top N`: List the top N accounts (0 for all)
- `--min N`: Minimum balance threshold

**Features:**
- Account balance analysis
- Token distribution statistics
- CSV output format
- Configurable thresholds

**Output:**
- Account URLs
- Token balances
- Account types
- Ranking information

## Account Commands

### `debug account`

Account analysis and routing utilities.

#### `debug account id <url>`

Shows an account's internal ID and routing information.

**Usage:**
```bash
./debug account id acc://example.acme
./debug account id acc://example.acme/tokens
```

**Features:**
- Account ID calculation
- Routing information
- Partition assignment
- Hash computation

#### `debug account route <network-endpoint> <url>`

Calculates the routing path for an account.

**Usage:**
```bash
./debug account route https://mainnet.accumulatenetwork.io/v3 acc://example.acme
```

**Features:**
- Routing path calculation
- Partition determination
- Network topology analysis
- Load balancing information

## Genesis Commands

### `debug genesis`

Genesis block and initial state utilities.

#### `debug genesis ingest [output] [inputs...]`

Ingests multiple snapshots and merges them for genesis creation.

**Usage:**
```bash
./debug genesis ingest genesis.db partition1.snap partition2.snap partition3.snap
```

**Features:**
- Multi-partition snapshot merging
- System data stripping
- Genesis state preparation
- Database optimization

**Process:**
1. Reads multiple partition snapshots
2. Strips system-specific data
3. Merges into unified genesis database
4. Optimizes for initial network state

## Utility Commands

### `debug encode`

Encoding and decoding utilities for Accumulate data structures.

**Usage:**
```bash
./debug encode [data]
./debug sign [transaction]
```

**Features:**
- Protocol message encoding/decoding
- Transaction signing
- Hash computation
- Format conversion

### `debug verify`

Verification utilities for data integrity and signatures.

**Usage:**
```bash
./debug verify [file-or-data]
```

**Features:**
- Signature verification
- Data integrity checks
- Hash validation
- Protocol compliance

### `debug sequence`

Sequence number analysis and debugging.

**Usage:**
```bash
./debug sequence [account-url]
```

**Features:**
- Sequence number tracking
- Gap detection
- Synchronization analysis
- Chain integrity verification

### `debug watch-tx`

Transaction monitoring and tracking utilities.

**Usage:**
```bash
./debug watch-tx [transaction-hash]
```

**Features:**
- Real-time transaction tracking
- Status monitoring
- Cross-partition following
- Event logging

### `debug read-logs`

Log file analysis and parsing utilities.

**Usage:**
```bash
./debug read-logs [log-file]
```

**Features:**
- Structured log parsing
- Error extraction
- Performance analysis
- Event correlation

### `debug loadtest`

Load testing utilities for network stress testing.

**Usage:**
```bash
./debug loadtest [network]
```

**Features:**
- Configurable load generation
- Performance metrics
- Stress testing scenarios
- Network capacity analysis

### `debug comet`

CometBFT-specific debugging utilities.

**Usage:**
```bash
./debug comet [command]
```

**Features:**
- Consensus debugging
- Block analysis
- Validator monitoring
- Network consensus health

### `debug badger`

BadgerDB-specific utilities for database debugging.

**Usage:**
```bash
./debug badger [database-path]
```

**Features:**
- BadgerDB analysis
- Key-value inspection
- Performance tuning
- Corruption detection

### `debug peer-db`

Peer database management utilities.

**Usage:**
```bash
./debug peer-db [command]
```

**Features:**
- Peer information management
- Connection tracking
- Network topology caching
- Peer discovery optimization

### `debug check-node`

Node health checking and validation.

**Usage:**
```bash
./debug check-node [node-address]
```

**Features:**
- Node health assessment
- API endpoint validation
- Service availability
- Performance metrics

## Configuration and Caching

### Cache Directory

The debug app uses a cache directory for storing temporary data:
- **Location**: `~/.accumulate/cache/`
- **Contents**: Network scans, peer databases, light client data

### Cache Files

| File Pattern | Purpose |
|--------------|---------|
| `{network}-peers.json` | Peer database for network |
| `{network}.db` | Light client database |
| `network-scan-*.json` | Cached network scans |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ACCUMULATE_BOOTSTRAP` | Bootstrap servers | Built-in defaults |
| `ACCUMULATE_CACHE_DIR` | Cache directory | `~/.accumulate/cache` |

## Best Practices

### Network Analysis

1. **Use Cached Scans**: For repeated operations, use `--cached-scan` to avoid redundant network scanning
2. **JSON Output**: Use `--json` flag for automation and scripting
3. **Verbose Logging**: Enable `--verbose` for troubleshooting

### Database Operations

1. **Backup First**: Always backup databases before patching or modifying
2. **Dry Run**: Use `--dry-run` flags when available to test operations
3. **Monitor Progress**: Use progress indicators for long-running operations

### Healing Operations

1. **Start with Pretend**: Use `--pretend` to analyze issues before applying fixes
2. **Use Continuous Mode**: For ongoing maintenance, use `--continuous` flag
3. **Monitor Resources**: Healing can be resource-intensive, monitor system usage

### Performance Optimization

1. **Peer Database**: Use persistent peer databases to avoid repeated discovery
2. **Light Client**: Leverage light client databases for efficient state tracking
3. **Caching**: Utilize caching mechanisms for repeated operations

## Troubleshooting

### Common Issues

#### Network Connection Problems

**Symptoms:**
- Connection timeouts
- Peer discovery failures
- API endpoint errors

**Solutions:**
```bash
# Test network connectivity
./debug network scan-node https://mainnet.accumulatenetwork.io/v3

# Use alternative bootstrap servers
./debug --bootstrap https://alternative.server.com network scan mainnet

# Enable debug logging
./debug --debug network status mainnet
```

#### Database Issues

**Symptoms:**
- Corruption errors
- Performance degradation
- Missing records

**Solutions:**
```bash
# Analyze database health
./debug db analyze /path/to/database

# Synchronize with network
./debug db sync /path/to/database

# Clone to new location
./debug db clone /corrupted/db /new/db
```

#### Healing Problems

**Symptoms:**
- Persistent message failures
- Cross-partition sync issues
- Anchor chain problems

**Solutions:**
```bash
# Analyze without changes
./debug heal anchor --pretend mainnet

# Use fresh network scan
./debug heal anchor --cached-scan "" mainnet

# Enable detailed logging
./debug heal anchor --debug mainnet
```

### Performance Issues

#### Slow Operations

**Causes:**
- Large databases
- Network latency
- Resource constraints

**Solutions:**
- Use SSD storage for databases
- Increase system resources
- Use local network endpoints
- Enable caching mechanisms

#### Memory Usage

**Monitoring:**
```bash
# Enable pprof for memory profiling
./debug heal anchor --pprof :6060 mainnet
```

**Optimization:**
- Limit concurrent operations
- Use streaming where available
- Clear caches periodically

### Error Codes

| Exit Code | Meaning |
|-----------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Network error |
| 4 | Database error |
| 5 | Permission error |

### Getting Support

For additional support:

1. **Documentation**: Check the official Accumulate documentation
2. **Logs**: Enable verbose logging for detailed error information
3. **Community**: Join the Accumulate developer community
4. **Issues**: Report bugs on the official repository

## Advanced Usage Examples

### Network Health Monitoring

```bash
#!/bin/bash
# Network monitoring script
NETWORK="mainnet"
CACHE_DIR="$HOME/.accumulate/cache"

# Scan network and cache results
./debug network scan --json $NETWORK > "$CACHE_DIR/scan-$(date +%Y%m%d-%H%M%S).json"

# Check network status
./debug network status --cached-scan "$CACHE_DIR/scan-latest.json" $NETWORK

# Continuous healing
./debug heal anchor --continuous --peer-db "$CACHE_DIR/$NETWORK-peers.json" $NETWORK &
./debug heal synth --continuous --light-db "$CACHE_DIR/$NETWORK.db" $NETWORK &
```

### Database Maintenance

```bash
#!/bin/bash
# Database maintenance script
DB_PATH="/path/to/database"
BACKUP_PATH="/backup/database-$(date +%Y%m%d)"

# Create backup
./debug db clone "$DB_PATH" "$BACKUP_PATH"

# Analyze database
./debug db analyze "$DB_PATH" > analysis.json

# Synchronize with network
./debug db sync "$DB_PATH"

# Serve for inspection
./debug db serve-api --port 16695 "$DB_PATH" &
```

### Snapshot Analysis

```bash
#!/bin/bash
# Snapshot analysis pipeline
SNAPSHOT="partition.snap"

# Extract rich list
./debug snap rich --top 1000 "$SNAPSHOT" > rich-accounts.csv

# Analyze account distribution
./debug snap rich --min 1000000 "$SNAPSHOT" | wc -l

# Generate summary report
echo "Snapshot Analysis Report" > report.txt
echo "======================" >> report.txt
echo "Total accounts with >1M tokens: $(./debug snap rich --min 1000000 "$SNAPSHOT" | wc -l)" >> report.txt
echo "Top 10 accounts:" >> report.txt
./debug snap rich --top 10 "$SNAPSHOT" >> report.txt
```

This comprehensive reference covers all aspects of the Accumulate debug app, providing both basic usage instructions and advanced operational guidance for network administrators and developers.

## See Also

### Related Documentation
- [**network-initialization.md**](network-initialization.md) - Network initialization using debug commands
- [**network-json-structure.md**](network-json-structure.md) - Network configuration validation
- [**consensus-creation-workflow.md**](consensus-creation-workflow.md) - Consensus section management

### Cyclops Validator Documentation
- [**cyclops/cyclops-preparation.md**](cyclops/cyclops-preparation.md) - Using debug commands in validator prep
- [**cyclops/cyclops-deployment.md**](cyclops/cyclops-deployment.md) - Debug commands for deployment
- [**cyclops/cyclops-automation.md**](cyclops/cyclops-automation.md) - Automated debug workflows

### Technical References
- [**technical/snapshot-format.md**](technical/snapshot-format.md) - Snapshot format for debug snap commands
- [**technical/genesis-format.md**](technical/genesis-format.md) - Genesis format for debug genesis commands
- [**technical/record-format.md**](technical/record-format.md) - Database record format for debug db commands

### API Documentation
- [**api/analyze-commands.md**](api/analyze-commands.md) - Analyze tool commands (complementary to debug)
- [**api/accumulated-daemon-commands.md**](api/accumulated-daemon-commands.md) - Accumulated daemon commands

### Network References
- [**network/accumulate-mainnet-reference.md**](network/accumulate-mainnet-reference.md) - Mainnet debug procedures
- [**network/network-boot-procedures.md**](network/network-boot-procedures.md) - Network boot debugging

## Command Cross-Reference

### Network Operations
- `debug network scan` → See [network-initialization.md](network-initialization.md) for network setup
- `debug network status` → See [cyclops/cyclops-deployment.md](cyclops/cyclops-deployment.md) for validator status

### Snapshot Operations
- `debug snap collect` → See [network-initialization.md](network-initialization.md) for collection workflows
- `debug snap rich-list` → See [technical/snapshot-format.md](technical/snapshot-format.md) for format details

### Genesis Operations
- `debug genesis ingest` → See [network-initialization.md](network-initialization.md) for complete workflow
- Genesis creation → See [consensus-creation-workflow.md](consensus-creation-workflow.md)

### Database Operations
- `debug db analyze` → See [technical/record-format.md](technical/record-format.md) for record structure
- `debug db heal` → See [cyclops/cyclops-deployment.md](cyclops/cyclops-deployment.md) for healing procedures

### Healing Operations
- `debug heal anchor` → See [network/network-boot-procedures.md](network/network-boot-procedures.md)
- `debug heal synth` → See [cyclops/cyclops-automation.md](cyclops/cyclops-automation.md)

## Source Code References

- `tools/cmd/debug/main.go` - Debug app entry point
- `tools/cmd/debug/network.go` - Network scanning commands
- `tools/cmd/debug/db.go` - Database operation commands
- `tools/cmd/debug/snap.go` - Snapshot operation commands
- `tools/cmd/debug/genesis.go` - Genesis operation commands
- `tools/cmd/debug/heal_common.go` - Healing operation commands
- `tools/cmd/debug/account.go` - Account operation commands

---
*This document is part of the [Accumulate Network Documentation](README.md) - optimized for AI assistance and developer productivity.*
