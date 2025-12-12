# Deploying an Accumulate Follower Node

## Overview

A follower node tracks the Accumulate blockchain without participating in consensus. Followers:
- **Do NOT sign blocks** (voting_power = 0)
- **Use transient validator keys** (generated randomly on startup)
- **Follow network consensus** without block production
- **Require NO registration** or permission from the network

**Use Cases:**
- Running a local copy of the blockchain
- Building applications that query blockchain data
- Operating infrastructure for wallets/explorers
- Testing and development

---

## Prerequisites

### System Requirements
- **CPU:** 4+ cores recommended
- **RAM:** 8 GB minimum, 16 GB recommended
- **Disk:** 50 GB minimum (grows over time)
- **Network:** Stable internet connection, public IP recommended
- **OS:** Linux (Ubuntu 20.04+ or similar)

### Software Requirements
- Docker (for containerized deployment)
- OR: Go 1.21+ (for source build)

### Network Ports
Open the following ports for P2P and API access:

**Directory Network (DN):**
- `16591/tcp` - P2P (CometBFT gossip)
- `16592/tcp` - RPC (CometBFT HTTP API)
- `16593/tcp` - API (Accumulate HTTP API)

**Block Validator Network (BVN):**
- `16691/tcp` - P2P (CometBFT gossip)
- `16692/tcp` - RPC (CometBFT HTTP API)
- `16693/tcp` - API (Accumulate HTTP API)

---

## Quick Start (Docker)

### 1. Obtain Genesis Snapshots

Genesis snapshots initialize the blockchain state. You have two options:

**Option A: Sync from Genesis (slower, complete history)**
```bash
# Download genesis snapshots from public source
# MainNet genesis files are typically ~2 GB
wget https://[genesis-snapshot-url]/directory-genesis.snap
wget https://[genesis-snapshot-url]/cyclops-genesis.snap
```

**Option B: Recent Snapshot (faster, recommended)**
```bash
# Use a recent snapshot from a trusted validator
# This allows fast-sync to current height
# Contact network operators for snapshot access
```

### 2. Find Bootstrap Peers

Bootstrap peers are validators your follower connects to for P2P gossip.

**Known MainNet Validators:**

See [bootstrap-peers.md](bootstrap-peers.md) for current list.

**BVN0 Production Validator (23.22.212.106):**
- CometBFT Node ID: `3029240e829e58e399bc7b6115bb6bc947cc24c7`
- Convert to libp2p format (see step 3)

### 3. Convert Node IDs to libp2p Format

Accumulate uses libp2p multiaddr format, not raw CometBFT node IDs.

**Using convert-node-id tool:**
```bash
cd ~/accumulate
go run ./tools/cmd/convert-node-id 3029240e829e58e399bc7b6115bb6bc947cc24c7
# Output: QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD
```

See [convert-node-id README](../../tools/cmd/convert-node-id/README.md) for details.

### 4. Create Configuration File

Create `accumulate.toml`:

```toml
# Network selection
network = "MainNet"

# Follower configuration
[[configurations]]
  # CRITICAL: Use "follower" type (not "coreValidator")
  type = "follower"

  # Mode: "dual" runs both DN and BVN partitions
  # Options: "dn", "bvn", "dual"
  mode = "dual"

  # BVN partition name (for MainNet Cyclops BVN)
  bvn = "Cyclops"

  # Genesis snapshot files
  dn-genesis = "directory-genesis.snap"
  bvn-genesis = "cyclops-genesis.snap"

  # P2P listen address
  listen = "/ip4/0.0.0.0/tcp/16591"

  # Storage backend
  # Options: "badger" (default), "levelDB", "bolt"
  storage-type = "badger"

  # Disable these for followers
  enable-healing = false
  enable-snapshots = false

  # Bootstrap peers for Directory Network
  # Format: /ip4/<ip>/tcp/<port>/p2p/<peer-id>
  dn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

  # Bootstrap peers for BVN
  bvn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

# Logging configuration
[logging]
  format = "plain"

  [[logging.rules]]
    level = "info"
```

**Important Configuration Notes:**

- **type = "follower"**: This is CRITICAL. Using "coreValidator" will attempt to sign blocks.
- **Bootstrap peers**: Add at least 2-3 peers for redundancy. Single peer = single point of failure.
- **mode = "dual"**: Runs both DN and BVN. Use "dn" or "bvn" for single partition.
- **Storage type**: "badger" is default and well-tested. "levelDB" and "bolt" are alternatives.

### 5. Deploy with Docker

**Create data directory:**
```bash
mkdir -p /var/lib/accumulate-follower
cd /var/lib/accumulate-follower

# Place genesis snapshots here
cp directory-genesis.snap .
cp cyclops-genesis.snap .
cp accumulate.toml .
```

**Run follower:**
```bash
docker run -d \
  --name accumulate_follower \
  --restart unless-stopped \
  -v /var/lib/accumulate-follower:/node \
  -p 16591-16593:16591-16593 \
  -p 16691-16693:16691-16693 \
  registry.gitlab.com/accumulatenetwork/accumulate:latest \
  run-dual /node/dnn /node/bvnn
```

**For single partition:**
```bash
# DN only
docker run -d ... accumulated run /node/dnn

# BVN only
docker run -d ... accumulated run /node/bvnn
```

### 6. Verify Deployment

**Check container status:**
```bash
docker ps --filter name=accumulate_follower
docker logs -f accumulate_follower
```

**Check sync status:**
```bash
# Directory Network
curl -s http://localhost:16592/status | jq '.result | {
  network: .node_info.network,
  height: .sync_info.latest_block_height,
  catching_up: .sync_info.catching_up,
  voting_power: .validator_info.voting_power
}'

# Expected output:
# {
#   "network": "MainNet.Directory",
#   "height": "7694000",
#   "catching_up": true,
#   "voting_power": "0"   <- MUST be 0 for followers
# }

# Block Validator Network
curl -s http://localhost:16692/status | jq '.result | {
  network: .node_info.network,
  height: .sync_info.latest_block_height,
  catching_up: .sync_info.catching_up,
  voting_power: .validator_info.voting_power
}'
```

**CRITICAL CHECKS:**
- ✅ `voting_power` MUST be `"0"` - if not, you're running as a validator!
- ✅ `catching_up` should be `true` initially (syncing)
- ✅ `height` should be increasing over time

**Check P2P connections:**
```bash
# Check peer count
curl -s http://localhost:16592/net_info | jq '.result.n_peers'

# View connected peers
curl -s http://localhost:16592/net_info | jq '.result.peers[] | {
  moniker: .node_info.moniker,
  remote_ip: .remote_ip
}'
```

**Expected:** At least 1 peer per partition. More peers = better redundancy.

---

## Automated Deployment Tools

For simplified deployment and monitoring, use the provided tools:

### deploy-follower

Automates follower deployment from snapshots:

```bash
cd tools/deploy-follower
go build -o deploy-follower

./deploy-follower \
  --work-dir /path/to/follower-data \
  --dn-snapshot /path/to/directory.snap \
  --bvn-snapshot /path/to/cyclops.snap \
  --accumulated /path/to/accumulated \
  --network mainnet \
  --bvn Cyclops \
  --start
```

This tool:
- Creates the required directory structure
- Initializes partitions from snapshots
- Generates configuration files
- Creates start/stop scripts

See [deploy-follower README](../../tools/deploy-follower/README.md) for full documentation.

### follower-monitor

Web-based dashboard for monitoring and controlling followers:

```bash
cd tools/follower-monitor
go build -o follower-monitor

./follower-monitor --work-dir /path/to/follower-data
```

Features:
- Real-time sync status monitoring
- Progress tracking against mainnet
- Live log viewing with filtering
- Start/stop control via web UI

Default: http://localhost:9999 (bound to localhost for security)

See [follower-monitor README](../../tools/follower-monitor/README.md) for full documentation.

---

## Source Build (Alternative to Docker)

### 1. Build from Source

```bash
git clone https://gitlab.com/accumulatenetwork/accumulate.git
cd accumulate
git checkout <version-tag>  # e.g., v1.4.3-fix-the-fix
make build
```

### 2. Install Binary

```bash
sudo cp accumulated /usr/local/bin/
```

### 3. Setup Data Directory

```bash
sudo mkdir -p /var/lib/accumulate-follower
cd /var/lib/accumulate-follower

# Copy genesis snapshots
sudo cp /path/to/directory-genesis.snap .
sudo cp /path/to/cyclops-genesis.snap .

# Create configuration
sudo nano accumulate.toml
# (paste configuration from step 4 above)
```

### 4. Create Systemd Service

Create `/etc/systemd/system/accumulate-follower.service`:

```ini
[Unit]
Description=Accumulate Follower Node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=accumulate
Group=accumulate
WorkingDirectory=/var/lib/accumulate-follower
ExecStart=/usr/local/bin/accumulated run-dual /var/lib/accumulate-follower/dnn /var/lib/accumulate-follower/bvnn
Restart=on-failure
RestartSec=10
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

### 5. Start Service

```bash
# Create service user
sudo useradd -r -s /bin/false accumulate
sudo chown -R accumulate:accumulate /var/lib/accumulate-follower

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable accumulate-follower
sudo systemctl start accumulate-follower

# Check status
sudo systemctl status accumulate-follower
sudo journalctl -u accumulate-follower -f
```

---

## Configuration Options

### Partition Modes

**Dual Mode (Both DN and BVN):**
```toml
mode = "dual"
bvn = "Cyclops"
dn-genesis = "directory-genesis.snap"
bvn-genesis = "cyclops-genesis.snap"
dn-bootstrap-peers = [...]
bvn-bootstrap-peers = [...]
```

**DN Only:**
```toml
mode = "dn"
dn-genesis = "directory-genesis.snap"
dn-bootstrap-peers = [...]
```

**BVN Only:**
```toml
mode = "bvn"
bvn = "Cyclops"
bvn-genesis = "cyclops-genesis.snap"
bvn-bootstrap-peers = [...]
```

### Storage Types

**Badger (Default):**
```toml
storage-type = "badger"
```
- Default and well-tested
- Good performance
- Recommended for most deployments

**LevelDB:**
```toml
storage-type = "levelDB"
```
- Alternative key-value store
- Lower memory usage than Badger
- Good for resource-constrained systems

**Bolt:**
```toml
storage-type = "bolt"
```
- Another alternative
- Simple and reliable
- Good for testing

### Logging Configuration

**Minimal Logging:**
```toml
[logging]
  format = "plain"

  [[logging.rules]]
    level = "info"
```

**Debug P2P Issues:**
```toml
[logging]
  format = "plain"

  [[logging.rules]]
    level = "info"

  [[logging.rules]]
    level = "debug"
    modules = ["p2p", "pex", "addrbook"]
```

**Debug Consensus:**
```toml
  [[logging.rules]]
    level = "debug"
    modules = ["consensus", "blocksync"]
```

**Full Debug (Not Recommended for Production):**
```toml
  [[logging.rules]]
    level = "debug"
```

---

## Sync Strategies

### Option 1: Genesis Sync (Slowest, Complete History)

**Pros:**
- Full blockchain history from block 0
- Independent verification of entire chain
- No trust in snapshot provider

**Cons:**
- Very slow (days to weeks)
- High bandwidth usage
- Large disk space requirement

**Setup:**
- Use genesis snapshots
- Configure bootstrap peers
- Wait for full sync

**Estimated Time:** 1-2 weeks for full MainNet sync

### Option 2: Recent Snapshot (Fast, Recommended)

**Pros:**
- Fast sync (hours to days)
- Reduced bandwidth
- Smaller initial disk usage

**Cons:**
- Requires trusted snapshot source
- No history before snapshot height

**Setup:**
1. Obtain recent snapshot from trusted validator
2. Copy snapshot databases to data directory
3. Start follower - syncs from snapshot height forward

**Estimated Time:** 1-2 days to catch up to current

### Option 3: State Sync (Fastest, Future)

State sync is a future enhancement. See FOLLOWER-TYPE-README.md "Future Improvements."

---

## Troubleshooting

### Problem: voting_power > 0

**Symptom:**
```json
{
  "voting_power": "1"  // Should be "0"!
}
```

**Cause:** Follower is running as a validator

**Solution:**
1. Check `accumulate.toml`: MUST have `type = "follower"`
2. Verify TransientPrivateKey is being used (automatic with type=follower)
3. Ensure you didn't copy validator keys from another node
4. Restart container/service

### Problem: Not Syncing (Height Not Increasing)

**Symptom:**
- Height stuck at genesis or snapshot height
- `catching_up = false` but behind network

**Diagnosis:**
```bash
# Check peer connections
curl -s http://localhost:16592/net_info | jq '.result.n_peers'
# Should be > 0
```

**Causes & Solutions:**

**1. No P2P Peers Connected**
```bash
# Check if bootstrap peers are reachable
nc -zv 23.22.212.106 16591
```
- Solution: Verify bootstrap peer IPs/ports in config
- Solution: Check firewall allows outbound connections
- Solution: Add more bootstrap peers

**2. Wrong Peer IDs**
- Solution: Use `convert-node-id` tool to get correct libp2p format
- Solution: Verify peer IDs match actual validators

**3. Genesis Mismatch**
- Solution: Ensure genesis files match network (MainNet vs TestNet)
- Solution: Re-download genesis from official source

**4. Database Corruption**
- Solution: Delete data directory and restart from genesis/snapshot

### Problem: High Memory Usage

**Symptom:** Container/process using excessive RAM

**Solutions:**
1. Switch to LevelDB storage (lower memory)
```toml
storage-type = "levelDB"
```

2. Run single partition instead of dual
```toml
mode = "dn"  # or "bvn"
```

3. Increase Docker memory limit
```bash
docker run --memory="4g" ...
```

### Problem: Disk Full

**Symptom:** Sync stops, errors about disk space

**Solutions:**
1. Monitor disk usage
```bash
du -sh /var/lib/accumulate-follower/*
```

2. Enable snapshot pruning (when available)

3. Run on larger disk
```bash
# Move to larger volume
sudo mv /var/lib/accumulate-follower /mnt/large-disk/
sudo ln -s /mnt/large-disk/accumulate-follower /var/lib/accumulate-follower
```

### Problem: Slow Sync

**Symptom:** Blocks syncing very slowly

**Solutions:**
1. Add more bootstrap peers (2-5 recommended)

2. Check network bandwidth
```bash
iftop  # Monitor network usage
```

3. Use recent snapshot instead of genesis sync

4. Check peer connection quality
```bash
curl -s http://localhost:16592/net_info | jq '.result.peers[]'
```

### Problem: Container Crashes/Restarts

**Check logs:**
```bash
docker logs accumulate_follower --tail 100
```

**Common causes:**
- Out of memory: Add `--memory` limit
- Corrupted database: Remove data directory and resync
- Config error: Validate `accumulate.toml` syntax
- Port conflict: Check ports aren't already in use

---

## Monitoring

### Health Checks

**Basic Health Check Script:**
```bash
#!/bin/bash
# health-check.sh

DN_HEIGHT=$(curl -s http://localhost:16592/status | jq -r '.result.sync_info.latest_block_height')
BVN_HEIGHT=$(curl -s http://localhost:16692/status | jq -r '.result.sync_info.latest_block_height')

echo "DN Height: $DN_HEIGHT"
echo "BVN Height: $BVN_HEIGHT"

# Alert if heights haven't changed in 10 minutes
# (implement your alerting logic)
```

### Metrics to Monitor

1. **Block Height** - Should increase continuously
2. **Peer Count** - Should be > 0, ideally 2-5
3. **Disk Usage** - Monitor growth, plan capacity
4. **Memory Usage** - Watch for leaks
5. **voting_power** - MUST remain 0

### Prometheus Metrics

Accumulate exposes Prometheus metrics (when configured):
```toml
[configurations]
  # ... other config ...

  metrics-namespace = "accumulate"
```

Access metrics:
```bash
curl http://localhost:16593/metrics
```

---

## Upgrading

### Docker Upgrade

1. **Stop follower:**
```bash
docker stop accumulate_follower
```

2. **Pull new image:**
```bash
docker pull registry.gitlab.com/accumulatenetwork/accumulate:<new-version>
```

3. **Remove old container (data preserved):**
```bash
docker rm accumulate_follower
```

4. **Start with new image:**
```bash
docker run -d \
  --name accumulate_follower \
  -v /var/lib/accumulate-follower:/node \
  -p 16591-16593:16591-16593 \
  -p 16691-16693:16691-16693 \
  registry.gitlab.com/accumulatenetwork/accumulate:<new-version> \
  run-dual /node/dnn /node/bvnn
```

### Source Upgrade

1. **Stop service:**
```bash
sudo systemctl stop accumulate-follower
```

2. **Build new version:**
```bash
cd ~/accumulate
git fetch --tags
git checkout <new-version>
make build
sudo cp accumulated /usr/local/bin/
```

3. **Start service:**
```bash
sudo systemctl start accumulate-follower
```

### Checking Version

```bash
accumulated version
```

---

## Security Considerations

### Firewall Configuration

**Minimal (P2P only):**
```bash
# Allow P2P inbound (recommended for better connectivity)
sudo ufw allow 16591/tcp  # DN P2P
sudo ufw allow 16691/tcp  # BVN P2P
```

**Full Access (API exposed):**
```bash
# Add API ports if serving public
sudo ufw allow 16592/tcp  # DN RPC
sudo ufw allow 16593/tcp  # DN API
sudo ufw allow 16692/tcp  # BVN RPC
sudo ufw allow 16693/tcp  # BVN API
```

**Production recommendation:**
- Expose P2P ports publicly
- Restrict API ports to internal network
- Use reverse proxy (nginx) for public API access

### Data Security

- Follower nodes contain NO private keys (transient keys only)
- Blockchain data is public (no encryption needed)
- Secure the host system with standard practices
- Regular backups not critical (can re-sync)

---

## Performance Tuning

### For High-Performance Systems

**Use Badger with tuning:**
```toml
storage-type = "badger"
```

**Allocate sufficient resources:**
```bash
docker run --cpus="4" --memory="8g" ...
```

### For Resource-Constrained Systems

**Use LevelDB:**
```toml
storage-type = "levelDB"
```

**Run single partition:**
```toml
mode = "dn"  # or "bvn"
```

**Limit logging:**
```toml
[[logging.rules]]
  level = "error"  # Only log errors
```

---

## FAQ

**Q: Can I run a follower on the same machine as a validator?**
A: Yes, but use different ports and data directories. Not recommended for production validators.

**Q: Do I need to register my follower with the network?**
A: No. Followers are permissionless and require no registration.

**Q: How much disk space will I need long-term?**
A: Estimate ~10-50 MB/day per partition. Plan for at least 100 GB for long-term operation.

**Q: Can I pause and resume sync?**
A: Yes. Stop the follower, data is preserved, restart continues from last block.

**Q: What happens if my follower goes offline?**
A: It will catch up when restarted. If offline very long (weeks), may need fresh snapshot.

**Q: Can I use my follower for transaction submission?**
A: Yes, followers can accept and forward transactions like validators.

**Q: What's the difference between a follower and a validator?**
A: Validators sign blocks (consensus), followers just observe. Followers have voting_power=0.

---

## Additional Resources

- **Bootstrap Peers:** See [bootstrap-peers.md](bootstrap-peers.md)
- **Tool Documentation:** [convert-node-id](../../tools/cmd/convert-node-id/README.md)
- **Technical Details:** [FOLLOWER-TYPE-README.md](../../cmd/accumulated/run/FOLLOWER-TYPE-README.md)
- **Main Documentation:** https://docs.accumulatenetwork.io/
- **Network Status:** https://explorer.accumulatenetwork.io/

---

## Support

- **Issues:** https://gitlab.com/accumulatenetwork/accumulate/-/issues
- **Discord:** [Accumulate Discord Server]
- **Forum:** [Community Forum]

---

*Last Updated: 2025-12-07*
