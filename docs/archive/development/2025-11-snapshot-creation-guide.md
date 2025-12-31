# Snapshot Creation Guide

This document describes how to create genesis snapshots for Accumulate follower deployment.

## Data Sources

### Primary Source: backupdbs MCP Repository

The `backupdbs` repository (`gitlab.com/AccumulateNetwork/backupdbs`) provides:
- Complete validator node backups with full databases
- Catalog system for tracking and managing database backups
- MCP integration for AI-assisted database operations

**Key Paths:**
```
/media/paul/Expansion/databases/
├── .catalog/catalog.db           # Metadata catalog
├── backup/                       # Compressed backup archives
└── validator_backup_20251117/    # November 17, 2025 complete nodes
    └── extracted/
        ├── dnn/data/             # Directory Network node
        │   └── accumulate.db     # Badger database
        └── bvnn/data/            # BVN (Cyclops) node
            └── accumulate.db     # Badger database
```

### Using backupdbs MCP

```bash
# Start the backupdbs MCP server
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/backupdbs
go run ./cmd/mcp-server

# MCP tools available:
# - list_databases: List all cataloged databases
# - get_database: Get metadata for a specific database
# - search_databases: Search by partition, date, tags
```

## Snapshot Creation

### Tool: create-snap

Location: `cmd/create-snap/main.go`

This tool creates v2 snapshots with consensus sections from existing Accumulate databases.

### Building the Tool

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
go build -o create-snap ./cmd/create-snap
```

### Creating Snapshots

#### Directory Network (DN) Snapshot

```bash
./create-snap \
  -db /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db \
  -output /mnt/secondary/snapshots/directory-genesis.snap \
  -partition Directory \
  -type badger
```

#### Block Validator Network (BVN) Snapshot

BVN snapshots require the DN database to read the NetworkDefinition for validator information:

```bash
./create-snap \
  -db /media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/data/accumulate.db \
  -dn-db /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db \
  -output /mnt/secondary/snapshots/cyclops-genesis.snap \
  -partition Cyclops \
  -type badger
```

### Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `-db` | Yes | Path to the Accumulate database (accumulate.db directory) |
| `-dn-db` | For BVN | Path to DN database for reading network definition |
| `-output` | Yes | Output .snap file path |
| `-partition` | Yes | Partition name: `Directory`, `Apollo`, `Yutu`, `Cyclops` |
| `-type` | Yes | Database type: `badger` or `leveldb` |

### Expected Output

```
Opening badger database at /path/to/accumulate.db...
Opening DN database at /path/to/dn/accumulate.db for network definition...
Creating v2 snapshot file: /path/to/output.snap
Collecting v2 snapshot for partition Cyclops...
Partition URL: acc://cyclops.acme
Successfully read network definition with 3 validators
Writing consensus section...
Added 3 validators to consensus section

V2 Snapshot successfully created!
  File: /path/to/output.snap
  Size: 2048.50 MB
```

### Snapshot Structure

A valid v2 snapshot contains:

1. **Header Section** - Version, height, timestamp, root hash
2. **Consensus Section** - Chain ID, validators, consensus params
3. **BPT Section** - Binary Patricia Trie entries
4. **Records Section** - Account and transaction data

## Validating Snapshots

### Using debug tool

```bash
go run ./tools/cmd/debug snapshot dump /path/to/snapshot.snap | head -50
```

Expected output for a valid snapshot:
```
Header section at 64 (size 115)
  Version    2
  Height     1
  Time       2025-07-13 13:49:18 +0000 UTC
  State hash b166048d9c3c89417ea3aec01afa0e671332391e08660f9d8c1ee6605bacb79b
Consensus section at 256 (size 152)
  {
    "chainID": "MainNet.Directory",
    "validators": [...]
  }
Bpt section at 512 (size ...)
Records section at ... (size ...)
```

### Validation Checklist

- [ ] Version is 2
- [ ] Has consensus section
- [ ] Chain ID matches partition (e.g., "MainNet.Directory", "MainNet.Cyclops")
- [ ] Has at least 1 validator
- [ ] State hash is non-zero

## Common Issues

### "not enough data" Error

**Symptom:** Snapshot restore fails with `restore Account.acc://...MainChain.States.XXX: not enough data`

**Cause:** Snapshot has truncated record data, likely from:
- Interrupted snapshot collection
- Disk space issue during creation
- Source database corruption

**Solution:** Regenerate the snapshot from the original database:
```bash
# Delete corrupted snapshot
rm /path/to/corrupted.snap

# Regenerate from source database
./create-snap -db /path/to/source/accumulate.db -output /path/to/new.snap -partition <partition> -type badger
```

### Missing Consensus Section

**Symptom:** Restore succeeds but genesis.json has empty app_hash

**Cause:** Snapshot was created by daemon's collectSnapshot() which doesn't include consensus section

**Solution:** Use `create-snap` tool which explicitly adds consensus section with validators

### Wrong Database Type

**Symptom:** `failed to open database: not a valid database`

**Cause:** Specified wrong `-type` parameter

**Solution:** Check database format:
- Badger: Has `*.vlog`, `*.sst`, `MANIFEST` files
- LevelDB: Has `*.ldb`, `CURRENT`, `MANIFEST-*` files

## Automation via MCP

### Adding Snapshot Creation to MCP

The accumulate MCP should have a tool for creating snapshots:

```go
// mcp/server/tools_snapshot_create.go
func (s *Server) createSnapshot(args map[string]interface{}) (map[string]interface{}, error) {
    dbPath := args["db_path"].(string)
    dnDbPath, _ := args["dn_db_path"].(string)  // Optional, for BVN
    outputPath := args["output_path"].(string)
    partition := args["partition"].(string)
    dbType := args["db_type"].(string)

    // Create snapshot with consensus section
    // ...
}
```

### Error Handling

When snapshot creation or restore fails:
1. Log the error with full context
2. Create a task/issue for investigation
3. Preserve the original database for retry
4. Document the failure in the catalog

## Reference

### Source Database Locations (Nov 17, 2025)

| Partition | Database Path |
|-----------|---------------|
| Directory | `/media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db` |
| Cyclops (BVN2) | `/media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/data/accumulate.db` |

### Expected Snapshot Sizes

| Partition | Approximate Size |
|-----------|------------------|
| Directory | 2-5 MB |
| BVN (Cyclops) | 2-3 GB |

### Related Documentation

- [Snapshot Deployment Guide](../deployment/follower/snapshot-deployment-guide.md)
- [Snapshot Restore Implementation](./SNAPSHOT-RESTORE-IMPLEMENTATION.md)
- [backupdbs README](/home/paul/go/src/gitlab.com/AccumulateNetwork/backupdbs/README.md)
