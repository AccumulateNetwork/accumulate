# Quick Start: Deploy Follower from Local Backup

**Last Updated:** 2025-11-16

This guide shows you exactly how to deploy an Accumulate follower from a local database backup, step-by-step.

---

## Scenario

**You have:** A node backup directory on disk (e.g., from a previous validator or snapshot)
**You want:** A running follower node
**Time:** ~10 minutes

---

## Prerequisites Check

Before starting, verify you have:

### 1. **Complete Node Backup Directories**

You need TWO complete node directories (DN and BVN):

```bash
# Check DN database
ls -la /path/to/dn-backup/
# Should show:
#   config/
#   data/accumulate.db/
#   data/blockstore.db/
#   data/cs.wal/
#   data/state.db/

# Check BVN database
ls -la /path/to/bvn-backup/
# Should show the same structure
```

**Required Structure:**
```
node-backup/
├── config/
│   ├── accumulate.toml
│   ├── config.toml
│   ├── genesis.json (or .snap file)
│   ├── node_key.json
│   └── priv_validator_key.json
└── data/
    ├── accumulate.db/      ← MUST EXIST and have files
    │   ├── MANIFEST
    │   ├── *.vlog
    │   └── *.sst
    ├── blockstore.db/
    ├── cs.wal/
    ├── evidence.db/
    └── state.db/
```

### 2. **Sufficient Disk Space**

```bash
# Check available space
df -h /var/lib/

# You need: 100+ GB free
# - Each database: 20-50 GB
# - Operating room: 50 GB
```

### 3. **Docker Running**

```bash
# Verify Docker is running
docker ps

# If not running:
sudo systemctl start docker
```

### 4. **Optional: Genesis Files**

```bash
# Check if you have genesis snapshot files
ls ~/.accumulate/*genesis.snap

# These are helpful but not required if your backup is recent
```

---

## Deployment Methods

Choose the method that fits your needs:

### **Method 1: MCP Tools (Recommended)**

✅ **Best for:**
- Automated deployment
- Docker containers
- Preserving your backup
- Production use

**Go to:** [Method 1 - MCP Tools](#method-1-mcp-tools)

---

### **Method 2: Manual with `accumulated` Binary**

✅ **Best for:**
- Native (non-Docker) deployment
- Custom configurations
- Development/testing

**Go to:** [Method 2 - Manual Deployment](#method-2-manual-deployment)

---

## Method 1: MCP Tools

The MCP (Model Context Protocol) server provides automated deployment tools.

### Step 1: Get the MCP Server

```bash
cd /path/to/accumulate/mcp

# Build the server
go build -o mcp-server .

# Or download pre-built binary (if available)
```

### Step 2: Verify Your Backups

```bash
# Set your backup paths
export DN_BACKUP=/path/to/dn-backup
export BVN_BACKUP=/path/to/bvn-backup

# Verify they exist and have accumulate.db
test -d "$DN_BACKUP/data/accumulate.db" && echo "DN OK" || echo "DN MISSING accumulate.db"
test -d "$BVN_BACKUP/data/accumulate.db" && echo "BVN OK" || echo "BVN MISSING accumulate.db"
```

### Step 3: Run Deployment Script

```bash
# Use the complete deployment example
cd examples/

# Set your configuration
export DN_DATABASE="$DN_BACKUP"
export BVN_DATABASE="$BVN_BACKUP"
export WORK_DIR=/var/lib/accumulate-follower
export NETWORK=MainNet

# Run deployment
./deploy-follower-complete.sh
```

**What this does:**
1. Validates database backups
2. Checks for genesis files
3. Initializes follower (copies databases, creates config)
4. Starts follower in Docker
5. Verifies deployment

### Step 4: Verify Follower is Running

```bash
# Check container status
docker ps | grep accumulate-follower

# Check logs
docker logs -f accumulate-follower

# Check sync status
curl http://localhost:16592/status | jq '.result.sync_info'
```

**Success indicators:**
- Container status: "Up"
- Logs show: "Indexed block"
- Sync info shows increasing `latest_block_height`

### Step 5: Monitor Sync Progress

```bash
# Check current sync height
curl -s http://localhost:16592/status | jq '.result.sync_info | {height: .latest_block_height, catching_up: .catching_up}'

# Compare with network height
curl -s https://mainnet.accumulatenetwork.io/v3/status | jq '.result.sync_info.latest_block_height'
```

**Done!** Your follower is syncing. It may take hours to fully catch up depending on how old your backup is.

---

## Method 2: Manual Deployment

For native deployment without Docker.

### Step 1: Prepare Work Directory

```bash
# Create work directory
mkdir -p /var/lib/accumulate-follower
cd /var/lib/accumulate-follower

# Copy databases (PRESERVES originals)
cp -r /path/to/dn-backup ./dnn
cp -r /path/to/bvn-backup ./bvnn

# Verify copies
ls -la dnn/data/accumulate.db/
ls -la bvnn/data/accumulate.db/
```

### Step 2: Create Configuration File

Create `accumulate.toml`:

```toml
[run]
network = "MainNet"

[[run.node]]
name = "follower"
mode = "dual"

listen = [
  "/ip4/0.0.0.0/tcp/16591"
]

dn-bootstrap-peers = [
  "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
]

bvn-bootstrap-peers = [
  "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
]

# DN partition
[[run.node.partition]]
type = "directory"
id = "Directory"
listen = ["/ip4/0.0.0.0/tcp/16591"]

# BVN partition (adjust for your BVN - Cyclops, Apollo, etc.)
[[run.node.partition]]
type = "block-validator"
id = "Cyclops"
listen = ["/ip4/0.0.0.0/tcp/16691"]
```

**Important:** Adjust the `id` on the BVN partition to match your backup (Cyclops, Apollo, etc.).

### Step 3: Copy Genesis Files (if available)

```bash
# If you have genesis snapshot files
cp ~/.accumulate/dn-genesis.snap ./
cp ~/.accumulate/bvn1-genesis.snap ./bvn-genesis.snap

# Or copy from backup
cp /path/to/dn-backup/dn-genesis.snap ./
cp /path/to/bvn-backup/bvn-genesis.snap ./
```

### Step 4: Run Follower

```bash
# From /var/lib/accumulate-follower
accumulated run-dual dnn bvnn
```

**Expected output:**
```
INFO Starting consensus state  module=consensus
INFO Started node               module=main
```

### Step 5: Verify in Another Terminal

```bash
# Check status
curl http://localhost:16591/status

# Check sync progress
curl http://localhost:16591/status | jq '.result.sync_info'
```

---

## Troubleshooting

### Error: "source database not found"

**Cause:** Database path doesn't exist or is wrong

**Fix:**
```bash
# Verify paths
ls -la /path/to/dn-backup
ls -la /path/to/bvn-backup

# Check if they're directories
test -d /path/to/dn-backup && echo "DN exists" || echo "DN not found"
```

---

### Error: "accumulate.db directory is empty"

**Cause:** Database snapshot is corrupted or incomplete

**Fix:**
```bash
# Check if database has files
ls -la /path/to/backup/data/accumulate.db/

# Should show MANIFEST, *.vlog, *.sst files
# If empty, your backup is corrupted - use a different backup
```

---

### Error: "read dnn: is a directory"

**Cause:** Trying to pass directory where file is expected, or missing config files

**Fix:**
```bash
# Verify structure
ls -la dnn/config/

# Should have config.toml and accumulate.toml
# If missing, run: accumulated init dual --help
```

---

### Error: "Unsupported network type PartitionType:0"

**Cause:** Protocol incompatibility or version mismatch

**Fix:**
```bash
# Option 1: Use MCP tools instead
./examples/deploy-follower-complete.sh

# Option 2: Skip version check
accumulated init dual --skip-version-check tcp://peer:port

# Option 3: Use backups directly (Method 1)
```

---

### Error: "port already in use"

**Cause:** Ports 16591 or 16691 already occupied

**Fix:**
```bash
# Find what's using the port
sudo netstat -tulpn | grep -E '16591|16691'

# Stop conflicting service or use different ports
```

---

### Follower Not Syncing

**Symptoms:** `catching_up: true` but block height not increasing

**Checks:**
```bash
# 1. Check peer connections
curl http://localhost:16592/net_info | jq '.result.n_peers'
# Should be > 0

# 2. Check logs for errors
docker logs accumulate-follower | grep -i error

# 3. Verify bootstrap peers are reachable
curl http://23.22.212.106:16592/status
```

---

## Compatibility Matrix

| Backup Source | Works with MCP? | Works with Manual? | Notes |
|---------------|-----------------|--------------------|----|
| **Validator node** | ✅ Yes | ✅ Yes | Complete structure |
| **Follower node** | ✅ Yes | ✅ Yes | Complete structure |
| **Database-only export** | ⚠️ Partial | ❌ No | Missing config/, won't work manually |
| **Corrupted snapshot** | ❌ No | ❌ No | Empty accumulate.db/ |
| **Old backup (6+ months)** | ✅ Yes | ✅ Yes | Will take long time to sync |

---

## Performance Expectations

### Initial Deployment
- **Time:** 5-10 minutes
- **Disk I/O:** High (copying databases)
- **Network:** Minimal (local files)

### Sync Time (from backup)
| Backup Age | Expected Sync Time |
|------------|-------------------|
| 1 day old | 10-30 minutes |
| 1 week old | 1-2 hours |
| 1 month old | 4-8 hours |
| 6+ months old | 24-48 hours |

**Factors affecting sync speed:**
- Network bandwidth
- Disk I/O speed
- Number of bootstrap peers
- Network transaction volume

---

## Next Steps

### Monitor Your Follower

```bash
# Continuous sync status
watch -n 10 'curl -s http://localhost:16592/status | jq ".result.sync_info | {height, catching_up}"'

# Check peer count
curl -s http://localhost:16592/net_info | jq '.result.n_peers'

# View logs
docker logs -f accumulate-follower
```

### Query the Follower

Once synced, you can query your follower:

```bash
# Check account balance
curl -X POST http://localhost:16592/v3 \
  -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"url":"acc://your.acme/tokens"}}'

# Check network status
curl http://localhost:16592/v3/metrics
```

### Backup Your Follower

```bash
# Stop follower
docker stop accumulate-follower

# Create backup
tar -czf follower-backup-$(date +%Y-%m-%d).tar.gz \
  /var/lib/accumulate-follower/dnn/data \
  /var/lib/accumulate-follower/bvnn/data

# Restart follower
docker start accumulate-follower
```

---

## Additional Resources

- **Full MCP Documentation:** [FOLLOWER_DOCKER_GUIDE.md](FOLLOWER_DOCKER_GUIDE.md)
- **Troubleshooting Guide:** [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Genesis Files:** [GENESIS_FILES_GUIDE.md](GENESIS_FILES_GUIDE.md)
- **MCP vs Accman:** [MCP_ARCHITECTURE.md](MCP_ARCHITECTURE.md)
- **Integration Examples:** [examples/README.md](examples/README.md)

---

## Common Paths Reference

### Mainnet Follower (Docker):
- **DN API:** `http://localhost:16591`
- **BVN API:** `http://localhost:16691`
- **DN RPC:** `http://localhost:16592`
- **BVN RPC:** `http://localhost:16692`

### File Locations:
- **Work directory:** `/var/lib/accumulate-follower/`
- **DN database:** `/var/lib/accumulate-follower/dnn/data/accumulate.db/`
- **BVN database:** `/var/lib/accumulate-follower/bvnn/data/accumulate.db/`
- **Config:** `/var/lib/accumulate-follower/accumulate.toml`
- **Genesis files:** `/var/lib/accumulate-follower/*.snap`

---

**Summary:**
This guide walked you through deploying an Accumulate follower from a local backup using either MCP tools (automated, Docker) or manual deployment (native binary). Choose the method that fits your needs and follow the step-by-step instructions.

For automated deployment with validation and error handling, use **Method 1 (MCP Tools)**.
For custom native deployment with full control, use **Method 2 (Manual)**.
