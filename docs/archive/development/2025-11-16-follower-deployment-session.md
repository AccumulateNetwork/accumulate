# Follower Deployment Session - November 16, 2025

**Date**: 2025-11-16
**Objective**: Deploy Accumulate mainnet follower using July 13, 2025 genesis files
**Status**: Blocked - needs code-level debugging

---

## Background

Deploying a follower node for Accumulate MainNet using:
- **Genesis Date**: July 13, 2025
- **Bootstrap Server**: bootstrap.accumulate.defidevs.io (3.138.61.111)
- **Network**: MainNet
- **BVN**: Cyclops
- **Genesis Files**:
  - DN: `/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/directory-genesis.snap` (2.0 MB)
  - BVN: `/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/cyclops-genesis.snap` (2.1 GB)

---

## Deployment Attempts

### Attempt 1: Using Accumulate MCP Tools

**Command**: Used `accumulate_init_follower` to prepare work directory

```bash
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_init_follower",
    "arguments": {
      "dn_database": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/dnn",
      "bvn_database": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/bvnn",
      "work_dir": "/home/paul/accumulate-follower",
      "dn_genesis_snap": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/directory-genesis.snap",
      "bvn_genesis_snap": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/cyclops-genesis.snap",
      "dn_bootstrap_peers": ["/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"],
      "bvn_bootstrap_peers": ["/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"],
      "network": "MainNet",
      "bvn_name": "Cyclops"
    }
  }
}' | /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/mcp/mcp-server
```

**Result**: Successfully created:
- `/home/paul/accumulate-follower/accumulate.toml`
- `/home/paul/accumulate-follower/dnn/` (database directory)
- `/home/paul/accumulate-follower/bvnn/` (database directory)
- `/home/paul/accumulate-follower/dn-genesis.snap`
- `/home/paul/accumulate-follower/bvn-genesis.snap`

**Issue**: Database consistency error when deploying

---

### Attempt 2: Manual Docker with Database Backups

**Command**:
```bash
docker run -d \
  --name accumulate-follower \
  --restart unless-stopped \
  -v /home/paul/accumulate-follower/dnn:/node/dnn \
  -v /home/paul/accumulate-follower/bvnn:/node/bvnn \
  -v /home/paul/accumulate-follower/accumulate.toml:/node/accumulate.toml \
  -v /home/paul/accumulate-follower/dn-genesis.snap:/node/directory-genesis.snap \
  -v /home/paul/accumulate-follower/bvn-genesis.snap:/node/cyclops-genesis.snap \
  -p 16591-16593:16591-16593 \
  -p 16691-16693:16691-16693 \
  accumulated:follower-p2p-fix \
  run-dual /node/dnn /node/bvnn
```

**Error**:
```
panic: StoreBlockHeight (6643031) > StateBlockHeight + 1 (6642937)
```

**Analysis**: October 1 database backup has inconsistent block heights:
- Block store height: 6,643,031
- State height: 6,642,937
- Gap: 94 blocks

**Resolution**: Switched to genesis-only deployment

---

### Attempt 3: Genesis-Only Deployment

**Steps**:
1. Removed inconsistent database backups:
   ```bash
   rm -rf /home/paul/accumulate-follower/dnn /home/paul/accumulate-follower/bvnn
   mkdir -p /home/paul/accumulate-follower/dnn /home/paul/accumulate-follower/bvnn
   ```

2. Deployed with empty databases:
   ```bash
   docker run -d \
     --name accumulate-follower \
     --restart unless-stopped \
     -v /home/paul/accumulate-follower/dnn:/node/dnn \
     -v /home/paul/accumulate-follower/bvnn:/node/bvnn \
     -v /home/paul/accumulate-follower/accumulate.toml:/node/accumulate.toml \
     -v /home/paul/accumulate-follower/dn-genesis.snap:/node/directory-genesis.snap \
     -v /home/paul/accumulate-follower/bvn-genesis.snap:/node/cyclops-genesis.snap \
     -p 16591-16593:16591-16593 \
     -p 16691-16693:16691-16693 \
     accumulated:follower-p2p-fix \
     run-dual /node/dnn /node/bvnn
   ```

**Error**:
```
Error: start service consensus: initialize consensus: read /node: is a directory
```

---

### Attempt 4: Genesis Files in Partition Config Directories

**Research Finding**: Based on code analysis, genesis files should be in `{partition}/config/genesis.json`

**Steps**:
1. Created config directories and copied genesis files:
   ```bash
   docker run --rm --entrypoint /bin/sh \
     -v /home/paul/accumulate-follower:/work \
     accumulated:follower-p2p-fix \
     -c "mkdir -p /work/dnn/config /work/bvnn/config && \
         cp /work/dn-genesis.snap /work/dnn/config/genesis.json && \
         cp /work/bvn-genesis.snap /work/bvnn/config/genesis.json"
   ```

2. Verified files in place:
   - `/home/paul/accumulate-follower/dnn/config/genesis.json` (2.0 MB)
   - `/home/paul/accumulate-follower/bvnn/config/genesis.json` (2.1 GB)

3. Deployed container:
   ```bash
   docker run -d \
     --name accumulate-follower \
     --restart unless-stopped \
     -v /home/paul/accumulate-follower/dnn:/node/dnn \
     -v /home/paul/accumulate-follower/bvnn:/node/bvnn \
     -v /home/paul/accumulate-follower/accumulate.toml:/node/accumulate.toml \
     -p 16591-16593:16591-16593 \
     -p 16691-16693:16691-16693 \
     accumulated:follower-p2p-fix \
     run-dual /node/dnn /node/bvnn
   ```

**Error** (same as before):
```
Error: start service consensus: initialize consensus: read /node: is a directory
```

---

## Current Configuration Files

### accumulate.toml

Located at: `/home/paul/accumulate-follower/accumulate.toml`

```toml
network = "MainNet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "badger"
  enable-healing = false
  enable-snapshots = false

  dn-bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
  ]

  bvn-bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
  ]

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
```

### Directory Structure

```
/home/paul/accumulate-follower/
├── accumulate.toml
├── dn-genesis.snap (2.0 MB)
├── bvn-genesis.snap (2.1 GB)
├── dnn/
│   └── config/
│       ├── genesis.json (2.0 MB - copy of dn-genesis.snap)
│       └── tendermint.toml (created by previous run attempt)
└── bvnn/
    ├── config/
    │   └── genesis.json (2.1 GB - copy of bvn-genesis.snap)
    └── data/
```

---

## Error Analysis

### Primary Error

```
Error: start service consensus: initialize consensus: read /node: is a directory
```

**Location**: Occurs during consensus service initialization
**Frequency**: Every startup attempt

### Secondary Warnings

```
INFO Unable to connect to bootstrap peer
error="failed to dial: failed to dial 12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg:
all dials failed
  * [/ip4/3.138.61.111/tcp/16593] failed to negotiate security protocol:
    peer id mismatch: expected 12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg,
    but remote key matches 12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
```

**Analysis**:
- Configured peer ID: `12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx` (correct)
- Expected peer ID: `12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg` (stale/cached?)
- This suggests old peer information may be cached somewhere

---

## Code Investigation

### MCP Tool Bug Confirmed

**File**: `mcp/server/tools_follower.go:165-177`

**Issue**: `accumulate_run_follower` does NOT mount genesis files into Docker container

**Current mounts**:
```go
dockerArgs := []string{
    "run",
    "-d",
    "--name", containerName,
    "--restart", "unless-stopped",
    "-v", fmt.Sprintf("%s:/node/dnn", dnnPath),
    "-v", fmt.Sprintf("%s:/node/bvnn", bvnnPath),
    "-v", fmt.Sprintf("%s:/node/accumulate.toml", configPath),
    "-p", "16591-16593:16591-16593",
    "-p", "16691-16693:16691-16693",
    dockerImage,
    "run-dual", "/node/dnn", "/node/bvnn",
}
```

**Missing mounts**:
- DN genesis file
- BVN genesis file

### Genesis File Path Research

From code search in `cmd/accumulated/cmd_init.go`:
- `--dn-genesis-doc` flag accepts path to DN genesis document
- `--bvn-genesis-doc` flag accepts path to BVN genesis document
- Historical evidence shows genesis files at `{partition}/config/genesis.json`

Example from `scripts/old/ci/validate-admin.sh:35`:
```bash
--genesis-doc="${NODES_DIR}/node-1/dnn/config/genesis.json"
```

### New-Style Config Detection

From `cmd/accumulated/cmd_run_dual.go:62-67`:
```go
// Detect new-style configuration
c := new(run.Config)
if c.LoadFrom(filepath.Join(args[0], "..", "accumulate.toml")) == nil {
    runCfg(c, nil)
    return "run complete", nil
}
```

The code loads `accumulate.toml` from parent directory of first partition argument.

---

## Blockers

1. **Genesis File Path Error**: The "read /node: is a directory" error suggests the binary is trying to read `/node` as a file, not accessing genesis files correctly

2. **Insufficient Access**: Debugging requires access to:
   - Full source code in `cmd/accumulated/run/` package
   - `internal/node/consensus/` initialization code
   - Understanding of how new-style config processes genesis files
   - Ability to trace through the initialization sequence

3. **Repository Context**: Currently working in `backupdbs` repository, but need access to `accumulate` repository for proper debugging

---

## Next Steps

### Recommended Approach

**Switch to Accumulate Repository** to:

1. Trace the "read /node: is a directory" error to its source
2. Understand how new-style config (`accumulate.toml`) handles genesis files
3. Determine if genesis files should be:
   - Specified in `accumulate.toml`
   - Passed as command-line arguments
   - Auto-discovered in partition config directories
   - Mounted at specific Docker paths

4. Fix the MCP tool bug by adding genesis file mounts:
   ```go
   "-v", fmt.Sprintf("%s:/node/directory-genesis.snap", filepath.Join(workDir, "dn-genesis.snap")),
   "-v", fmt.Sprintf("%s:/node/cyclops-genesis.snap", filepath.Join(workDir, "bvn-genesis.snap")),
   ```

5. Determine correct naming convention for genesis files in Docker container

### Investigation Paths

1. **Check `run.Config` structure**:
   - Does it have genesis file fields?
   - How does `runCfg()` process the configuration?

2. **Check consensus initialization**:
   - Where does "initialize consensus" code read files?
   - What path is it trying to access when it errors on `/node`?

3. **Test `init dual` command**:
   - Can we use `accumulated init dual` with genesis docs to initialize properly?
   - What does a properly initialized follower structure look like?

---

## Environment Details

- **Working Directory**: `/home/paul/accumulate-follower`
- **Docker Image**: `accumulated:follower-p2p-fix`
- **Binary Version**: v1.4.1 compatible
- **Container Ports**:
  - DN: 16591-16593
  - BVN: 16691-16693

---

## Related Documentation

- **Bootstrap Server Analysis**: `network_bootstrap_analysis.md` (in backupdbs repo)
- **MCP Deployment Summary**: `MCP_DEPLOYMENT_SUMMARY.md` (in backupdbs repo)
- **MCP Genesis Bug**: `MCP_BUG_GENESIS_FILES_NOT_MOUNTED.md` (in backupdbs repo)
- **Deployment Plan**: `follower_deployment_july13_genesis.md` (in backupdbs repo)

---

## Summary

Successfully prepared follower deployment environment using Accumulate MCP tools, but deployment is blocked by genesis file handling issue in new-style config. The error "read /node: is a directory" indicates the binary is attempting to access a file at path `/node`, but encountering the directory instead. This requires code-level debugging from the Accumulate repository to:

1. Understand new-style config genesis file handling
2. Trace the source of the error
3. Determine correct genesis file placement/configuration
4. Fix MCP tool to mount genesis files properly
5. Successfully deploy follower from July 13, 2025 genesis
