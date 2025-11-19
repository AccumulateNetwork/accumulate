# Accumulate MCP → Accman Integration Guide

Complete guide for preparing follower deployment artifacts using the Accumulate MCP and deploying them with accman.

## Overview

The Accumulate MCP now provides tools to prepare all artifacts needed by accman to deploy followers to mainnet or testnet. This creates a seamless workflow from database snapshots to production deployment.

## Workflow

```
Database Snapshots → Accumulate MCP → Deployment Artifacts → Accman → Running Follower
```

1. **Accumulate MCP**: Prepare deployment artifacts from database snapshots
2. **Accman**: Deploy follower using the prepared artifacts

## New MCP Tools

### 1. `accumulate_prepare_accman_artifacts`

**One-command artifact preparation** - Creates everything accman needs for deployment.

**What it creates:**
- ✅ DN database archive (tar.gz)
- ✅ BVN database archive (tar.gz)
- ✅ Deployment metadata (JSON)
- ✅ Deployment script (bash)
- ✅ Bootstrap peer configuration

**Usage:**
```json
{
  "tool": "accumulate_prepare_accman_artifacts",
  "arguments": {
    "dn_node_dir": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_node_dir": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "output_dir": "/home/paul/accman-artifacts",
    "network": "mainnet",
    "partition": "dual"
  }
}
```

**Response:**
```json
{
  "status": "success",
  "artifacts": {
    "dn_archive": "/home/paul/accman-artifacts/dn-node-mainnet-20251115-143022.tar.gz",
    "bvn_archive": "/home/paul/accman-artifacts/bvn-node-mainnet-20251115-143022.tar.gz",
    "metadata_file": "/home/paul/accman-artifacts/deployment-metadata-20251115-143022.json",
    "deployment_script": "/home/paul/accman-artifacts/deploy-20251115-143022.sh"
  },
  "metadata": {
    "network": "mainnet",
    "partition": "dual",
    "dn_snapshot": ".../dn-database-mainnet-20251115-143022.tar.gz",
    "bvn_snapshot": ".../bvn-database-mainnet-20251115-143022.tar.gz",
    "dn_bootstrap_peers": [...],
    "bvn_bootstrap_peers": [...],
    "created_at": "2025-11-15T14:30:22Z"
  },
  "deployment": {
    "command": "accman-mcp deploy_follower ...",
    "script": "/home/paul/accman-artifacts/deploy-20251115-143022.sh"
  },
  "next_steps": [
    "Transfer artifacts to deployment server",
    "Run deployment script: bash deploy-20251115-143022.sh",
    "Or use accman-mcp network_bootstrap_and_deploy"
  ]
}
```

### 2. `accumulate_create_node_archive`

Create individual node directory archives (includes CometBFT config/ and data/).

**Usage:**
```json
{
  "tool": "accumulate_create_node_archive",
  "arguments": {
    "node_dir": "/media/paul/Expansion/databases/2025-10-13-dn",
    "output_file": "/tmp/dn-node.tar.gz",
    "node_type": "DN"
  }
}
```

**Response:**
```json
{
  "status": "success",
  "archive": "/tmp/dn-node.tar.gz",
  "source": "/media/paul/Expansion/databases/2025-10-13-dn",
  "node_type": "DN",
  "size_bytes": 1073741824,
  "size_human": "1.0 GiB",
  "created_at": "2025-11-15T14:30:22Z",
  "structure": {
    "includes": [
      "config/ - CometBFT configuration",
      "data/ - CometBFT and Accumulate data"
    ]
  }
}
```

### 3. `accumulate_get_bootstrap_peers`

Get default bootstrap peers for a network.

**Usage:**
```json
{
  "tool": "accumulate_get_bootstrap_peers",
  "arguments": {
    "network": "mainnet"
  }
}
```

**Response:**
```json
{
  "network": "mainnet",
  "dn_bootstrap_peers": [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ],
  "bvn_bootstrap_peers": [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ],
  "note": "These are default bootstrap peers. Update based on current network topology."
}
```

## Complete Deployment Workflow

### Step 1: Prepare Artifacts (Accumulate MCP)

Use your 2025-10-13 database snapshots:

```json
{
  "tool": "accumulate_prepare_accman_artifacts",
  "arguments": {
    "dn_node_dir": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_node_dir": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "output_dir": "/home/paul/mainnet-deployment",
    "network": "mainnet",
    "partition": "dual"
  }
}
```

**Creates:**
```
/home/paul/mainnet-deployment/
├── dn-node-mainnet-20251115-143022.tar.gz
├── bvn-node-mainnet-20251115-143022.tar.gz
├── deployment-metadata-20251115-143022.json
├── deploy-20251115-143022.sh
└── verify-artifacts-20251115-143022.sh
```

### Step 2: Deploy with Accman

**Option A: Use the generated script**

```bash
bash /home/paul/mainnet-deployment/deploy-20251115-143022.sh
```

The script automatically runs:
```bash
accman-mcp deploy_follower \
  --partition dual \
  --dn-snapshot /home/paul/mainnet-deployment/dn-node-mainnet-20251115-143022.tar.gz \
  --bvn-snapshot /home/paul/mainnet-deployment/bvn-node-mainnet-20251115-143022.tar.gz \
  --use-volume \
  --network mainnet
```

**Option B: Manual accman-mcp call**

```bash
accman-mcp deploy_follower \
  --partition dual \
  --dn-snapshot /path/to/dn-node.tar.gz \
  --bvn-snapshot /path/to/bvn-node.tar.gz \
  --use-volume
```

**Option C: One-command bootstrap and deploy (accman)**

If accman can download snapshots itself:

```bash
accman-mcp network_bootstrap_and_deploy \
  --network mainnet \
  --partition dual \
  --use-volume
```

### Step 3: Monitor Deployment

```bash
# Check container status
docker ps | grep accumulate-follower

# View logs
docker logs -f accumulate-follower-dual

# Query endpoints
curl http://localhost:52001/status | jq
curl http://localhost:52101/status | jq
```

## Artifact Details

### Node Archives

**Format:** tar.gz compressed archives
**Contents:** Complete node directory structure with CometBFT and Accumulate data
**Naming:** `{partition}-node-{network}-{timestamp}.tar.gz`

**Example:**
```
dn-node-mainnet-20251115-143022.tar.gz
└── 2025-10-13-dn/
    ├── config/
    │   ├── config.toml
    │   ├── tendermint.toml
    │   └── addrbook.json
    └── data/
        ├── accumulate.db/
        ├── blockstore.db/
        ├── state.db/
        └── tx_index.db/
```

### Deployment Metadata

**Format:** JSON
**Purpose:** Machine-readable deployment configuration

**Example:**
```json
{
  "network": "mainnet",
  "partition": "dual",
  "dn_snapshot": "/path/to/dn-node-mainnet-20251115-143022.tar.gz",
  "bvn_snapshot": "/path/to/bvn-node-mainnet-20251115-143022.tar.gz",
  "dn_bootstrap_peers": [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ],
  "bvn_bootstrap_peers": [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ],
  "created_at": "2025-11-15T14:30:22Z",
  "source_nodes": {
    "dn": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn": "/media/paul/Expansion/databases/2025-10-13-bvn"
  }
}
```

### Deployment Script

**Format:** Bash script
**Purpose:** Automated deployment

**Contains:**
- Accman availability check
- Full deployment command
- Post-deployment verification steps
- Monitoring commands

## Network Configurations

### Mainnet

**Default Bootstrap Peers:**
- DN: `/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD`
- BVN: `/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD`

**Ports:**
- DN: 52000-52002 (P2P, RPC, API)
- BVN: 52100-52102 (P2P, RPC, API)

### Testnet

**Default Bootstrap Peers:**
- DN: `/ip4/testnet.accumulate.defidevs.io/tcp/16591/p2p/QmTestNodeID`
- BVN: `/ip4/testnet.accumulate.defidevs.io/tcp/16691/p2p/QmTestNodeID`

**Ports:**
- Same as mainnet (52000-52002, 52100-52102)

## Custom Bootstrap Peers

Override default peers:

```json
{
  "tool": "accumulate_prepare_accman_artifacts",
  "arguments": {
    "dn_node_dir": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_node_dir": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "output_dir": "/home/paul/accman-artifacts",
    "network": "mainnet",
    "dn_bootstrap_peers": [
      "/ip4/CUSTOM_IP/tcp/16591/p2p/CUSTOM_PEER_ID",
      "/ip4/ANOTHER_IP/tcp/16591/p2p/ANOTHER_PEER_ID"
    ],
    "bvn_bootstrap_peers": [
      "/ip4/CUSTOM_IP/tcp/16691/p2p/CUSTOM_PEER_ID"
    ]
  }
}
```

## Advanced Use Cases

### Multiple Network Deployments

Prepare artifacts for both mainnet and testnet:

```json
// Mainnet
{
  "tool": "accumulate_prepare_accman_artifacts",
  "arguments": {
    "dn_node_dir": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_node_dir": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "output_dir": "/home/paul/mainnet-artifacts",
    "network": "mainnet"
  }
}

// Testnet
{
  "tool": "accumulate_prepare_accman_artifacts",
  "arguments": {
    "dn_node_dir": "/media/paul/Expansion/databases/2025-10-13-testnet-dn",
    "bvn_node_dir": "/media/paul/Expansion/databases/2025-10-13-testnet-bvn",
    "output_dir": "/home/paul/testnet-artifacts",
    "network": "testnet"
  }
}
```

### Separate Partition Deployments

**DN-only follower:**
```json
{
  "tool": "accumulate_create_node_archive",
  "arguments": {
    "node_dir": "/media/paul/Expansion/databases/2025-10-13-dn",
    "output_file": "/home/paul/dn-only.tar.gz",
    "node_type": "DN"
  }
}
```

Then deploy with accman:
```bash
accman-mcp deploy_follower \
  --partition dn \
  --dn-snapshot /home/paul/dn-only.tar.gz
```

**BVN-only follower:**
```json
{
  "tool": "accumulate_create_node_archive",
  "arguments": {
    "node_dir": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "output_file": "/home/paul/bvn-only.tar.gz",
    "node_type": "BVN"
  }
}
```

Then deploy:
```bash
accman-mcp deploy_follower \
  --partition bvn \
  --bvn-snapshot /home/paul/bvn-only.tar.gz
```

## Troubleshooting

### Archive Creation Fails

**Problem:** `failed to create archive`

**Solutions:**
```bash
# Check source directory exists
ls -la /media/paul/Expansion/databases/2025-10-13-dn

# Check disk space
df -h /home/paul/accman-artifacts

# Check permissions
ls -ld /media/paul/Expansion/databases/2025-10-13-dn
```

### Accman Deployment Fails

**Problem:** `accman-mcp not found`

**Solution:**
```bash
# Install accman
cd /path/to/accman
go build -o accman-mcp ./cmd/accman-mcp

# Add to PATH
export PATH=$PATH:/path/to/accman
```

**Problem:** `Invalid snapshot file`

**Solutions:**
```bash
# Verify archive integrity
tar -tzf /path/to/dn-node.tar.gz | head

# Check file size (should be > 100MB typically)
ls -lh /path/to/dn-node.tar.gz

# Use verification script
bash /path/to/verify-artifacts-*.sh
```

### Follower Won't Start

**Problem:** Container stops immediately

**Solutions:**
```bash
# Check logs
docker logs accumulate-follower-dual

# Verify port availability
sudo netstat -tulpn | grep -E '52000|52001|52100|52101'

# Check volume mounting
docker inspect accumulate-follower-dual | jq '.Mounts'
```

## Integration Patterns

### CI/CD Pipeline

```yaml
# .gitlab-ci.yml or GitHub Actions
deploy-follower:
  steps:
    - name: Prepare artifacts
      run: |
        # Call Accumulate MCP to prepare artifacts
        curl -X POST http://accumulate-mcp/tools/call \
          -d '{
            "name": "accumulate_prepare_accman_artifacts",
            "arguments": {
              "dn_node_dir": "$DN_NODE_DIR",
              "bvn_node_dir": "$BVN_NODE_DIR",
              "output_dir": "./artifacts",
              "network": "mainnet"
            }
          }'

    - name: Deploy with accman
      run: |
        bash ./artifacts/deploy-*.sh
```

### Automated Snapshots

```bash
#!/bin/bash
# snapshot-and-deploy.sh

# 1. Prepare artifacts from latest snapshots
curl -X POST http://accumulate-mcp/tools/call \
  -d '{
    "name": "accumulate_prepare_accman_artifacts",
    "arguments": {
      "dn_node_dir": "/latest/dn",
      "bvn_node_dir": "/latest/bvn",
      "output_dir": "/artifacts/$(date +%Y%m%d)",
      "network": "mainnet"
    }
  }' > response.json

# 2. Extract deployment script path
SCRIPT=$(jq -r '.result.artifacts.deployment_script' response.json)

# 3. Deploy
bash "$SCRIPT"
```

## Best Practices

1. **Version Control Metadata**
   - Store deployment metadata in git
   - Track which database snapshots were used
   - Document custom bootstrap peers

2. **Test Before Production**
   - Deploy to testnet first
   - Verify sync completes
   - Test query endpoints

3. **Archive Storage**
   - Keep database archives for rollback
   - Compress with maximum settings for storage
   - Use checksums to verify integrity

4. **Bootstrap Peer Management**
   - Use multiple bootstrap peers for redundancy
   - Update peers based on network health
   - Monitor peer connectivity

5. **Monitoring**
   - Set up alerts for sync status
   - Monitor disk usage growth
   - Track peer count

## References

- [Accman Documentation](../../../accman/README.md)
- [Accman MCP Server](../../../accman/README-MCP.md)
- [Follower Docker Guide](./FOLLOWER_DOCKER_GUIDE.md)
- [Database MCP Specification](./MCP_DATABASE_ACCESS_INVESTIGATION.md)

## Support

For issues with:
- **Artifact Preparation**: Accumulate MCP
- **Deployment**: Accman
- **Running Follower**: Docker + Accumulate

## Changelog

- **2025-11-16**: Documentation fixes
  - Fixed parameter names: `dn_node_dir`/`bvn_node_dir` (was `dn_database`/`bvn_database`)
  - Fixed tool name: `accumulate_create_node_archive` (was `accumulate_create_database_archive`)
  - Clarified that node directories must contain complete CometBFT + Accumulate structure
- **2025-11-15**: Initial release
  - `accumulate_prepare_accman_artifacts` tool
  - `accumulate_create_node_archive` tool
  - `accumulate_get_bootstrap_peers` tool
  - Integration with accman-mcp
