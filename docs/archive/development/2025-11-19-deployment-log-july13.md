# Follower Deployment Log - July 13 Genesis
**Date**: Wed Nov 19 11:45:43 AM CST 2025
**Container**: DO-NOT-MODIFY-accumulate-follower-July13
**Ports**: DN 17091-17093, BVN 17191-17193

## Deployment Steps
### STEP 1: Locate accman MCP
- Found: /home/paul/go/src/gitlab.com/AccumulateNetwork/accman/accman-mcp
- Verified executable

### STEP 2: Verify Genesis Files
-rwxr-xr-x 1 paul paul 2.1G Jul 13 09:05 /media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/cyclops-genesis.snap
-rwxr-xr-x 1 paul paul 2.0M Jul 13 09:05 /media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/directory-genesis.snap

### STEP 3: Deploy Follower via accman MCP
```json
{
  "method": "deploy_follower",
  "params": {
    "partition": "dual",
    "dn_genesis": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/directory-genesis.snap",
    "bvn_genesis": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/cyclops-genesis.snap",
    "binary_path": "/home/paul/go/bin/accumulated",
    "container_name": "DO-NOT-MODIFY-accumulate-follower-July13",
    "dn_p2p_port": "17091",
    "dn_rpc_port": "17092",
    "dn_rpc_json_port": "17093",
    "bvn_p2p_port": "17191",
    "bvn_rpc_port": "17192",
    "bvn_rpc_json_port": "17193"
  }
}
```

**Result**: ✅ SUCCESS
```json
{"container_name":"DO-NOT-MODIFY-accumulate-follower-July13","data_dir":"/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulate-dual-data","partition":"dual","ports":{"dn-p2p":"17091","dn-rpc":"17092","dn-rpc-json":"17093","bvn-p2p":"17191","bvn-rpc":"17192","bvn-rpc-json":"17193"}}
```

### ISSUE DETECTED
Container failing: dynamically linked binary incompatible with Alpine
Exit code: 255 - exec /accumulated: no such file or directory

**Options**:
1. Build statically-linked accumulated binary
2. Use existing Docker image (accumulated:follower-p2p-fix)

### STEP 4: Build Static Binary
- Built statically-linked accumulated: /tmp/accumulated-static (63MB)
- Verified: not a dynamic executable

### STEP 5: Retry Deployment with Static Binary
**Result**: ❌ FAILED - Container restarting, missing node directories

### ISSUE 2: Missing Node Directories
- Accman only copied genesis snapshot files
- Did not initialize node directory structures (dnn/, bvnn/)
- Missing: config/tendermint.toml, config/genesis.json, data/priv_validator_state.json

### STEP 6: Use Accumulate MCP to Restore from Snapshots
Used `accumulate_restore_from_snapshots` to properly initialize node directories:
```json
{
  "dn_snapshot": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/directory-genesis.snap",
  "bvn_snapshot": "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/cyclops-genesis.snap",
  "work_dir": "/home/paul/DO-NOT-MODIFY-accumulate-follower-July13",
  "ports": {
    "dn_listen": 17091, "dn_api": 17092, "dn_p2p": 17093,
    "bvn_listen": 17191, "bvn_api": 17192, "bvn_p2p": 17193
  }
}
```

**Result**: ✅ SUCCESS - Created dnn/ and bvnn/ directories with proper structure

### STEP 7: Copy Initialized Directories to Accman Data Location
```bash
cp -r /home/paul/DO-NOT-MODIFY-accumulate-follower-July13/* \
      /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulate-dual-data/
```

### ISSUE 3: Missing genesis.json and priv_validator_state.json Files
Accumulate MCP restore created Tendermint configs but not all required files:
- Missing: config/genesis.json (required by CometBFT)
- Wrong location: data/priv_validator_state.json was in config/, not data/

### STEP 8: Fix Missing Files
1. Copied genesis.json files from previous deployment:
   ```bash
   cp /home/paul/accumulate-follower-genesis/dnn/config/genesis.json \
      /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulate-dual-data/dnn/config/
   cp /home/paul/accumulate-follower-genesis/bvnn/config/genesis.json \
      /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulate-dual-data/bvnn/config/
   ```

2. Copied priv_validator_state.json to data/ directories:
   ```bash
   cp .../dnn/config/priv_validator_state.json .../dnn/data/
   cp .../bvnn/config/priv_validator_state.json .../bvnn/data/
   ```

### ISSUE 4: Incorrect Command for Dual-Node Setup
- Accman used: `/accumulated run -w /node`
- This command expects single-node structure: /node/config/
- Actual structure: /node/dnn/config/ and /node/bvnn/config/

**Discovery**: Found `accumulated run-dual` command in cmd/accumulated/cmd_run_dual.go
- Correct command: `/accumulated run-dual /node/dnn /node/bvnn`

### STEP 9: Deploy with Correct Docker Command
```bash
docker run -d \
  --name DO-NOT-MODIFY-accumulate-follower-July13 \
  --restart unless-stopped \
  -p 17091:17091 -p 17092:17092 -p 17093:17093 \
  -p 17191:17191 -p 17192:17192 -p 17193:17193 \
  -v /tmp/accumulated-static:/accumulated:ro \
  -v /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulate-dual-data:/node \
  alpine /accumulated run-dual /node/dnn /node/bvnn
```

**Result**: ✅ SUCCESS

## Final Status
### Container Status
- **Name**: DO-NOT-MODIFY-accumulate-follower-July13
- **Status**: Running (stable for 60+ seconds)
- **Directory Node**: Running at http://0.0.0.0:17092
- **Block Validator Node**: Running at http://0.0.0.0:17192

### Port Mappings
- DN Ports: 17091 (P2P), 17092 (API), 17093 (RPC)
- BVN Ports: 17191 (P2P), 17192 (API), 17193 (RPC)

### Data Directory
`/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulate-dual-data/`
- `accumulate.toml` - Dual-node configuration
- `dnn/` - Directory Network node
- `bvnn/` - Cyclops BVN node

### Binary
- Path: `/tmp/accumulated-static`
- Size: 63MB
- Type: Statically linked (Alpine compatible)

### Lessons Learned
1. Accman's `deploy_follower` with partition="dual" only copies genesis files, doesn't initialize directories
2. Accumulate MCP's `accumulate_restore_from_snapshots` properly initializes node directories
3. Genesis.json files must be obtained separately (not included in .snap files)
4. Dual-node deployments require `accumulated run-dual` command, not `accumulated run -w`
5. File locations matter: priv_validator_state.json must be in data/, not config/

## Completion
**Date**: Wed Nov 19 12:10:55 PM CST 2025
**Status**: ✅ DEPLOYED AND RUNNING
