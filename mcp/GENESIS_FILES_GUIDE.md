# Genesis Snapshot Files - Complete Guide

**Date:** 2025-11-16
**Purpose:** Guide for using genesis snapshot files in Accumulate follower deployment

---

## What Are Genesis Snapshot Files?

Genesis snapshot files contain the initial network state for Accumulate partitions. They are critical for:
- Initializing new follower nodes
- Ensuring correct network state from the beginning
- Bootstrapping network connectivity

### File Format: `.snap` Files

**IMPORTANT:** Genesis files use `.snap` format, which is a **binary snapshot format**, NOT JSON.

- **Format:** Binary snapshot (Badger database snapshot)
- **Extension:** `.snap`
- **Size:** Typically 10-100 KB for genesis, but can be larger (2+ GB for full snapshots)
- **Cannot be viewed** with text editors (binary format)

**Do NOT confuse with:**
- Genesis JSON documents (`.json`) - Different format used by some `accumulated` commands
- These are snapshot files, not configuration files

### File Naming Convention

- **DN Genesis:** `dn-genesis.snap` or `directory-genesis.snap`
- **BVN Genesis:** `bvn1-genesis.snap`, `bvn2-genesis.snap`, `bvn3-genesis.snap`, `bvn4-genesis.snap`
  - OR: `cyclops-genesis.snap`, `apollo-genesis.snap`, etc. (named by BVN partition)

### Standard Location

Genesis files are typically located in: `~/.accumulate/`

Example:
```
/home/paul/.accumulate/
├── dn-genesis.snap          (14 KB - network genesis)
├── directory-genesis.snap   (2.0 MB - alternate name)
├── bvn1-genesis.snap        (13 KB - BVN1 genesis)
├── cyclops-genesis.snap     (2.1 GB - full Cyclops snapshot)
├── bvn2-genesis.snap
├── bvn3-genesis.snap
└── bvn4-genesis.snap
```

**Size Differences:**
- **Small files (10-100 KB):** Network genesis state
- **Large files (1-2+ GB):** Full partition snapshots including transaction history

### Relationship to `accumulated` Command Flags

**MCP Tools:** Use `.snap` files directly (binary snapshots)
```json
{
  "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
  "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap"
}
```

**`accumulated` Commands:** Some use JSON, some use .snap
- `accumulated run-dual` - Uses `.snap` files in parent directory
- `accumulated init dual --dn-genesis-doc` - Expects JSON format (different!)
- `accumulated init dual --follow` - Downloads genesis automatically

**Key Point:** Don't pass `.snap` files to commands expecting JSON genesis docs!

---

## MCP Tools for Genesis Files

### 1. `accumulate_get_genesis_files`

**Purpose:** Locate standard genesis snapshot files for your system

**Usage:**
```json
{
  "tool": "accumulate_get_genesis_files",
  "arguments": {
    "network": "mainnet",
    "bvn": "1"
  }
}
```

**Response:**
```json
{
  "network": "mainnet",
  "bvn": "1",
  "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
  "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap",
  "accumulate_directory": "/home/paul/.accumulate",
  "dn_genesis_exists": true,
  "bvn_genesis_exists": true
}
```

**If files are missing:**
```json
{
  "network": "mainnet",
  "bvn": "1",
  "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
  "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap",
  "accumulate_directory": "/home/paul/.accumulate",
  "dn_genesis_exists": false,
  "bvn_genesis_exists": false,
  "warning": "Genesis files not found: [dn-genesis.snap bvn1-genesis.snap]. These may need to be obtained from network bootstrap.",
  "note": "Genesis files are typically located in ~/.accumulate/ directory"
}
```

---

### 2. Updated `accumulate_init_follower`

**NEW:** Now accepts genesis snapshot file paths

**Usage WITH genesis files:**
```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/tmp/follower",
    "network": "MainNet",
    "bvn_name": "Cyclops",
    "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
    "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap"
  }
}
```

**Response:**
```json
{
  "status": "initialized",
  "work_dir": "/tmp/follower",
  "dn_path": "/tmp/follower/dnn",
  "bvn_path": "/tmp/follower/bvnn",
  "config_path": "/tmp/follower/accumulate.toml",
  "container_name": "accumulate-follower",
  "dn_genesis_snap": "/tmp/follower/dn-genesis.snap",
  "bvn_genesis_snap": "/tmp/follower/bvn-genesis.snap",
  "next_steps": "Use accumulate_run_follower to start the follower node in Docker"
}
```

**Files created in work directory:**
```
/tmp/follower/
├── dnn/                      (DN database copy)
├── bvnn/                     (BVN database copy)
├── accumulate.toml           (Follower configuration)
├── dn-genesis.snap           (DN genesis snapshot)
└── bvn-genesis.snap          (BVN genesis snapshot)
```

---

## Complete Deployment Workflow with Genesis Files

### Step 1: Locate Genesis Files

```json
{
  "tool": "accumulate_get_genesis_files",
  "arguments": {
    "network": "mainnet",
    "bvn": "1"
  }
}
```

**Save the paths from the response** - you'll use them in the next step.

---

### Step 2: Initialize Follower with Genesis Files

```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/var/lib/accumulate-follower",
    "network": "MainNet",
    "bvn_name": "Cyclops",
    "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
    "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap",
    "dn_bootstrap_peers": [
      "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
    ],
    "bvn_bootstrap_peers": [
      "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
    ]
  }
}
```

---

### Step 3: Run Follower

```json
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/var/lib/accumulate-follower"
  }
}
```

---

### Step 4: Verify Deployment

```json
{
  "tool": "accumulate_follower_status",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

---

## Command Syntax Verification

The MCP uses the correct `accumulated` command:

```bash
accumulated run-dual /node/dnn /node/bvnn
```

This is confirmed from:
- Dockerfile: `CMD ["run-dual", "/node/dnn", "/node/bvnn"]`
- Deploy scripts: `run-dual /node/dnn /node/bvnn`
- cmd_run_dual.go: `Use: "run-dual <primary> <secondary>"`

✅ **CONFIRMED: Using correct command syntax**

---

## Bootstrap Peer Configuration

The MCP provides default bootstrap peers and allows customization:

**Default Mainnet Peers:**
- DN: `/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD`
- BVN: `/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD`

**Get current peers:**
```json
{
  "tool": "accumulate_get_bootstrap_peers",
  "arguments": {
    "network": "mainnet"
  }
}
```

**Custom peers:**
```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    ...
    "dn_bootstrap_peers": [
      "/ip4/YOUR_VALIDATOR_IP/tcp/16591/p2p/YOUR_PEER_ID"
    ]
  }
}
```

---

## What's Provided vs What You Need

### ✅ Automatically Provided by MCP

1. **Correct accumulated command** - `run-dual`
2. **Default bootstrap peers** - Mainnet/testnet defaults
3. **Configuration generation** - `accumulate.toml` created automatically
4. **Database copying** - Preserves historical snapshots
5. **Docker deployment** - Proper container setup
6. **Genesis file handling** - Copies to work directory

### 📋 You Must Provide

1. **Database snapshots** - Complete node directories with:
   - `config/` (CometBFT config)
   - `data/accumulate.db/` (Accumulate database)
   - `data/blockstore.db/` (CometBFT blocks)

2. **Genesis snapshot files** - Located at:
   - `~/.accumulate/dn-genesis.snap`
   - `~/.accumulate/bvnN-genesis.snap`

3. **Optional: Custom bootstrap peers** - If using your own validator

---

## Obtaining Genesis Files

### Option 1: From Existing Node

If you have a running validator or node:
```bash
cp /path/to/node/dn-genesis.snap ~/.accumulate/
cp /path/to/node/bvn1-genesis.snap ~/.accumulate/
```

### Option 2: From Network Bootstrap

Genesis files can be obtained from network bootstrap sources:
- Official Accumulate repositories
- Network deployment artifacts
- Validator operators

### Option 3: From accman

If using accman for deployment, it may provide genesis files as part of the bootstrap process.

---

## Troubleshooting

### Genesis Files Not Found

**Error:** "Genesis files not found"

**Solution:**
```bash
# Check if files exist
ls -lh ~/.accumulate/*genesis.snap

# If missing, obtain from network or existing node
# Then verify with MCP tool:
```

```json
{
  "tool": "accumulate_get_genesis_files",
  "arguments": {
    "network": "mainnet"
  }
}
```

---

### Wrong BVN Genesis File

**Problem:** Using bvn1-genesis.snap but need bvn2

**Solution:**
```json
{
  "tool": "accumulate_get_genesis_files",
  "arguments": {
    "network": "mainnet",
    "bvn": "2"
  }
}
```

Then use the returned path in `accumulate_init_follower`.

---

### Genesis Files Required?

**Question:** Are genesis files always required?

**Answer:** It depends on your deployment method:

- **New follower initialization:** Genesis files may be needed to establish correct initial state
- **Existing node with snapshot:** May already have genesis state embedded
- **accman deployment:** May handle genesis files automatically

**Recommendation:** Always provide genesis files when available to ensure correct initialization.

---

## API Summary

### New Tool: `accumulate_get_genesis_files`

**Parameters:**
- `network` (optional): "mainnet" or "testnet" (default: "mainnet")
- `bvn` (optional): BVN partition number (default: "1")

**Returns:**
- `dn_genesis_snap`: Path to DN genesis file
- `bvn_genesis_snap`: Path to BVN genesis file
- `dn_genesis_exists`: Boolean - file exists
- `bvn_genesis_exists`: Boolean - file exists
- `warning`: Message if files missing
- `note`: Additional information

---

### Updated Tool: `accumulate_init_follower`

**New Parameters:**
- `dn_genesis_snap` (optional): Path to DN genesis snapshot file
- `bvn_genesis_snap` (optional): Path to BVN genesis snapshot file

**Example:**
```json
{
  "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
  "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap"
}
```

If not provided, follower will initialize without genesis files (may work depending on database state).

---

## Complete Example

### Full deployment with all configuration:

```json
// Step 1: Get genesis file locations
{
  "tool": "accumulate_get_genesis_files",
  "arguments": {
    "network": "mainnet",
    "bvn": "1"
  }
}

// Response shows files exist at:
// - /home/paul/.accumulate/dn-genesis.snap
// - /home/paul/.accumulate/bvn1-genesis.snap

// Step 2: Get bootstrap peers
{
  "tool": "accumulate_get_bootstrap_peers",
  "arguments": {
    "network": "mainnet"
  }
}

// Step 3: Initialize follower with everything
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/var/lib/accumulate-follower",
    "network": "MainNet",
    "bvn_name": "Cyclops",
    "dn_genesis_snap": "/home/paul/.accumulate/dn-genesis.snap",
    "bvn_genesis_snap": "/home/paul/.accumulate/bvn1-genesis.snap",
    "dn_bootstrap_peers": [
      "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
    ],
    "bvn_bootstrap_peers": [
      "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
    ]
  }
}

// Step 4: Run follower
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/var/lib/accumulate-follower"
  }
}

// Step 5: Check status
{
  "tool": "accumulate_follower_status",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

---

## What Changed

### Before (Missing Genesis Support):
- ❌ No genesis file handling
- ❌ No way to locate genesis files
- ❌ Deployment might fail due to missing genesis state

### After (With Genesis Support):
- ✅ Genesis file parameters in `accumulate_init_follower`
- ✅ New `accumulate_get_genesis_files` tool
- ✅ Automatic copying to work directory
- ✅ Proper file existence checking
- ✅ Helpful warnings when files missing

---

## Summary

**What MCP Now Provides:**

1. ✅ **Correct accumulated syntax** - `run-dual` command
2. ✅ **Bootstrap peer management** - Default peers + custom support
3. ✅ **Genesis file handling** - Discovery, validation, copying
4. ✅ **Complete configuration** - All necessary files in work directory

**You just need to provide:**
- Database snapshots (complete node directories)
- Genesis files (located via MCP or obtained from network)
- Optional: Custom bootstrap peers (validator seed nodes)

**The deployment is now complete and ready to test!**

---

## References

- Main implementation: `mcp/server/tools_follower.go`
- Tool definitions: `mcp/server/tool_definitions.go`
- Deployment guide: `mcp/FOLLOWER_DOCKER_GUIDE.md`
- Review document: `mcp/IMPLEMENTATION_REVIEW.md`
