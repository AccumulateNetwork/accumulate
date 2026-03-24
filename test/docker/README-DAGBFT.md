# DAG-BFT Multi-Node Docker Network

## ⚠️ CRITICAL REQUIREMENTS ⚠️

### 1. Bootstrap Server (MANDATORY - DO NOT SKIP)
**The bootstrap server MUST be running for the network to function.**

The bootstrap server is NOT optional. It provides peer discovery for all validators. Without it:
- Validators cannot find each other
- No peer connections will be established
- Consensus will not start
- The network will appear to run but do nothing

**Service name**: `bootstrap`  
**Port**: 16593  
**Command**: `accumulated-bootstrap /ip4/0.0.0.0/tcp/16593`

This requirement is enforced in `docker-compose.yml` via service dependencies.

### 2. Network Initialization
The `init` service must complete before validators start. It generates:
- Genesis snapshots (directory-genesis.snap, bvn{1,2,3}-genesis.snap)
- Node configurations (accumulate.toml for each validator)
- Network topology and peer addresses

### 3. New Configuration Format
This setup uses the **NEW** accumulate.toml format:
- Main command: `accumulated /path/to/accumulate.toml` (NOT `accumulated run`)
- Configuration array: `[[configurations]]` sections
- Validator type: `type = "coreValidator"`
- No CometBFT dependencies

## Architecture

### Network Topology
```
                    ┌─────────────────┐
                    │ Bootstrap Server│ (REQUIRED)
                    │   Port: 16593   │
                    └────────┬────────┘
                             │
            ┌────────────────┼────────────────┐
            │                │                │
       ┌────▼────┐      ┌────▼────┐     ┌────▼────┐
       │  BVN1   │      │  BVN2   │     │  BVN3   │
       │ 4 vals  │      │ 4 vals  │     │ 4 vals  │
       └─────────┘      └─────────┘     └─────────┘
```

- **Bootstrap**: 1 server (peer discovery)
- **BVNs**: 3 networks (BVN1, BVN2, BVN3)
- **Validators**: 4 per BVN = 12 total
- **Directory**: Integrated with BVN1-val1

### Service Dependencies
```
bootstrap (must start first)
    ↓
init (runs once, exits)
    ↓
validators (all 12, depend on both)
```

## Usage

### Start Everything
```bash
# Build images
docker compose -f test/docker/docker-compose.yml build

# Start bootstrap + all validators
docker compose -f test/docker/docker-compose.yml up -d

# Verify bootstrap is running
docker ps | grep bootstrap
# Should show: acc-bootstrap ... Up ... 16593->16593/tcp

# Check logs
docker compose -f test/docker/docker-compose.yml logs -f
```

### Verify Network is Running
```bash
# Check all containers
docker compose -f test/docker/docker-compose.yml ps

# Should see:
# - acc-bootstrap: Up (healthy or starting)
# - acc-bvn{1,2,3}-val{1,2,3,4}: Up (12 validators)
# - acc-init: Exited (0) - this is normal

# Check bootstrap server logs
docker logs acc-bootstrap

# Check validator logs
docker logs acc-bvn1-val1
```

### Stop Network
```bash
# Stop all containers
docker compose -f test/docker/docker-compose.yml down

# Stop and remove volumes (full reset)
docker compose -f test/docker/docker-compose.yml down -v
```

## Troubleshooting

### Bootstrap Server Not Running
**Symptom**: Validators log "Unable to connect to bootstrap peer" errors

**Check**:
```bash
docker ps | grep bootstrap
docker logs acc-bootstrap
```

**Fix**:
```bash
# Restart bootstrap
docker compose -f test/docker/docker-compose.yml restart bootstrap

# Or rebuild and restart
docker compose -f test/docker/docker-compose.yml up -d --force-recreate bootstrap
```

### Validators Won't Start
**Check dependencies**:
```bash
# Verify init completed successfully
docker logs acc-init
# Should end with exit code 0

# Verify bootstrap is running
docker ps | grep bootstrap
# Should show "Up"
```

**Fix**:
```bash
# If init failed, recreate it
docker compose -f test/docker/docker-compose.yml up init

# If bootstrap missing, start it
docker compose -f test/docker/docker-compose.yml up -d bootstrap
```

### No Consensus Activity
**Symptom**: Validators start but no blocks are produced

**Common causes**:
1. Bootstrap server not running → Check logs for "connection refused" to port 16593
2. Network configuration using wrong IPs → Check accumulate.toml has correct addresses
3. Validators can't reach each other → Check Docker network connectivity

**Debug**:
```bash
# Check if validators are finding peers
docker logs acc-bvn1-val1 | grep -i peer

# Check consensus activity
docker logs acc-bvn1-val1 | grep -i "round\|commit\|leader"

# Check bootstrap connections
docker logs acc-bootstrap | tail -20
```

## Container Details

### Bootstrap Server
- **Name**: `acc-bootstrap`
- **Image**: Built from `Dockerfile.bootstrap`
- **Port**: 16593 (exposed to host)
- **Purpose**: Peer discovery coordinator
- **Restart**: `unless-stopped` (auto-restarts if crashes)
- **Health**: Critical - network fails without it

### Init Container
- **Name**: `acc-init`
- **Lifecycle**: Runs once, then exits
- **Purpose**: Generate genesis files and configurations
- **Volumes**: Shares `network-config` volume with validators
- **Output**: Creates 12 node directories in `/root/.accumulate/`

### Validator Containers (×12)
- **Names**: `acc-bvn{1,2,3}-val{1,2,3,4}`
- **Ports**: 26660-26671 (one per validator)
- **Working Dir**: `/root/.accumulate/bvn{X}-{Y}/`
- **Config File**: `/root/.accumulate/bvn{X}-{Y}/accumulate.toml`
- **Command**: `accumulated -w=<dir> <config-file>`

## Port Mapping

| Container | Host Port | Container Port | Purpose |
|-----------|-----------|----------------|---------|
| acc-bootstrap | 16593 | 16593 | Peer discovery |
| acc-bvn1-val1 | 26660 | 26660 | API |
| acc-bvn1-val2 | 26661 | 26660 | API |
| acc-bvn1-val3 | 26662 | 26660 | API |
| acc-bvn1-val4 | 26663 | 26660 | API |
| acc-bvn2-val1 | 26664 | 26660 | API |
| acc-bvn2-val2 | 26665 | 26660 | API |
| acc-bvn2-val3 | 26666 | 26660 | API |
| acc-bvn2-val4 | 26667 | 26660 | API |
| acc-bvn3-val1 | 26668 | 26660 | API |
| acc-bvn3-val2 | 26669 | 26660 | API |
| acc-bvn3-val3 | 26670 | 26660 | API |
| acc-bvn3-val4 | 26671 | 26660 | API |

## Files Modified for DAG-BFT

### CometBFT Removal
The following files were modified to remove CometBFT dependencies:

1. **`internal/node/daemon/run.go`**
   - Made `loadKeys()` check file existence before loading
   - Prevents CometBFT LoadFilePV from panicking on missing files

2. **`cmd/accumulated/run/key.go`**
   - `CometPrivValFile.get()` generates transient keys if file missing
   - `CometNodeKeyFile.get()` generates transient keys if file missing

3. **`internal/node/config/config.go`**
   - Made `loadTendermint()` optional in `loadFile()`
   - Uses default config if tendermint.toml doesn't exist

### Configuration System
4. **`test/docker/docker-compose.yml`**
   - Changed command from `run` to `<config-file>` (uses new system)
   - Added bootstrap server dependency
   - Uses full paths to accumulate.toml files

## Key Differences from Old System

| Aspect | Old (CometBFT) | New (DAG-BFT) |
|--------|----------------|---------------|
| Command | `accumulated run` | `accumulated config.toml` |
| Consensus | CometBFT/Tendermint | DAG-BFT (Bullshark) |
| Keys | Required priv_validator_key.json | Transient keys auto-generated |
| Config | tendermint.toml + accumulate.toml | accumulate.toml only |
| Bootstrap | Not required | **REQUIRED** for peer discovery |

## Next Steps

After starting the network:
1. Wait ~30 seconds for validators to connect
2. Verify consensus is running (check logs for block commits)
3. Test API endpoints: `curl http://localhost:26660/v3/describe`
4. Run load tests or transaction submissions

## Related Documentation

- Network configuration: `../dagbft-network.yml`
- Main README: `README.md`
- Dockerfile: `../../Dockerfile`
- Bootstrap Dockerfile: `../../Dockerfile.bootstrap`
