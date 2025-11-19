# Accumulate MCP & Accman MCP - Architecture and Integration

**Purpose:** Clarify the relationship between Accumulate MCP and Accman MCP, when to use each, and how they work together.

---

## Overview

There are **two separate MCP servers** that work with Accumulate:

1. **Accumulate MCP** (this repository) - Low-level `accumulated` binary operations
2. **Accman MCP** (separate repository) - High-level deployment orchestration

Both can be used independently or together depending on your needs.

---

## Accumulate MCP

**Repository:** `gitlab.com/accumulatenetwork/accumulate/mcp/`
**Purpose:** Direct interface to `accumulated` binary functionality

### What It Does

- **Follower Deployment:** Initialize and run follower nodes using Docker
- **Database Operations:** Copy, validate, and manage database snapshots
- **Configuration Management:** Generate `accumulate.toml` configurations
- **Genesis File Handling:** Locate and copy genesis snapshot files
- **Status Monitoring:** Check follower container status

### Key Tools

```
accumulate_init_follower         - Initialize from database snapshots
accumulate_run_follower          - Start follower in Docker
accumulate_follower_status       - Check container status
accumulate_get_genesis_files     - Locate genesis snapshots
accumulate_get_bootstrap_peers   - Get network peers
```

### When to Use Accumulate MCP

✅ **Use when:**
- You have database snapshots/backups
- You want direct Docker deployment control
- You need fine-grained control over configuration
- You're scripting custom deployment workflows
- You want to integrate follower management into applications

❌ **Don't use when:**
- You need network-level orchestration
- You want automated snapshot downloads
- You need binary distribution
- You're deploying multiple nodes

---

## Accman MCP

**Repository:** `gitlab.com/accumulatenetwork/accman/`
**Purpose:** High-level deployment and network management

### What It Does

- **Network Bootstrapping:** Download binaries, snapshots, seed nodes automatically
- **One-Command Deployment:** Bootstrap AND deploy in single operation
- **Snapshot Management:** Create, verify, and list snapshots
- **Volume Operations:** Docker volume backup/restore
- **Resource Fetching:** Auto-download from GitLab releases and snapshot servers

### Key Tools

```
network_bootstrap                 - Download network resources
network_bootstrap_and_deploy      - One-step deployment
deploy_follower                   - Deploy from existing artifacts
snapshot_create                   - Create database snapshots
snapshot_verify                   - Verify snapshot integrity
volume_backup/restore             - Docker volume management
```

### When to Use Accman MCP

✅ **Use when:**
- You want automated resource fetching (binaries, snapshots)
- You need "one-command" deployment
- You want deployment orchestration
- You need snapshot creation from running nodes
- You're deploying to production

❌ **Don't use when:**
- You already have all needed files locally
- You need custom database handling
- You want more control over initialization

---

## Comparison Matrix

| Feature | Accumulate MCP | Accman MCP |
|---------|----------------|------------|
| **Complexity** | Low-level | High-level |
| **Auto-download** | ❌ No | ✅ Yes |
| **Binary fetching** | ❌ No | ✅ Yes (GitLab) |
| **Snapshot download** | ❌ No | ✅ Yes (servers) |
| **Genesis handling** | ✅ Copy from local | ✅ Fetch or copy |
| **Docker deployment** | ✅ Direct | ✅ Via artifacts |
| **Custom databases** | ✅ Full control | ⚠️ Limited |
| **Snapshot creation** | ❌ No | ✅ Yes |
| **Volume backup** | ❌ No | ✅ Yes |
| **Use case** | Custom workflows | Production deploy |

---

## Integration Patterns

### Pattern 1: Pure Accumulate MCP

**Use Case:** You have database backups and want custom deployment

```bash
# 1. Get genesis files
./examples/invoke-mcp-tool.sh accumulate_get_genesis_files '{"network":"mainnet"}'

# 2. Initialize follower
./examples/invoke-mcp-tool.sh accumulate_init_follower '{
  "dn_database": "/backup/dn",
  "bvn_database": "/backup/bvn",
  "work_dir": "/var/lib/follower"
}'

# 3. Run follower
./examples/invoke-mcp-tool.sh accumulate_run_follower '{
  "work_dir": "/var/lib/follower"
}'
```

**Pros:**
- Full control over every step
- Works with any database source
- No network dependencies

**Cons:**
- Manual resource management
- More steps

---

### Pattern 2: Pure Accman MCP

**Use Case:** You want automated deployment from network

```bash
# One command does everything
accman-mcp network_bootstrap_and_deploy \
  --network mainnet \
  --partition dual \
  --use-volume
```

**What happens:**
1. Downloads latest `accumulated` binary from GitLab
2. Downloads latest DN and BVN snapshots
3. Fetches seed nodes from network
4. Generates configuration
5. Deploys follower

**Pros:**
- Simplest approach
- Always gets latest versions
- Production-ready

**Cons:**
- Less control
- Requires network access
- Can't use custom databases

---

### Pattern 3: Hybrid Approach

**Use Case:** Prepare artifacts with Accumulate MCP, deploy with Accman MCP

```bash
# Step 1: Use Accumulate MCP to prepare artifacts
./examples/invoke-mcp-tool.sh accumulate_prepare_accman_artifacts '{
  "dn_node_dir": "/backup/dn",
  "bvn_node_dir": "/backup/bvn",
  "output_dir": "/artifacts",
  "network": "mainnet"
}'

# Step 2: Use Accman MCP to deploy
accman-mcp deploy_follower \
  --partition dual \
  --dn-snapshot /artifacts/dn-node-mainnet-*.tar.gz \
  --bvn-snapshot /artifacts/bvn-node-mainnet-*.tar.gz \
  --use-volume
```

**Pros:**
- Best of both worlds
- Use local databases
- Leverage Accman deployment

**Cons:**
- Requires both MCP servers
- More complex setup

---

### Pattern 4: Accumulate MCP + Manual Deployment

**Use Case:** Research, development, custom configurations

```bash
# Use Accumulate MCP to prepare
./examples/deploy-follower-complete.sh

# Then manage manually
docker exec -it accumulate-follower accumulated version
docker logs -f accumulate-follower
```

---

## Decision Tree: Which MCP to Use?

```
Do you have database backups locally?
├─ YES → Do you want custom configuration?
│   ├─ YES → Use Accumulate MCP (Pattern 1)
│   └─ NO  → Use Accman MCP artifacts (Pattern 3)
└─ NO  → Do you want automated download?
    ├─ YES → Use Accman MCP (Pattern 2)
    └─ NO  → Get backups first, then use Accumulate MCP
```

**Quick answers:**
- **"Just deploy mainnet follower"** → Accman MCP
- **"Use my existing backup"** → Accumulate MCP
- **"Custom database setup"** → Accumulate MCP
- **"Production deployment"** → Accman MCP
- **"Development/testing"** → Accumulate MCP

---

## How They Work Together

### Data Flow

```
┌─────────────────────┐
│  Node Backup        │
│  /backup/dnn        │
│  /backup/bvnn       │
└──────────┬──────────┘
           │
           ▼
    ┌──────────────────────┐
    │  Accumulate MCP      │
    │  - Validate DBs      │
    │  - Copy files        │
    │  - Create config     │
    │  - Archive for       │
    │    accman            │
    └──────────┬───────────┘
               │
               ▼
        ┌──────────────────┐
        │  Artifacts       │
        │  *.tar.gz        │
        │  metadata.json   │
        └──────┬───────────┘
               │
               ▼
        ┌──────────────────┐
        │  Accman MCP      │
        │  - Deploy to     │
        │    Docker        │
        │  - Manage        │
        │    containers    │
        └──────────────────┘
```

---

## Communication Between MCPs

**They DON'T communicate directly.** Each is independent.

**Accumulate MCP** can prepare artifacts that **Accman MCP** consumes:

```bash
# Accumulate MCP creates:
/artifacts/
├── dn-node-mainnet-*.tar.gz      ← Accman can use this
├── bvn-node-mainnet-*.tar.gz     ← Accman can use this
├── deployment-metadata-*.json    ← Metadata for Accman
└── deploy-*.sh                   ← Calls Accman

# Accman MCP deploys from artifacts
accman-mcp deploy_follower --dn-snapshot /artifacts/dn-*.tar.gz ...
```

---

## API Compatibility

Both MCPs use the **Model Context Protocol (MCP)** standard:

- JSON-RPC 2.0 format
- `tools/call` method
- Stdio or HTTP transport

**Same invocation pattern:**
```bash
# Accumulate MCP
echo '{"method":"tools/call","params":{...}}' | ./mcp-server

# Accman MCP
echo '{"method":"tools/call","params":{...}}' | ./accman-mcp
```

---

## Production Recommendations

### Small Deployments (1-5 followers)

**Use:** Accumulate MCP
- More control
- Simpler stack
- Direct management

### Large Deployments (5+ followers)

**Use:** Accman MCP or Hybrid
- Orchestration features
- Volume management
- Snapshot automation

### Development/Testing

**Use:** Accumulate MCP
- Faster iteration
- More flexible
- Better for debugging

### Network Operations

**Use:** Accman MCP
- Designed for this
- Better tooling
- Production features

---

## Example Scenarios

### Scenario 1: Restore from Backup

**Best Tool:** Accumulate MCP

```bash
# You have: Node backup from /backup/mainnet/
# You want: Running follower

./examples/deploy-follower-complete.sh
```

**Why Accumulate MCP:**
- Works directly with backups
- No artifact conversion needed
- Full database control

---

### Scenario 2: Deploy Latest Mainnet

**Best Tool:** Accman MCP

```bash
# You want: Latest mainnet follower

accman-mcp network_bootstrap_and_deploy --network mainnet
```

**Why Accman MCP:**
- Auto-downloads everything
- Always latest version
- One command

---

### Scenario 3: Transfer Deployment

**Best Tool:** Hybrid

```bash
# Server A: Prepare artifacts
./mcp-server accumulate_prepare_accman_artifacts ...

# Transfer /artifacts/ to Server B

# Server B: Deploy with Accman
accman-mcp deploy_follower --dn-snapshot /artifacts/dn.tar.gz ...
```

**Why Hybrid:**
- Decouple preparation from deployment
- Can transfer artifacts
- Best for multi-server

---

### Scenario 4: CI/CD Pipeline

**Best Tool:** Hybrid or Accman MCP

```yaml
# .gitlab-ci.yml
deploy:
  script:
    - ./mcp-server prepare_artifacts
    - scp artifacts/* deploy-server:/
    - ssh deploy-server "accman-mcp deploy_follower ..."
```

---

## Summary

**Accumulate MCP:**
- ✅ Low-level control
- ✅ Works with backups
- ✅ Custom configurations
- ✅ Direct Docker management

**Accman MCP:**
- ✅ High-level orchestration
- ✅ Auto-downloads resources
- ✅ Production deployment
- ✅ Snapshot/volume management

**Use Together:**
- ✅ Accumulate MCP prepares
- ✅ Accman MCP deploys
- ✅ Best of both

**Choose Based On:**
- **Have backups?** → Accumulate MCP
- **Need latest?** → Accman MCP
- **Need control?** → Accumulate MCP
- **Production?** → Accman MCP
- **Complex workflow?** → Both

---

## References

**Accumulate MCP:**
- `FOLLOWER_DOCKER_GUIDE.md`
- `GENESIS_FILES_GUIDE.md`
- `examples/README.md`

**Accman MCP:**
- `accman/README-MCP.md`
- `accman/pkg/accman/`

**Integration:**
- `ACCMAN_INTEGRATION_GUIDE.md`
- This document

---

**Last Updated:** 2025-11-16
