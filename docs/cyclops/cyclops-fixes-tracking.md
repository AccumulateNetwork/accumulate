# Cyclops Mainnet Single Node - Fixes Tracking

## Overview
This document tracks all identified issues that need to be fixed for successful Cyclops validator node deployment. Each issue is documented with exact locations, root causes, and proposed solutions.

## Status Legend
- 🔴 **Critical** - Blocks node startup
- 🟡 **Important** - Affects functionality but not startup
- 🟢 **Minor** - Cosmetic or optimization
- ✅ **Fixed** - Issue resolved
- 📋 **Documented** - Issue analyzed and documented

## Critical Issues (Blocking Node Startup)

### 1. Routing Table Missing Value Field ✅
**Status:** ✅ Fixed and Tested
**Error:** `panic: expected values with 10 at 2:0, found none`
**Location:** `/internal/api/routing/tree.go:89` in `buildPrefixTree()`
**File:** `/tmp/cyclops/node/artifacts/cyclops-network.json`
**Problem:** First routing entry missing `"value": 0`
```json
// CURRENT (BROKEN):
{
  "length": 2,
  "partition": "Directory"
}

// NEEDS TO BE:
{
  "length": 2,
  "partition": "Directory",
  "value": 0
}
```
**Impact:** Node fails during genesis initialization
**Solution:** Add `"value": 0` to first routing table entry
**Fix Applied:** 2025-07-07 02:05 CDT
**Validation:** 
- JSON structure validated with `jq`
- Extract command successfully parses network config
- Routing table builds without panic
- Partitions detected correctly (2 partitions: bvn-cyclops, Directory)

### 2. Ed25519 Private Key Seed Length Issue ✅
**Status:** 🔧 **FIXED AND TESTED**
**Error:** `ed25519: bad seed length: 96` → `panic: ed25519: bad seed length: 64`
**Location:** `/internal/node/daemon/run.go:649` in `StartP2P()` function
**Root Cause:** Code was passing 64-byte Ed25519 private key directly to `ed25519.NewKeyFromSeed()` which expects only 32-byte seed

**Fix Applied:** 2025-07-07 02:15 CDT
```go
// BEFORE (Broken):
privKeySeed := d.nodeKey.PrivKey.Bytes()  // 64 bytes
p2pKey := ed25519.NewKeyFromSeed(privKeySeed)  // PANIC: expects 32 bytes

// AFTER (Fixed):
privKeyBytes := d.nodeKey.PrivKey.Bytes()
switch len(privKeyBytes) {
case ed25519.SeedSize: // 32 bytes - seed only
    p2pKey = ed25519.NewKeyFromSeed(privKeyBytes)
case ed25519.PrivateKeySize: // 64 bytes - seed + public key
    // Extract the first 32 bytes as the seed
    p2pKey = ed25519.NewKeyFromSeed(privKeyBytes[:ed25519.SeedSize])
default:
    return errors.UnknownError.WithFormat("invalid ed25519 private key length: want 32 or 64, got %d", len(privKeyBytes))
}
```

**Technical Details:**
- **Ed25519 Format**: Private key JSON contains 64 bytes (32-byte seed + 32-byte public key)
- **NewKeyFromSeed()**: Expects exactly 32 bytes (seed only)
- **Solution**: Extract first 32 bytes as seed, following pattern in `address.FromED25519PrivateKey()`
- **Validation**: Added proper length checking with descriptive error messages

### 3. Missing Individual Partition Snapshots 🔴
**Status:** 📋 Documented
**Location:** `init genesis` command expects individual snapshots
**Problem:** Have unified `cyclops-genesis.snap` but need:
- `Directory-partition.snap`
- `bvn-cyclops-partition.snap`
**Impact:** Genesis initialization fails without proper snapshot format
**Solution:** Extract individual partition snapshots from unified snapshot

## Important Issues (Functionality Impact)

### 4. Partition Type Configuration 🟡
**Status:** 📋 Documented
**Error:** `unknown partition type PartitionType:0`
**Location:** `accumulate.toml` configuration
**Problem:** Partition type not in correct TOML section
```toml
# CURRENT (WRONG):
[configurations]
  type = "coreValidator"

# NEEDS TO BE:
[describe]
  type = "directory"  # or "blockValidator"
  partition-id = "Directory"  # or "bvn-cyclops"
```
**Impact:** Node doesn't know its partition role
**Solution:** Move partition configuration to `[describe]` section

### 5. File Permissions on Validator Keys 🟡
**Status:** 📋 Documented
**Location:** `.accumulate/config/` directory
**Problem:** Validator key files may not have secure permissions
**Files:**
- `node_key.json`
- `priv_validator_key.json`
**Required:** 600 (read/write owner only)
**Impact:** Security risk and potential startup warnings
**Solution:** `chmod 600` on all validator key files

---

## TOML Configuration File Generation Analysis

### Key Discovery: Missing TOML Config Files

The node startup failure with "missing tendermint.toml" indicates that the TOML configuration files were not generated during the network initialization process. Based on codebase analysis, here's how TOML config generation works:

### Configuration File Structure

#### Main Config Object
```go
type Config struct {
    tm.Config      // Embedded CometBFT configuration
    Accumulate Accumulate
}
```

#### Key Configuration Files Generated
1. **`tendermint.toml`** - CometBFT consensus configuration
2. **`accumulate.toml`** - Accumulate-specific settings

### TOML Generation Functions

#### Primary Generation Functions
**Location:** `/internal/node/config/config.go`

```go
// Store both TOML files
func Store(config *Config) error {
    // Write CometBFT configuration
    tm.WriteConfigFile(filepath.Join(config.RootDir, configDir, tmConfigFile), &config.Config)
    
    // Write Accumulate configuration  
    return StoreAcc(config, filepath.Join(config.RootDir, configDir))
}

// Store Accumulate-specific TOML
func StoreAcc(config *Config, dir string) error {
    return writeTomlFile(config.Accumulate, filepath.Join(dir, accConfigFile))
}

// Generic TOML file writer
func writeTomlFile(v any, file string) error {
    f, err := os.Create(file)
    if err != nil {
        return err
    }
    defer f.Close()
    return toml.NewEncoder(f).Encode(v)  // Uses github.com/pelletier/go-toml
}
```

#### Default Configuration Generation
```go
func Default(netName string, net protocol.PartitionType, nodeType NodeType, partitionId string) *Config {
    c := new(Config)
    c.Accumulate.Network.Id = netName
    c.Accumulate.NetworkType = net
    c.Accumulate.PartitionId = partitionId
    c.Accumulate.API.TxMaxWaitTime = 10 * time.Minute
    c.Accumulate.Storage.Type = BadgerStorage
    c.Accumulate.Storage.Path = filepath.Join("data", "accumulate.db")
    c.Accumulate.Snapshots.Enable = false
    c.Config = *tm.DefaultConfig()  // CometBFT defaults
    return c
}
```

### Expected TOML File Structure

#### `accumulate.toml` Sections
```toml
[describe]
network-type = "directory"  # or "block-validator"
partition-id = "Directory"  # or "bvn-cyclops"

[describe.network]
id = "cyclops"

[storage]
type = "badger"  # or "leveldb", "memory"
path = "data/accumulate.db"

[api]
listen-address = "tcp://0.0.0.0:26660"
tx-max-wait-time = "10m0s"
connection-limit = 500

[p2p]
listen = ["/tcp/16591"]
bootstrap-peers = ["tcp://bootstrap.accumulate.io:16591"]

[snapshots]
enable = false
directory = "snapshots"
retain = 10
```

#### `tendermint.toml` (CometBFT Configuration)
- Generated by `tm.WriteConfigFile()` from CometBFT library
- Contains consensus, P2P, RPC, and mempool settings
- Uses CometBFT's default configuration as base

### File Locations
```
node-directory/
├── config/
│   ├── accumulate.toml          # Accumulate-specific config
│   ├── tendermint.toml          # CometBFT consensus config
│   ├── priv_validator_key.json  # Validator private key
│   └── node_key.json            # Node identity key
└── data/
    └── accumulate.db/           # Database storage
```

### Root Cause Analysis

**Problem:** The Cyclops node directories are missing TOML configuration files because:
1. The network initialization process may not be calling `config.Store()` properly
2. The node directory structure wasn't created with proper config generation
3. The `accumulated init` commands weren't executed for individual nodes

**Solution Path:**
1. **Investigate** where `config.Store()` should be called during network initialization
2. **Generate** missing TOML files using the configuration functions
3. **Validate** that both `tendermint.toml` and `accumulate.toml` are created with correct content
4. **Test** node startup after TOML file generation

### Next Investigation Steps
1. Find where node configuration should call `config.Store()`
2. Determine correct configuration values for Cyclops network
3. Generate missing TOML files manually or via proper initialization
4. Validate node startup with complete configuration

---

## Minor Issues (Optimization/Cosmetic)

### 6. Inconsistent Key Encoding Documentation 🟢
**Status:** 📋 Documented
**Problem:** Mixed documentation about hex vs base64 encoding
**Files:** Various documentation files
**Impact:** Developer confusion
**Solution:** Standardize documentation on encoding formats

## File-Specific Fix Requirements

### `/tmp/cyclops/node/artifacts/cyclops-network.json`
- [ ] Add `"value": 0` to first routing entry (line 71)
- [ ] Verify all routing entries have proper value fields
- [ ] Validate JSON structure with `jq`

### `/tmp/cyclops/node/artifacts/.accumulate/config/accumulate.toml`
- [ ] Move `type` and `partition-id` to `[describe]` section
- [ ] Set correct partition type: `"directory"` or `"blockValidator"`
- [ ] Set correct partition ID: `"Directory"` or `"bvn-cyclops"`

### Key Files
- [ ] Verify `node_key.json` has hex-encoded private key
- [ ] Verify `priv_validator_key.json` has base64-encoded keys
- [ ] Set permissions to 600 on all validator keys

### Snapshot Files
- [ ] Extract `Directory-partition.snap` from `cyclops-genesis.snap`
- [ ] Extract `bvn-cyclops-partition.snap` from `cyclops-genesis.snap`
- [ ] Verify snapshot integrity and format

## Code Locations Requiring Investigation

### Genesis Initialization Chain
1. `cmd/accumulated/cmd_init_network.go` - `initGenesis()` function
2. `internal/node/daemon/init.go` - `BuildGenesisDocs()` function
3. `internal/node/genesis/bootstrap.go` - `Init()` function (line 102)
4. `internal/api/routing/router.go` - `NewRouter()` function
5. `internal/api/routing/tree.go` - `NewRouteTree()` and `buildPrefixTree()`

### Key Handling Chain
1. `cmd/accumulated/cmd_init_network.go` - `loadNetworkConfiguration()`
2. `internal/node/daemon/init.go` - Key conversion logic
3. `tools/cmd/analyze/cmd_generate_key.go` - `PrivValidatorKey` struct
4. `tools/cmd/analyze/cmd_update_network_keys.go` - Network key integration

## Testing Strategy

### Phase 1: Fix Critical Issues
1. Fix routing table value field
2. Extract individual partition snapshots
3. Fix key format handling

### Phase 2: Configuration Fixes
1. Update `accumulate.toml` partition configuration
2. Set proper file permissions
3. Validate all configuration files

### Phase 3: Integration Testing
1. Run `accumulated init genesis` with fixed configuration
2. Verify genesis files are created successfully
3. Test node startup with `accumulated run`

## Validation Commands

### Pre-Fix Validation
```bash
# Validate network JSON structure
jq '.' cyclops-network.json

# Check routing table completeness
jq '.globals.routing.routes[] | select(.length == 2)' cyclops-network.json

# Verify snapshot files exist
ls -la *.snap

# Check key file permissions
ls -la .accumulate/config/*.json
```

### Post-Fix Validation
```bash
# Test genesis initialization
./accumulated init genesis cyclops-network.json --work-dir .accumulate --snapshot Directory-partition.snap --snapshot bvn-cyclops-partition.snap

# Verify genesis files created
ls -la .accumulate/

# Test node startup (dry run)
./accumulated run --work-dir .accumulate --dry-run
```

## Dependencies Between Fixes

### Critical Path
1. **Routing Table Fix** → **Genesis Initialization** → **Node Startup**
2. **Partition Snapshots** → **Genesis Initialization** → **Node Startup**
3. **Key Format Fix** → **Genesis Building** → **Node Startup**

### Configuration Path
1. **Partition Type Fix** → **Node Configuration** → **Runtime Behavior**
2. **File Permissions** → **Security** → **Production Readiness**

## Documentation Updates Required

### After Fixes Applied
- [ ] Update `fixing_mainnet_single_node.md` with resolution status
- [ ] Create `cyclops-deployment-checklist.md` with validated steps
- [ ] Update `cyclops-node-directory-design.md` with working configuration
- [ ] Document validated command sequences in deployment guides

## Success Criteria

### Node Startup Success
- [ ] No routing table panics
- [ ] No key format errors
- [ ] No partition type errors
- [ ] Genesis files created successfully
- [ ] Node starts without critical errors
- [ ] Consensus participation begins
- [ ] Peer connections established

### Security Compliance
- [ ] All validator keys have 600 permissions
- [ ] No sensitive data in logs
- [ ] Proper key backup procedures documented

### Documentation Complete
- [ ] All fixes documented with before/after examples
- [ ] Troubleshooting guide updated
- [ ] Deployment automation updated
- [ ] Validation procedures documented

---

**Last Updated:** 2025-07-07T02:00:14-05:00
**Next Review:** After each critical fix is applied
**Maintainer:** AI Assistant (Cascade)
