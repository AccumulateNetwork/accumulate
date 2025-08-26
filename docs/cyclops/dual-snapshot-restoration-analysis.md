# Dual Snapshot Restoration Analysis

## Critical Discovery: Partition-Specific Snapshot Restoration

**Date**: 2025-07-07 04:42 CDT  
**Issue**: Understanding how `restore-snapshot` works with dual nodes  
**Status**: ✅ **RESOLVED** - Root cause identified and solution documented

---

## Executive Summary

The `restore-snapshot` command is **partition-specific** and requires separate execution for each partition in a dual node setup. This is a fundamental architectural requirement that was not initially understood.

## Root Cause Analysis

### 1. LoadSnapshot Function Behavior

**Location**: `/internal/node/daemon/snapshots.go:231-246`

```go
func (d *Daemon) LoadSnapshot(file ioutil2.SectionReader) error {
    db, err := coredb.Open(d.Config, d.Logger)  // ← Opens SINGLE database
    if err != nil {
        return fmt.Errorf("failed to open database: %v", err)
    }

    defer func() {
        _ = db.Close()
    }()

    err = snapshot.FullRestore(db, file, d.Logger, d.Config.Accumulate.Describe.PartitionUrl())
    //                                                ↑ Uses SINGLE partition URL
    if err != nil {
        return fmt.Errorf("failed to restore database: %v", err)
    }
    return nil
}
```

**Key Findings**:
1. **Single Database**: Opens only one database per call
2. **Single Partition**: Uses only one partition URL from config
3. **No Multi-Partition Support**: Cannot restore multiple partitions in one call

### 2. Database Path Resolution

**Location**: `/internal/database/database.go:68-83`

```go
func Open(cfg *config.Config, logger log.Logger) (*Database, error) {
    switch cfg.Accumulate.Storage.Type {
    case config.BadgerStorage:
        return OpenBadger(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)
    case config.LevelDBStorage:
        return OpenLevelDB(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)
    //                                        ↑ cfg.RootDir determines database location
    }
}
```

**Key Findings**:
1. **Config-Dependent Path**: Database path determined by `cfg.RootDir`
2. **Partition-Specific Config**: Each partition has its own config with different `RootDir`

### 3. Dual Node Directory Structure

**Location**: `/cmd/accumulated/cmd_init.go:630-641`

```go
func netDir(networkType protocol.PartitionType) string {
    switch networkType {
    case protocol.PartitionTypeDirectory:
        return "dnn"        // Directory Network Node
    case protocol.PartitionTypeBlockValidator:
        return "bvnn"       // Block Validator Network Node
    }
}
```

**Actual Directory Structure**:
```
work-dir/
├── dnn/              # Directory Network Node
│   ├── config/
│   │   ├── accumulate.toml
│   │   ├── config.toml
│   │   └── priv_validator_key.json
│   └── data/
│       ├── accumulate.db/
│       └── priv_validator_state.json
└── bvnn/             # Block Validator Network Node
    ├── config/
    │   ├── accumulate.toml
    │   ├── config.toml
    │   └── priv_validator_key.json
    └── data/
        ├── accumulate.db/
        └── priv_validator_state.json
```

## Solution: Dual Snapshot Restoration Workflow

### Step 1: Restore Directory Partition Snapshot
```bash
./accumulated restore-snapshot "Directory-partition.snap" \
    --work-dir "$PWD/artifacts/dnn"
```

**What happens**:
1. Loads config from `artifacts/dnn/config/accumulate.toml`
2. Opens database at `artifacts/dnn/data/accumulate.db`
3. Restores Directory partition snapshot to DN database

### Step 2: Restore BVN Partition Snapshot
```bash
./accumulated restore-snapshot "bvn-cyclops-partition.snap" \
    --work-dir "$PWD/artifacts/bvnn"
```

**What happens**:
1. Loads config from `artifacts/bvnn/config/accumulate.toml`
2. Opens database at `artifacts/bvnn/data/accumulate.db`
3. Restores BVN partition snapshot to BVN database

## Technical Implementation Details

### Configuration Requirements

**DN Config** (`dnn/config/accumulate.toml`):
```toml
[describe]
  type = "directory"
  partition-id = "Directory"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
```

**BVN Config** (`bvnn/config/accumulate.toml`):
```toml
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
```

### Database Isolation

Each partition maintains:
- **Separate database files**
- **Separate configuration**
- **Separate validator keys**
- **Separate state files**

This ensures complete isolation between DN and BVN operations.

## Previous Misunderstanding

### What We Thought
- Single `restore-snapshot` call could handle both partitions
- Snapshots could be restored to a unified database
- Work-dir pointed to root `.accumulate` directory

### What Actually Happens
- Each `restore-snapshot` call handles exactly one partition
- Each partition has its own isolated database
- Work-dir must point to specific partition subdirectory (`dnn` or `bvnn`)

## Validation Commands

### Verify DN Snapshot Restoration
```bash
# Check DN database exists
ls -la artifacts/dnn/data/accumulate.db/

# Verify DN config
cat artifacts/dnn/config/accumulate.toml
```

### Verify BVN Snapshot Restoration
```bash
# Check BVN database exists
ls -la artifacts/bvnn/data/accumulate.db/

# Verify BVN config
cat artifacts/bvnn/config/accumulate.toml
```

### Verify Dual Node Structure
```bash
# Check complete structure
tree artifacts/
```

## Impact on Deployment Scripts

### Before (Incorrect)
```bash
# Wrong: Single work-dir for both snapshots
./accumulated restore-snapshot "Directory-partition.snap" --work-dir "$PWD/artifacts"
./accumulated restore-snapshot "bvn-cyclops-partition.snap" --work-dir "$PWD/artifacts"
```

### After (Correct)
```bash
# Correct: Partition-specific work-dirs
./accumulated restore-snapshot "Directory-partition.snap" --work-dir "$PWD/artifacts/dnn"
./accumulated restore-snapshot "bvn-cyclops-partition.snap" --work-dir "$PWD/artifacts/bvnn"
```

## Node Startup Implications

### Dual Node Startup
For dual nodes, the startup process must:
1. **Load both partition configs** from `dnn/` and `bvnn/`
2. **Access both databases** separately
3. **Run both partitions** in the same process

### Work-Dir for Startup
The startup work-dir should point to the **parent directory** containing both `dnn/` and `bvnn/` subdirectories.

```bash
# Correct startup work-dir
./accumulated run --work-dir "$PWD/artifacts"
```

This allows the dual node process to access both partition configurations.

## Code References

### Key Files Analyzed
1. `/internal/node/daemon/snapshots.go` - LoadSnapshot implementation
2. `/internal/database/database.go` - Database opening logic
3. `/cmd/accumulated/cmd_init.go` - Directory structure creation
4. `/cmd/accumulated/cmd_init_dual_node.go` - Dual node initialization

### Key Functions
1. `Daemon.LoadSnapshot()` - Snapshot restoration entry point
2. `database.Open()` - Database path resolution
3. `netDir()` - Partition directory naming
4. `initNode()` - Single partition initialization

## Best Practices

### 1. Always Use Partition-Specific Work-Dirs
```bash
# DN restoration
--work-dir "$PWD/artifacts/dnn"

# BVN restoration  
--work-dir "$PWD/artifacts/bvnn"
```

### 2. Verify Database Isolation
- Check that each partition has its own database directory
- Ensure no cross-partition data contamination
- Validate partition-specific configurations

### 3. Sequential Restoration
- Restore DN snapshot first
- Restore BVN snapshot second
- Verify both restorations before node startup

### 4. Comprehensive Validation
- Check directory structure
- Verify database files exist
- Validate configuration files
- Test node startup

## Troubleshooting

### Common Issues

1. **"Database not found"**
   - **Cause**: Wrong work-dir path
   - **Fix**: Use partition-specific work-dir (`dnn` or `bvnn`)

2. **"Snapshot format mismatch"**
   - **Cause**: Wrong snapshot for partition
   - **Fix**: Ensure Directory snapshot goes to `dnn`, BVN snapshot goes to `bvnn`

3. **"Configuration error"**
   - **Cause**: Missing or incorrect partition config
   - **Fix**: Verify `accumulate.toml` exists in partition config directory

### Debug Commands

```bash
# Check work-dir contents
ls -la artifacts/dnn/
ls -la artifacts/bvnn/

# Verify configs
cat artifacts/dnn/config/accumulate.toml
cat artifacts/bvnn/config/accumulate.toml

# Check database directories
ls -la artifacts/dnn/data/
ls -la artifacts/bvnn/data/
```

## Conclusion

The dual snapshot restoration workflow is now fully understood and documented. The key insight is that `restore-snapshot` is fundamentally partition-specific and requires separate execution for each partition with partition-specific work directories.

This architectural design ensures proper database isolation and partition independence, which is critical for dual node operation.

## Next Steps

1. ✅ Update deployment scripts with correct work-dir paths
2. ✅ Update validation scripts to check dual node structure
3. ⏳ Test complete dual snapshot restoration workflow
4. ⏳ Validate node startup with restored snapshots
5. ⏳ Document operational procedures for dual node management

---

**Status**: Documentation complete - Ready for implementation testing
