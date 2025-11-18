# Snapshot Restore Implementation Status

**Date**: 2025-11-18
**Status**: Initial Implementation Complete - NOT TESTED
**Branch**: `3691-mcp-server-for-accumulate`

## Summary

Initial implementation of `accumulate_restore_from_snapshots` MCP tool completed. The tool provides basic functionality to restore follower nodes from snapshot files (.snap format) with configurable ports.

**⚠️ WARNING: This implementation has NOT been tested and contains known gaps that must be addressed before production use.**

## What Was Implemented

### Files Created/Modified

1. **`mcp/server/tools_snapshot_restore.go`** (NEW)
   - Core implementation of snapshot restore functionality
   - ~200 lines of code
   - Functions: `restoreFromSnapshots()`, `restorePartition()`, `createDualNodeConfig()`, `parsePort()`

2. **`mcp/server/tool_definitions.go`** (MODIFIED)
   - Added tool definition for `accumulate_restore_from_snapshots`
   - Comprehensive parameter documentation
   - Support for both port_offset and explicit ports

3. **`mcp/server/server.go`** (MODIFIED)
   - Added dispatcher case for new tool
   - Routes to `s.restoreFromSnapshots(args)`

### Features Implemented

✅ **Port Configuration Flexibility**
- Supports `port_offset` for simple incremental ports
- Supports explicit `ports` object for full control
- Explicit ports override port_offset (accman-friendly)

✅ **Dual Partition Support**
- Handles DN (Directory Network) partition
- Handles BVN (Block Validation Network) partition
- Creates separate node directories (dnn/, bvnn/)

✅ **Config Generation**
- Uses `config.Default()` to create partition configs
- Uses `config.Store()` to write both accumulate.toml and tendermint.toml
- Configures ports in generated configs

✅ **Snapshot Validation**
- Checks that snapshot files exist before proceeding
- Returns clear error messages

✅ **Parameter Validation**
- Validates required parameters (dn_snapshot, bvn_snapshot, work_dir)
- Provides sensible defaults (network="MainNet", bvn_name="Cyclops")

## Known Gaps and Risks

### 🔴 Critical Issues (Must Fix Before Testing)

#### 1. No Node Key Generation
**Problem**: Node requires `node_key.json` and `priv_validator_key.json` files
**Impact**: Node won't start without these files
**Location**: tools_snapshot_restore.go:136-165
**Fix Required**: Copy key generation logic from `internal/node/daemon/init.go:367-415`

#### 2. No Bootstrap Peers Configuration
**Problem**: Generated config may not have correct bootstrap peers
**Impact**: Node won't connect to network
**Location**: tools_snapshot_restore.go:155
**Fix Required**: Add bootstrap peer configuration based on network

#### 3. createDualNodeConfig() Not Implemented
**Problem**: Function returns nil without creating runtime config
**Impact**: No dual-node configuration for Docker deployment
**Location**: tools_snapshot_restore.go:179-187
**Fix Required**: Implement dual-node config with [[configurations]] sections

#### 4. Untested restore-snapshot Command
**Problem**: exec.Command("accumulated", "restore-snapshot") has never been executed
**Impact**: Unknown if command works as expected
**Location**: tools_snapshot_restore.go:168-174
**Risk**: Command might fail, need wrong arguments, or require different context

### 🟡 Medium Priority Issues

#### 5. No Genesis File Handling
**Assumption**: restore-snapshot handles genesis automatically
**Not Verified**: May need to create/copy genesis files
**Reference**: `internal/node/daemon/init.go:359-362`

#### 6. Incomplete Port Configuration
**Current**: Only sets 3 ports per partition (P2P, RPC, API)
**Missing**: Prometheus port, external address for P2P
**Impact**: Monitoring may not work, P2P discovery may fail

#### 7. No Snapshot Compatibility Checks
**Problem**: Doesn't verify snapshot version, network ID, or block height compatibility
**Impact**: May restore incompatible snapshots
**Risk**: Node corruption or sync failures

#### 8. Working Directory Context Unclear
**Problem**: restore-snapshot execution context not fully tested
**Questions**:
- Does it need config files to exist first?
- Does it read config or just restore data?
- Does it expect to be run from nodeDir?

### 🟢 Low Priority Issues

#### 9. Error Handling Could Be Improved
**Current**: Basic error returns with context
**Improvement**: More detailed validation, recovery mechanisms

#### 10. No Logging/Debugging
**Current**: Relies on returned error messages
**Improvement**: Add structured logging for troubleshooting

## Research Findings

### Key Discovery: Tendermint Config Handling

**Question**: How does restore-snapshot handle config/tendermint.toml?

**Answer**: The `config.Store(cfg)` function writes BOTH files:
```go
// From internal/node/config/config.go:327
tm.WriteConfigFile(filepath.Join(config.RootDir, configDir, tmConfigFile), &config.Config)
return StoreAcc(config, filepath.Join(config.RootDir, configDir))
```

This means:
- `config/tendermint.toml` is written by CometBFT's WriteConfigFile
- `config/accumulate.toml` is written by StoreAcc
- Both are created automatically when we call config.Store()

**Impact**: Simplifies our implementation - just need to create Config object and call Store()

## Testing Status

### Build Status
- ⏳ Build initiated but not confirmed successful
- 📦 Dependencies downloading
- ❌ No compilation verification yet

### Test Coverage
- ❌ Zero unit tests
- ❌ Zero integration tests
- ❌ Zero end-to-end tests
- ❌ No manual testing performed

### What Needs Testing

**Phase 1: Basic Functionality**
1. Verify code compiles
2. Test config file generation
3. Inspect generated accumulate.toml and tendermint.toml
4. Verify port configuration in configs

**Phase 2: Integration Testing**
5. Test restore-snapshot command execution
6. Verify database files are created
7. Check directory structure
8. Validate file permissions

**Phase 3: End-to-End Testing**
9. Attempt to start restored node with `accumulated run`
10. Verify node connects to network
11. Check if node syncs blocks
12. Test both port configuration modes (offset vs explicit)

**Phase 4: Docker Deployment**
13. Test with actual Docker container
14. Verify port mapping works
15. Test dual-node configuration
16. Validate accman integration

## Implementation Details

### Port Configuration Logic

**Default (port_offset=0)**:
```
DN:  16591 (listen), 16592 (api), 16593 (p2p)
BVN: 16691 (listen), 16692 (api), 16693 (p2p)
```

**Accman Convention (explicit ports)**:
```
DN:  52000 (listen), 52001 (api), 52002 (p2p)
BVN: 52100 (listen), 52101 (api), 52102 (p2p)
```

**Precedence**: Explicit `ports` parameter overrides `port_offset`

### restore-snapshot Command Execution

```go
cmd := exec.Command("accumulated", "restore-snapshot", snapshotPath)
cmd.Dir = nodeDir
cmd.Env = append(os.Environ(), fmt.Sprintf("ACC_WORKDIR=%s", nodeDir))
```

**Assumptions**:
- `accumulated` binary is in PATH
- Command runs successfully from partition directory
- Environment variable ACC_WORKDIR is respected
- Config files must exist before restore

### Directory Structure Created

```
work_dir/
├── dnn/
│   ├── config/
│   │   ├── accumulate.toml
│   │   └── tendermint.toml
│   └── data/
│       └── (restored from dn_snapshot)
├── bvnn/
│   ├── config/
│   │   ├── accumulate.toml
│   │   └── tendermint.toml
│   └── data/
│       └── (restored from bvn_snapshot)
└── accumulate.toml (TODO: dual-node config)
```

## Next Steps

### Immediate Actions Required

1. **Fix Critical Issues**
   - Add node key generation
   - Configure bootstrap peers
   - Implement createDualNodeConfig()

2. **Verify Build**
   - Confirm compilation succeeds
   - Fix any build errors

3. **Manual Testing**
   - Test with actual snapshots from `/tmp/current-snapshots/`
   - Verify restored node directory structure
   - Attempt to start node

4. **Iterate Based on Results**
   - Fix issues discovered during testing
   - Add missing functionality
   - Improve error handling

### Future Enhancements

- Automated snapshot download (out of initial scope)
- Snapshot verification/signing
- Incremental/delta snapshots
- Better error messages and validation
- Comprehensive test suite
- Performance optimization
- Monitoring and metrics

## Related Documentation

- **Design Docs**: `mcp/snapshot_restore_readme.md`
- **Implementation Plan**: `mcp/implementation_clarity_assessment.md`
- **Accman Review**: `mcp/accman_snapshot_restore_review.md`
- **Deployment Guide**: `accman/SNAPSHOT_RESTORE_DEPLOYMENT.md` (in accman repo)

## Timeline

**Implementation**: ~4 hours (research + coding)
**Testing**: Not started
**Production Ready**: Estimated 1-2 days additional work

## Conclusion

Initial implementation provides a working foundation for snapshot-based follower deployment. However, **this code is NOT production-ready** and requires:

1. Fixing critical gaps (node keys, bootstrap peers, dual-node config)
2. Thorough testing (build, integration, e2e)
3. Addressing medium-priority issues
4. Documentation updates based on test results

**Recommended**: Treat this as a prototype/proof-of-concept that demonstrates the approach but needs refinement before deployment.

---

**Last Updated**: 2025-11-18
**Author**: Claude Code
**Branch**: 3691-mcp-server-for-accumulate
**Commit**: (pending)
